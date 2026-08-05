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
package transform

import (
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// OptimizeDivisions is a fast mode optimization that rewrites integer divisions and remainders by a constant
// power-of-two divisor into a (logical) right shift and a bitwise AND respectively.  That is, bytecodes of the form
//
//	$4 = 0x2^k ; q = x / $4   =>   $4 = 0xk ; q = x >> $4
//	$5 = 0x2^k ; r = x % $5   =>   $5 = 0x2^k-1 ; r = x & $5
//
// Because each bytecode maps to exactly one bytecode, no register is left dead
// and the bytecode count is unchanged (so branch / skip offsets are unaffected).
//
// To stay sound, a divisor register is only repurposed when it holds a single
// statically-known power-of-two constant and is read exactly once (i.e. only by
// this division / remainder); otherwise — including the case where a divisor
// constant is bound to a variable and shared across instructions — the operation
// is left unchanged.
//
// NOTE: we could apply more optimization here like:
// - deal with generic constants
// - when doing remainder and division by the same constant, we can compute the quotient and remainder together
func OptimizeDivisions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = optimizeDivisionFunction[W](fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func optimizeDivisionFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	// Determine which divisor registers can be repurposed, mapping each to the new
	// constant value its load should hold (k for division, 2^k - 1 for remainder).
	reloads := planDivisionReloads[W](fn)
	//
	if len(reloads) == 0 {
		// Nothing to optimize.
		return fn
	}
	//
	var (
		regs    = fn.Registers()
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
	)
	//
	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			return rewriteDivisionInsn[W](insn, reloads, regs)
		})
	}
	// Registers are reused in place, so the register set is unchanged.
	return descriptor.NewFunction(fn.Name(), regs, fn.Kind(), fn.Effects(), nvecs)
}

// planDivisionReloads scans a function and returns, for each divisor register
// that can be repurposed, the new constant value its load should hold: the shift
// amount (k) for a division by 2^k, or the mask (2^k - 1) for a remainder by
// 2^k.  A register qualifies only when it holds a single statically-known
// power-of-two constant and is read exactly once.
func planDivisionReloads[W word.Word[W]](fn *descriptor.Function[W]) map[bytecode.RegisterId]W {
	var (
		defs     = make(map[bytecode.RegisterId]int)
		uses     = make(map[bytecode.RegisterId]int)
		constVal = make(map[bytecode.RegisterId]W)
		hasConst = make(map[bytecode.RegisterId]bool)
	)
	// First, gather definition / use counts and record constant loads.
	for _, vec := range fn.Vectors() {
		for _, insn := range vec.Bytecodes {
			for _, r := range insn.Uses() {
				uses[r]++
			}
			//
			for _, r := range insn.Definitions() {
				defs[r]++
			}
			//
			if r, c, ok := asConstantLoad[W](insn); ok {
				constVal[r] = c
				hasConst[r] = true
			}
		}
	}
	// Second, decide which divisor registers to repurpose.
	reloads := make(map[bytecode.RegisterId]W)
	//
	for _, vec := range fn.Vectors() {
		for _, insn := range vec.Bytecodes {
			dr, ok := insn.(*bytecode.DivRem[W])
			if !ok {
				continue
			}
			//
			r := dr.Divisor
			// The divisor must hold a single statically-known power-of-two constant
			// and be read exactly once, so repurposing its value affects nothing else.
			if defs[r] != 1 || uses[r] != 1 || !hasConst[r] {
				continue
			}
			//
			k, ok := powerOfTwoExponent[W](constVal[r])
			if !ok {
				continue
			}
			//
			if dr.Opcode == encoding.DIV {
				// x / 2^k == x >> k: the divisor register becomes the shift amount.
				reloads[r] = word.Const64[W](uint64(k))
			} else {
				// x % 2^k == x & (2^k - 1): the divisor register becomes the mask.
				reloads[r], _ = constVal[r].Sub(word.Const64[W](1))
			}
		}
	}
	//
	return reloads
}

// rewriteDivisionInsn rewrites a single bytecode according to the reload plan: a
// repurposed divisor's constant load is updated to hold the shift amount / mask,
// and the division / remainder over that register becomes a right shift / bitwise
// AND.  Each bytecode maps to exactly one bytecode.  The operation width (carried
// explicitly by the bitwise bytecode) is recovered from the dividend register,
// mirroring how a DivRem's width is recovered when decompiled.
func rewriteDivisionInsn[W word.Word[W]](insn Bytecode[W], reloads map[bytecode.RegisterId]W,
	regs []descriptor.Register[W]) []Bytecode[W] {
	//
	switch insn := insn.(type) {
	case *bytecode.Arith[W]:
		// Repurpose a divisor's constant load to hold the shift amount / mask.
		if r, _, ok := asConstantLoad[W](insn); ok {
			if v, ok := reloads[r]; ok {
				return []Bytecode[W]{bytecode.LoadConst(r, v)}
			}
		}
	case *bytecode.DivRem[W]:
		if _, ok := reloads[insn.Divisor]; ok {
			width := uint16(regs[insn.Dividend].Bitwidth().UnwrapOr(0))
			//
			switch insn.Opcode {
			case encoding.DIV:
				return []Bytecode[W]{
					bytecode.NewBitwise[W](bytecode.OP_SHR, insn.Target, insn.Dividend, insn.Divisor, width),
				}
			case encoding.REM:
				return []Bytecode[W]{
					bytecode.NewBitwise[W](bytecode.OP_AND, insn.Target, insn.Dividend, insn.Divisor, width),
				}
			}
		}
	}
	//
	return []Bytecode[W]{insn}
}

// asConstantLoad reports whether the given bytecode is a pure constant load (an
// integer addition with no source registers and a single target, as emitted for
// a constant literal), returning the target register and the loaded value.
func asConstantLoad[W word.Word[W]](insn Bytecode[W]) (bytecode.RegisterId, W, bool) {
	if a, ok := insn.(*bytecode.Arith[W]); ok &&
		a.Op == bytecode.OP_ADD && len(a.Source) == 0 && len(a.Target) == 1 {
		//
		return a.Target[0], a.Constant, true
	}
	//
	var zero W
	//
	return 0, zero, false
}

// powerOfTwoExponent returns k such that w == 2^k, together with a flag
// indicating whether w is in fact a (strictly positive) power of two.
func powerOfTwoExponent[W word.Word[W]](w W) (uint, bool) {
	v := w.BigInt()
	// Must be strictly positive.
	if v.Sign() <= 0 {
		return 0, false
	}
	// v is a power of two iff exactly one bit is set, i.e. v & (v-1) == 0.
	var tmp big.Int
	tmp.Sub(v, big.NewInt(1))
	tmp.And(v, &tmp)
	//
	if tmp.Sign() != 0 {
		return 0, false
	}
	// v == 2^k where k is the (zero-based) index of the single set bit.
	return uint(v.BitLen() - 1), true
}
