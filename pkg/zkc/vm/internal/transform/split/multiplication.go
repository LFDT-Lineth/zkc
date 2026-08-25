// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package split

import (
	"math/big"
	"math/bits"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Multiplication splits a multiplication instruction "target = k · s0 · … · sn"
// into a sequence of narrow multiply/add instructions which are field-agnostic
// (every register is at most RegisterWidth bits) and bandwidth-safe (every
// emitted multiply and addition fits within the field bandwidth on its own).
// It does NOT rely on any downstream constraint-level splitting.
//
// The approach is schoolbook long multiplication over g-bit "sub-limbs", with
// the granularity g chosen by mulGranularity such that everything below
// provably fits the field bandwidth.  Operands are cut on a g-grid from bit
// zero (see decomposeToSubLimbs); each partial product aᵢ·bⱼ is then at most
// 2·g bits — a low and a high half landing in adjacent weight columns — and
// each column is summed by a single add whose carry feeds the column above.
// For two 2-sub-limb operands:
//
//	                        a₁     a₀
//	             ×          b₁     b₀
//	             ─────────────────────
//	                     a₀b₀ʰ  a₀b₀ˡ      one narrow mul per pair;
//	              a₁b₀ʰ  a₁b₀ˡ             ˡ/ʰ = low/high g bits, at
//	              a₀b₁ʰ  a₀b₁ˡ             column weight i+j / i+j+1
//	       a₁b₁ʰ  a₁b₁ˡ
//	             ─────────────────────
//	out₃ ⟵c₂⟵ out₂ ⟵c₁⟵ out₁    out₀       one add per column, carries
//	                                       threaded into the next column
//
// The constant k is cut into g-bit chunks and folded in the same way by an
// initial scalar pass, and an n-ary product is a left-fold of binary steps —
// each fold keeps the full product width, since the multiply is
// overflow-checked rather than truncating.
//
// The FINAL fold is fused with the assignment to the target.  Instead of
// producing the product on its own g-grid and then reshaping it —
//
//	naive:  accumulate columns → clamp each column to the target width
//	        → re-cut at the target-limb boundaries → concatenate
//	        (a mov chain per column)
//
// — its columns are laid straight onto the target grid (see targetLayout):
// each column add writes output pieces already clamped and already cut at the
// limb boundaries, and a column which is already identical to its output is
// passed through without emitting anything.  For u16 × u16 → u32 with g = 15:
//
//	  col 2       col 1            col 0
//	|── 2 ──|─────── 15 ──────|─────── 15 ───────|  product columns
//	   m₂       p₁       p₀      (pass-through)     add c::p₁::p₀ = …
//	|── 2 ──|── 14 ───|─ 1 ──|─────── 15 ───────|   add m₂ = …
//	└────── hi ───────┘└────────── lo ──────────┘   one cat per limb
//
// Product bits at or beyond the target width fall into zero-width columns,
// whose adds are satisfiable only by zero: exactly the overflow check the
// unsplit multiply performs (a product that does not fit the target makes the
// constraints unsatisfiable / aborts execution).
func Multiplication[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	var (
		insns       []Bytecode[W]
		targetLimbs = applyLimbsMapReversed(mapping, insn.Target...)
		sourceLimbs = applyLimbsMapReversed(mapping, insn.Source...)
		operands    [][]RegisterId
	)
	//
	if len(insn.Source) == 0 {
		// Degenerate case: a product with no source registers is simply its
		// constant.  Codegen does not normally emit this, but handle it anyway.
		return loadConstant(alloc, targetLimbs, insn.Constant)
	} else if len(targetLimbs) == len(insn.Target) && len(sourceLimbs) == len(insn.Source) &&
		multiplicationFitsBandwidth(alloc, sourceLimbs, insn.Constant, mapping.BandWidth()) {
		// Preserve a multiplication which needs no register splitting and whose
		// largest possible result fits within the machine bandwidth.  Besides
		// avoiding needless temporaries, this retains the original instruction's
		// dynamic target-overflow check.
		return []Bytecode[W]{bytecode.MulVecConst(targetLimbs, sourceLimbs, insn.Constant)}
	}
	// Choose the multiply granularity for the full product width (the sum of
	// the operand widths plus the constant's width bounds the product).
	g := mulGranularity(mapping.RegisterWidth(), mapping.BandWidth(),
		groupWidth(alloc, sourceLimbs)+uint(insn.Constant.BigInt().BitLen()))
	// Decompose each source operand into g-wide sub-limbs (least-significant
	// first).
	for _, s := range insn.Source {
		subs, code := decomposeToSubLimbs(alloc, g, mapping.LimbIds(s))
		operands = append(operands, subs)
		insns = append(insns, code...)
	}
	// Fold the operands (and constant) left-to-right into a running product.
	// Intermediate folds accumulate onto the running product's own g-grid; the
	// last fold accumulates straight onto the target grid (the fusion pictured
	// above), after which only concatenations remain.
	var (
		acc    = operands[0]
		width  = groupWidth(alloc, acc)
		final  = targetLayout(g, registerWidths(alloc, targetLimbs))
		folds  = len(operands) - 1
		scalar = insn.Constant.Cmp64(1) != 0
	)
	//
	if scalar {
		// Fold in the constant via an initial scalar pass (k == 1 is the
		// identity and needs no pass).
		width += uint(insn.Constant.BigInt().BitLen())
		layout := final
		//
		if folds > 0 {
			layout = foldLayout(width, g)
		}
		//
		outs, code := scalarStep(alloc, g, acc, insn.Constant, layout)
		acc, insns = outs, append(insns, code...)
	} else if folds == 0 {
		// A single operand times one: nothing to fold, so re-chunk the
		// decomposed operand onto the target grid directly.
		outs, code := accumulate(alloc, g, final, singletonColumns(acc))
		acc, insns = outs, append(insns, code...)
	}
	// Fold in each remaining operand.
	for i, b := range operands[1:] {
		width += groupWidth(alloc, b)
		layout := final
		//
		if i+1 < folds {
			layout = foldLayout(width, g)
		}
		//
		outs, code := binaryStep(alloc, g, acc, b, layout)
		acc, insns = outs, append(insns, code...)
	}
	// Concatenate the target-grid pieces into the target limbs.
	return append(insns, reassembleToTarget(alloc, acc, targetLimbs)...)
}

// multiplicationFitsBandwidth determines whether the largest value this
// multiplication can produce fits within the machine bandwidth.  The exact
// unsigned upper bound is k * product(2^width(source)-1).
func multiplicationFitsBandwidth[W word.Word[W]](alloc Allocator[W], sources []RegisterId, constant W,
	bandwidth uint,
) bool {
	var (
		maximum = new(big.Int).Set(constant.BigInt())
		one     = big.NewInt(1)
	)
	//
	for _, source := range sources {
		width := alloc.Register(source).Bitwidth()
		// Native field registers do not have a finite declared bit width and
		// therefore cannot use the direct unsigned-multiplication path.
		if width.IsEmpty() {
			return false
		}
		// Determine the largest value representable by this source register.
		factor := new(big.Int).Lsh(big.NewInt(1), width.Unwrap())
		factor.Sub(factor, one)
		maximum.Mul(maximum, factor)
		// Stop as soon as the result is already too wide.
		if uint(maximum.BitLen()) > bandwidth {
			return false
		}
	}
	//
	return true
}

// mulGranularity determines the sub-limb width g used for multiplication.  It
// is the largest g <= min(RegisterWidth, BandWidth/2) — so a partial product of
// two g-bit sub-limbs fits within BandWidth — for which the column
// accumulation provably cannot wrap the field modulus.  A product of the given
// full width has at most T = 2·ceil(productWidth/g) partial-product sub-limbs
// in any accumulation column (one low and one high piece per contributing
// partial product); with the threaded carry, every column sum stays below
// (T+1)·2^g by induction (a carry is at most the previous sum shifted down by
// g, i.e. at most T).  Requiring g + bitlen(T+1) <= BandWidth therefore keeps
// every column addition faithful over the integers.
//
// When no g satisfies that bound (only tiny testing fields), fall back to the
// legacy rule — the largest divisor of RegisterWidth with 2·g <= BandWidth —
// preserving the previous behaviour rather than rejecting the program.
//
// Both variable operands and the constant are decomposed into g-bit sub-limbs
// (see binaryStep / scalarStep), so every partial product is at most 2·g bits
// regardless of the constant's magnitude.  Since g need not divide the
// register width, sub-limbs are cut on a grid which may cross limb boundaries
// (see decomposeToSubLimbs / targetLayout).
func mulGranularity(regWidth, bandWidth, productWidth uint) uint {
	//
	for g := min(regWidth, bandWidth/2); g >= 1; g-- {
		var terms = 2*ceilDiv(productWidth, g) + 1
		// Either the column accumulation fits the bandwidth, or we have reached
		// the legacy (divisor-rule) granularity.
		if g+uint(bits.Len(terms)) <= bandWidth || regWidth%g == 0 {
			return g
		}
	}
	//
	panic("cannot find multiply granularity fitting field bandwidth")
}

func registerWidths[W word.Word[W]](alloc Allocator[W], regs []RegisterId) []uint {
	var widths = make([]uint, len(regs))
	//
	for i, reg := range regs {
		widths[i] = alloc.Register(reg).Bitwidth().Unwrap()
	}
	//
	return widths
}

// decomposeToSubLimbs re-expresses an operand's (little-endian) limbs as
// g-wide sub-limbs cut on a grid starting at bit zero of the operand.  Since g
// need not divide the limb widths, a sub-limb may span a limb boundary, so the
// limbs are cut at both sets of boundaries and the pieces of each grid cell
// then joined; e.g. for two u16 limbs with g = 15:
//
//	limbs:   |─────── 16 ──────|─────── 16 ───────|      RegisterWidth limbs
//	g-grid:  |────── 15 ──────|─────── 15 ──────|─2─|    wanted sub-limbs
//	pieces:  |────── 15 ──────|─1─|──── 14 ─────|─2─|    cut at both
//
// Each limb crossed by a grid boundary is destructured once (a vectored add),
// each grid cell spanning several pieces is joined once (a concatenation), and
// a limb or piece already spanning a whole cell is used directly.
func decomposeToSubLimbs[W word.Word[W]](alloc Allocator[W], g uint, limbs []RegisterId,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		widths        = gridWidths(groupWidth(alloc, limbs), g)
		pieces, insns = splitAtBoundaries(alloc, "s", limbs, widths)
		subs          []RegisterId
	)
	//
	for _, width := range widths {
		var cell []RegisterId
		//
		cell, pieces = takePieces(alloc, pieces, width)
		//
		if len(cell) == 1 {
			subs = append(subs, cell[0])
		} else {
			var sub = alloc.Allocate("s", util.Some(width))
			//
			insns = append(insns, bytecode.AssignV[W]([]RegisterId{sub}, cell...))
			subs = append(subs, sub)
		}
	}
	//
	return subs, insns
}

// splitAtBoundaries destructures a little-endian register group such that
// every boundary of the given widths (cumulative bit offsets within the group)
// coincides with a register boundary, returning the resulting little-endian
// piece sequence.  A register which no boundary crosses is passed through
// untouched; every other register is destructured at the boundaries crossing
// it (one vectored add each).  Zero-width registers carry no bits and are
// dropped.
func splitAtBoundaries[W word.Word[W]](alloc Allocator[W], prefix string, group []RegisterId, widths []uint,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		pieces []RegisterId
		insns  []Bytecode[W]
		cuts   []uint
		offset uint
	)
	//
	for _, width := range widths {
		offset += width
		cuts = append(cuts, offset)
	}
	//
	offset = 0
	//
	for _, reg := range group {
		var (
			width       = alloc.Register(reg).Bitwidth().Unwrap()
			lo          = offset
			pieceWidths []uint
		)
		// Determine the boundaries falling strictly inside this register.
		for _, cut := range cuts {
			if cut > lo && cut < offset+width {
				pieceWidths = append(pieceWidths, cut-lo)
				lo = cut
			}
		}
		//
		switch {
		case width == 0:
			// contributes no bits
		case len(pieceWidths) == 0:
			pieces = append(pieces, reg)
		default:
			pieceWidths = append(pieceWidths, offset+width-lo)
			//
			var ps = make([]RegisterId, len(pieceWidths))
			//
			for i, w := range pieceWidths {
				ps[i] = alloc.Allocate(prefix, util.Some(w))
			}
			//
			insns = append(insns, bytecode.AddVec[W](ps, []RegisterId{reg}))
			pieces = append(pieces, ps...)
		}
		//
		offset += width
	}
	//
	return pieces, insns
}

// takePieces consumes leading pieces totalling exactly width bits, returning
// them along with the remaining pieces.  The pieces were cut at the consumer's
// boundaries (see splitAtBoundaries), so the total always lands exactly.
func takePieces[W word.Word[W]](alloc Allocator[W], pieces []RegisterId, width uint,
) (cell, rest []RegisterId) {
	var total uint
	//
	for total < width {
		total += alloc.Register(pieces[0]).Bitwidth().Unwrap()
		cell, pieces = append(cell, pieces[0]), pieces[1:]
	}
	//
	util.Assert(total == width, "piece boundary mismatch")
	//
	return cell, pieces
}

// gridWidths returns the widths of the ceil(total/g) grid cells spanning total
// bits: each cell is g bits except the most-significant, which holds the
// remainder.
func gridWidths(total, g uint) []uint {
	var widths []uint
	//
	for off := uint(0); off < total; off += g {
		widths = append(widths, min(g, total-off))
	}
	//
	return widths
}

// scalarStep multiplies the sub-limb accumulator a by the constant k.  The
// constant is decomposed into g-wide sub-limbs kⱼ (exactly as the variable
// operands are), and every partial product aᵢ·kⱼ is materialised and bucketed
// against its column weight (i+j) — the same shape as binaryStep, but with a
// constant rather than a variable second operand.  Because each aᵢ·kⱼ multiplies
// two g-bit values it is at most 2·g bits, so however wide k is the multiply
// still fits the field bandwidth (see mulGranularity).  Zero sub-limbs of k are
// skipped: they contribute nothing to any column.
func scalarStep[W word.Word[W]](alloc Allocator[W], g uint, a []RegisterId, k W, layout [][]uint,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		kSubs    = descriptor.SplitConstant(k, g)
		partials = map[uint][]RegisterId{}
		insns    []Bytecode[W]
	)
	//
	for i, ai := range a {
		var wa = alloc.Register(ai).Bitwidth().Unwrap()
		//
		for j, kj := range kSubs {
			if kj.Cmp64(0) == 0 {
				continue
			}
			//
			var pp = allocSubLimbs(alloc, "pp", g, wa+uint(kj.BigInt().BitLen()))
			//
			insns = append(insns, bytecode.MulVecConst(pp, []RegisterId{ai}, kj))
			bucket(partials, uint(i+j), pp)
		}
	}
	//
	outs, code := accumulate(alloc, g, layout, partials)
	//
	return outs, append(insns, code...)
}

// binaryStep multiplies two sub-limb operands together, materialising every
// partial product aᵢ·bⱼ into fresh sub-limbs, bucketing them by column weight
// and accumulating the columns onto the given layout.
func binaryStep[W word.Word[W]](alloc Allocator[W], g uint, a, b []RegisterId, layout [][]uint,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		one      W
		partials = map[uint][]RegisterId{}
		insns    []Bytecode[W]
	)
	//
	one = one.SetUint64(1)
	//
	for i, ai := range a {
		var wa = alloc.Register(ai).Bitwidth().Unwrap()
		//
		for j, bj := range b {
			var (
				wb = alloc.Register(bj).Bitwidth().Unwrap()
				pp = allocSubLimbs(alloc, "pp", g, wa+wb)
			)
			//
			insns = append(insns, bytecode.MulVecConst(pp, []RegisterId{ai, bj}, one))
			bucket(partials, uint(i+j), pp)
		}
	}
	//
	outs, code := accumulate(alloc, g, layout, partials)
	//
	return outs, append(insns, code...)
}

// A column layout gives, for each accumulation column, the widths of its
// output pieces (little-endian, summing to the column's width).  Columns
// beyond the layout are zero-width: their contents are forced to zero.

// foldLayout is the layout of an intermediate fold holding a value of the
// given total width: one whole output per column — g bits each, with the
// most-significant column holding the remainder.
func foldLayout(total, g uint) [][]uint {
	var layout [][]uint
	//
	for _, width := range gridWidths(total, g) {
		layout = append(layout, []uint{width})
	}
	//
	return layout
}

// targetLayout is the layout of the final fold, which lays the product
// straight onto the target: the target's bits are cut first on the g-grid and
// then at the limb boundaries, so every output piece falls entirely within one
// target limb and the limbs are reassembled by concatenation alone:
//
//	g-grid:  |───── col 0 ─────|───── col 1 ─────|─ col 2 ─|
//	limbs:   |────── limb 0 ──────|─────── limb 1 ─────────|
//	pieces:  |─────── p₀ ──────|p₁|────── p₂ ─────|── p₃ ──|
//
//	⟹  layout [[p₀] [p₁ p₂] [p₃]]
//
// Product bits at or beyond the target width fall into columns outside this
// layout — zero-width, forcing them to zero: the overflow check.
func targetLayout(g uint, limbWidths []uint) [][]uint {
	var (
		layout [][]uint
		cuts   []uint
		total  uint
	)
	// Determine the cumulative limb boundaries.
	for _, width := range limbWidths {
		total += width
		cuts = append(cuts, total)
	}
	// Cut each grid cell at the limb boundaries falling strictly inside it.
	for lo := uint(0); lo < total; lo += g {
		var (
			pieces []uint
			hi     = min(lo+g, total)
			at     = lo
		)
		//
		for _, cut := range cuts {
			if cut > at && cut < hi {
				pieces = append(pieces, cut-at)
				at = cut
			}
		}
		//
		layout = append(layout, append(pieces, hi-at))
	}
	//
	return layout
}

// bucket records each sub-limb of a partial product against its column weight.
// A product whose least-significant sub-limb sits at column base has its tth
// sub-limb at column base+t.
func bucket(partials map[uint][]RegisterId, base uint, pp []RegisterId) {
	for t, pl := range pp {
		var w = base + uint(t)
		//
		partials[w] = append(partials[w], pl)
	}
}

// accumulate sums each column's bucketed sub-limbs — plus the carry from the
// column below — into that column's output pieces, one vectored add per
// column:
//
//	add  carry :: pieceₙ ‥ piece₀  =  partials[c]… (+ previous carry)
//
// A carry holds the bits of the column sum above 2^g and so has weight c+1: it
// is spliced into the next column's sources.  Two rules apply:
//
//   - Pass-through: a column holding a single partial of exactly its single
//     piece already IS its output — no instruction is emitted.
//
//   - Only a full g-bit column carries.  A clamped column (width < g — cut
//     short by the target width or by the value's own tail) gets no carry: for
//     any in-range value its sum provably fits the clamped width (the columns
//     reconstruct the value, so a sum reaching the clamped-off bits would
//     exceed the value's bound).  Its add therefore rejects exactly the
//     out-of-range products — the overflow check — and never a valid one.
//     This also keeps every carry small (rhs − g bits) whatever the layout.
func accumulate[W word.Word[W]](alloc Allocator[W], g uint, layout [][]uint, partials map[uint][]RegisterId,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		zero    W
		numCols = uint(len(layout))
		carry   = util.None[RegisterId]()
		outs    []RegisterId
		insns   []Bytecode[W]
	)
	// Ensure every bucketed sub-limb has a home (columns beyond the layout are
	// zero-width, forcing their contents to zero).
	for c := range partials {
		numCols = max(numCols, c+1)
	}
	//
	for c := range numCols {
		var (
			pieces  = []uint{0}
			sources = partials[c]
		)
		//
		if c < uint(len(layout)) {
			pieces = layout[c]
		}
		//
		width := math.Sum(pieces...)
		// Splice in the carry from the previous column.
		if carry.HasValue() {
			sources = append(sources, carry.Unwrap())
			carry = util.None[RegisterId]()
		}
		// Pass through a column which is already exactly its output.
		if len(pieces) == 1 && len(sources) == 1 &&
			alloc.Register(sources[0]).Bitwidth().Unwrap() == width {
			outs = append(outs, sources[0])
			continue
		}
		// Allocate the column's output pieces (little-endian).
		var targets = make([]RegisterId, len(pieces))
		//
		for i, w := range pieces {
			targets[i] = alloc.Allocate("m", util.Some(w))
		}
		//
		outs = append(outs, targets...)
		// Add a carry when a full g-bit column's sum can overflow its output
		// (clamped columns must not overflow, and the final column has nowhere
		// to carry to).
		rhs := descriptor.CalculateAddBitwidth(sources, zero, alloc).Unwrap()
		//
		if width == g && rhs > width && c+1 < numCols {
			var cr = alloc.Allocate("c", util.Some(rhs-width))
			//
			targets = append(targets, cr)
			carry = util.Some(cr)
		}
		//
		insns = append(insns, bytecode.AddVecConst(targets, sources, zero))
	}
	//
	return outs, insns
}

// singletonColumns places each sub-limb of a value into its own column (the tth
// sub-limb at column t).  This re-chunks a fully-computed value for a final
// column-wise assignment.
func singletonColumns(subs []RegisterId) map[uint][]RegisterId {
	var partials = map[uint][]RegisterId{}
	//
	for i, s := range subs {
		partials[uint(i)] = []RegisterId{s}
	}
	//
	return partials
}

// reassembleToTarget joins the column output pieces into the target's
// RegisterWidth limbs, emitting one concatenation per target limb.  The final
// accumulation cut its outputs at the target-limb boundaries (see
// targetLayout), so every target limb is a whole number of consecutive pieces
// and splitAtBoundaries merely partitions them — emitting nothing.  The higher
// (zero-width) overflow columns carry no bits and are dropped.
func reassembleToTarget[W word.Word[W]](alloc Allocator[W], outs, targetLimbs []RegisterId,
) []Bytecode[W] {
	//
	var (
		zero          W
		widths        = registerWidths(alloc, targetLimbs)
		pieces, insns = splitAtBoundaries(alloc, "r", outs, widths)
	)
	//
	for i, tl := range targetLimbs {
		var cell []RegisterId
		//
		cell, pieces = takePieces(alloc, pieces, widths[i])
		//
		if len(cell) == 0 {
			insns = append(insns, bytecode.LoadConst(tl, zero))
		} else {
			insns = append(insns, bytecode.AssignV[W]([]RegisterId{tl}, cell...))
		}
	}
	//
	return insns
}

// loadConstant assigns a constant across the target limbs (used only for the
// degenerate no-source product).
func loadConstant[W word.Word[W]](alloc Allocator[W], targetLimbs []RegisterId, k W) []Bytecode[W] {
	var (
		insns []Bytecode[W]
		acc   = k
	)
	//
	for _, tl := range targetLimbs {
		var (
			tw    = alloc.Register(tl).Bitwidth().Unwrap()
			piece = acc.Slice(tw)
		)
		//
		acc = acc.Shr64(uint64(tw))
		//
		insns = append(insns, bytecode.AddVecConst([]RegisterId{tl}, nil, piece))
	}
	//
	return insns
}

// allocSubLimbs allocates ceil(width/g) fresh registers spanning width bits,
// each of width g except the most-significant which holds the remainder.
func allocSubLimbs[W word.Word[W]](alloc Allocator[W], prefix string, g, width uint) []RegisterId {
	var out []RegisterId
	//
	for off := uint(0); off < width; off += g {
		out = append(out, alloc.Allocate(prefix, util.Some(min(g, width-off))))
	}
	//
	return out
}

// groupWidth returns the total bit-width of a limb group.
func groupWidth[W word.Word[W]](alloc Allocator[W], limbs []RegisterId) uint {
	var w uint
	//
	for _, l := range limbs {
		w += alloc.Register(l).Bitwidth().Unwrap()
	}
	//
	return w
}

// ceilDiv computes the ceiling of a/b for positive b.
func ceilDiv(a, b uint) uint {
	return (a + b - 1) / b
}
