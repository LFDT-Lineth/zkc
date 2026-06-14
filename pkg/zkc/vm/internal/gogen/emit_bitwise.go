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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// emitTypeB emits the single-target WordTypeB ops: bitwise (executeAnd/Or/Xor/
// Not), shifts (executeShl/Shr) and integer division (executeDiv/Rem).
// AND/OR/XOR/SHR map to the plain Go operators; NOT and SHL additionally mask
// to the operation bit width (word.Not / word.Shl).  DIV/REM fail on a zero
// divisor.  Wide (two-limb) operands compute lane-wise, with runtime shifts
// going through the shl128/shr128 helpers.  The result is then stored with the
// usual width check.
func (g *generator) emitTypeB(c *code, fn *wordFunction, x *instruction.WordTypeB) error {
	target, err := g.limbOf(fn, x.Target)
	if err != nil {
		return err
	}

	lhs, err := g.operand(fn, x.LeftSource)
	if err != nil {
		return err
	}

	// BIT_NOT is unary; the rest read a right operand.
	var rhs operand
	if x.Op != opcode.BIT_NOT {
		if rhs, err = g.operand(fn, x.RightSource); err != nil {
			return err
		}
	}

	if x.Bitwidth > 128 && (x.Op == opcode.BIT_NOT || x.Op == opcode.BIT_SHL) {
		return fmt.Errorf("gogen: %s with bit width u%d unsupported (exceeds 128 bits)", opName(x.Op), x.Bitwidth)
	}

	var val operand

	switch x.Op {
	case opcode.BIT_AND:
		val = operand{expr: fmt.Sprintf("%s & %s", lhs.expr, rhs.expr), max: bigMin(lhs.max, rhs.max)}
		if lhs.wide() && rhs.wide() {
			val.hi = fmt.Sprintf("%s & %s", lhs.hi, rhs.hi)
		}
	case opcode.BIT_OR:
		val = wideLanes(lhs, rhs, "|", orMax(lhs.max, rhs.max))
	case opcode.BIT_XOR:
		val = wideLanes(lhs, rhs, "^", orMax(lhs.max, rhs.max))
	case opcode.BIT_NOT:
		// (^x) mod 2^bw: bits of x above bw are dropped by the mask; bits of a
		// narrow x in 64..bw-1 are zero and flip to one.
		if x.Bitwidth <= 64 {
			val = operand{expr: maskExpr(fmt.Sprintf("^%s", lhs.expr), x.Bitwidth), max: widthMax(x.Bitwidth)}
		} else {
			val = operand{
				expr: fmt.Sprintf("^%s", lhs.expr),
				hi:   maskExpr(fmt.Sprintf("^%s", lhs.hiOr0()), x.Bitwidth-64),
				max:  widthMax(x.Bitwidth),
			}
		}
	case opcode.BIT_SHL:
		// (x << n) mod 2^bw.  For bw ≤ 64 only the low limb can contribute
		// (result bit j < 64 comes from x bit j-n, also below 64); Go's
		// variable shift already yields 0 once the count reaches 64.
		if x.Bitwidth <= 64 {
			val = operand{expr: maskExpr(fmt.Sprintf("%s << %s", lhs.expr, rhs.expr), x.Bitwidth), max: widthMax(x.Bitwidth)}
			break
		}

		g.usesShl128 = true

		return g.pairCall(c, "shl128", lhs, rhs, target, func(lo, hi string) operand {
			return operand{expr: lo, hi: maskExpr(hi, x.Bitwidth-64), max: widthMax(x.Bitwidth)}
		})
	case opcode.BIT_SHR:
		if !lhs.wide() {
			val = operand{expr: fmt.Sprintf("%s >> %s", lhs.expr, rhs.expr), max: lhs.max}
			break
		}

		g.usesShr128 = true

		return g.pairCall(c, "shr128", lhs, rhs, target, func(lo, hi string) operand {
			return operand{expr: lo, hi: hi, max: lhs.max}
		})
	case opcode.INT_DIV, opcode.INT_REM:
		if lhs.wide() || rhs.wide() {
			return fmt.Errorf("gogen: division operand wider than 64 bits unsupported")
		}

		return g.emitDivRem(c, x.Op, target, lhs, rhs)
	default:
		return fmt.Errorf("gogen: unsupported op %s", opName(x.Op))
	}

	return g.storeValue(c, storeView{single: &target, total: target.width}, val)
}

// wideLanes builds a lane-wise OR/XOR over possibly-wide operands.
func wideLanes(lhs, rhs operand, op string, bound *big.Int) operand {
	val := operand{expr: fmt.Sprintf("%s %s %s", lhs.expr, op, rhs.expr), max: bound}

	switch {
	case lhs.wide() && rhs.wide():
		val.hi = fmt.Sprintf("%s %s %s", lhs.hi, op, rhs.hi)
	case lhs.wide():
		val.hi = lhs.hi
	case rhs.wide():
		val.hi = rhs.hi
	}

	return val
}

// pairCall binds a two-result helper call (shl128/shr128) to block-scoped
// locals and stores the shaped result; the block keeps the temporaries from
// colliding across instructions.
func (g *generator) pairCall(c *code, helper string, lhs, rhs operand, target limb,
	shape func(lo, hi string) operand) error {
	var inner error

	c.block(func() {
		c.linef("tlo, thi := %s(%s, %s, %s)", helper, lhs.expr, lhs.hiOr0(), rhs.expr)
		inner = g.storeValue(c, storeView{single: &target, total: target.width}, shape("tlo", "thi"))
	})

	return inner
}

// emitDivRem emits INT_DIV / INT_REM: a zero divisor fails (executeDiv/Rem),
// otherwise the result is the plain Go quotient/remainder.
func (g *generator) emitDivRem(c *code, op opcode.OpCode, target limb, lhs, rhs operand) error {
	switch {
	case rhs.isZero():
		c.linef("fail(%q) // divisor is the constant zero", "division by zero")
		return nil
	case rhs.val == nil:
		c.linef("if %s == 0 {", rhs.expr)
		c.line(`fail("division by zero")`)
		c.line("}")
	}

	goOp, bound := "/", lhs.max
	if op == opcode.INT_REM {
		// The remainder is below the divisor (and never above the dividend).
		goOp = "%"
		bound = bigMin(lhs.max, new(big.Int).Sub(rhs.max, big.NewInt(1)))
	}

	g.assignSingle(c, target, operand{expr: fmt.Sprintf("%s %s %s", lhs.expr, goOp, rhs.expr), max: bound})

	return nil
}

// maskExpr masks expr to the low bitwidth bits, mirroring word.mask64: a width
// of 64 or more needs no mask.
func maskExpr(expr string, bitwidth uint) string {
	if bitwidth >= 64 {
		return expr
	}

	return fmt.Sprintf("(%s) & (1<<%d - 1)", expr, bitwidth)
}

// bigMin returns the smaller of two bounds.
func bigMin(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return a
	}

	return b
}

// orMax bounds `a | b` (and also `a ^ b`): all bits up to the wider operand.
func orMax(a, b *big.Int) *big.Int {
	n := a.BitLen()
	if m := b.BitLen(); m > n {
		n = m
	}

	return widthMax(uint(n))
}

// emitShiftHelpers writes the 128-bit variable shifts, each only when
// referenced.  Counts of 128 or more clear the value, matching the unbounded
// word semantics under a ≤128-bit mask.
func (g *generator) emitShiftHelpers(c *code) {
	if g.usesShl128 {
		c.line("func shl128(lo, hi, n uint64) (uint64, uint64) {")
		c.line("switch {")
		c.line("case n >= 128:")
		c.line("return 0, 0")
		c.line("case n >= 64:")
		c.line("return 0, lo << (n - 64)")
		c.line("case n == 0:")
		c.line("return lo, hi")
		c.line("default:")
		c.line("return lo << n, hi<<n | lo>>(64-n)")
		c.line("}")
		c.line("}")
		c.line("")
	}

	if g.usesShr128 {
		c.line("func shr128(lo, hi, n uint64) (uint64, uint64) {")
		c.line("switch {")
		c.line("case n >= 128:")
		c.line("return 0, 0")
		c.line("case n >= 64:")
		c.line("return hi >> (n - 64), 0")
		c.line("case n == 0:")
		c.line("return lo, hi")
		c.line("default:")
		c.line("return lo>>n | hi<<(64-n), hi >> n")
		c.line("}")
		c.line("}")
		c.line("")
	}
}
