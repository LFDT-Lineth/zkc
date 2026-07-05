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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerComparisons rewrites SkipIf bytecodes with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
//
// NOTE: this transform must run after LowerBitwise.
func LowerComparisons[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerComparisonFunction[W](fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func lowerComparisonFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = newRegAllocator(fn.Registers())
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return lowerComparisonCode[W](b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.IsNative(), nvecs)
}

func lowerComparisonCode[W word.Word[W]](
	b Bytecode[W],
	registers *regAllocator[W],
) []Bytecode[W] {
	si, ok := b.(*bytecode.SkipIf)
	if !ok || !isRelationalCondition(si.Op) {
		return []Bytecode[W]{b}
	}

	return lowerRelationalSkipIf[W](si, registers)
}

func isRelationalCondition(cond opcode.Condition) bool {
	switch cond {
	case opcode.LT, opcode.GT, opcode.LTEQ, opcode.GTEQ:
		return true
	default:
		return false
	}
}

// lowerRelationalSkipIf lowers a SkipIf with a relational condition into an
// arithmetic sequence. castBandWidth = max(lhsWidth, rhsWidth)+1.
// When lhsWidth == castBandWidth-1 (LT/GTEQ after normalisation), lhs is used
// directly in BitConcat with no cast. Otherwise (GT/LTEQ after swap), lhs is
// first widened to castBandWidth-1 via aBase.
//
//	one    = 1
//	biased = BitConcat([lhs, one])          // 1::lhs, avoids underflow in diff
//	lo, sign = biased - rhs                  // sign=1 iff lhs >= rhs
//	zero   = 0
//	SkipIf(EQ/NEQ, sign, zero, skip)
func lowerRelationalSkipIf[W word.Word[W]](
	si *bytecode.SkipIf,
	registers *regAllocator[W],
) []Bytecode[W] {
	lhs, rhs, skipOnZero := normalizeRelational(si)
	lhsWidth := registers.Register(lhs).Bitwidth().Unwrap()
	rhsWidth := registers.Register(rhs).Bitwidth().Unwrap()

	castBandWidth := max(lhsWidth, rhsWidth) + 1

	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	oneReg := registers.Allocate("", util.Some[uint](1))
	biased := registers.Allocate("", util.Some(castBandWidth))
	lo := registers.Allocate("", util.Some(castBandWidth-1))
	sign := registers.Allocate("", util.Some[uint](1))
	zeroReg := registers.Allocate("", util.Some[uint](1))

	// rhs is always cast to castBandWidth
	castRhs := []Bytecode[W]{
		bytecode.LoadConst(oneReg, one),
	}
	// when creating 1::lhs, we don't need to cast lhs if it's of size castBandWidth-1 already.
	var castLhs = bytecode.Concat([]bytecode.RegisterId{biased}, []bytecode.RegisterId{lhs, oneReg})

	subtractAndDestruct := []Bytecode[W]{
		bytecode.SubVecConst([]bytecode.RegisterId{lo, sign}, []bytecode.RegisterId{biased, rhs}, zero),
		bytecode.LoadConst(zeroReg, zero),
	}

	insns := append(append(castRhs, castLhs), subtractAndDestruct...)

	// Finally emit the SkipIf with the appropriate condition on the sign bit
	finalCond := opcode.EQ
	if !skipOnZero {
		finalCond = opcode.NEQ
	}

	return append(insns, bytecode.NewSkipIf(finalCond, si.Skip, sign, zeroReg))
}

// normalizeRelational returns (lhs, rhs, skipOnZero) for a relational SkipIf.
// GT and LTEQ swap operands so the sign bit gives exact strict/inclusive semantics:
//
//	LT(a,b)   → lhs=a, rhs=b, skipOnZero=true  (skip if sign==0 i.e. a < b)
//	GTEQ(a,b) → lhs=a, rhs=b, skipOnZero=false (skip if sign==1 i.e. a >= b)
//	GT(a,b)   → lhs=b, rhs=a, skipOnZero=true  (sign==0 iff b < a iff a > b)
//	LTEQ(a,b) → lhs=b, rhs=a, skipOnZero=false (sign==1 iff b >= a iff a <= b)
func normalizeRelational(si *bytecode.SkipIf) (lhs, rhs bytecode.RegisterId, skipOnZero bool) {
	left := si.Left.Registers()
	right := si.Right.Registers()
	//
	if len(left) != 1 || len(right) != 1 {
		panic("cannot lower comparisons after register splitting")
	}
	//
	lhs = left[0]
	rhs = right[0]
	//
	switch si.Op {
	case opcode.LT:
		return lhs, rhs, true
	case opcode.GTEQ:
		return lhs, rhs, false
	case opcode.GT:
		return rhs, lhs, true
	case opcode.LTEQ:
		return rhs, lhs, false
	default:
		panic("normalizeRelational called with non-relational condition")
	}
}
