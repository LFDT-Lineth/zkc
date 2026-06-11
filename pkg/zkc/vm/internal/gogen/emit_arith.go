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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// On the word.Uint machine arithmetic is exact — accumulation never overflows
// (executeAdd/Mul never trap) — and the only failure point is the store, where
// the value must fit the target register(s).  The generator therefore picks a
// representation per result from the static bound of its operands:
//
//   - bound < 2^64 and all operands narrow: a single Go expression;
//   - otherwise (up to 2^128): a lo/hi pair built with math/bits — this also
//     covers two-limb (64 < width ≤ 128) registers as operands and targets;
//   - beyond 2^128: a clean "unsupported" error.

// emitArith dispatches the WordTypeA opcodes (the vector-target instructions).
func (g *generator) emitArith(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint]) error {
	store, err := g.buildStore(fn, x.Target)
	if err != nil {
		return err
	}

	srcs, err := g.operands(fn, x.Sources)
	if err != nil {
		return err
	}

	if x.Op == opcode.BIT_CONCAT {
		return g.emitConcat(c, fn, x, store)
	}

	konst, err := uintConst(x.Constant)
	if err != nil {
		return err
	}

	switch x.Op {
	case opcode.INT_ADD:
		return g.emitAdd(c, srcs, konst, store)
	case opcode.INT_SUB:
		if len(srcs) == 0 {
			return fmt.Errorf("gogen: INT_SUB with no sources unsupported")
		}

		return g.emitSub(c, srcs, konst, store)
	case opcode.INT_MUL:
		return g.emitMul(c, srcs, konst, store)
	default:
		return fmt.Errorf("gogen: unsupported arithmetic op %s", opName(x.Op))
	}
}

// anyWide reports whether some operand is a lo/hi pair.
func anyWide(ops []operand) bool {
	for _, o := range ops {
		if o.wide() {
			return true
		}
	}

	return false
}

// emitAdd emits `target = const + Σ sources` (executeAdd: exact sum, then the
// store decides).  Constant-zero terms add nothing and are dropped.
func (g *generator) emitAdd(c *code, srcs []operand, konst uint64, store storeView) error {
	terms := []operand{}
	if konst != 0 {
		terms = append(terms, exact(new(big.Int).SetUint64(konst)))
	}

	for _, s := range srcs {
		if !s.isZero() {
			terms = append(terms, s)
		}
	}
	// Exact operands fold at generation time (also covers the no-source case).
	if allExact(terms) {
		sum := big.NewInt(0)
		for _, t := range terms {
			sum.Add(sum, t.val)
		}

		return g.storeValue(c, store, exactWide(sum))
	}

	bound := big.NewInt(0)
	for _, t := range terms {
		bound.Add(bound, t.max)
	}

	if fitsU64(bound) && !anyWide(terms) {
		if len(terms) == 1 {
			return g.storeValue(c, store, terms[0])
		}

		return g.storeValue(c, store, operand{expr: strings.Join(exprsOf(terms), " + "), max: bound})
	}

	if !fitsU128(bound) {
		return fmt.Errorf("gogen: addition bound exceeds 128 bits (unsupported)")
	}
	// Pair accumulation: carries land in hi rather than trapping.
	g.usesBits = true

	if len(terms) == 1 {
		return g.storeValue(c, store, terms[0])
	}

	var inner error

	c.block(func() {
		first, second := terms[0], terms[1]
		if len(terms) == 2 && !first.wide() && !second.wide() {
			c.linef("lo, hi := bits.Add64(%s, %s, 0)", first.expr, second.expr)
			inner = g.storePair(c, store, bound)

			return
		}

		c.linef("lo, hi := %s, uint64(%s)", first.expr, first.hiOr0())
		c.line("var c uint64")

		for _, t := range terms[1:] {
			c.linef("lo, c = bits.Add64(lo, %s, 0)", t.expr)

			if t.wide() {
				c.linef("hi += %s + c", t.hi)
			} else {
				c.line("hi += c")
			}
		}

		inner = g.storePair(c, store, bound)
	})

	return inner
}

// emitSub emits `target = sources[0] - sources[1] - … - const`, each step
// checked for underflow (executeSub: a negative intermediate is an error).
func (g *generator) emitSub(c *code, srcs []operand, konst uint64, store storeView) error {
	minuend := srcs[0]

	subtrahends := slices0(srcs[1:])
	if konst != 0 {
		// The constant is subtracted last, matching executeSub.
		subtrahends = append(subtrahends, exact(new(big.Int).SetUint64(konst)))
	}
	// Subtracting a provable zero never underflows and changes nothing.
	kept := subtrahends[:0]

	for _, s := range subtrahends {
		if !s.isZero() {
			kept = append(kept, s)
		}
	}

	subtrahends = kept

	if len(subtrahends) == 0 {
		return g.storeValue(c, store, minuend)
	}

	if allExact(append([]operand{minuend}, subtrahends...)) {
		val := new(big.Int).Set(minuend.val)
		for _, s := range subtrahends {
			val.Sub(val, s.val)
		}

		if val.Sign() < 0 {
			c.linef("fail(%q) // constant subtraction always underflows", "arithmetic underflow")
			return nil
		}

		return g.storeValue(c, store, exactWide(val))
	}

	g.usesBits = true

	var inner error

	if !minuend.wide() && !anyWide(subtrahends) {
		// Narrow path: the running value stays below the minuend.
		c.block(func() {
			c.linef("v, borrow := bits.Sub64(%s, %s, 0)", minuend.expr, subtrahends[0].expr)
			c.line("if borrow != 0 {")
			c.line(`fail("arithmetic underflow")`)
			c.line("}")

			for _, s := range subtrahends[1:] {
				c.linef("v, borrow = bits.Sub64(v, %s, 0)", s.expr)
				c.line("if borrow != 0 {")
				c.line(`fail("arithmetic underflow")`)
				c.line("}")
			}

			inner = g.storeNamed(c, store, operand{expr: "v", max: minuend.max})
		})

		return inner
	}
	// Pair path: subtract limb-wise with the borrow chained into hi; a borrow
	// out of hi is the underflow.
	c.block(func() {
		c.linef("lo, hi := %s, uint64(%s)", minuend.expr, minuend.hiOr0())
		c.line("var b uint64")

		for _, s := range subtrahends {
			c.linef("lo, b = bits.Sub64(lo, %s, 0)", s.expr)
			c.linef("hi, b = bits.Sub64(hi, %s, b)", s.hiOr0())
			c.line("if b != 0 {")
			c.line(`fail("arithmetic underflow")`)
			c.line("}")
		}

		inner = g.storePair(c, store, minuend.max)
	})

	return inner
}

// emitMul emits `target = const · Π sources` (executeMul: exact product, then
// the store decides).  The constant leads the factor list, matching the
// oracle's evaluation order, and is dropped when it is the identity.
func (g *generator) emitMul(c *code, srcs []operand, konst uint64, store storeView) error {
	factors := []operand{}
	if konst != 1 || len(srcs) == 0 {
		factors = append(factors, exact(new(big.Int).SetUint64(konst)))
	}

	factors = append(factors, srcs...)

	// A zero factor (or all-exact factors) folds at generation time.
	bound := big.NewInt(1)
	exactVal := big.NewInt(1)

	for _, f := range factors {
		bound.Mul(bound, f.max)

		if exactVal != nil && f.val != nil {
			exactVal.Mul(exactVal, f.val)
		} else {
			exactVal = nil
		}
	}

	if bound.Sign() == 0 {
		exactVal = big.NewInt(0)
	}

	if exactVal != nil {
		return g.storeValue(c, store, exactWide(exactVal))
	}
	// Drop exact-one factors (multiplicative identity); zero factors were
	// handled above, so what remains are genuine runtime factors.
	terms := []operand{}

	for _, f := range factors {
		if f.val == nil || f.val.Cmp(big.NewInt(1)) != 0 {
			terms = append(terms, f)
		}
	}

	if len(terms) == 1 {
		return g.storeValue(c, store, operand{expr: terms[0].expr, hi: terms[0].hi, max: bound})
	}

	if fitsU64(bound) {
		// A bound below 2^64 forces every factor narrow (wide ⇒ max ≥ 2^64).
		return g.storeValue(c, store, operand{expr: strings.Join(exprsOf(terms), " * "), max: bound})
	}

	if !fitsU128(bound) {
		return fmt.Errorf("gogen: multiplication bound exceeds 128 bits (unsupported)")
	}

	g.usesBits = true

	var inner error

	c.block(func() {
		first, second := terms[0], terms[1]
		// Seed the running pair with the first product.  All cross terms fit:
		// the product bound is below 2^128, so any partial high word is below
		// 2^64 and the hi additions cannot overflow.
		switch {
		case !first.wide() && !second.wide():
			c.linef("hi, lo := bits.Mul64(%s, %s)", first.expr, second.expr)
		case first.wide() && !second.wide():
			c.linef("hi, lo := bits.Mul64(%s, %s)", first.expr, second.expr)
			c.linef("hi += %s * %s", first.hi, second.expr)
		case !first.wide() && second.wide():
			c.linef("hi, lo := bits.Mul64(%s, %s)", first.expr, second.expr)
			c.linef("hi += %s * %s", first.expr, second.hi)
		default:
			c.linef("hi, lo := bits.Mul64(%s, %s)", first.expr, second.expr)
			c.linef("hi += %s*%s + %s*%s", first.expr, second.hi, first.hi, second.expr)
		}

		for i, f := range terms[2:] {
			assign := "="
			if i == 0 {
				assign = ":="
			}

			c.linef("hi2, lo2 %s bits.Mul64(lo, %s)", assign, f.expr)

			if f.wide() {
				c.linef("hi = hi*%s + lo*%s + hi2", f.expr, f.hi)
			} else {
				c.linef("hi = hi*%s + hi2", f.expr)
			}

			c.line("lo = lo2")
		}

		inner = g.storePair(c, store, bound)
	})

	return inner
}

// emitConcat emits a BIT_CONCAT (`tn::…::t0 = sn::…::s0`): the sources pack
// into one value with sources[0] in the least-significant bits (executeConcat),
// which is then stored across the (possibly multi-limb) target.  Widths are the
// declared register widths, and each source is known to fit its width.
func (g *generator) emitConcat(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint], store storeView) error {
	srcs := make([]operand, len(x.Sources))
	widths := make([]uint, len(x.Sources))
	total := uint(0)

	for i, id := range x.Sources {
		o, err := g.operand(fn, id)
		if err != nil {
			return err
		}

		w, err := g.regWidth(fn, id)
		if err != nil {
			return err
		}

		if o.wide() {
			return fmt.Errorf("gogen: concatenation source wider than 64 bits unsupported")
		}

		srcs[i] = o
		widths[i] = w
		total += w
	}

	bound := widthMax(total)

	if allExact(srcs) {
		val := big.NewInt(0)
		for i := len(srcs) - 1; i >= 0; i-- {
			val.Lsh(val, widths[i])
			val.Or(val, srcs[i].val)
		}

		return g.storeValue(c, store, exactWide(val))
	}

	if total <= 64 {
		// Single expression: fold sources MSB-first, sources[0] lowest.
		expr := srcs[len(srcs)-1].expr
		for i := len(srcs) - 2; i >= 0; i-- {
			expr = fmt.Sprintf("(%s<<%d | %s)", expr, widths[i], srcs[i].expr)
		}

		return g.storeValue(c, store, operand{expr: expr, max: bound})
	}

	if total > 128 {
		return fmt.Errorf("gogen: concatenation of %d bits exceeds 128 bits (unsupported)", total)
	}

	var inner error

	c.block(func() {
		c.linef("lo, hi := %s, uint64(0)", srcs[len(srcs)-1].expr)

		for i := len(srcs) - 2; i >= 0; i-- {
			w := widths[i]
			if w == 64 {
				c.line("hi = lo")
				c.linef("lo = %s", srcs[i].expr)
			} else {
				c.linef("hi = hi<<%d | lo>>%d", w, 64-w)
				c.linef("lo = lo<<%d | %s", w, srcs[i].expr)
			}
		}

		inner = g.storePair(c, store, bound)
	})

	return inner
}

// ===========================================================================
// Stores (mirroring StackFrame.Store / StoreAcross)
// ===========================================================================

// limb identifies a target register and its bit width.  A width above 64 means
// the register is a two-limb pair (rN_0 / rN_1).
type limb struct {
	id    register.Id
	reg   string // Go base name, e.g. "r3"
	width uint
}

// lo / hiName are the Go lvalues for the register's limbs.
func (l limb) lo() string {
	if l.width > 64 {
		return l.reg + "_0"
	}

	return l.reg
}

func (l limb) hiName() string { return l.reg + "_1" }

func (g *generator) limbOf(fn *wordFunction, id register.Id) (limb, error) {
	w, err := g.regWidth(fn, id)
	if err != nil {
		return limb{}, err
	}

	return limb{id: id, reg: reg(id), width: w}, nil
}

// storeView models a store target: exactly one of single / limbs is set.  A
// multi-register target receives the value distributed lowest-register-first
// (least significant limb first), per StoreAcross.  Registers in a multi-limb
// vector must be narrow (a wide register inside a destructure has not been
// observed and stays unsupported).
type storeView struct {
	single *limb
	limbs  []limb
	total  uint // total bit width across all target registers
}

// buildStore translates a target register vector into a storeView.
func (g *generator) buildStore(fn *wordFunction, vec register.Vector) (storeView, error) {
	regs := vec.Registers()
	if len(regs) == 1 {
		l, err := g.limbOf(fn, regs[0])
		if err != nil {
			return storeView{}, err
		}

		return storeView{single: &l, total: l.width}, nil
	}

	limbs := make([]limb, len(regs))
	total := uint(0)

	for i, id := range regs {
		l, err := g.limbOf(fn, id)
		if err != nil {
			return storeView{}, err
		}

		if l.width > 64 {
			return storeView{}, fmt.Errorf("gogen: wide register %q inside a multi-register store unsupported", l.reg)
		}

		limbs[i] = l
		total += l.width
	}

	return storeView{limbs: limbs, total: total}, nil
}

// storeValue is the store entry point: it dispatches on the value's form
// (exact / narrow expression / lo-hi pair) and the target's shape.
func (g *generator) storeValue(c *code, store storeView, op operand) error {
	switch {
	case op.val != nil && !op.wide():
		g.storeKnown(c, store, op.val)
		return nil
	case !op.wide():
		if store.single != nil {
			g.assignSingle(c, *store.single, op)
			return nil
		}

		var inner error

		c.block(func() {
			c.linef("v := %s", op.expr)
			inner = g.storeNamed(c, store, operand{expr: "v", max: op.max})
		})

		return inner
	default:
		// A pair value: route through lo/hi locals so storePair's shifting
		// and checks apply uniformly (single-assignments avoid the block).
		if store.single != nil && store.single.width > 64 {
			// Tuple assignment: the pair expressions may read the target.
			c.linef("%s, %s = %s, %s", store.single.lo(), store.single.hiName(), op.expr, op.hiOr0())
			g.checkWidth(c, operand{expr: store.single.lo(), hi: store.single.hiName(), max: op.max}, store.single.width)
			g.iv.assign(store.single.id, op.max)

			return nil
		}

		var inner error

		c.block(func() {
			c.linef("lo, hi := %s, %s", op.expr, op.hiOr0())
			inner = g.storePair(c, store, op.max)
		})

		return inner
	}
}

// assignSingle stores a narrow (uint64) value into a single register,
// width-checked; a wide target register zeroes its high limb.
func (g *generator) assignSingle(c *code, l limb, op operand) {
	if l.width > 64 {
		c.linef("%s, %s = %s, 0", l.lo(), l.hiName(), op.expr)
		g.iv.assign(l.id, op.max)

		return
	}

	c.linef("%s = %s", l.lo(), op.expr)
	g.checkWidth(c, operand{expr: l.lo(), max: op.max}, l.width)
	g.iv.assign(l.id, op.max)
}

// storeNamed distributes an already-named narrow uint64 variable into the
// target.  The variable is consumed (shifted) by a multi-register store.
func (g *generator) storeNamed(c *code, store storeView, op operand) error {
	if store.single != nil {
		g.assignSingle(c, *store.single, op)
		return nil
	}

	v := op.expr
	rest := new(big.Int).Set(op.max) // bound of the not-yet-distributed bits

	for i, l := range store.limbs {
		if l.width < 64 {
			c.linef("%s = %s & (1<<%d - 1)", l.reg, v, l.width)
		} else {
			c.linef("%s = %s", l.reg, v)
		}

		g.iv.assign(l.id, rest)
		rest.Rsh(rest, l.width)
		// The shift after the last limb only matters for the leftover check.
		if i < len(store.limbs)-1 || !fits(op.max, store.total) {
			c.linef("%s >>= %d", v, l.width)
		}
	}

	if !fits(op.max, store.total) {
		c.linef("if %s != 0 {", v)
		c.linef("fail(%q)", widthFailMsg(store.total))
		c.line("}")
	}

	return nil
}

// storeKnown stores a generation-time known value: limbs become literals and
// the width check resolves statically.
func (g *generator) storeKnown(c *code, store storeView, val *big.Int) {
	if store.single != nil {
		l := *store.single
		if l.width > 64 {
			c.linef("%s, %s = %s, %s", l.lo(), l.hiName(), bigLit(slice(val, 64)), bigLit(new(big.Int).Rsh(val, 64)))

			if !fits(val, l.width) {
				c.linef("fail(%q) // constant value exceeds the target width", widthFailMsg(l.width))
			}

			g.iv.assign(l.id, val)

			return
		}

		c.linef("%s = %s", l.lo(), bigLit(val))
		g.checkWidth(c, operand{expr: bigLit(val), max: val, val: val}, l.width)
		g.iv.assign(l.id, val)

		return
	}

	rest := new(big.Int).Set(val)
	for _, l := range store.limbs {
		piece := slice(rest, l.width)
		c.linef("%s = %s", l.reg, bigLit(piece))
		g.iv.assign(l.id, piece)
		rest.Rsh(rest, l.width)
	}

	if rest.Sign() != 0 {
		c.linef("fail(%q) // constant value exceeds the target width", widthFailMsg(store.total))
	}
}

// exactWide wraps an exact value, splitting it into a pair when it exceeds 64
// bits (so storeValue can route it).
func exactWide(v *big.Int) operand {
	if fitsU64(v) {
		return exact(v)
	}

	return operand{
		expr: bigLit(slice(v, 64)),
		hi:   bigLit(new(big.Int).Rsh(v, 64)),
		max:  v,
		val:  v,
	}
}

// storePair distributes a lo/hi pair (named exactly `lo` and `hi`, in scope)
// across the target, mirroring StoreAcross over a 128-bit value.
func (g *generator) storePair(c *code, store storeView, bound *big.Int) error {
	checked := !fits(bound, store.total)

	if store.single != nil {
		l := *store.single
		if l.width > 64 {
			g.checkWidth(c, operand{expr: "lo", hi: "hi", max: bound}, l.width)
			c.linef("%s, %s = lo, hi", l.lo(), l.hiName())
			g.iv.assign(l.id, bound)

			return nil
		}

		w := l.width
		switch {
		case w == 64 && checked:
			c.line("if hi != 0 {")
			c.linef("fail(%q)", widthFailMsg(w))
			c.line("}")
		case w < 64:
			c.linef("if hi != 0 || lo >= 1<<%d {", w)
			c.linef("fail(%q)", widthFailMsg(w))
			c.line("}")
		}

		c.linef("%s = lo", l.lo())
		g.iv.assign(l.id, bound)

		return nil
	}

	rest := new(big.Int).Set(bound)

	for i, l := range store.limbs {
		last := i == len(store.limbs)-1

		if l.width < 64 {
			c.linef("%s = lo & (1<<%d - 1)", l.reg, l.width)
		} else {
			c.linef("%s = lo", l.reg)
		}

		g.iv.assign(l.id, rest)
		rest.Rsh(rest, l.width)

		if last && !checked {
			break
		}

		if l.width == 64 {
			c.line("lo, hi = hi, 0")
		} else {
			c.linef("lo = lo>>%d | hi<<%d", l.width, 64-l.width)
			c.linef("hi >>= %d", l.width)
		}
	}

	if checked {
		c.line("if lo|hi != 0 {")
		c.linef("fail(%q)", widthFailMsg(store.total))
		c.line("}")
	}

	return nil
}

// slice returns the low w bits of v (a fresh value).
func slice(v *big.Int, w uint) *big.Int {
	mask := new(big.Int).Lsh(big.NewInt(1), w)
	mask.Sub(mask, big.NewInt(1))

	return mask.And(mask, v)
}

// bigLit renders a non-negative big integer as a Go literal (hex past 9 for
// readability).
func bigLit(v *big.Int) string {
	if v.BitLen() <= 3 {
		return v.String()
	}

	return "0x" + v.Text(16)
}

// slices0 copies a slice so appends never alias the caller's backing array.
func slices0(ops []operand) []operand {
	return append([]operand{}, ops...)
}
