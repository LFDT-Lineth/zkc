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
	"math/big"

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

	srcs, err := g.operands(fn, x.Sources)
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

func (g *generator) emitUintToField(c *code, fn *descFunction, x *bytecode.UintToField[word.Uint]) error {
	if g.modulus.BitLen() > 64 {
		return fmt.Errorf("gogen: uint-to-field unsupported for modulus wider than 64 bits")
	}

	srcs, err := g.operands(fn, x.Source)
	if err != nil {
		return err
	}

	if anyWide(srcs) {
		return fmt.Errorf("gogen: field-cast operand wider than 64 bits unsupported")
	}

	widths := make([]uint, len(srcs))
	total := uint(0)

	for i, id := range x.Source {
		w, err := g.regWidth(fn, id)
		if err != nil {
			return err
		}

		widths[i] = w
		total += w
	}

	if total > 64 {
		return fmt.Errorf("gogen: field-cast operand wider than 64 bits unsupported")
	}
	// Fold sources MSB-first (sources[0] lowest).
	expr := srcs[len(srcs)-1].expr
	for i := len(srcs) - 2; i >= 0; i-- {
		expr = fmt.Sprintf("(%s<<%d | %s)", expr, widths[i], srcs[i].expr)
	}

	target, err := g.limbOf(fn, x.Target)
	if err != nil {
		return err
	}

	combined := operand{expr: expr, max: widthMax(total)}
	g.assignSingle(c, target, combined)
	// Canonicality check, unless the value is statically < P.
	pm1 := new(big.Int).Sub(g.modulus, big.NewInt(1))
	if combined.max.Cmp(pm1) > 0 {
		c.linef("if %s >= %s {", target.lo(), bigLit(g.modulus))
		c.linef("fail(%q) // value exceeds the field modulus", "field overflow")
		c.line("}")
	}

	return nil
}

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

// emitIntrinsic emits the DIV_HINT intrinsic (executeDivHint), the only intrinsic
// operation gogen supports: targets[0] = q, targets[1] = r, targets[2] = w where
//
//	q = dividend / divisor,  r = dividend % divisor,  w = divisor - r - 1.
//
// A zero divisor fails.  Since r < divisor, w never underflows (the oracle's
// underflow checks are unreachable), so none are emitted.  Each operand is a
// register vector: single-limb vectors read/write a single register, while
// multi-limb vectors (produced by register-splitting a value on a narrow field)
// are reconstructed / distributed across their limbs (see assembleHintOperand
// and storeNamed).  Operands whose total width exceeds 64 bits stay unsupported.
func (g *generator) emitIntrinsic(c *code, fn *descFunction, x *bytecode.Intrinsic[word.Uint]) error {
	if x.Op != bytecode.DIV_HINT {
		return fmt.Errorf("gogen: unsupported intrinsic operation (%d)", x.Op)
	}

	if len(x.Sources) != 2 || len(x.Targets) != 3 {
		return fmt.Errorf("gogen: malformed division hint (%d sources, %d targets)", len(x.Sources), len(x.Targets))
	}

	dividend, err := g.assembleHintOperand(fn, x.Sources[0])
	if err != nil {
		return err
	}

	divisor, err := g.assembleHintOperand(fn, x.Sources[1])
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

// assembleHintOperand reconstructs a (possibly multi-limb) hint operand from its
// register vector into a single narrow (≤64-bit) uint64 expression, mirroring
// the interpreter's loadIntrinsicOperand.  The vector's Base register holds the
// most-significant limb, so limbs fold MSB-first: value = value<<width | limb.
// A single-limb vector reads the register directly (preserving its sharpened
// bound and any known value); anything wider than 64 bits stays unsupported.
func (g *generator) assembleHintOperand(fn *descFunction, vec bytecode.RegisterVector) (operand, error) {
	regs := vec.Registers() // Base .. Base+Len-1 == most- .. least-significant limb

	if len(regs) == 1 {
		o, err := g.operand(fn, regs[0])
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
		o, err := g.operand(fn, id)
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
