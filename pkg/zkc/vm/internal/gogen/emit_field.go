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
	g.usesBits = true                                 // the mod-P helpers build on math/bits

	var expr string

	switch x.Op {
	case bytecode.OP_ADDMOD_P:
		// executeFieldAdd: val = constant; val = (val + src) mod P per source.
		// With no sources the (unreduced) constant is stored as-is.
		if len(srcs) == 0 {
			g.storeKnown(c, storeView{single: &target, total: w}, new(big.Int).SetUint64(konst))
			return nil
		}

		g.usesModP.add = true
		expr = fmt.Sprintf("%d", konst)

		for _, s := range srcs {
			expr = fmt.Sprintf("addModP(%s, %s)", expr, s.expr)
		}
	case bytecode.OP_SUBMOD_P:
		// executeFieldSub: val = src0 - src1 - … (mod P), then always - constant
		// (mod P) — so the result is reduced even when the constant is zero.
		// With no sources the seed is the zero word.
		g.usesModP.sub = true
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

		g.usesModP.mul = true
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

// emitHint emits the DIV_HINT hint (executeDivHint), the only supported hint
// operation: targets[0] = q, targets[1] = r, targets[2] = w where
//
//	q = dividend / divisor,  r = dividend % divisor,  w = divisor - r - 1.
//
// A zero divisor fails.  Since r < divisor, w never underflows (the oracle's
// underflow checks are unreachable), so none are emitted.
func (g *generator) emitHint(c *code, fn *descFunction, x *bytecode.Hint) error {
	if x.Op != bytecode.DIV_HINT {
		return fmt.Errorf("gogen: unsupported hint operation (%d)", x.Op)
	}

	if len(x.Sources) != 2 || len(x.Targets) != 3 {
		return fmt.Errorf("gogen: malformed division hint (%d sources, %d targets)", len(x.Sources), len(x.Targets))
	}
	// gogen only supports narrow (single-limb) hint operands; a value split
	// across several limb registers is not yet handled here.
	sourceIds, err := hintRegisters(x.Sources)
	if err != nil {
		return err
	}

	targetIds, err := hintRegisters(x.Targets)
	if err != nil {
		return err
	}

	srcs, err := g.operands(fn, sourceIds)
	if err != nil {
		return err
	}

	dividend, divisor := srcs[0], srcs[1]
	if anyWide(srcs) {
		return fmt.Errorf("gogen: division hint operand wider than 64 bits unsupported")
	}

	if divisor.isZero() {
		c.linef("fail(%q) // divisor is the constant zero", "division by zero")
		return nil
	}

	targets := make([]limb, len(targetIds))

	for i, id := range targetIds {
		l, err := g.limbOf(fn, id)
		if err != nil {
			return err
		}

		targets[i] = l
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
			if inner = g.storeNamed(c, storeView{single: &targets[i], total: targets[i].width}, op); inner != nil {
				return
			}
		}
	})

	return inner
}

// hintRegisters flattens single-register hint operand vectors into their
// register ids.  gogen only supports narrow (single-limb) hint operands; a
// multi-limb vector (produced by register-splitting a wide value) is not yet
// supported here.
func hintRegisters(vecs []bytecode.RegisterVector) ([]regId, error) {
	ids := make([]regId, len(vecs))

	for i, v := range vecs {
		if v.Len != 1 {
			return nil, fmt.Errorf("gogen: multi-limb division hint operand unsupported")
		}

		ids[i] = v.Base
	}

	return ids, nil
}
