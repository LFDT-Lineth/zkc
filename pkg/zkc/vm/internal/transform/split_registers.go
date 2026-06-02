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
	"fmt"

	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// SplitRegisters all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func SplitRegisters[W word.Word[W]](mapping module.LimbsMap, m *machine.Word[W]) *machine.Word[W] {
	var (
		mods = make([]Module, len(m.Modules()))
	)
	//
	for i, ith := range m.Modules() {
		// Determine limb mapping for this module
		limbsMap := mapping.Module(uint(i))
		//
		mods[i] = splitModule[W](limbsMap, ith)
	}
	//
	return machine.NewWord[W](mapping.Field(), mods...)
}

func splitModule[W word.Word[W]](mapping register.LimbsMap, m Module) Module {
	switch m := m.(type) {
	case *function.Function[WordInstruction]:
		return splitFunction[W](mapping, *m)
	case memory.Memory[W]:
		return splitMemory(mapping, m)
	default:
		panic("unknown module encountered")
	}
}

func splitMemory[W word.Word[W]](mapping register.LimbsMap, m memory.Memory[W]) Module {
	var (
		registers = mapping.Limbs()
	)
	//
	switch m := m.(type) {
	case *memory.WriteOnce[W]:
		return &memory.WriteOnce[W]{
			StaticArray: memory.NewStaticArray[W](m.Name(), m.Kind(), registers),
		}
	case *memory.ReadOnly[W]:
		return &memory.ReadOnly[W]{
			StaticArray: memory.NewStaticArray[W](m.Name(), m.Kind(), registers),
		}
	case *memory.StaticReadOnly[W]:
		panic("support subdivision for static ROM")
	default:
		panic(fmt.Sprintf("unknown memory \"%s\"", m.Name()))
	}
}

func splitFunction[W word.Word[W]](mapping register.LimbsMap, m function.Function[WordInstruction]) Module {
	var (
		registers = mapping.Limbs()
		code      = splitInstructions[W](mapping, m.Code())
	)
	//
	return function.New(m.Name(), m.IsNative(), registers, code)
}

func splitInstructions[W word.Word[W]](mapping register.LimbsMap, code []VectorInstruction) []VectorInstruction {
	var ncode = make([]VectorInstruction, len(code))
	//
	for i, c := range code {
		ncode[i] = splitInstruction[W](mapping, c)
	}
	//
	return ncode
}

func splitInstruction[W word.Word[W]](limbsMap register.LimbsMap, vec VectorInstruction) VectorInstruction {
	var (
		insns []WordInstruction
	)
	// skipif
	//
	for _, c := range vec.Codes {
		switch c.OpCode() {
		// =======================================================
		// Base instructions
		// =======================================================
		case opcode.CALL:
			insns = append(insns, splitRegisters(limbsMap, c))
		case opcode.DEBUG:
			c := c.(*instruction.Debug)
			insns = append(insns, splitFormatting(limbsMap, false, c.Chunks))
		case opcode.FAIL:
			c := c.(*instruction.Fail)
			insns = append(insns, splitFormatting(limbsMap, true, c.Chunks))
		case opcode.JUMP:
			insns = append(insns, c)
		case opcode.MEMORY_READ:
			insns = append(insns, splitRegisters(limbsMap, c))
		case opcode.MEMORY_WRITE:
			insns = append(insns, splitRegisters(limbsMap, c))
		case opcode.RETURN:
			insns = append(insns, c)
		case opcode.SKIP:
			insns = append(insns, c)
		case opcode.SKIP_IF:
			insns = append(insns, splitRegisters(limbsMap, c))

		// =======================================================
		// Arithmetic instructions
		// =======================================================

		case opcode.INT_ADD:
			c := c.(*instruction.WordTypeA[W])
			insns = append(insns, splitAddition(limbsMap, c)...)
		case opcode.INT_SUB:
			insns = append(insns, splitSubtraction(limbsMap, c)...)
		case opcode.INT_MUL:
			insns = append(insns, splitMultiplication(limbsMap, c)...)
		default:
			panic("unsupported instruction")
		}
	}

	//
	return instruction.NewVector(insns...)
}

func splitRegisters(limbsMap register.LimbsMap, insn WordInstruction) WordInstruction {
	switch c := insn.(type) {
	case *instruction.Call:
		args := register.ApplyLimbsMap(limbsMap, c.Arguments...)
		rets := register.ApplyLimbsMap(limbsMap, c.Returns...)
		//
		return instruction.NewCall(c.Id, args, rets)
	case *instruction.MemRead:
		addr := register.ApplyLimbsMap(limbsMap, c.Arguments...)
		data := register.ApplyLimbsMap(limbsMap, c.Returns...)
		//
		return instruction.NewMemRead(c.Id, addr, data)
	case *instruction.MemWrite:
		addr := register.ApplyLimbsMap(limbsMap, c.Arguments...)
		data := register.ApplyLimbsMap(limbsMap, c.Returns...)
		//
		return instruction.NewMemWrite(c.Id, addr, data)
	case *instruction.SkipIf:
		left := register.ApplyLimbsMap(limbsMap, c.Left.Registers()...)
		right := register.ApplyLimbsMap(limbsMap, c.Right.Registers()...)
		// Construct vectored form of skip_if
		return instruction.NewSkipIfVec(c.Cond, register.NewVector(left...), register.NewVector(right...), c.Skip)
	default:
		panic("unsupported instruction")
	}
}

func splitFormatting(limbsMap register.LimbsMap, fail bool, chunks []instruction.FormattedChunk) WordInstruction {
	var (
		nchunks = make([]instruction.FormattedChunk, len(chunks))
	)
	//
	for i, chunk := range chunks {
		// split registers
		arg := register.ApplyLimbsMap(limbsMap, chunk.Argument.Registers()...)
		//
		nchunks[i] = instruction.FormattedChunk{
			Text:     chunk.Text,
			Format:   chunk.Format,
			Argument: register.NewVector(arg...),
		}
	}
	//
	if fail {
		return instruction.NewFail(nchunks...)
	}
	//
	return instruction.NewDebug(nchunks...)
}

func splitAddition[W word.Word[W]](limbsMap register.LimbsMap, insn *instruction.WordTypeA[W]) []WordInstruction {
	var (
		target  = register.ApplyLimbsMap(limbsMap, insn.Target.Registers()...)
		sources = register.ApplyLimbsMap(limbsMap, insn.Sources...)
	)
	// FIXME: this is a temporary place holder to allow some tests to actually
	// run.  It is not a proper implementation of this function.
	if len(target) > 1 {
		// TODO: this is where we actually need to do something
		panic("todo")
	}
	//
	return []WordInstruction{instruction.UintAdd(target[0], sources, insn.Constant)}
}

func splitSubtraction(limbsMap register.LimbsMap, insn WordInstruction) []WordInstruction {
	panic("todo")
}

func splitMultiplication(limbsMap register.LimbsMap, insn WordInstruction) []WordInstruction {
	panic("todo")
}
