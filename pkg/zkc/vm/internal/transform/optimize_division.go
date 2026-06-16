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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// OptimizeDivisions is a fast mode optimization that rewrites integer divisions and remainders by a constant
// power-of-two divisor into a (logical) right shift and a bitwise AND respectively.  That is, instructions of the form
//
//	$4 = 0x2^k ; q = x / $4   =>   $4 = 0xk ; q = x >> $4
//	$5 = 0x2^k ; r = x % $5   =>   $5 = 0x2^k-1 ; r = x & $5
//
// Because each instruction maps to exactly one instruction, no register is left
// dead and the instruction count is unchanged (so branch / skip offsets are
// unaffected).
//
// To stay sound, a divisor register is only repurposed when it holds a single
// statically-known power-of-two constant and is read exactly once (i.e. only by
// this division / remainder); otherwise — including the case where a divisor
// constant is bound to a variable and shared across instructions — the operation
// is left unchanged.
//
// Note: we could apply more optimization here like:
// - deal with generic constants
// - when doing remainder and division by the same constant, we can compute the quotient and remainder together
func OptimizeDivisions[W word.Word[W]](modules []Module) []Module {
	out := append([]Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*WordFunction); ok {
			out[i] = optimizeDivisionFunction[W](fn)
		}
	}

	return out
}

func optimizeDivisionFunction[W word.Word[W]](fn *WordFunction) *WordFunction {
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
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
	)
	//
	for i, vec := range code {
		ncode[i] = vec.Map(func(_ uint, insn WordInstruction) []WordInstruction {
			return rewriteDivisionInsn[W](insn, reloads)
		})
	}
	// Registers are reused in place, so the register set is unchanged.
	return function.New(fn.Name(), fn.IsNative(), fn.Registers(), ncode)
}

// planDivisionReloads scans a function and returns, for each divisor register
// that can be repurposed, the new constant value its load should hold: the shift
// amount (k) for a division by 2^k, or the mask (2^k - 1) for a remainder by
// 2^k.  A register qualifies only when it holds a single statically-known
// power-of-two constant and is read exactly once.
func planDivisionReloads[W word.Word[W]](fn *WordFunction) map[uint]W {
	var (
		defs     = make(map[uint]int)
		uses     = make(map[uint]int)
		constVal = make(map[uint]W)
		hasConst = make(map[uint]bool)
	)
	// First, gather definition / use counts and record constant loads.
	for _, vec := range fn.Code() {
		for _, insn := range vec.Codes {
			for _, r := range insn.Uses() {
				uses[r.Unwrap()]++
			}
			//
			for _, r := range insn.Definitions() {
				defs[r.Unwrap()]++
			}
			//
			if r, c, ok := asConstantLoad[W](insn); ok {
				constVal[r.Unwrap()] = c
				hasConst[r.Unwrap()] = true
			}
		}
	}
	// Second, decide which divisor registers to repurpose.
	reloads := make(map[uint]W)
	//
	for _, vec := range fn.Code() {
		for _, insn := range vec.Codes {
			op := insn.OpCode()
			//
			if op != opcode.INT_DIV && op != opcode.INT_REM {
				continue
			}
			//
			r := insn.(*instruction.WordTypeB).RightSource.Unwrap()
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
			if op == opcode.INT_DIV {
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

// rewriteDivisionInsn rewrites a single instruction according to the reload
// plan: a repurposed divisor's constant load is updated to hold the shift amount
// / mask, and the division / remainder over that register becomes a right shift
// / bitwise AND.  Each instruction maps to exactly one instruction.
func rewriteDivisionInsn[W word.Word[W]](insn WordInstruction, reloads map[uint]W) []WordInstruction {
	switch insn.OpCode() {
	case opcode.INT_ADD:
		// Repurpose a divisor's constant load to hold the shift amount / mask.
		if r, _, ok := asConstantLoad[W](insn); ok {
			if v, ok := reloads[r.Unwrap()]; ok {
				return []WordInstruction{instruction.UintConst(r, v)}
			}
		}
	case opcode.INT_DIV:
		insn := insn.(*instruction.WordTypeB)
		if _, ok := reloads[insn.RightSource.Unwrap()]; ok {
			return []WordInstruction{
				instruction.BitShr(insn.Bitwidth, insn.Target, insn.LeftSource, insn.RightSource),
			}
		}
	case opcode.INT_REM:
		insn := insn.(*instruction.WordTypeB)
		if _, ok := reloads[insn.RightSource.Unwrap()]; ok {
			return []WordInstruction{
				instruction.BitAnd(insn.Bitwidth, insn.Target, insn.LeftSource, insn.RightSource),
			}
		}
	}
	//
	return []WordInstruction{insn}
}

// asConstantLoad reports whether the given instruction is a pure constant load
// (an integer addition with no source registers and a single target, as emitted
// for a constant literal), returning the target register and the loaded value.
func asConstantLoad[W word.Word[W]](insn WordInstruction) (register.Id, W, bool) {
	if a, ok := insn.(*instruction.WordTypeA[W]); ok &&
		a.Op == opcode.INT_ADD && len(a.Sources) == 0 && a.Target.Len() == 1 {
		//
		return a.Target.AsRegister(), a.Constant, true
	}
	//
	var zero W
	//
	return register.UnusedId(), zero, false
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
