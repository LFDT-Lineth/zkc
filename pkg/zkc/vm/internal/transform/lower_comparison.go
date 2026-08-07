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
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return lowerComparisonCode[W](b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

func lowerComparisonCode[W word.Word[W]](
	b Bytecode[W],
	registers split.Allocator[W],
) []Bytecode[W] {
	si, ok := b.(*bytecode.SkipIf[W])
	if !ok || !isRelationalCondition(si.Op) {
		return []Bytecode[W]{b}
	}

	if si.Right.IsConstant() {
		return lowerRelationalSkipIfConst[W](si, registers)
	}

	return lowerRelationalSkipIf[W](si, registers)
}

func isRelationalCondition(cond bytecode.Condition) bool {
	switch cond {
	case bytecode.CONDITION_LT, bytecode.CONDITION_GT, bytecode.CONDITION_LTEQ, bytecode.CONDITION_GTEQ:
		return true
	default:
		return false
	}
}

// lowerRelationalSkipIf lowers a SkipIf with a relational condition into an
// arithmetic sequence:
//
//	sign::lo = lhs - rhs
//	zero     = 0
//	SkipIf(EQ/NEQ, sign, zero, skip)
//
// The subtraction wraps (two's complement) at the width computed by
// CalculateSubBitwidth, i.e. 1+max(lhsWidth, rhsWidth), so the sign bit is 1
// exactly when lhs < rhs.  GT/LTEQ conditions swap operands first (see
// normalizeRelational), leaving the condition decided by the sign bit alone.
func lowerRelationalSkipIf[W word.Word[W]](
	si *bytecode.SkipIf[W],
	registers split.Allocator[W],
) []Bytecode[W] {
	lhs, rhs, skipOnZero := normalizeRelational(si)
	lhsWidth := registers.Register(lhs).Bitwidth().Unwrap()
	rhsWidth := registers.Register(rhs).Bitwidth().Unwrap()
	// Determine number of bits required to hold the result of the subtraction, plus the sign bit.
	castBandWidth := max(lhsWidth, rhsWidth) + 1

	zero := word.Const64[W](0)
	// create temporary (throw away) register
	lo := registers.Allocate("", util.Some(castBandWidth-1))
	// create sign bit for comparison
	sign := registers.Allocate("", util.Some[uint](1))
	//
	insns := []Bytecode[W]{
		// sign::lo = lhs - rhs
		bytecode.SubVecConst([]bytecode.RegisterId{lo, sign}, []bytecode.RegisterId{lhs, rhs}, zero),
	}
	// Finally emit the SkipIf with the appropriate condition on the sign bit
	finalCond := bytecode.CONDITION_EQ
	if !skipOnZero {
		finalCond = bytecode.CONDITION_NEQ
	}
	//
	return append(insns, bytecode.NewSkipIf[W](finalCond, si.Skip,
		bytecode.NewRegisterVector(sign),
		bytecode.NewConstantOperand[W](zero)))
}

// lowerRelationalSkipIfConst lowers a SkipIf comparing a register against a
// constant into an arithmetic sequence, mirroring lowerRelationalSkipIf:
//
//	sign::lo = lhs - c
//	zero     = 0
//	SkipIf(EQ/NEQ, sign, zero, skip)
//
// where the sign bit is 1 exactly when lhs < c.  The condition and constant
// are first normalised (see normalizeRelationalConst) such that the sign bit
// decides the condition directly.
func lowerRelationalSkipIfConst[W word.Word[W]](
	si *bytecode.SkipIf[W],
	registers split.Allocator[W],
) []Bytecode[W] {
	var (
		left   = si.Left.Registers()
		consts = si.Right.AsConstants()
	)
	//
	if len(left) != 1 || len(consts) > 1 {
		panic("cannot lower comparisons after register splitting")
	}
	//
	constant, skipOnZero := normalizeRelationalConst(si)
	//
	var (
		lhs      = left[0]
		lhsWidth = registers.Register(lhs).Bitwidth().Unwrap()
		// Mirror descriptor.CalculateSubBitwidth: one is subtracted from the
		// subtrahend because negative values do not need to encode zero, so a
		// power-of-two constant does not cost an extra bit.
		rhsWidth uint
	)
	//
	if constant.Cmp64(0) > 0 {
		cm1, _ := constant.Sub64(1)
		rhsWidth = cm1.BitLen()
	} else {
		// A zero subtrahend cannot underflow; CalculateSubBitwidth still
		// yields one bit for it (BitLen of -1).
		rhsWidth = 1
	}
	// Determine number of bits required to hold the result of the
	// subtraction, plus the sign bit.
	castBandWidth := max(lhsWidth, rhsWidth) + 1

	zero := word.Const64[W](0)
	// create temporary (throw away) register
	lo := registers.Allocate("", util.Some(castBandWidth-1))
	// create sign bit for comparison
	sign := registers.Allocate("", util.Some[uint](1))
	//
	insns := []Bytecode[W]{
		// sign::lo = lhs - constant
		bytecode.SubVecConst([]bytecode.RegisterId{lo, sign}, []bytecode.RegisterId{lhs}, constant),
	}
	// Finally emit the SkipIf with the appropriate condition on the sign bit
	finalCond := bytecode.CONDITION_EQ
	if !skipOnZero {
		finalCond = bytecode.CONDITION_NEQ
	}
	//
	return append(insns, bytecode.NewSkipIf[W](finalCond, si.Skip,
		bytecode.NewRegisterVector(sign),
		bytecode.NewConstantOperand[W](zero)))
}

// normalizeRelationalConst returns (constant, skipOnZero) for a relational
// SkipIf against a constant, such that the sign bit of "lhs - constant"
// (which is 1 exactly when lhs < constant) decides the condition directly.
// Since operands cannot be swapped here, GT/LTEQ are instead normalised by
// adjusting the constant:
//
//	LT(x,c)   → constant=c,   skipOnZero=false (skip if sign==1 i.e. x < c)
//	GTEQ(x,c) → constant=c,   skipOnZero=true  (skip if sign==0 i.e. x >= c)
//	GT(x,c)   → constant=c+1, skipOnZero=true  (x > c iff x >= c+1)
//	LTEQ(x,c) → constant=c+1, skipOnZero=false (x <= c iff x < c+1)
//
// When c+1 overflows the word, the condition is statically decided: the
// returned constant is zero (whose subtraction has an always-zero sign bit),
// with skipOnZero chosen so the skip never (x > max) or always (x <= max)
// triggers.
func normalizeRelationalConst[W word.Word[W]](si *bytecode.SkipIf[W]) (W, bool) {
	var constant W
	// NOTE: an empty constant vector denotes zero.
	if consts := si.Right.AsConstants(); len(consts) == 1 {
		constant = consts[0]
	}
	//
	switch si.Op {
	case bytecode.CONDITION_LT:
		return constant, false
	case bytecode.CONDITION_GTEQ:
		return constant, true
	case bytecode.CONDITION_GT:
		if c1, overflow := constant.Add64(1); !overflow {
			return c1, true
		}
		// x > max is trivially false: skipping on sign != 0 never triggers.
		return constant.SetUint64(0), false
	case bytecode.CONDITION_LTEQ:
		if c1, overflow := constant.Add64(1); !overflow {
			return c1, false
		}
		// x <= max is trivially true: skipping on sign == 0 always triggers.
		return constant.SetUint64(0), true
	default:
		panic("normalizeRelationalConst called with non-relational condition")
	}
}

// normalizeRelational returns (lhs, rhs, skipOnZero) for a relational SkipIf.
// GT and LTEQ swap operands so the sign bit (which is 1 exactly when
// lhs < rhs) gives exact strict/inclusive semantics:
//
//	LT(a,b)   → lhs=a, rhs=b, skipOnZero=false (skip if sign==1 i.e. a < b)
//	GTEQ(a,b) → lhs=a, rhs=b, skipOnZero=true  (skip if sign==0 i.e. a >= b)
//	GT(a,b)   → lhs=b, rhs=a, skipOnZero=false (sign==1 iff b < a iff a > b)
//	LTEQ(a,b) → lhs=b, rhs=a, skipOnZero=true  (sign==0 iff b >= a iff a <= b)
func normalizeRelational[W word.Word[W]](si *bytecode.SkipIf[W]) (lhs, rhs bytecode.RegisterId, skipOnZero bool) {
	left := si.Left.Registers()
	right := si.Right.AsRegisters()
	//
	if len(left) != 1 || len(right) != 1 {
		panic("cannot lower comparisons after register splitting")
	}
	//
	lhs = left[0]
	rhs = right[0]
	//
	switch si.Op {
	case bytecode.CONDITION_LT:
		return lhs, rhs, false
	case bytecode.CONDITION_GTEQ:
		return lhs, rhs, true
	case bytecode.CONDITION_GT:
		return rhs, lhs, false
	case bytecode.CONDITION_LTEQ:
		return rhs, lhs, true
	default:
		panic("normalizeRelational called with non-relational condition")
	}
}
