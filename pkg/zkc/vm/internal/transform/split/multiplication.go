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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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
// The approach is schoolbook long multiplication over "sub-limbs":
//
//  1. A multiply granularity g is chosen (see mulGranularity) such that a
//     partial product of two g-bit values fits the field bandwidth.  Each
//     operand — and the constant — is decomposed (least-significant first) into
//     g-bit sub-limbs.
//  2. For a binary product a·b, every partial product aᵢ·bⱼ is materialised into
//     fresh sub-limb registers via a narrow MulVecConst and bucketed against its
//     column weight (i+j, plus the sub-limb offset within the product).  The
//     constant k is decomposed into g-bit sub-limbs and folded in via an initial
//     scalar pass; n-ary products are reduced to a left-fold of binary products.
//     Each fold keeps the full product width, since the multiply is
//     overflow-checked rather than truncating.
//  3. Each column is a multi-operand addition, lowered by reusing the carry
//     machinery of Addition (insertAddCarryLines / addAssignment): a carry from
//     column c always has weight c+1, so it threads into the next column.
//  4. The full product is finally laid into the target's RegisterWidth limbs.
//     Bits that lie beyond the target width are accumulated into zero-width
//     columns, which forces them to be zero: exactly the overflow check the
//     unsplit multiply performs (a product that does not fit the target makes
//     the constraints unsatisfiable / aborts execution).
func Multiplication[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	var (
		one          W
		insns        []Bytecode[W]
		targetLimbs  = applyLimbsMapReversed(mapping, insn.Target...)
		sourceLimbs  = applyLimbsMapReversed(mapping, insn.Source...)
		targetWidth  = groupWidth(alloc, targetLimbs)
		targetWidths = registerWidths(alloc, targetLimbs)
		g            = mulGranularity(mapping.RegisterWidth(), mapping.BandWidth(), targetWidths...)
		operands     [][]RegisterId
	)
	//
	one = one.SetUint64(1)
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
	// Decompose each source operand into g-wide sub-limbs (least-significant
	// first).
	for _, s := range insn.Source {
		subs, code := decomposeToSubLimbs(alloc, g, mapping.LimbIds(s))
		operands = append(operands, subs)
		insns = append(insns, code...)
	}
	// Fold the operands (and constant) left-to-right into a single sub-limb
	// accumulator holding the full-width product.
	acc := operands[0]
	// Apply the constant via an initial scalar-multiply pass (k == 1 is the
	// identity and needs no pass).
	if insn.Constant.Cmp64(1) != 0 {
		outs, code := scalarStep(alloc, g, acc, insn.Constant)
		acc, insns = outs, append(insns, code...)
	}
	// Multiply in each remaining operand.
	for _, b := range operands[1:] {
		outs, code := binaryStep(alloc, g, acc, b, one)
		acc, insns = outs, append(insns, code...)
	}
	// Lay the full product into the target limbs, forcing any bits beyond the
	// target width to zero (the overflow check).
	return append(insns, assignToTarget(alloc, g, acc, targetLimbs, targetWidth)...)
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
// is the largest divisor of RegisterWidth such that: (1) a partial product of
// two g-bit values fits within BandWidth; and (2) every boundary between target
// limbs is also a g-bit boundary.  The latter matters when several narrow
// targets receive one product.  For example, a u32::u32 target on a u64 machine
// requires g <= 32; otherwise reassembly would try to place one u64 product
// column into the first u32 target.
//
// Both variable operands and the constant are decomposed into g-bit sub-limbs
// (see binaryStep / scalarStep), so every partial product is at most 2·g bits
// regardless of the constant's magnitude.
func mulGranularity(regWidth, bandWidth uint, targetWidths ...uint) uint {
	//
	for g := regWidth; g >= 1; g-- {
		if regWidth%g == 0 && 2*g <= bandWidth && targetBoundariesAlign(g, targetWidths) {
			return g
		}
	}
	//
	panic("cannot find multiply granularity fitting field bandwidth")
}

// targetBoundariesAlign checks that each boundary before the final target limb
// coincides with a sub-limb boundary.  The final boundary needs no check: the
// last multiplication column is already narrowed to the product's total width.
func targetBoundariesAlign(g uint, widths []uint) bool {
	var offset uint
	//
	for _, width := range widths[:max(0, len(widths)-1)] {
		offset += width
		//
		if offset%g != 0 {
			return false
		}
	}
	//
	return true
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

// decomposeToSubLimbs splits each (RegisterWidth-aligned) limb of an operand
// into g-wide sub-limbs, least-significant first, emitting a destructure
// (vectored add) for any limb wider than g.  A limb already no wider than g is
// used directly.
func decomposeToSubLimbs[W word.Word[W]](alloc Allocator[W], g uint, limbs []RegisterId,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		subs  []RegisterId
		insns []Bytecode[W]
	)
	//
	for _, limb := range limbs {
		var w = alloc.Register(limb).Bitwidth().Unwrap()
		//
		if w <= g {
			subs = append(subs, limb)
			continue
		}
		// Wider than g: destructure into g-wide pieces.
		var pieces = allocSubLimbs(alloc, "s", g, w)
		//
		insns = append(insns, bytecode.AddVec[W](pieces, []RegisterId{limb}))
		subs = append(subs, pieces...)
	}
	//
	return subs, insns
}

// scalarStep multiplies the sub-limb accumulator a by the constant k.  The
// constant is decomposed into g-wide sub-limbs kⱼ (exactly as the variable
// operands are), and every partial product aᵢ·kⱼ is materialised and bucketed
// against its column weight (i+j) — the same shape as binaryStep, but with a
// constant rather than a variable second operand.  Because each aᵢ·kⱼ multiplies
// two g-bit values it is at most 2·g bits, so however wide k is the multiply
// still fits the field bandwidth (see mulGranularity).  Zero sub-limbs of k are
// skipped: they contribute nothing to any column.
func scalarStep[W word.Word[W]](alloc Allocator[W], g uint, a []RegisterId, k W,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		kSubs     = descriptor.SplitConstant(k, g)
		fullWidth = uint(k.BigInt().BitLen()) + groupWidth(alloc, a)
		partials  = map[uint][]RegisterId{}
		insns     []Bytecode[W]
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
	outs, code := accumulate(alloc, columns(fullWidth, g, partials), columnWidth(fullWidth, g), partials)
	//
	return outs, append(insns, code...)
}

// binaryStep multiplies two sub-limb operands together, materialising every
// partial product aᵢ·bⱼ into fresh sub-limbs, bucketing them by column weight
// and accumulating the columns into the full-width product.
func binaryStep[W word.Word[W]](alloc Allocator[W], g uint, a, b []RegisterId, one W,
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		fullWidth = groupWidth(alloc, a) + groupWidth(alloc, b)
		partials  = map[uint][]RegisterId{}
		insns     []Bytecode[W]
	)
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
	outs, code := accumulate(alloc, columns(fullWidth, g, partials), columnWidth(fullWidth, g), partials)
	//
	return outs, append(insns, code...)
}

// assignToTarget lays the accumulated full-width product into the target's
// RegisterWidth limbs.  Columns below the target width are output at their
// natural width and reassembled into the target limbs; columns at or beyond the
// target width are output at width zero, which forces the corresponding product
// bits to be zero — i.e. rejects any product that overflows the target.
func assignToTarget[W word.Word[W]](alloc Allocator[W], g uint, product, targetLimbs []RegisterId, targetWidth uint,
) []Bytecode[W] {
	//
	var (
		partials = singletonColumns(product)
		numCols  = max(ceilDiv(targetWidth, g), uint(len(product)))
	)
	// Columns at or beyond the target width get zero-width outputs (columnWidth
	// returns 0), which forces those product bits to zero — the overflow check.
	outs, insns := accumulate(alloc, numCols, columnWidth(targetWidth, g), partials)
	//
	return append(insns, reassembleToTarget(alloc, g, outs, targetLimbs)...)
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

// columns returns the number of accumulation columns needed to hold a product
// of the given full width, ensuring every bucketed sub-limb has a home.
func columns(fullWidth, g uint, partials map[uint][]RegisterId) uint {
	var n = ceilDiv(fullWidth, g)
	//
	for w := range partials {
		n = max(n, w+1)
	}
	//
	return n
}

// columnWidth returns a column-width function for a value of the given total
// bit-width: column c holds min(g, total - c·g) bits, and zero once the column
// lies entirely at or beyond total.  A zero-width column forces its bits to
// zero, which is how bits beyond a target width are rejected as overflow.
func columnWidth(total, g uint) func(uint) uint {
	return func(c uint) uint {
		if lo := c * g; lo < total {
			return min(g, total-lo)
		}
		//
		return 0
	}
}

// accumulate lowers a set of weight-bucketed sub-limbs into one output sub-limb
// per column, threading carries between columns.  It reuses the addition carry
// machinery: each column is a multi-operand add whose overflow (a carry of
// weight column+1) is spliced into the next column, and the final column has no
// carry so any leftover overflow makes the constraints unsatisfiable.
func accumulate[W word.Word[W]](alloc Allocator[W], numCols uint, colWidth func(uint) uint,
	partials map[uint][]RegisterId) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		chunks = make([]partAdd[W], numCols)
		outs   []RegisterId
	)
	//
	for c := range numCols {
		var o = alloc.Allocate("m", util.Some(colWidth(c)))
		//
		outs = append(outs, o)
		chunks[c].targets = []RegisterId{o}
		chunks[c].sources = partials[c]
	}
	// Thread carries between columns (reused from Addition).
	chunks = insertAddCarryLines(alloc, chunks)
	// Lower each column to a vectored add (reused from Addition).
	return outs, array.Map(chunks, addAssignment[W])
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

// reassembleToTarget joins the low column sub-limbs into the target's
// RegisterWidth limbs, emitting one concatenation per target limb.  Because g
// divides RegisterWidth, every target-limb boundary is also a sub-limb
// boundary, so each target limb consumes a whole number of consecutive column
// sub-limbs.  Any higher (zero-width) columns are left untouched.
func reassembleToTarget[W word.Word[W]](alloc Allocator[W], g uint, outs, targetLimbs []RegisterId,
) []Bytecode[W] {
	//
	var (
		zero  W
		insns []Bytecode[W]
		slot  uint
	)
	//
	for _, tl := range targetLimbs {
		var (
			tw  = alloc.Register(tl).Bitwidth().Unwrap()
			n   = ceilDiv(tw, g)
			end = min(slot+n, uint(len(outs)))
		)
		//
		if slot == end {
			insns = append(insns, bytecode.LoadConst(tl, zero))
		} else {
			insns = append(insns, bytecode.Concat[W]([]RegisterId{tl}, outs[slot:end]))
		}
		//
		slot += n
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
