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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerDivisions rewrites INT_DIV and INT_REM bytecodes into a non-deterministic
// hint followed by arithmetic validation:
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy = q * y
//	z0 = x - qy - r          // written into a 0-width register: asserts == 0
//	z1 = y - r - w - 1       // written into a 0-width register: asserts == 0
//
// NOTE: this transform must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerDivisionFunction[W](fn)
		}
	}

	return descriptor.NewProgram(out...)
}

func lowerDivisionFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = newRegAllocator(fn.Registers())
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return lowerDivisionCode[W](b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.IsNative(), nvecs)
}

func lowerDivisionCode[W word.Word[W]](
	b Bytecode[W],
	registers *regAllocator[W],
) []Bytecode[W] {
	dr, ok := b.(*bytecode.DivRem)
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	switch dr.Opcode {
	case encoding.DIV:
		return expandDivision[W](dr.Target, dr.Dividend, dr.Divisor, registers)
	case encoding.REM:
		return expandRemainder[W](dr.Target, dr.Dividend, dr.Divisor, registers)
	default:
		return []Bytecode[W]{b}
	}
}

// expandDivision replaces INT_DIV(q, x, y) with the hint+validation sequence.
// qy holds q*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandDivision[W word.Word[W]](q, x, y bytecode.RegisterId, registers *regAllocator[W]) []Bytecode[W] {
	var (
		nX   = registers.Register(x).Bitwidth().Unwrap()
		nY   = registers.Register(y).Bitwidth().Unwrap()
		r    = registers.Allocate("", util.Some(nY))
		w    = registers.Allocate("", util.Some(nY))
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("", util.Some(nX))
		// NOTE: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("", util.Some[uint](0))
		z1 = registers.Allocate("", util.Some[uint](0))
	)
	//
	return []Bytecode[W]{
		bytecode.NewHint(bytecode.DIV_HINT,
			[]bytecode.RegisterVector{
				bytecode.NewRegisterVector(q), bytecode.NewRegisterVector(r), bytecode.NewRegisterVector(w),
			},
			[]bytecode.RegisterVector{bytecode.NewRegisterVector(x), bytecode.NewRegisterVector(y)}),
		bytecode.MulConst(qy, []bytecode.RegisterId{q, y}, one),
		bytecode.SubConst(z0, []bytecode.RegisterId{x, qy, r}, zero),
		bytecode.SubConst(z1, []bytecode.RegisterId{y, r, w}, one),
	}
}

// expandRemainder replaces INT_REM(r, x, y) with the hint+validation sequence.
// qy holds q*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandRemainder[W word.Word[W]](r, x, y bytecode.RegisterId, registers *regAllocator[W]) []Bytecode[W] {
	var (
		nX   = registers.Register(x).Bitwidth().Unwrap()
		nY   = registers.Register(y).Bitwidth().Unwrap()
		q    = registers.Allocate("", util.Some(nX))
		w    = registers.Allocate("", util.Some(nY))
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("", util.Some(nX))
		// NOTE: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("", util.Some[uint](0))
		z1 = registers.Allocate("", util.Some[uint](0))
	)
	//
	return []Bytecode[W]{
		bytecode.NewHint(bytecode.DIV_HINT,
			[]bytecode.RegisterVector{
				bytecode.NewRegisterVector(q), bytecode.NewRegisterVector(r), bytecode.NewRegisterVector(w),
			},
			[]bytecode.RegisterVector{bytecode.NewRegisterVector(x), bytecode.NewRegisterVector(y)}),
		bytecode.MulConst(qy, []bytecode.RegisterId{q, y}, one),
		bytecode.SubConst(z0, []bytecode.RegisterId{x, qy, r}, zero),
		bytecode.SubConst(z1, []bytecode.RegisterId{y, r, w}, one),
	}
}
