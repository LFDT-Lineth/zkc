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
package machine

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// WordInstruction is a useful alias
type WordInstruction = instruction.Word

// VectorInstruction is a useful alias
type VectorInstruction = instruction.Vector[WordInstruction]

// Subdivide all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func Subdivide[W word.Word[W]](mapping module.LimbsMap, m *Word[W]) *Word[W] {
	var (
		mods = make([]Module, len(m.Modules()))
	)
	//
	for i, ith := range m.Modules() {
		// Determine limb mapping for this module
		limbsMap := mapping.Module(uint(i))
		//
		mods[i] = subdivideModule[W](limbsMap, ith)
	}
	//
	return NewWord[W](mapping.Field(), mods...)
}

func subdivideModule[W word.Word[W]](mapping register.LimbsMap, m Module) Module {
	switch m := m.(type) {
	case *function.Function[instruction.Word]:
		return subdivideFunction[W](mapping, *m)
	case memory.Memory[W]:
		return subdivideMemory(mapping, m)
	default:
		panic("unknown module encountered")
	}
}

func subdivideMemory[W word.Word[W]](mapping register.LimbsMap, m memory.Memory[W]) Module {
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

func subdivideFunction[W word.Word[W]](mapping register.LimbsMap, m function.Function[instruction.Word]) Module {
	var (
		registers = mapping.Limbs()
		code      = subdivideInstructions[W](mapping, m.Code())
	)
	//
	return function.New(m.Name(), m.IsNative(), registers, code)
}

func subdivideInstructions[W word.Word[W]](mapping register.LimbsMap, code []VectorInstruction) []VectorInstruction {
	var ncode = make([]instruction.Vector[instruction.Word], len(code))
	//
	for i, c := range code {
		ncode[i] = subdivideInstruction[W](mapping, c)
	}
	//
	return ncode
}

func subdivideInstruction[W word.Word[W]](limbsMap register.LimbsMap, vec VectorInstruction) VectorInstruction {
	var (
		insns []instruction.Word
	)
	// skipif
	//
	for _, c := range vec.Codes {
		switch c := c.(type) {
		// =======================================================
		// Base instructions
		// =======================================================
		case *instruction.Call:
			insns = append(insns, subdivideRegisters(limbsMap, c))
		case *instruction.Debug:
			insns = append(insns, subdivideFormatting(limbsMap, false, c.Chunks))
		case *instruction.Destruct:
			insns = append(insns, subdivideRegisters(limbsMap, c))
		case *instruction.Fail:
			insns = append(insns, subdivideFormatting(limbsMap, true, c.Chunks))
		case *instruction.Jump:
			insns = append(insns, c)
		case *instruction.MemRead:
			insns = append(insns, subdivideRegisters(limbsMap, c))
		case *instruction.MemWrite:
			insns = append(insns, subdivideRegisters(limbsMap, c))
		case *instruction.Return:
			insns = append(insns, c)
		case *instruction.Skip:
			insns = append(insns, c)
		case *instruction.SkipIf:
			insns = append(insns, subdivideRegisters(limbsMap, c))

		// =======================================================
		// Arithmetic instructions
		// =======================================================

		case *instruction.IntAdd[W]:
			insns = append(insns, subdivideAddition(limbsMap, c)...)
		case *instruction.IntSub[W]:
			insns = append(insns, subdivideSubtraction(limbsMap, c)...)
		case *instruction.IntMul[W]:
			insns = append(insns, subdivideMultiplication(limbsMap, c)...)
		default:
			panic("unsupported instruction")
		}
	}

	//
	return instruction.NewVector(insns...)
}

func subdivideRegisters(limbsMap register.LimbsMap, insn instruction.Word) instruction.Word {
	switch c := insn.(type) {
	case *instruction.Call:
		args := register.ApplyLimbsMap(limbsMap, c.Arguments...)
		rets := register.ApplyLimbsMap(limbsMap, c.Returns...)
		//
		return instruction.NewCall(c.Id, args, rets)
	case *instruction.Destruct:
		// addr := register.ApplyLimbsMap(limbsMap, c.Targets...)
		// //
		// return instruction.NewDestruct(addr, data)
		panic("todo")
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

func subdivideFormatting(limbsMap register.LimbsMap, fail bool, chunks []instruction.FormattedChunk) instruction.Word {
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

func subdivideAddition[W word.Word[W]](limbsMap register.LimbsMap, insn *instruction.IntAdd[W]) []instruction.Word {
	var (
		target  = register.ApplyLimbsMap(limbsMap, insn.Target)
		sources = register.ApplyLimbsMap(limbsMap, insn.Sources...)
	)
	// FIXME: this is a temporary place holder to allow some tests to actually
	// run.  It is not a proper implementation of this function.
	if len(target) > 1 {
		// TODO: this is where we actually need to do something
		panic("todo")
	}
	//
	return []instruction.Word{instruction.NewIntAdd(target[0], sources, insn.Constant)}
}

func subdivideSubtraction(limbsMap register.LimbsMap, insn instruction.Word) []instruction.Word {
	panic("todo")
}

func subdivideMultiplication(limbsMap register.LimbsMap, insn instruction.Word) []instruction.Word {
	panic("todo")
}
