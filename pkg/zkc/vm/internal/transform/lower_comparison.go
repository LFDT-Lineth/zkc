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

// LowerComparisons rewrites SkipIf instructions with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
// This pass must run after LowerBitwise.
func LowerComparisons[W word.Word[W]](modules []Module) []Module {
	out := append([]Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*WordFunction); ok {
			out[i] = lowerComparisonFunction[W](fn)
		}
	}

	return out
}

func lowerComparisonFunction[W word.Word[W]](fn *WordFunction) *WordFunction {
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith WordInstruction) []WordInstruction {
			return lowerComparisonCode[W](ith, alloc)
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

func lowerComparisonCode[W word.Word[W]](
	code WordInstruction,
	registers RegisterAllocator,
) []WordInstruction {
	si, ok := code.(*instruction.SkipIf)
	if !ok || !isRelationalCondition(si.Cond) {
		return []WordInstruction{code}
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
//	[aBase = cast(lhs, castBandWidth-1)]   // only when lhsWidth < castBandWidth-1
//	b_wide = cast(rhs, castBandWidth)
//	one    = 1
//	biased = BitConcat([lhs_or_aBase, one])  // 1::lhs, avoids underflow in diff
//	diff   = biased - b_wide
//	lo, sign = Destruct(diff)               // sign=1 iff lhs >= rhs
//	zero   = 0
//	SkipIf(EQ/NEQ, sign, zero, skip)
func lowerRelationalSkipIf[W word.Word[W]](
	si *instruction.SkipIf,
	registers RegisterAllocator,
) []WordInstruction {
	lhs, rhs, cond := normalizeRelational(si)
	lhsWidth := registers.Register(lhs).Width()
	rhsWidth := registers.Register(rhs).Width()

	zero := word.Const64[W](0)
	delta := registers.Allocate("", max(lhsWidth, rhsWidth))
	sign := registers.Allocate("", 1)
	zeroReg := registers.Allocate("", 1)

	insns := []WordInstruction{
		instruction.UintSubV(register.NewVector(delta, sign), []register.Id{lhs, rhs}, zero),
		instruction.UintConst(zeroReg, zero),
	}

	return append(insns, instruction.NewSkipIf(cond, sign, zeroReg, si.Skip))
}

// normalizeRelational returns (lhs, rhs, skipOnZero) for a relational SkipIf.
// GT and LTEQ swap operands so the sign bit gives exact strict/inclusive semantics:
//
//	LT(a,b)   → lhs=a, rhs=b, c==NEQ  (skip if borrow!=0 i.e. a < b)
//	GTEQ(a,b) → lhs=a, rhs=b, c==EQ   (skip if borrow==0 i.e. a >= b)
//	GT(a,b)   → lhs=b, rhs=a, c==NEQ  (skip if borrow!=0 i.e. b < a)
//	LTEQ(a,b) → lhs=b, rhs=a, c==EQ   (skip if borrow==0 i.e. b >= a )
func normalizeRelational(si *instruction.SkipIf) (lhs, rhs register.Id, c opcode.Condition) {
	if len(si.Left.Registers()) != 1 || len(si.Right.Registers()) != 1 {
		panic("cannot lower comparisons after register splitting")
	}
	//
	lhs = si.Left.Registers()[0]
	rhs = si.Right.Registers()[0]
	//
	switch si.Cond {
	case opcode.LT:
		return lhs, rhs, opcode.NEQ
	case opcode.GTEQ:
		return lhs, rhs, opcode.EQ
	case opcode.GT:
		return rhs, lhs, opcode.NEQ
	case opcode.LTEQ:
		return rhs, lhs, opcode.EQ
	default:
		panic("normalizeRelational called with non-relational condition")
	}
}
