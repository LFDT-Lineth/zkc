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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// emitBitwise emits the single-target bitwise ops: and/or/xor (executeAnd/Or/
// Xor), not (executeNot) and shifts (executeShl/Shr).  AND/OR/XOR/SHR map to
// the plain Go operators; NOT and SHL additionally mask to the operation bit
// width (word.Not / word.Shl).  Wide (two-limb) operands compute lane-wise,
// with runtime shifts going through the shl128/shr128 helpers.  The result is
// then stored with the usual width check.
func (g *generator) emitBitwise(c *code, fn *descFunction, x *bytecode.Bitwise[word.Uint]) error {
	target, err := g.limbOf(fn, x.Target)
	if err != nil {
		return err
	}

	lhs, err := g.registerOperand(fn, x.Left)
	if err != nil {
		return err
	}

	// NOT is unary (its operand is duplicated across Left and Right); the rest
	// read a right operand.
	var rhs operand
	if x.Op != bytecode.OP_NOT {
		if rhs, err = g.registerOperand(fn, x.Right); err != nil {
			return err
		}
	}

	bitwidth := uint(x.Bitwidth)
	if bitwidth > 128 && (x.Op == bytecode.OP_NOT || x.Op == bytecode.OP_SHL) {
		return fmt.Errorf("gogen: %s with bit width u%d unsupported (exceeds 128 bits)", bitwiseName(x.Op), bitwidth)
	}

	var val operand

	switch x.Op {
	case bytecode.OP_AND:
		val = operand{expr: fmt.Sprintf("%s & %s", lhs.expr, rhs.expr), max: bigMin(lhs.max, rhs.max)}
		if lhs.wide() && rhs.wide() {
			val.hi = fmt.Sprintf("%s & %s", lhs.hi, rhs.hi)
		}
	case bytecode.OP_OR:
		val = wideLanes(lhs, rhs, "|", orMax(lhs.max, rhs.max))
	case bytecode.OP_XOR:
		val = wideLanes(lhs, rhs, "^", orMax(lhs.max, rhs.max))
	case bytecode.OP_NOT:
		// (^x) mod 2^bw: bits of x above bw are dropped by the mask; bits of a
		// narrow x in 64..bw-1 are zero and flip to one.
		if bitwidth <= 64 {
			val = operand{expr: maskExpr(fmt.Sprintf("^%s", lhs.expr), bitwidth), max: widthMax(bitwidth)}
		} else {
			val = operand{
				expr: fmt.Sprintf("^%s", lhs.expr),
				hi:   maskExpr(fmt.Sprintf("^%s", lhs.hiOr0()), bitwidth-64),
				max:  widthMax(bitwidth),
			}
		}
	case bytecode.OP_SHL:
		// (x << n) mod 2^bw.  For bw ≤ 64 only the low limb can contribute
		// (result bit j < 64 comes from x bit j-n, also below 64); Go's
		// variable shift already yields 0 once the count reaches 64.
		if bitwidth <= 64 {
			val = operand{expr: maskExpr(fmt.Sprintf("%s << %s", lhs.expr, rhs.expr), bitwidth), max: widthMax(bitwidth)}
			break
		}

		g.useHelper(helperShl128)

		return g.pairCall(c, "shl128", lhs, rhs, target, func(lo, hi string) operand {
			return operand{expr: lo, hi: maskExpr(hi, bitwidth-64), max: widthMax(bitwidth)}
		})
	case bytecode.OP_SHR:
		if !lhs.wide() {
			val = operand{expr: fmt.Sprintf("%s >> %s", lhs.expr, rhs.expr), max: lhs.max}
			break
		}

		g.useHelper(helperShr128)

		return g.pairCall(c, "shr128", lhs, rhs, target, func(lo, hi string) operand {
			return operand{expr: lo, hi: hi, max: lhs.max}
		})
	default:
		return fmt.Errorf("gogen: unsupported bitwise operation (%d)", x.Op)
	}

	return g.storeValue(c, storeView{single: &target, total: target.width}, val)
}

// bitwiseName names a bitwise operation for error messages (Operation.Prefix
// covers only the arithmetic and and/or/xor operations).
func bitwiseName(op bytecode.Operation) string {
	switch op {
	case bytecode.OP_NOT:
		return "BIT_NOT"
	case bytecode.OP_SHL:
		return "BIT_SHL"
	case bytecode.OP_SHR:
		return "BIT_SHR"
	default:
		return fmt.Sprintf("op(%d)", op)
	}
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

// emitDivRem emits DIV / REM (executeDiv/Rem): a zero divisor fails, otherwise
// the result is the plain Go quotient/remainder.
func (g *generator) emitDivRem(c *code, fn *descFunction, x *bytecode.DivRem[word.Uint]) error {
	target, err := g.limbOf(fn, x.Target)
	if err != nil {
		return err
	}

	lhs, err := g.registerOperand(fn, x.Dividend)
	if err != nil {
		return err
	}

	var rhs operand

	if x.Divisor.IsConstant() {
		rhs, err = constOperand(x.Divisor.AsConstant())
	} else {
		rhs, err = g.registerOperand(fn, x.Divisor.AsRegister())
	}

	if err != nil {
		return err
	}

	if lhs.wide() || rhs.wide() {
		return fmt.Errorf("gogen: division operand wider than 64 bits unsupported")
	}

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
	if x.Opcode == encoding.REM {
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

// emitShiftHelpers writes the variable-shift helpers actually referenced by
// the generated program.
func (g *generator) emitShiftHelpers(c *code) {
	if g.usesHelper(helperShl128) {
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

	if g.usesHelper(helperShr128) {
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

	if g.usesHelper(helperWideShl) {
		c.line("// wideShl shifts little-endian words into a zeroed destination.")
		c.line("func wideShl(dst, src []uint64, n uint64) {")
		c.line("word, shift := int(n/64), n%64")
		c.line("for i, v := range src {")
		c.line("j := i + word")
		c.line("if j >= len(dst) {")
		c.line("break")
		c.line("}")
		c.line("dst[j] |= v << shift")
		c.line("if shift != 0 && j+1 < len(dst) {")
		c.line("dst[j+1] |= v >> (64 - shift)")
		c.line("}")
		c.line("}")
		c.line("}")
		c.line("")
	}

	if g.usesHelper(helperWideShr) {
		c.line("// wideShr shifts little-endian words into a zeroed destination.")
		c.line("func wideShr(dst, src []uint64, n uint64) {")
		c.line("word, shift := int(n/64), n%64")
		c.line("for j := range dst {")
		c.line("i := j + word")
		c.line("if i >= len(src) {")
		c.line("break")
		c.line("}")
		c.line("dst[j] = src[i] >> shift")
		c.line("if shift != 0 && i+1 < len(src) {")
		c.line("dst[j] |= src[i+1] << (64 - shift)")
		c.line("}")
		c.line("}")
		c.line("}")
		c.line("")
	}
}
