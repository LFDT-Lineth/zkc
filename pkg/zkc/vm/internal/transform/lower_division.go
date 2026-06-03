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

// LowerDivisions rewrites INT_DIV and INT_REM instructions into a
// non-deterministic hint followed by arithmetic validation:
//
//	FieldHint{targets:[wideQ, wideR], sources:[x, y]}  // prover fills both at 2n bits
//	q = cast(wideQ, n) ; r = cast(wideR, n)           // write results to n-bit outputs
//	wideX, wideY = cast(x, 2n), cast(y, 2n)
//	sum = wideQ * wideY                                // exact 2n-bit product
//	sum = sum + wideR
//	SkipIf(EQ, sum, wideX, 1)
//	Fail
//	SkipIf(LT, r, y, 1)                        // expanded later by LowerComparisons
//	Fail
//
// This pass must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](modules []Module) []Module {
	out := append([]Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*WordFunction); ok {
			out[i] = lowerDivisionFunction[W](fn)
		}
	}

	return out
}

func lowerDivisionFunction[W word.Word[W]](fn *WordFunction) *WordFunction {
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith WordInstruction) []WordInstruction {
			return lowerDivisionCode[W](ith, alloc)
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

func lowerDivisionCode[W word.Word[W]](
	code WordInstruction,
	registers RegisterAllocator,
) []WordInstruction {
	switch code.OpCode() {
	case opcode.INT_DIV:
		insn := code.(*instruction.WordTypeB)
		return expandDivision[W](insn.Target, insn.LeftSource, insn.RightSource, registers)
	case opcode.INT_REM:
		insn := code.(*instruction.WordTypeB)
		return expandRemainder[W](insn.Target, insn.LeftSource, insn.RightSource, registers)
	default:
		return []WordInstruction{code}
	}
}

// expandDivision replaces INT_DIV(q, x, y) with the hint+validation sequence.
// sum holds q*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandDivision[W word.Word[W]](q, x, y register.Id, registers RegisterAllocator) []WordInstruction {
	var (
		nX   = registers.Register(x).Width()
		nY   = registers.Register(y).Width()
		r    = registers.Allocate("", nY)
		w    = registers.Allocate("", nY)
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("", nX)
		// NOTE: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("", 0)
		z1 = registers.Allocate("", 0)
	)
	//
	return []WordInstruction{
		instruction.NewFieldHint([]register.Id{q, r, w}, []register.Id{x, y}),
		instruction.UintMul(qy, []register.Id{q, y}, one),
		instruction.UintSub(z0, []register.Id{x, qy, r}, zero),
		instruction.UintSub(z1, []register.Id{y, r, w}, one),
	}
}

// expandRemainder replaces INT_REM(r, x, y) with the hint+validation sequence.
// sum holds qTmp*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandRemainder[W word.Word[W]](r, x, y register.Id, registers RegisterAllocator) []WordInstruction {
	var (
		nX   = registers.Register(x).Width()
		nY   = registers.Register(y).Width()
		q    = registers.Allocate("", nX)
		w    = registers.Allocate("", nY)
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("", nX)
		// NOTE: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("", 0)
		z1 = registers.Allocate("", 0)
	)
	//
	return []WordInstruction{
		instruction.NewFieldHint([]register.Id{q, r, w}, []register.Id{x, y}),
		instruction.UintMul(qy, []register.Id{q, y}, one),
		instruction.UintSub(z0, []register.Id{x, qy, r}, zero),
		instruction.UintSub(z1, []register.Id{y, r, w}, one),
	}
}
