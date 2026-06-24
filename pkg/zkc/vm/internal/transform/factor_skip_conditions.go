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
// branch condition is materialised once into a fresh 1-bit register.
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
		// Decide up-front which SkipIf codes in this vector are worth factoring.
		// This needs the whole vector body (to size each branch), which the Map
		// closure cannot see one instruction at a time.
		factor := factorableSkips(insn.Codes, alloc)
		//
		ncode[i] = insn.Map(func(idx uint, ith WordInstruction) []WordInstruction {
			if factor[idx] {
				return factorSkipIf[W](ith.(*instruction.SkipIf), alloc)
			}
			//
			return []WordInstruction{ith}
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

// factorableSkips returns the set of code indices holding a SkipIf worth factoring.
func factorableSkips(codes []WordInstruction, registers RegisterAllocator) map[uint]bool {
	factor := make(map[uint]bool)
	//
	for i, code := range codes {
		si, ok := code.(*instruction.SkipIf)
		if !ok {
			continue
		}
		// Nothing to factorize if the condition is a (in)equality on a bit register
		if isEqualityCondition(si.Cond) && !generatesInverse(si, registers) {
			factor[uint(i)] = false
			continue
		}

		thenHasCall, elseHasCall := branchContainsCall(codes, uint(i), si.Skip)
		// Factorize if it guards a call, as it is needed for the source selector of the lookup.
		if thenHasCall || elseHasCall {
			factor[uint(i)] = true
			continue
		}

		thenSize, elseSize := branchSizes(codes, uint(i), si.Skip)
		// Performance improvment: compute the condition only once and then check against a boolean.
		// It reduces the constraint degree.
		if generatesInverse(si, registers) && (elseSize > 0 || thenSize > 1) {
			factor[uint(i)] = true
		}
	}
	//
	return factor
}

// branchContainsCall reports whether the "then" (conditionally-skipped) block
// of a SkipIf at index i, and/or the "else" block reached after it, contain a
// function call.  The block boundaries are computed exactly as in branchSizes.
func branchContainsCall(codes []WordInstruction, i, skip uint) (thenHasCall, elseHasCall bool) {
	var (
		end   = min(i+1+skip, uint(len(codes)))
		block = codes[i+1 : end]
	)
	// Determine the "else" block (i.e. the code reached when the condition
	// holds), which depends on how the "then" block ends.
	if m := len(block); m > 0 {
		switch last := block[m-1].(type) {
		case *instruction.Skip:
			// The then block jumps over a contiguous else block.
			elseEnd := min(end+last.Skip, uint(len(codes)))
			elseHasCall = containsCall(codes[end:elseEnd])
			block = block[:m-1]
		case *instruction.Fail, *instruction.Return:
			// The then block terminates, so the code that follows is only reached
			// when the condition holds (the skip is taken).
			elseHasCall = containsCall(codes[end:])
		}
	}
	//
	thenHasCall = containsCall(block)
	//
	return thenHasCall, elseHasCall
}

// containsCall reports whether the given block contains a (conditional) function call.
func containsCall(block []WordInstruction) bool {
	for _, code := range block {
		if _, ok := code.(*instruction.Call); ok {
			return true
		}
	}
	//
	return false
}

// branchSizes estimates how many instructions a SkipIf located at index i (with
// the given skip distance) guards.  The first result is the size of the
// conditionally-skipped block; the second is the size of the block reached
// after it (the "other" branch), read from the trailing unconditional Skip that
// the skipped block uses to jump over it.  An if without an else has no such
// trailing skip, so its else size is reported as zero.
func branchSizes(codes []WordInstruction, i, skip uint) (thenSize, elseSize uint) {
	var (
		end   = min(i+1+skip, uint(len(codes)))
		block = codes[i+1 : end]
	)
	//
	thenSize = uint(len(block))
	// If the skipped block ends with an unconditional Skip, that skip jumps over
	// the other branch and its distance is the other branch's size.
	if m := len(block); m > 0 {
		if s, ok := block[m-1].(*instruction.Skip); ok {
			thenSize = uint(m - 1)
			elseSize = s.Skip
		}
	}
	//
	return thenSize, elseSize
}

func isEqualityCondition(cond opcode.Condition) bool {
	return cond == opcode.EQ || cond == opcode.NEQ
}

// generatesInverse reports whether the comparison performed by a SkipIf would
// lower to an inverse normalisation.  This is only the case when some operand
// is wider than a single bit.
func generatesInverse(si *instruction.SkipIf, registers RegisterAllocator) bool {
	for _, r := range si.Uses() {
		reg := registers.Register(r)
		// Native registers are wider than a single bit.
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
		// skip 1  => jump over "b = 1"
		&instruction.Skip{Skip: 1},
		// b = 1  (condition holds)
		instruction.UintConst(b, one),
		// skip_if b != 0 S  (original skip, now testing the bit)
		instruction.NewSkipIf(opcode.NEQ, b, zr, si.Skip),
	}
}
