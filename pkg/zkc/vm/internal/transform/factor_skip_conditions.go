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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FactorSkipConditions rewrites each equality SkipIf (EQ/NEQ) whose comparison
// would otherwise be replicated across the guarded writes of its branch.  The
// branch condition is materialised once into a fresh 1-bit register, and the
// skip is rewritten to test that bit.  Since equality between a bit register
// and zero is normalisation-free (see the AIR `normalise` gadget), each of the
// branch's guarded writes then references the bit directly and the expensive
// equality normalisation is emitted exactly once (where the bit is defined)
// rather than once per guarded write.
//
// Concretely, a skip of the form:
//
//	skip_if L == R S   (ifBranch) (elseBranch)
//
// is rewritten into the following diamond (`b` and `zero` are fresh):
//
//	zero = 0
//	skip_if L == R 2     // condition holds => jump to b = 1
//	b = 0                // condition does not hold
//	skip 1
//	b = 1                // condition holds
//	skip_if b != 0 S     // original skip, now testing the bit
//	(ifBranch) (elseBranch)
//
// The two writes to `b` lie on disjoint paths (so they do not conflict), and
// the skip offsets are recomputed automatically by Vector.Map.
//
// This pass must run after vectorisation (so the branch's guarded writes share
// the condition) and before register splitting (so comparison operands remain
// single registers).
func FactorSkipConditions[W word.Word[W]](modules []Module) []Module {
	out := append([]Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*WordFunction); ok {
			out[i] = factorSkipConditionsFunction[W](fn)
		}
	}

	return out
}

func factorSkipConditionsFunction[W word.Word[W]](fn *WordFunction) *WordFunction {
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith WordInstruction) []WordInstruction {
			return factorSkipConditionCode[W](ith, alloc)
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

func factorSkipConditionCode[W word.Word[W]](
	code WordInstruction,
	registers RegisterAllocator,
) []WordInstruction {
	si, ok := code.(*instruction.SkipIf)
	if !ok || !isEqualityCondition(si.Cond) || !generatesInverse(si, registers) {
		return []WordInstruction{code}
	}

	return factorSkipIf[W](si, registers)
}

func isEqualityCondition(cond opcode.Condition) bool {
	return cond == opcode.EQ || cond == opcode.NEQ
}

// generatesInverse reports whether the comparison performed by a SkipIf would
// lower to an inverse normalisation.  This is only the case when some operand
// is wider than a single bit; equality involving only bit registers is
// normalisation-free and so factoring it would add instructions for no benefit.
func generatesInverse(si *instruction.SkipIf, registers RegisterAllocator) bool {
	for _, r := range si.Uses() {
		reg := registers.Register(r)
		// Native registers are full-field-width (and have no fixed bitwidth), so
		// they are certainly wider than a single bit.
		if reg.IsNative() || reg.Width() > 1 {
			return true
		}
	}

	return false
}

// factorSkipIf expands an equality SkipIf into the diamond described on
// FactorSkipConditions.  The condition of the inner skip is preserved
// (unnegated): when it holds, execution jumps to `b = 1`; otherwise it falls
// through to `b = 0`.
func factorSkipIf[W word.Word[W]](
	si *instruction.SkipIf,
	registers RegisterAllocator,
) []WordInstruction {
	var (
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		b    = registers.Allocate("", 1)
		zr   = registers.Allocate("", 1)
	)
	//
	return []WordInstruction{
		// zero = 0
		instruction.UintConst(zr, zero),
		// skip_if (cond) 2  => condition holds, jump to "b = 1"
		instruction.NewSkipIfVec(si.Cond, si.Left, si.Right, 2),
		// b = 0  (condition does not hold)
		instruction.UintConst(b, zero),
		// skip 1  => jump over "b = 0"
		&instruction.Skip{Skip: 1},
		// b = 1  (condition holds)
		instruction.UintConst(b, one),
		// skip_if b != 0 S  (original skip, now testing the bit)
		instruction.NewSkipIf(opcode.NEQ, b, zr, si.Skip),
	}
}
