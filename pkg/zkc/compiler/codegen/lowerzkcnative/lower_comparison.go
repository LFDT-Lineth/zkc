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
package lowerzkcnative

import (
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// LowerComparisons rewrites SkipIf instructions with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
// This pass must run after LowerBitwise.
func LowerComparisons[W vm.Word[W]](modules []vm.Module) []vm.Module {
	out := append([]vm.Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*vm.WordFunction); ok {
			out[i] = lowerComparisonFunction[W](fn)
		}
	}

	return out
}

func lowerComparisonFunction[W vm.Word[W]](fn *vm.WordFunction) *vm.WordFunction {
	var (
		code      = fn.Code()
		ncode     = make([]vectorInstruction, len(code))
		registers = append([]register.Register{}, fn.Registers()...)
	)

	for i, insn := range code {
		ncodes := lowerComparisonCodes[W](insn.Codes, &registers)
		ncode[i] = vectorInstruction{Codes: ncodes}
	}

	return vm.NewFunction(fn.Name(), fn.IsNative(), registers, ncode)
}

func lowerComparisonCodes[W vm.Word[W]](
	codes []vm.WordInstruction,
	registers *[]register.Register,
) []vm.WordInstruction {
	ncodes := make([]vm.WordInstruction, 0, len(codes))

	for _, code := range codes {
		ncodes = append(ncodes, lowerComparisonCode[W](code, registers)...)
	}

	return ncodes
}

func lowerComparisonCode[W vm.Word[W]](
	code vm.WordInstruction,
	registers *[]register.Register,
) []vm.WordInstruction {
	si, ok := code.(*instruction.SkipIf)
	if !ok || !isRelationalCondition(si.Cond) {
		return []vm.WordInstruction{code}
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
func lowerRelationalSkipIf[W vm.Word[W]](
	si *instruction.SkipIf,
	registers *[]register.Register,
) []vm.WordInstruction {
	lhs, rhs, skipOnZero := normalizeRelational(si)

	lhsWidth := (*registers)[lhs.Unwrap()].Width()
	rhsWidth := (*registers)[rhs.Unwrap()].Width()

	castBandWidth := max(lhsWidth, rhsWidth) + 1

	zero := vm.Uint64[W](0)
	one := vm.Uint64[W](1)

	bWide := allocTmp(registers, castBandWidth)
	oneReg := allocTmp(registers, 1)
	biased := allocTmp(registers, castBandWidth)
	diff := allocTmp(registers, castBandWidth)
	lo := allocTmp(registers, castBandWidth-1)
	sign := allocTmp(registers, 1)
	zeroReg := allocTmp(registers, 1)

	// rhs is always cast to castBandWidth
	castRhs := []vm.WordInstruction{
		instruction.NewCast(bWide, rhs, castBandWidth),
		instruction.NewIntAdd(oneReg, nil, one),
	}

	// when creating 1::lhs, we don't need to cast lhs if it's of size castBandWidth-1 already.
	var castLhs []vm.WordInstruction
	if lhsWidth == castBandWidth-1 {
		castLhs = []vm.WordInstruction{
			instruction.NewBitConcat[W](biased, []register.Id{lhs, oneReg}),
		}
	} else {
		aBase := allocTmp(registers, castBandWidth-1)
		castLhs = []vm.WordInstruction{
			instruction.NewCast(aBase, lhs, castBandWidth-1),
			instruction.NewBitConcat[W](biased, []register.Id{aBase, oneReg}),
		}
	}

	subtractAnsDestruct := []vm.WordInstruction{
		instruction.NewIntSub(diff, []register.Id{biased, bWide}, zero),
		instruction.NewDestruct([]register.Id{lo, sign}, diff),
		instruction.NewIntAdd(zeroReg, nil, zero),
	}

	insns := append(append(castRhs, castLhs...), subtractAnsDestruct...)

	// Finally emit the SkipIf with the appropriate condition on the sign bit
	finalCond := opcode.EQ
	if !skipOnZero {
		finalCond = opcode.NEQ
	}

	return append(insns, instruction.NewSkipIf(finalCond, sign, zeroReg, si.Skip))
}

// normalizeRelational returns (lhs, rhs, skipOnZero) for a relational SkipIf.
// GT and LTEQ swap operands so the sign bit gives exact strict/inclusive semantics:
//
//	LT(a,b)   → lhs=a, rhs=b, skipOnZero=true  (skip if sign==0 i.e. a < b)
//	GTEQ(a,b) → lhs=a, rhs=b, skipOnZero=false (skip if sign==1 i.e. a >= b)
//	GT(a,b)   → lhs=b, rhs=a, skipOnZero=true  (sign==0 iff b < a iff a > b)
//	LTEQ(a,b) → lhs=b, rhs=a, skipOnZero=false (sign==1 iff b >= a iff a <= b)
func normalizeRelational(si *instruction.SkipIf) (lhs, rhs register.Id, skipOnZero bool) {
	if len(si.Left.Registers()) != 1 || len(si.Right.Registers()) != 1 {
		panic("cannot lower comparisons after register splitting")
	}
	//
	lhs = si.Left.Registers()[0]
	rhs = si.Right.Registers()[0]
	//
	switch si.Cond {
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
