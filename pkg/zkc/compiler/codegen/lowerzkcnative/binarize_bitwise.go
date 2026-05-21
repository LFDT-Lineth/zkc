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
	"fmt"
	"math/big"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// RegisterAllocator provides a simple means of allocating new registers
type RegisterAllocator = register.Allocator[int]

// BinarizeBitwise splits any AND/OR/XOR instruction with more than two source
// registers into a left-fold chain of binary instructions.  This must run
// before LowerBitwise so that the helper modules it generates never need more
// than two inputs.
func BinarizeBitwise[W vm.Word[W]](modules []vm.Module) []vm.Module {
	out := append([]vm.Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*vm.WordFunction); ok {
			out[i] = binarizeBitwiseFunction[W](fn)
		}
	}

	return out
}

func binarizeBitwiseFunction[W vm.Word[W]](fn *vm.WordFunction) *vm.WordFunction {
	var (
		code  = fn.Code()
		ncode = make([]vectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		ncode[i] = insn.Map(func(_ uint, ith vm.WordInstruction) []vm.WordInstruction {
			return binarizeBitwiseCode[W](ith, alloc)
		})
	}

	return vm.NewFunction(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

func binarizeBitwiseCode[W vm.Word[W]](code vm.WordInstruction, registers RegisterAllocator,
) []vm.WordInstruction {
	var (
		op       instruction.OpCode
		target   register.Id
		sources  []register.Id
		constant W
	)

	switch code.OpCode() {
	case opcode.BIT_AND:
		t := code.(*instruction.WordTypeB[W])
		op, target, sources, constant = t.OpCode(), t.Target, t.Sources, t.Constant
	case opcode.BIT_OR:
		t := code.(*instruction.WordTypeB[W])
		op, target, sources, constant = t.OpCode(), t.Target, t.Sources, t.Constant
	case opcode.BIT_XOR:
		t := code.(*instruction.WordTypeB[W])
		op, target, sources, constant = t.OpCode(), t.Target, t.Sources, t.Constant
	default:
		return []vm.WordInstruction{code}
	}

	width := registers.Register(target).Width()
	identity := bitwiseIdentity[W](op, width)

	insns := make([]vm.WordInstruction, 0, len(sources))

	// If the constant is not the identity, materialise it as a register and add it to sources.
	if constant.Cmp(identity) != 0 {
		cstReg := registers.Allocate("", width)
		insns = append(insns, instruction.UintAdd(cstReg, nil, constant))
		sources = append(sources, cstReg)
	}

	switch len(sources) {
	case 0:
		panic(fmt.Sprintf("unexpected bitwise instruction with no sources: %T", code))
	case 1:
		// Trivial assignment: target = sources[0]
		return append(insns, instruction.UintAssign[W](target, sources[0]))
	case 2:
		// Happy path: just one binary op, possibly with a constant.
		return append(insns, newBinaryBitOp(op, target, sources[0], sources[1], identity))
	default:
		acc := sources[0]
		for _, src := range sources[1 : len(sources)-1] {
			tmp := registers.Allocate("", width)
			insns = append(insns, newBinaryBitOp(op, tmp, acc, src, identity))
			acc = tmp
		}

		return append(insns, newBinaryBitOp(op, target, acc, sources[len(sources)-1], identity))
	}
}

// bitwiseIdentity returns the identity element for the given bitwise operation:
// 0b111... for AND, 0b000... for OR/XOR.
func bitwiseIdentity[W vm.Word[W]](op instruction.OpCode, width uint) W {
	var z W

	if op == opcode.BIT_AND {
		maskBig := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width), big.NewInt(1))
		return z.SetBigInt(maskBig)
	}

	return z
}

func newBinaryBitOp[W vm.Word[W]](op instruction.OpCode, target, lhs, rhs register.Id, constant W,
) vm.WordInstruction {
	sources := []register.Id{lhs, rhs}

	switch op {
	case opcode.BIT_AND:
		return instruction.BitAnd[W](target, sources, constant)
	case opcode.BIT_OR:
		return instruction.BitOr[W](target, sources, constant)
	case opcode.BIT_XOR:
		return instruction.BitXor[W](target, sources, constant)
	default:
		panic(fmt.Sprintf("unexpected bitwise opcode: %d", op))
	}
}
