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
		code  = fn.Code()
		ncode = make([]vectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith vm.WordInstruction) []vm.WordInstruction {
			return lowerDivisionCode[W](ith, alloc)
		})
	}

	return vm.NewFunction(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

func lowerDivisionCode[W vm.Word[W]](
	code vm.WordInstruction,
	registers RegisterAllocator,
) []vm.WordInstruction {
	switch code.OpCode() {
	case opcode.INT_DIV:
		insn := code.(*instruction.WordTypeB)
		return expandDivision[W](insn.Target, insn.LeftSource, insn.RightSource, registers)
	case opcode.INT_REM:
		insn := code.(*instruction.WordTypeB)
		return expandRemainder[W](insn.Target, insn.LeftSource, insn.RightSource, registers)
	default:
		return []vm.WordInstruction{code}
	}
}

// expandDivision replaces INT_DIV(q, x, y) with the hint+validation sequence.
// sum holds q*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandDivision[W vm.Word[W]](q, x, y register.Id, registers RegisterAllocator) []vm.WordInstruction {
	var (
		nX      = registers.Register(x).Width()
		nY      = registers.Register(y).Width()
		r       = registers.Allocate("", nY)
		w       = registers.Allocate("", nY)
		zero    = vm.Uint64[W](0)
		one     = vm.Uint64[W](1)
		qy      = registers.Allocate("", nX)
		qy_r    = registers.Allocate("", nX)
		y_r_w_1 = registers.Allocate("", nY)
		// NOTE: deprecate following when skip_if supports constants
		z = registers.Allocate("", 0)
	)
	//
	return []vm.WordInstruction{
		instruction.NewFieldHint([]register.Id{q, r, w}, []register.Id{x, y}),
		instruction.UintMul(qy, []register.Id{q, y}, one),
		instruction.UintAdd(qy_r, []register.Id{qy, r}, zero),
		instruction.NewSkipIf(opcode.EQ, qy_r, x, 1),
		instruction.NewFail(),
		instruction.UintSub(y_r_w_1, []register.Id{y, r, w}, one),
		instruction.UintConst(z, zero),
		instruction.NewSkipIf(opcode.EQ, y_r_w_1, z, 1),
		instruction.NewFail(),
	}
}

// expandRemainder replaces INT_REM(r, x, y) with the hint+validation sequence.
// sum holds qTmp*y and must be 2*nX bits so the product is exact: a cheating prover
// could otherwise pick q' = q + 2^nX, satisfying q'*y + r ≡ x (mod 2^nX).
func expandRemainder[W vm.Word[W]](r, x, y register.Id, registers RegisterAllocator) []vm.WordInstruction {
	var (
		nX      = registers.Register(x).Width()
		nY      = registers.Register(y).Width()
		q       = registers.Allocate("", nX)
		w       = registers.Allocate("", nY)
		zero    = vm.Uint64[W](0)
		one     = vm.Uint64[W](1)
		qy      = registers.Allocate("", nX)
		qy_r    = registers.Allocate("", nX)
		y_r_w_1 = registers.Allocate("", nY)
		// NOTE: deprecate following when skip_if supports constants
		z = registers.Allocate("", 0)
	)
	//
	return []vm.WordInstruction{
		instruction.NewFieldHint([]register.Id{q, r, w}, []register.Id{x, y}),
		instruction.UintMul(qy, []register.Id{q, y}, one),
		instruction.UintAdd(qy_r, []register.Id{qy, r}, zero),
		instruction.NewSkipIf(opcode.EQ, qy_r, x, 1),
		instruction.NewFail(),
		instruction.UintSub(y_r_w_1, []register.Id{y, r, w}, one),
		instruction.UintConst(z, zero),
		instruction.NewSkipIf(opcode.EQ, y_r_w_1, z, 1),
		instruction.NewFail(),
	}
}
