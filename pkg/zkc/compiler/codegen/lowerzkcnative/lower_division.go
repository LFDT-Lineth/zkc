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
func LowerDivisions[W vm.Word[W]](modules []vm.Module) []vm.Module {
	out := append([]vm.Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*vm.WordFunction); ok {
			out[i] = lowerDivisionFunction[W](fn)
		}
	}

	return out
}

func lowerDivisionFunction[W vm.Word[W]](fn *vm.WordFunction) *vm.WordFunction {
	var (
		code      = fn.Code()
		ncode     = make([]vectorInstruction, len(code))
		registers = append([]register.Register{}, fn.Registers()...)
	)

	for i, insn := range code {
		ncodes := lowerDivisionCodes[W](insn.Codes, &registers)
		ncode[i] = vectorInstruction{Codes: ncodes}
	}

	return vm.NewFunction(fn.Name(), fn.IsNative(), registers, ncode)
}

func lowerDivisionCodes[W vm.Word[W]](
	codes []vm.WordInstruction,
	registers *[]register.Register,
) []vm.WordInstruction {
	ncodes := make([]vm.WordInstruction, 0, len(codes))

	for _, code := range codes {
		ncodes = append(ncodes, lowerDivisionCode[W](code, registers)...)
	}

	return ncodes
}

func lowerDivisionCode[W vm.Word[W]](
	code vm.WordInstruction,
	registers *[]register.Register,
) []vm.WordInstruction {
	switch code.OpCode() {
	case opcode.INT_DIV:
		insn := code.(*instruction.IntDiv[W])
		return expandDivision[W](insn.Target, insn.Sources[0], insn.Sources[1], registers)
	case opcode.INT_REM:
		insn := code.(*instruction.IntRem[W])
		return expandRemainder[W](insn.Target, insn.Sources[0], insn.Sources[1], registers)
	default:
		return []vm.WordInstruction{code}
	}
}

// expandDivision replaces INT_DIV(q, x, y) with the hint+validation sequence.
// sum holds q*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandDivision[W vm.Word[W]](q, x, y register.Id, registers *[]register.Register) []vm.WordInstruction {
	var (
		nX      = (*registers)[x.Unwrap()].Width()
		zero    = vm.Uint64[W](0)
		one     = vm.Uint64[W](1)
		wideQ   = allocTmp(registers, 2*nX)
		rTmp    = allocTmp(registers, 2*nX)
		wideX   = allocTmp(registers, 2*nX)
		wideY   = allocTmp(registers, 2*nX)
		product = allocTmp(registers, 2*nX)
	)

	return []vm.WordInstruction{
		instruction.NewFieldHint([]register.Id{wideQ, rTmp}, []register.Id{x, y}),
		instruction.NewCast(q, wideQ, nX),
		instruction.NewCast(wideX, x, 2*nX),
		instruction.NewCast(wideY, y, 2*nX),
		instruction.NewIntMul(product, []register.Id{wideQ, wideY}, one),
		instruction.NewIntAdd(wideX, []register.Id{product, rTmp}, zero),
		instruction.NewSkipIf(opcode.LT, rTmp, y, 1),
		instruction.NewFail(),
	}
}

// expandRemainder replaces INT_REM(r, x, y) with the hint+validation sequence.
// sum holds qTmp*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandRemainder[W vm.Word[W]](r, x, y register.Id, registers *[]register.Register) []vm.WordInstruction {
	var (
		nX      = (*registers)[x.Unwrap()].Width()
		zero    = vm.Uint64[W](0)
		one     = vm.Uint64[W](1)
		qTmp    = allocTmp(registers, 2*nX)
		wideR   = allocTmp(registers, 2*nX)
		wideX   = allocTmp(registers, 2*nX)
		wideY   = allocTmp(registers, 2*nX)
		product = allocTmp(registers, 2*nX)
	)

	return []vm.WordInstruction{
		instruction.NewFieldHint([]register.Id{qTmp, wideR}, []register.Id{x, y}),
		instruction.NewCast(r, wideR, nX),
		instruction.NewCast(wideX, x, 2*nX),
		instruction.NewCast(wideY, y, 2*nX),
		instruction.NewIntMul(product, []register.Id{qTmp, wideY}, one),
		instruction.NewIntAdd(wideX, []register.Id{product, wideR}, zero),
		instruction.NewSkipIf(opcode.LT, wideR, y, 1),
		instruction.NewFail(),
	}
}
