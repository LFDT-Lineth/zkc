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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
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
// NOTE: this transform must run after vectorisation (so the branch's guarded
// writes share the condition) and before register splitting (so comparison
// operands remain single registers).
func FactorSkipConditions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = factorSkipConditionsFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func factorSkipConditionsFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		// Decide up-front which SkipIf codes in this vector are worth factoring.
		// This needs the whole vector body (to size each branch), which the Map
		// closure cannot see one bytecode at a time.
		factor := factorableSkips(vec.Bytecodes, alloc)
		//
		nvecs[i] = vec.Map(func(idx uint, ith Bytecode[W]) []Bytecode[W] {
			if factor[idx] {
				return factorSkipIf(ith.(*bytecode.SkipIf[W]), alloc)
			}
			//
			return []Bytecode[W]{ith}
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), nvecs)
}

// factorableSkips returns the set of code indices holding a SkipIf worth factoring.
func factorableSkips[W word.Word[W]](codes []Bytecode[W], registers split.Allocator[W]) map[uint]bool {
	factor := make(map[uint]bool)
	//
	for i, code := range codes {
		si, ok := code.(*bytecode.SkipIf[W])
		if !ok {
			continue
		}
		// Nothing to factorize if the condition is a (in)equality on a bit register
		if isEqualityCondition(si.Op) && !generatesInverse(si, registers) {
			factor[uint(i)] = false
			continue
		}
		// Nothing to factorize if the body of the skip is a bit equality like b = x == 0 ? 1 :0.
		// Note that as we lowerSwitch later, this pattern can't arise from lowerSwitch, but only
		// directly from .zkc program.
		if bodyContainsOnlyBitEquality(codes, uint(i), registers) {
			factor[uint(i)] = false
			continue
		}

		// In all other cases, we can factor the skip condition into a single bit register.
		factor[uint(i)] = true

		//TODO: perf: https://github.com/LFDT-Lineth/zkc/issues/2096
		// reuse an already defined bit in the body to guard the new condition

		continue
	}
	//
	return factor
}

func isEqualityCondition(cond bytecode.Condition) bool {
	return cond == bytecode.CONDITION_EQ || cond == bytecode.CONDITION_NEQ
}

// generatesInverse reports whether the comparison performed by a SkipIf would
// lower to an inverse normalisation.  This is only the case when some operand is
// wider than a single bit; equality involving only bit registers is
// normalisation-free and so factoring it would add bytecodes for no benefit.
func generatesInverse[W word.Word[W]](si *bytecode.SkipIf[W], registers split.Allocator[W]) bool {
	for _, r := range si.Uses() {
		reg := registers.Register(r)
		// Native registers are full-field-width (and have no fixed bitwidth), so
		// they are certainly wider than a single bit.
		if reg.IsNative() || reg.Bitwidth().Unwrap() > 1 {
			return true
		}
	}

	return false
}

// bodyContainsOnlyBitEquality reports whether the SkipIf at index i heads a diamond
// which merely selects between two constants for a single 1-bit register:
//
//	skip_if (cond) 2
//	b = k0
//	skip 1
//	b = k1
func bodyContainsOnlyBitEquality[W word.Word[W]](codes []Bytecode[W], i uint, registers split.Allocator[W]) bool {
	si := codes[i].(*bytecode.SkipIf[W])
	//
	if si.Skip != 2 || i+3 >= uint(len(codes)) {
		return false
	}
	//
	var (
		lo, okLo  = isLoadConst(codes[i+1])
		mid, okSk = codes[i+2].(*bytecode.Skip[W])
		hi, okHi  = isLoadConst(codes[i+3])
	)
	//
	if !okLo || !okSk || !okHi || mid.Skip != 1 || lo != hi {
		return false
	}
	//
	bit := registers.Register(lo)
	//
	return !bit.IsNative() && bit.Bitwidth().Unwrap() == 1
}

// isLoadConst recognises a load-constant bytecode (as constructed by
// bytecode.LoadConst), returning its single target register.
func isLoadConst[W word.Word[W]](code Bytecode[W]) (bytecode.RegisterId, bool) {
	if a, ok := code.(*bytecode.Arith[W]); ok &&
		a.Op == bytecode.OP_ADD && len(a.Source) == 0 && len(a.Target) == 1 {
		return a.Target[0], true
	}
	//
	return 0, false
}

// factorSkipIf expands an equality SkipIf into the diamond described on
// FactorSkipConditions.  The condition of the inner skip is preserved
// (unnegated): when it holds, execution jumps to `b = 1`; otherwise it falls
// through to `b = 0`.
func factorSkipIf[W word.Word[W]](
	si *bytecode.SkipIf[W],
	registers split.Allocator[W],
) []Bytecode[W] {
	var (
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		b    = registers.Allocate("", util.Some[uint](1))
	)
	//
	return []Bytecode[W]{
		// skip_if (cond) 2  => condition holds, jump to "b = 1"
		bytecode.NewSkipIf(si.Op, 2, si.Left, si.Right),
		// b = 0  (condition does not hold)
		bytecode.LoadConst(b, zero),
		// skip 1  => jump over "b = 1"
		bytecode.NewSkip[W](1),
		// b = 1  (condition holds)
		bytecode.LoadConst(b, one),
		// skip_if b != 0 S  (original skip, now testing the bit)
		bytecode.NewSkipIf(bytecode.CONDITION_NEQ, si.Skip,
			bytecode.NewRegisterVector(b),
			bytecode.NewConstantOperand(zero)),
	}
}
