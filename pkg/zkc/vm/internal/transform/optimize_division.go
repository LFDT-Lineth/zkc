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
	"math/bits"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// OptimizeDivisions is a fast mode optimization that rewrites integer divisions and remainders by a constant
// power-of-two divisor into a (logical) right shift and a bitwise AND respectively.  That is, instructions of the form
//
//	q = x / 2^k => q = x >> k
//	r = x % 2^k => r = x & (2^k - 1)
//
// Divisions/remainders whose divisor is not a statically-known power of two are
// left unchanged.
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
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
		// constants tracks, for each register known to hold a statically-known
		// constant, the value it holds.  Keyed by raw register index.
		constants = make(map[uint]W)
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith WordInstruction) []WordInstruction {
			return optimizeDivisionCode[W](ith, alloc, constants)
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

// optimizeDivisionCode rewrites a single instruction, replacing divisions and
// remainders by a constant power-of-two divisor with an equivalent right shift
// or bitwise AND respectively.  It also maintains the running map of registers
// known to hold constant values so that divisor registers can be recognised.
func optimizeDivisionCode[W word.Word[W]](
	code WordInstruction,
	registers RegisterAllocator,
	constants map[uint]W,
) []WordInstruction {
	result := []WordInstruction{code}
	//
	switch code.OpCode() {
	case opcode.INT_DIV:
		insn := code.(*instruction.WordTypeB)
		// Rewrite x / 2^k into x >> k.
		if k, ok := constantPowerOfTwo[W](insn.RightSource, constants); ok {
			// amt holds the shift amount (k); allocate a register wide enough to
			// hold it and load the constant into it.
			amt := registers.Allocate("", shiftAmountWidth(k))
			result = []WordInstruction{
				instruction.UintConst(amt, word.Const64[W](uint64(k))),
				instruction.BitShr(insn.Bitwidth, insn.Target, insn.LeftSource, amt),
			}
		}
	case opcode.INT_REM:
		insn := code.(*instruction.WordTypeB)
		// Rewrite x % 2^k into x & (2^k - 1).  The mask 2^k - 1 is simply the
		// divisor constant (2^k) minus one, and fits in exactly k bits.
		if k, ok := constantPowerOfTwo[W](insn.RightSource, constants); ok {
			mask := registers.Allocate("", k)
			maskVal, _ := constants[insn.RightSource.Unwrap()].Sub(word.Const64[W](1))
			result = []WordInstruction{
				instruction.UintConst(mask, maskVal),
				instruction.BitAnd(insn.Bitwidth, insn.Target, insn.LeftSource, mask),
			}
		}
	}
	// Update constant tracking based on this instruction's definitions.  This
	// must happen after the rewrite above so a division reads the divisor's
	// value rather than its own (clobbered) target.
	updateConstants[W](code, constants)
	//
	return result
}

// constantPowerOfTwo returns k such that the given register is known to hold the
// constant 2^k, together with a flag indicating whether this is the case.
func constantPowerOfTwo[W word.Word[W]](rid register.Id, constants map[uint]W) (uint, bool) {
	if c, ok := constants[rid.Unwrap()]; ok {
		return powerOfTwoExponent[W](c)
	}
	//
	return 0, false
}

// updateConstants records (or invalidates) the constant value held by each
// register defined by the given instruction.  A pure constant load (an integer
// addition with no source registers, e.g. as emitted for a constant literal)
// records its value; any other definition invalidates the target.
func updateConstants[W word.Word[W]](code WordInstruction, constants map[uint]W) {
	if a, ok := code.(*instruction.WordTypeA[W]); ok &&
		a.Op == opcode.INT_ADD && len(a.Sources) == 0 && a.Target.Len() == 1 {
		//
		constants[a.Target.AsRegister().Unwrap()] = a.Constant
		//
		return
	}
	// Any other definition means the register no longer holds a known constant.
	for _, def := range code.Definitions() {
		delete(constants, def.Unwrap())
	}
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

// shiftAmountWidth returns a register width sufficient to hold the shift amount
// k (always at least one bit so that a shift by zero is representable).
func shiftAmountWidth(k uint) uint {
	if w := uint(bits.Len(k)); w != 0 {
		return w
	}
	//
	return 1
}
