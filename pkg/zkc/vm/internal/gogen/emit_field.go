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

package gogen

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// fieldHelpers records which mod-P helpers the program references, so only
// those are emitted.
type fieldHelpers struct {
	add bool
	sub bool
	mul bool
}

func (f fieldHelpers) any() bool { return f.add || f.sub || f.mul }

// emitFieldOp emits the mod-P chains (executeFieldAdd/Sub/Mul) with the
// machine's prime modulus baked in as a constant.  Only moduli up to 64
// bits are supported — anything wider implies wide registers, equally out of
// scope for now.
func (g *generator) emitFieldOp(c *code, fn *descFunction, x *bytecode.FieldArith[word.Uint]) error {
	if g.modulus.BitLen() > 64 {
		return fmt.Errorf("gogen: modulus 0x%s wider than 64 bits unsupported", g.modulus.Text(16))
	}

	target, err := g.limbOf(fn, x.Target)
	if err != nil {
		return err
	}

	w := target.width

	srcs, err := g.registerOperands(fn, x.Sources)
	if err != nil {
		return err
	}

	if anyWide(srcs) {
		return fmt.Errorf("gogen: mod-P operand wider than 64 bits unsupported")
	}

	konst, err := uintConst(x.Constant)
	if err != nil {
		return err
	}

	pm1 := new(big.Int).Sub(g.modulus, big.NewInt(1)) // results are reduced: ≤ P-1

	var expr string

	// The mod-P helpers build on math/bits, so usesBits is set only where one is
	// actually emitted — the no-source add/mul paths store a constant directly.
	switch x.Op {
	case bytecode.OP_ADDMOD_P:
		// executeFieldAdd: val = constant; val = (val + src) mod P per source.
		// With no sources the (unreduced) constant is stored as-is.
		if len(srcs) == 0 {
			g.storeKnown(c, storeView{single: &target, total: w}, new(big.Int).SetUint64(konst))
			return nil
		}

		g.usesModP.add, g.usesBits = true, true
		expr = fmt.Sprintf("%d", konst)

		for _, s := range srcs {
			expr = fmt.Sprintf("addModP(%s, %s)", expr, s.expr)
		}
	case bytecode.OP_SUBMOD_P:
		// executeFieldSub: val = src0 - src1 - … (mod P), then always - constant
		// (mod P) — so the result is reduced even when the constant is zero.
		// With no sources the seed is the zero word.
		g.usesModP.sub, g.usesBits = true, true
		expr = "0"

		if len(srcs) > 0 {
			expr = srcs[0].expr
			for _, s := range srcs[1:] {
				expr = fmt.Sprintf("subModP(%s, %s)", expr, s.expr)
			}
		}

		expr = fmt.Sprintf("subModP(%s, %d)", expr, konst)
	case bytecode.OP_MULMOD_P:
		// executeFieldMul: val = constant; val = (val · src) mod P per source.
		// With no sources the (unreduced) constant is stored as-is.
		if len(srcs) == 0 {
			g.storeKnown(c, storeView{single: &target, total: w}, new(big.Int).SetUint64(konst))
			return nil
		}

		g.usesModP.mul, g.usesBits = true, true
		expr = fmt.Sprintf("%d", konst)

		for _, s := range srcs {
			expr = fmt.Sprintf("mulModP(%s, %s)", expr, s.expr)
		}
	default:
		return fmt.Errorf("gogen: unsupported field operation (%d)", x.Op)
	}

	g.assignSingle(c, target, operand{expr: expr, max: pm1})

	return nil
}

// emitUintToField assembles the uint sources into the native target — exactly a
// concatenation with a single native target — then reduces modulo P.  The
// reduction is elided when interval analysis proves the assembled value is
// already canonical (< P).
func (g *generator) emitUintToField(c *code, fn *descFunction, x *bytecode.UintToField[word.Uint]) error {
	total := uint(0)

	for _, id := range x.Source {
		w, err := g.regWidth(fn, id)
		if err != nil {
			return err
		}

		total += w
	}

	if total > 64 {
		return fmt.Errorf("gogen: field-cast operand wider than 64 bits unsupported")
	}

	if err := g.emitConcat(c, fn, &bytecode.Cat[word.Uint]{Targets: []regId{x.Target}, Sources: x.Source}); err != nil {
		return err
	}
	// A uint-to-field cast is reduction modulo P.
	if g.iv.boundOf(x.Target).Cmp(g.modulus) >= 0 {
		c.linef("%s %%= %s", reg(x.Target), bigLit(g.modulus))
		g.iv.assign(x.Target, new(big.Int).Sub(g.modulus, big.NewInt(1)))
	}

	return nil
}

// emitFieldToUint distributes the native source across the uint targets — exactly
// a concatenation with a single native source.  The targets' range checks enforce
// that the (canonical) value fits.
func (g *generator) emitFieldToUint(c *code, fn *descFunction, x *bytecode.FieldToUint[word.Uint]) error {
	return g.emitConcat(c, fn, &bytecode.Cat[word.Uint]{Targets: x.Target, Sources: []regId{x.Source}})
}

// emitModPHelpers writes the mod-P helpers (with the modulus baked in), each
// only when referenced.  Operands need not be pre-reduced: each helper reduces
// its inputs, matching word.Uint's AddMod/SubMod/MulMod (big.Int Mod yields a
// result in [0, P)).
func (g *generator) emitModPHelpers(c *code) {
	if !g.usesModP.any() {
		return
	}

	c.linef("const modP = %s // the machine's prime modulus", bigLit(g.modulus))
	c.line("")

	if g.usesModP.add {
		c.line("func addModP(a, b uint64) uint64 {")
		c.line("s, carry := bits.Add64(a%modP, b%modP, 0)")
		c.line("if carry != 0 || s >= modP {")
		c.line("s -= modP")
		c.line("}")
		c.line("return s")
		c.line("}")
		c.line("")
	}

	if g.usesModP.sub {
		c.line("func subModP(a, b uint64) uint64 {")
		c.line("d, borrow := bits.Sub64(a%modP, b%modP, 0)")
		c.line("if borrow != 0 {")
		c.line("d += modP")
		c.line("}")
		c.line("return d")
		c.line("}")
		c.line("")
	}

	if g.usesModP.mul {
		// The high product word is below modP (operands are reduced), which is
		// exactly bits.Div64's precondition.
		c.line("func mulModP(a, b uint64) uint64 {")
		c.line("hi, lo := bits.Mul64(a%modP, b%modP)")
		c.line("_, rem := bits.Div64(hi, lo, modP)")
		c.line("return rem")
		c.line("}")
		c.line("")
	}
}

// emitWideIntHelper writes the portable uint64-word to big.Int conversion used
// only by the multiword division fallback (see emitWideDivision).  SetBits
// adopts the prepared word slice in one step instead of rebuilding the integer
// through repeated shifts and ORs.  The input words are copied, not retained,
// so callers can pass slices of stack-backed arrays without forcing them onto
// the heap.
func (g *generator) emitWideIntHelper(c *code) {
	if !g.usesHelper(helperWideInt) {
		return
	}

	c.line("func wideInt(words []uint64) (out big.Int) {")
	c.line("if bits.UintSize == 64 {")
	c.line("limbs := make([]big.Word, len(words))")
	c.line("for i, word := range words {")
	c.line("limbs[i] = big.Word(word)")
	c.line("}")
	c.line("out.SetBits(limbs)")
	c.line("return out")
	c.line("}")
	c.line("limbs := make([]big.Word, 2*len(words))")
	c.line("for i, word := range words {")
	c.line("limbs[2*i] = big.Word(word)")
	c.line("limbs[2*i+1] = big.Word(word >> 32)")
	c.line("}")
	c.line("out.SetBits(limbs)")
	c.line("return out")
	c.line("}")
	c.line("")
}

// emitIntrinsic dispatches DIV_HINT and the wide operations introduced by
// register splitting.  Shifts stay on native uint64 words; division and
// remainder divide natively whenever both runtime values fit a single word,
// falling back to big.Int only for genuinely multiword values.
func (g *generator) emitIntrinsic(c *code, fn *descFunction, x *bytecode.Intrinsic[word.Uint]) error {
	switch x.Op {
	case bytecode.DIV_HINT:
		if len(x.Sources) != 2 || len(x.Targets) != 3 {
			return fmt.Errorf("gogen: malformed division hint (%d sources, %d targets)", len(x.Sources), len(x.Targets))
		}

		return g.emitDivHint(c, fn, x)
	case bytecode.WIDE_SHL, bytecode.WIDE_SHR, bytecode.WIDE_DIV, bytecode.WIDE_REM:
		if len(x.Sources) != 2 || len(x.Targets) != 1 {
			return fmt.Errorf("gogen: malformed wide intrinsic (%d sources, %d targets)",
				len(x.Sources), len(x.Targets))
		}

		if x.Op == bytecode.WIDE_DIV || x.Op == bytecode.WIDE_REM {
			return g.emitWideDivision(c, fn, x)
		}

		return g.emitWideShift(c, fn, x)
	default:
		return fmt.Errorf("gogen: unsupported intrinsic operation (%d)", x.Op)
	}
}

// emitDivHint emits DIV_HINT, which sets targets[0] = q, targets[1] = r,
// targets[2] = w where
//
//	q = dividend / divisor,  r = dividend % divisor,  w = divisor - r - 1.
//
// A zero divisor fails.  Since r < divisor, w never underflows (the oracle's
// underflow checks are unreachable), so none are emitted.  Each operand is a
// register vector: single-limb vectors read/write a single register, while
// multi-limb vectors (produced by register-splitting a value on a narrow field)
// are reconstructed / distributed across their limbs (see assembleHintOperand
// and storeNamed).  Operands whose total width exceeds 64 bits share the
// multiword division path (emitWideDivision).
func (g *generator) emitDivHint(c *code, fn *descFunction, x *bytecode.Intrinsic[word.Uint]) error {
	for _, source := range x.Sources {
		width, err := g.operandWidth(fn, source)
		if err != nil {
			return err
		}

		if width > 64 {
			return g.emitWideDivision(c, fn, x)
		}
	}

	dividend, err := g.assembleIntrinsicOperand(fn, x.Sources[0])
	if err != nil {
		return err
	}

	divisor, err := g.assembleIntrinsicOperand(fn, x.Sources[1])
	if err != nil {
		return err
	}

	if divisor.isZero() {
		c.linef("fail(%q) // divisor is the constant zero", "division by zero")
		return nil
	}
	// Each target vector is distributed least-significant-limb first (storeNamed's
	// convention), so the register list is reversed: RegisterVector.Base holds the
	// most-significant limb (see register_vector.go), the opposite of storeNamed.
	targets := make([]storeView, len(x.Targets))

	for i, vec := range x.Targets {
		store, err := g.buildStore(fn, reversedRegisters(vec))
		if err != nil {
			return err
		}

		targets[i] = store
	}

	var inner error

	c.block(func() {
		if divisor.val == nil {
			c.linef("if %s == 0 {", divisor.expr)
			c.line(`fail("division by zero")`)
			c.line("}")
		}
		// Snapshot quotient/remainder before touching any target register (a
		// target may alias a source).
		c.linef("q, r := %s / %s, %s %% %s", dividend.expr, divisor.expr, dividend.expr, divisor.expr)
		c.linef("w := %s - r - 1", divisor.expr)

		divisorMax := new(big.Int).Sub(divisor.max, big.NewInt(1))
		for i, op := range []operand{
			{expr: "q", max: dividend.max},
			{expr: "r", max: bigMin(dividend.max, divisorMax)},
			{expr: "w", max: divisorMax},
		} {
			if inner = g.storeNamed(c, targets[i], op); inner != nil {
				return
			}
		}
	})

	return inner
}

// vectorWidth is the total bit width of a register vector.
func (g *generator) vectorWidth(fn *descFunction, vec bytecode.RegisterVector) (uint, error) {
	var total uint

	for _, id := range vec.Registers() {
		w, err := g.regWidth(fn, id)
		if err != nil {
			return 0, err
		}

		total += w
	}

	return total, nil
}

// packWords flattens a register vector into little-endian 64-bit word
// expressions (plus the vector's total bit width).  Any limb layout is
// accepted: full 64-bit words (fast-mode splitting), narrow limbs packed
// several to a word (e.g. the 16-bit limbs of tracing-mode splitting), limbs
// straddling a word boundary, and two-limb (lo/hi pair) registers.
func (g *generator) packWords(fn *descFunction, vec bytecode.RegisterVector) ([]string, uint, error) {
	// A piece is one Go variable holding up to 64 contiguous bits of the value.
	type piece struct {
		name  string
		width uint
	}

	var (
		pieces []piece
		total  uint
	)

	for _, id := range reversedRegisters(vec) {
		limb, err := g.limbOf(fn, id)
		if err != nil {
			return nil, 0, err
		}

		if limb.width <= 64 {
			pieces = append(pieces, piece{limb.lo(), limb.width})
		} else {
			pieces = append(pieces,
				piece{limb.lo(), 64},
				piece{limb.hiName(), limb.width - 64})
		}

		total += limb.width
	}

	if total == 0 {
		return nil, 0, fmt.Errorf("gogen: wide intrinsic has an empty register vector")
	}

	parts := make([][]string, (total+63)/64)
	offset := uint(0)

	for _, p := range pieces {
		w, shift := offset/64, offset%64

		if shift == 0 {
			parts[w] = append(parts[w], p.name)
		} else {
			parts[w] = append(parts[w], fmt.Sprintf("%s<<%d", p.name, shift))
		}
		// Bits spilling past the word boundary land in the next word.
		if shift+p.width > 64 {
			parts[w+1] = append(parts[w+1], fmt.Sprintf("%s>>%d", p.name, 64-shift))
		}

		offset += p.width
	}

	words := make([]string, len(parts))
	for i, ps := range parts {
		words[i] = strings.Join(ps, " | ")
	}

	return words, total, nil
}

// unpackWords distributes little-endian 64-bit words (word(i) yields the i'th
// word expression) across a register vector — the reverse of packWords.  Each
// limb takes its bits from one word (or two, when it straddles a word
// boundary), masked to the limb's width.
func (g *generator) unpackWords(c *code, fn *descFunction, vec bytecode.RegisterVector,
	word func(i uint) string) error {
	offset := uint(0)

	assign := func(lvalue string, width uint) {
		var (
			w, shift = offset / 64, offset % 64
			expr     = word(w)
		)

		if shift != 0 {
			expr = fmt.Sprintf("%s>>%d", expr, shift)
		}

		if shift+width > 64 {
			expr = fmt.Sprintf("%s | %s<<%d", expr, word(w+1), 64-shift)
		}

		c.linef("%s = %s", lvalue, maskExpr(expr, width))
		offset += width
	}

	for _, id := range reversedRegisters(vec) {
		limb, err := g.limbOf(fn, id)
		if err != nil {
			return err
		}

		if limb.width <= 64 {
			assign(limb.lo(), limb.width)
		} else {
			assign(limb.lo(), 64)
			assign(limb.hiName(), limb.width-64)
		}

		g.iv.assign(id, widthMax(limb.width))
	}

	return nil
}

// emitWideShift emits an allocation-free shift over stack-backed uint64 word
// arrays.  A non-zero upper shift-count word, or a count at least as wide as
// the target, leaves the zero-initialised result unchanged.
func (g *generator) emitWideShift(c *code, fn *descFunction, x *bytecode.Intrinsic[word.Uint]) error {
	value, _, err := g.packOperandWords(fn, x.Sources[0])
	if err != nil {
		return err
	}

	amount, _, err := g.packOperandWords(fn, x.Sources[1])
	if err != nil {
		return err
	}

	width, err := g.vectorWidth(fn, x.Targets[0])
	if err != nil {
		return err
	}

	helper := helperWideShl
	if x.Op == bytecode.WIDE_SHR {
		helper = helperWideShr
	}
	//
	g.useHelper(helper)

	condition := fmt.Sprintf("n < %d", width)
	if len(amount) > 1 {
		condition = fmt.Sprintf("(%s) == 0 && %s",
			strings.Join(amount[1:], " | "), condition)
	}

	var inner error

	c.block(func() {
		c.linef("src := [...]uint64{%s}", strings.Join(value, ", "))
		c.linef("var dst [%d]uint64", (width+63)/64)
		c.linef("n := %s", amount[0])
		c.linef("if %s {", condition)
		c.linef("%s(dst[:], src[:], n)", helper)
		c.line("}")
		//
		inner = g.unpackWords(c, fn, x.Targets[0], func(i uint) string {
			return fmt.Sprintf("dst[%d]", i)
		})
	})

	return inner
}

// emitWideDivision emits WIDE_DIV / WIDE_REM and the wide (>64-bit) form of
// DIV_HINT.  Each operand is packed into a stack-backed word array, and values
// that both fit their least-significant word divide natively — so big.Int is
// confined to values that are genuinely multiword at runtime, not merely
// wide-typed.
func (g *generator) emitWideDivision(c *code, fn *descFunction, x *bytecode.Intrinsic[word.Uint]) error {
	dividend, nX, err := g.packOperandWords(fn, x.Sources[0])
	if err != nil {
		return err
	}

	divisor, nY, err := g.packOperandWords(fn, x.Sources[1])
	if err != nil {
		return err
	}
	// A result is one value of the division: its local word-array name, the
	// target vector receiving it, a bound on the value (q ≤ dividend; r < divisor
	// and never above the dividend; w = divisor - r - 1 < divisor), and its
	// single-word expression for the native fast path.
	type result struct {
		name  string
		vec   bytecode.RegisterVector
		bound *big.Int
		fast  string
	}

	var (
		divisorMax = new(big.Int).Sub(widthMax(nY), big.NewInt(1))
		results    []result
	)

	switch x.Op {
	case bytecode.WIDE_DIV:
		results = []result{{"q", x.Targets[0], widthMax(nX), "a[0] / b[0]"}}
	case bytecode.WIDE_REM:
		results = []result{{"r", x.Targets[0], bigMin(widthMax(nX), divisorMax), "a[0] % b[0]"}}
	default: // DIV_HINT (see emitDivHint)
		results = []result{
			{"q", x.Targets[0], widthMax(nX), "a[0] / b[0]"},
			{"r", x.Targets[1], bigMin(widthMax(nX), divisorMax), "a[0] % b[0]"},
			{"w", x.Targets[2], divisorMax, "b[0] - r[0] - 1"},
		}
	}
	// Word counts per result target; also the fits-check widths.
	widths := make([]uint, len(results))
	for i, res := range results {
		if widths[i], err = g.vectorWidth(fn, res.vec); err != nil {
			return err
		}
	}
	// High words of both operands: all zero at runtime means the values fit
	// their least-significant words and divide natively.
	var high []string
	for i := 1; i < len(dividend); i++ {
		high = append(high, fmt.Sprintf("a[%d]", i))
	}

	for i := 1; i < len(divisor); i++ {
		high = append(high, fmt.Sprintf("b[%d]", i))
	}

	if len(high) > 0 {
		g.useHelper(helperWideInt)
		g.usesBits = true
	}

	zero := make([]string, len(divisor))
	for i := range divisor {
		zero[i] = fmt.Sprintf("b[%d]", i)
	}

	var inner error

	c.block(func() {
		// Packing snapshots the sources before any target register is written
		// (a target may alias a source).
		c.linef("a := [%d]uint64{%s}", len(dividend), strings.Join(dividend, ", "))
		c.linef("b := [%d]uint64{%s}", len(divisor), strings.Join(divisor, ", "))
		c.linef("if (%s) == 0 {", strings.Join(zero, " | "))
		c.line(`fail("division by zero")`)
		c.line("}")

		for i, res := range results {
			c.linef("var %s [%d]uint64", res.name, (widths[i]+63)/64)
		}

		if len(high) > 0 {
			c.linef("if (%s) == 0 {", strings.Join(high, " | "))
		}

		for i, res := range results {
			c.linef("%s[0] = %s", res.name, res.fast)
			// A single word always fits a target of 64+ bits; narrower targets
			// need the runtime check only when the bound does not already fit.
			if widths[i] < 64 && !fits(res.bound, widths[i]) {
				c.linef("if %s[0] >= 1<<%d {", res.name, widths[i])
				c.linef("fail(%q)", widthFailMsg(widths[i]))
				c.line("}")
			}
		}

		if len(high) > 0 {
			bigs := make([]string, len(results))
			for i, res := range results {
				bigs[i] = res.name + "b"
			}

			c.line("} else {")
			c.line("x, y := wideInt(a[:]), wideInt(b[:])")
			c.linef("var %s big.Int", strings.Join(bigs, ", "))

			switch x.Op {
			case bytecode.WIDE_DIV:
				c.line("qb.Quo(&x, &y)")
			case bytecode.WIDE_REM:
				c.line("rb.Rem(&x, &y)")
			default:
				c.line("qb.QuoRem(&x, &y, &rb)")
				c.line("wb.Sub(&y, &rb)")
				c.line("wb.Sub(&wb, big.NewInt(1))")
			}

			for i, res := range results {
				if !fits(res.bound, widths[i]) {
					c.linef("if %s.BitLen() > %d {", bigs[i], widths[i])
					c.linef("fail(%q)", widthFailMsg(widths[i]))
					c.line("}")
				}
				// Extraction consumes the big value, shifting a word at a time.
				for w := uint(0); w < (widths[i]+63)/64; w++ {
					c.linef("%s[%d] = %s.Uint64()", res.name, w, bigs[i])

					if w+1 < (widths[i]+63)/64 {
						c.linef("%s.Rsh(&%s, 64)", bigs[i], bigs[i])
					}
				}
			}

			c.line("}")
		}

		for _, res := range results {
			name := res.name

			if inner = g.unpackWords(c, fn, res.vec, func(i uint) string {
				return fmt.Sprintf("%s[%d]", name, i)
			}); inner != nil {
				return
			}
		}
	})

	return inner
}

// assembleHintOperand reconstructs a (possibly multi-limb) hint operand from its
// register vector into a single narrow (≤64-bit) uint64 expression, mirroring
// the interpreter's loadIntrinsicOperand.  The vector's Base register holds the
// most-significant limb, so limbs fold MSB-first: value = value<<width | limb.
// A single-limb vector reads the register directly (preserving its sharpened
// bound and any known value); anything wider than 64 bits stays unsupported.
func (g *generator) assembleHintOperand(fn *descFunction, vec bytecode.RegisterVector) (operand, error) {
	regs := vec.Registers() // Base .. Base+Len-1 == most- .. least-significant limb

	if len(regs) == 1 {
		o, err := g.registerOperand(fn, regs[0])
		if err != nil {
			return operand{}, err
		}
		// A single register wider than 64 bits (a lo/hi pair) has no narrow
		// division path — reject it, matching the pre-multi-limb behaviour.
		if o.wide() {
			return operand{}, fmt.Errorf("gogen: division hint operand wider than 64 bits unsupported")
		}

		return o, nil
	}

	var (
		exprs  = make([]string, len(regs))
		widths = make([]uint, len(regs))
		total  uint
	)

	for i, id := range regs {
		o, err := g.registerOperand(fn, id)
		if err != nil {
			return operand{}, err
		}

		if o.wide() {
			return operand{}, fmt.Errorf("gogen: division hint operand wider than 64 bits unsupported")
		}

		w, err := g.regWidth(fn, id)
		if err != nil {
			return operand{}, err
		}

		exprs[i] = o.expr
		widths[i] = w
		total += w
	}

	if total > 64 {
		return operand{}, fmt.Errorf("gogen: division hint operand wider than 64 bits unsupported")
	}
	// Fold MSB-first: exprs[0] is most significant, so each lower limb shifts the
	// running value up by that limb's own width before folding it in.
	expr := exprs[0]
	for i := 1; i < len(exprs); i++ {
		expr = fmt.Sprintf("(%s<<%d | %s)", expr, widths[i], exprs[i])
	}

	return operand{expr: expr, max: widthMax(total)}, nil
}

// operandWidth returns the total bit width of an intrinsic operand: the
// combined register width for a register vector, or the (minimal) value width
// for a constant.
func (g *generator) operandWidth(fn *descFunction, op bytecode.Operand[word.Uint]) (uint, error) {
	if op.IsConstant() {
		return uint(max(op.AsConstant().BigInt().BitLen(), 1)), nil
	}
	//
	return g.vectorWidth(fn, op.AsRegisterVector())
}

// assembleIntrinsicOperand reconstructs a hint operand into a single operand
// expression: register vectors via assembleHintOperand, and (single limb)
// constants as exact immediates.
func (g *generator) assembleIntrinsicOperand(fn *descFunction, op bytecode.Operand[word.Uint]) (operand, error) {
	if op.IsConstant() {
		return constOperand(op.AsConstant())
	}
	//
	return g.assembleHintOperand(fn, op.AsRegisterVector())
}

// packOperandWords flattens an intrinsic operand into little-endian 64-bit
// word expressions (plus its total bit width): register vectors via packWords,
// and (single limb) constants as hex literals.
func (g *generator) packOperandWords(fn *descFunction, op bytecode.Operand[word.Uint]) ([]string, uint, error) {
	if !op.IsConstant() {
		return g.packWords(fn, op.AsRegisterVector())
	}
	//
	var (
		value = op.AsConstant().BigInt()
		width = uint(max(value.BitLen(), 1))
		mask  = new(big.Int).SetUint64(math.MaxUint64)
		words []string
	)
	//
	for shift := uint(0); shift < width; shift += 64 {
		word := new(big.Int).Rsh(value, shift)
		word.And(word, mask)
		//
		words = append(words, fmt.Sprintf("%#x", word.Uint64()))
	}
	//
	return words, width, nil
}

// reversedRegisters returns a hint operand vector's registers in
// least-significant-limb-first order.  RegisterVector.Base holds the
// most-significant limb, whereas storeNamed / buildStore distribute
// least-significant-limb first, so the order is reversed.
func reversedRegisters(vec bytecode.RegisterVector) []regId {
	regs := vec.Registers()
	for i, j := 0, len(regs)-1; i < j; i, j = i+1, j-1 {
		regs[i], regs[j] = regs[j], regs[i]
	}

	return regs
}
