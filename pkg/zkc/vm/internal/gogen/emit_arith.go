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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// emitArith dispatches the three integer arithmetic opcodes.  A single-register
// target is accumulated into directly (the common case); a multi-register target
// accumulates into a scratch `v` and distributes it via StoreAcross.
func (g *generator) emitArith(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint64]) error {
	// BIT_CONCAT shares the WordTypeA shape (vector target, register sources) but
	// has its own packing semantics.
	if x.Op == opcode.BIT_CONCAT {
		return g.emitConcat(c, fn, x)
	}

	store, err := g.buildStore(fn, x.Target)
	if err != nil {
		return err
	}

	srcs, err := g.operands(fn, x.Sources)
	if err != nil {
		return err
	}

	konst := x.Constant.Uint64()
	switch x.Op {
	case opcode.INT_ADD:
		g.emitAdd(c, srcs, konst, store)
	case opcode.INT_SUB:
		if len(srcs) == 0 {
			return fmt.Errorf("gogen: INT_SUB with no sources unsupported")
		}

		g.emitSub(c, srcs, konst, store)
	case opcode.INT_MUL:
		g.emitMul(c, srcs, konst, store)
	default:
		return fmt.Errorf("gogen: unsupported arithmetic op %s", opName(x.Op))
	}

	return nil
}

// emitAdd emits `target = const + Σ sources` with a carry-out check per addition
// (matching executeAdd's val.Add).  A single-register target accumulates directly
// into the register; otherwise it accumulates into a scratch `v` and distributes.
func (g *generator) emitAdd(c *code, sources []string, konst uint64, store storeView) {
	terms := addends(sources, konst)
	if store.single != nil && !aliasesAccumulator(store.single.reg, terms) {
		g.emitAddInto(c, *store.single, terms)
		return
	}

	g.emitAddScratch(c, sources, konst, store)
}

// emitAddInto accumulates `terms` (a non-empty addend list) directly into a
// single-register target, combining the first two addends so no zero seed is
// needed, then bit-width-checks the register.
func (g *generator) emitAddInto(c *code, target limb, terms []string) {
	if len(terms) == 1 {
		g.emitCopy(c, target, terms[0])
		return
	}

	g.usesBits = true
	g.curTemps.carry = true

	c.linef("%s, carry = bits.Add64(%s, %s, 0)", target.reg, terms[0], terms[1])
	g.emitOverflowCheck(c, runtimeCheck("carry != 0"), "arithmetic overflow")

	for _, t := range terms[2:] {
		c.linef("%s, carry = bits.Add64(%s, %s, 0)", target.reg, target.reg, t)
		g.emitOverflowCheck(c, runtimeCheck("carry != 0"), "arithmetic overflow")
	}

	g.emitWidthCheck(c, target, unknownValue())
}

// emitAddScratch is the multi-register (or self-aliasing) fallback: accumulate
// `const + Σ sources` into a scoped local `v`, then StoreAcross.  A constant-zero
// operand adds nothing and is skipped (it can never carry).
func (g *generator) emitAddScratch(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := uint64(%d)", konst)

		adds := nonZero(sources)
		if len(adds) > 0 {
			g.usesBits = true

			c.line("var carry uint64")
		}

		for _, s := range adds {
			c.linef("v, carry = bits.Add64(v, %s, 0)", s)
			g.emitOverflowCheck(c, runtimeCheck("carry != 0"), "arithmetic overflow")
		}

		if len(adds) == 0 {
			g.emitStoreValue(c, store, knownValue(konst))
		} else {
			g.emitStore(c, store)
		}
	})
}

// emitSub emits `target = sources[0] - sources[1] - … - const`, each step checked
// for underflow (matching executeSub).  Like emitAdd, a single-register target is
// accumulated into directly; otherwise a scratch `v` is distributed.
func (g *generator) emitSub(c *code, sources []string, konst uint64, store storeView) {
	if store.single != nil && !aliasesAccumulator(store.single.reg, sources) {
		g.emitSubInto(c, *store.single, sources, konst)
		return
	}

	g.emitSubScratch(c, sources, konst, store)
}

// emitSubInto subtracts directly into a single-register target.  The first
// subtraction combines the two leading operands (minuend and first subtrahend)
// so no copy seed is needed; the remaining subtrahends, then the constant, chain
// through the register.
func (g *generator) emitSubInto(c *code, target limb, sources []string, konst uint64) {
	subtrahends := sources[1:]
	if len(subtrahends) == 0 && konst == 0 {
		g.emitCopy(c, target, sources[0])
		return
	}

	g.usesBits = true
	g.curTemps.borrow = true

	if len(subtrahends) == 0 {
		// Only a constant subtrahend: sources[0] - const.
		c.linef("%s, borrow = bits.Sub64(%s, uint64(%d), 0)", target.reg, sources[0], konst)
		g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")
		g.emitWidthCheck(c, target, unknownValue())

		return
	}

	c.linef("%s, borrow = bits.Sub64(%s, %s, 0)", target.reg, sources[0], subtrahends[0])
	g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")

	for _, s := range subtrahends[1:] {
		c.linef("%s, borrow = bits.Sub64(%s, %s, 0)", target.reg, target.reg, s)
		g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")
	}

	if konst != 0 {
		c.linef("%s, borrow = bits.Sub64(%s, uint64(%d), 0)", target.reg, target.reg, konst)
		g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")
	}

	g.emitWidthCheck(c, target, unknownValue())
}

// emitSubScratch is the multi-register (or self-aliasing) fallback for INT_SUB.
func (g *generator) emitSubScratch(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := %s", sources[0])

		rest := sources[1:]
		if len(rest) > 0 || konst != 0 {
			g.usesBits = true

			c.line("var borrow uint64")
		}

		for _, s := range rest {
			c.linef("v, borrow = bits.Sub64(v, %s, 0)", s)
			g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")
		}

		if konst != 0 {
			c.linef("v, borrow = bits.Sub64(v, uint64(%d), 0)", konst)
			g.emitUnderflowCheck(c, runtimeCheck("borrow != 0"), "arithmetic underflow")
		}

		if len(rest) == 0 && konst == 0 {
			if value, ok := uint64Literal(sources[0]); ok {
				g.emitStoreValue(c, store, knownValue(value))

				return
			}
		}

		g.emitStore(c, store)
	})
}

// emitMul emits `target = const · Π sources`, flagging overflow when a 64-bit
// product overflows and the low word is non-zero (matching executeMul).  A
// single-register target is accumulated into directly; otherwise a scratch `v` is
// distributed.
func (g *generator) emitMul(c *code, sources []string, konst uint64, store storeView) {
	factors := mulFactors(sources, konst)
	if store.single != nil && !aliasesAccumulator(store.single.reg, factors) {
		g.emitMulInto(c, *store.single, factors)
		return
	}

	g.emitMulScratch(c, sources, konst, store)
}

// emitMulInto multiplies `factors` (a non-empty factor list) directly into a
// single-register target.  A single factor is a plain copy; two factors need only
// the high word of the product to detect overflow; three or more track overflow
// across the chain with `ov`.
func (g *generator) emitMulInto(c *code, target limb, factors []string) {
	switch len(factors) {
	case 1:
		g.emitCopy(c, target, factors[0])
		return
	case 2:
		g.usesBits = true
		g.curTemps.mulHi = true

		c.linef("hi, %s = bits.Mul64(%s, %s)", target.reg, factors[0], factors[1])
		g.emitOverflowCheck(c, runtimeCheck(fmt.Sprintf("hi != 0 && %s != 0", target.reg)), "arithmetic overflow")
	default:
		g.usesBits = true
		g.curTemps.mulHi = true
		g.curTemps.mulOv = true

		c.linef("hi, %s = bits.Mul64(%s, %s)", target.reg, factors[0], factors[1])
		c.line("ov = hi != 0")

		for _, f := range factors[2:] {
			c.linef("hi, %s = bits.Mul64(%s, %s)", target.reg, target.reg, f)
			c.line("ov = ov || hi != 0")
		}

		g.emitOverflowCheck(c, runtimeCheck(fmt.Sprintf("ov && %s != 0", target.reg)), "arithmetic overflow")
	}

	g.emitWidthCheck(c, target, unknownValue())
}

// emitMulScratch is the multi-register (or self-aliasing) fallback for INT_MUL.
func (g *generator) emitMulScratch(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := uint64(%d)", konst)

		if len(sources) > 0 {
			g.usesBits = true

			c.line("var ov bool")
			c.line("var hi uint64")

			for _, s := range sources {
				c.linef("hi, v = bits.Mul64(v, %s)", s)
				c.line("ov = ov || hi != 0")
			}

			g.emitOverflowCheck(c, runtimeCheck("ov && v != 0"), "arithmetic overflow")
		}

		if len(sources) == 0 {
			g.emitStoreValue(c, store, knownValue(konst))
		} else {
			g.emitStore(c, store)
		}
	})
}

// emitCopy assigns a single value expression into a single-register target and
// bit-width-checks it (a copy resolves the width check at generation time when
// the expression is a constant).
func (g *generator) emitCopy(c *code, target limb, expr string) {
	value := unknownValue()
	if v, ok := uint64Literal(expr); ok {
		value = knownValue(v)
	}

	if expr != target.reg {
		c.linef("%s = %s", target.reg, expr)
	}

	g.emitWidthCheck(c, target, value)
}

// emitWidthCheck bit-width-checks a single-register target against its declared
// width (a no-op at the machine word width), mirroring StackFrame.Store.
func (g *generator) emitWidthCheck(c *code, target limb, value valueInfo) {
	if target.width >= 64 {
		return
	}

	g.emitOverflowCheck(c, value.widthCheckExpr(target.reg, target.width),
		fmt.Sprintf("bit overflow (value exceeds u%d)", target.width))
}

// addends returns the addend list for INT_ADD with the sources first so the
// target register may safely alias the leading operands; a non-zero constant (or
// the lone constant when there are no sources) is appended.
func addends(sources []string, konst uint64) []string {
	if konst == 0 && len(sources) > 0 {
		return sources
	}

	return append(append([]string{}, sources...), fmt.Sprintf("uint64(%d)", konst))
}

// mulFactors returns the factor list for INT_MUL.  executeMul multiplies the
// constant first and then the sources in order, and overflow detection (whether
// any intermediate high word is non-zero) is order-sensitive — so the constant
// leads the list, unless it is the multiplicative identity with sources present,
// in which case it is dropped (a leading *1 never overflows and never reorders).
func mulFactors(sources []string, konst uint64) []string {
	if konst == 1 && len(sources) > 0 {
		return sources
	}

	return append([]string{fmt.Sprintf("uint64(%d)", konst)}, sources...)
}

// aliasesAccumulator reports whether the target register appears among the
// operands that are read after the accumulator is first written (everything past
// the first two operands, which are read together before the first write).  When
// it does, accumulating directly into the target would corrupt a later read, so
// the scratch-`v` fallback must be used instead.
func aliasesAccumulator(reg string, operands []string) bool {
	for i := 2; i < len(operands); i++ {
		if operands[i] == reg {
			return true
		}
	}

	return false
}

// emitStore writes the accumulated value `v` into the target, mirroring
// StackFrame.StoreAcross:
//   - single register: bit-width-check, then assign;
//   - multi register: distribute v big-endian (lowest register = LSB), masking
//     each register to its width; any bits left beyond the total width are an
//     overflow.  This is where carry bits land in the higher registers.
func (g *generator) emitStore(c *code, store storeView) {
	g.emitStoreValue(c, store, unknownValue())
}

func (g *generator) emitStoreValue(c *code, store storeView, value valueInfo) {
	if store.single != nil {
		w := store.single.width
		if w < 64 {
			g.emitOverflowCheck(c, value.widthCheckExpr("v", w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
		}

		c.linef("%s = v", store.single.reg)

		return
	}

	for _, l := range store.limbs {
		if l.width < 64 {
			c.linef("%s = v & ((1 << %d) - 1)", l.reg, l.width)
		} else {
			c.linef("%s = v", l.reg)
		}

		c.linef("v >>= %d", l.width)
	}

	if value.known {
		remaining := value.value
		for _, l := range store.limbs {
			remaining >>= l.width
		}

		g.emitOverflowCheck(c, knownCheck("v != 0", remaining != 0), "bit overflow (value exceeds total target width)")

		return
	}

	g.emitOverflowCheck(c, runtimeCheck("v != 0"), "bit overflow (value exceeds total target width)")
}

// emitConcat emits a BIT_CONCAT (`tn::…::t0 = sn::…::s0`): it packs the source
// registers into one value with sources[0] in the least-significant bits, then
// stores that value into the (possibly multi-limb) target.  Mirrors executeConcat
// in the reference word machine.
func (g *generator) emitConcat(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint64]) error {
	store, err := g.buildStore(fn, x.Target)
	if err != nil {
		return err
	}

	exprs := make([]string, len(x.Sources))
	widths := make([]uint, len(x.Sources))

	for i, id := range x.Sources {
		expr, err := g.operand(fn, id)
		if err != nil {
			return err
		}

		w, err := g.regWidth(fn, id)
		if err != nil {
			return err
		}

		exprs[i] = expr
		widths[i] = w
	}

	g.emitStoreExpr(c, store, concatExpr(exprs, widths))

	return nil
}

// concatExpr builds sn::…::s0 as a single expression with sources[0] in the
// least-significant bits (each higher source shifted in above the lower ones).
func concatExpr(exprs []string, widths []uint) string {
	expr := "uint64(0)"
	for i := len(exprs) - 1; i >= 0; i-- {
		expr = fmt.Sprintf("((%s << %d) | %s)", expr, widths[i], exprs[i])
	}

	return expr
}

// emitStoreExpr stores a single value expression into the target: a single
// register is assigned directly and bit-width-checked; a multi-register target
// distributes the value big-endian through a scratch `v` (StoreAcross).
func (g *generator) emitStoreExpr(c *code, store storeView, expr string) {
	if store.single != nil {
		g.emitCopy(c, *store.single, expr)
		return
	}

	c.block(func() {
		c.linef("v := %s", expr)
		g.emitStore(c, store)
	})
}
