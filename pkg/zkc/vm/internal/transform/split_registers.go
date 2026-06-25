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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// RegisterId provides a useful alias
type RegisterId = descriptor.RegisterId

// SplitRegisters splits all registers in a program to meet a given field's
// bandwidth and maximum register width.  This will split all registers wider
// than the maximum permitted width into two or more "limbs" (i.e. subregisters
// which do not exceeded the permitted width). For example, consider a register
// "r" of width u32. Subdividing this register into registers of at most 8bits
// will result in four limbs: r'0, r'1, r'2 and r'3 where (by convention) r'0 is
// the least significant.
func SplitRegisters[W word.Word[W]](cfg field.Config, program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		//
		out = make([]descriptor.Module[W], len(mods))
	)
	//
	for i, ith := range mods {
		// construct limbs map for this module
		mapping := descriptor.NewLimbsMap[W](cfg, ith)
		// split the module
		out[i] = splitModule(mapping, ith)
	}
	//
	return descriptor.NewProgram(out...)
}

func splitModule[W word.Word[W]](mapping descriptor.LimbsMap[W], m descriptor.Module[W]) descriptor.Module[W] {
	switch m := m.(type) {
	case *descriptor.Function[W]:
		return splitFunction(mapping, m)
	case *descriptor.Memory[W]:
		return splitMemory(mapping, m)
	default:
		panic("unknown module encountered")
	}
}

func splitMemory[W word.Word[W]](mapping descriptor.LimbsMap[W], m *descriptor.Memory[W]) descriptor.Module[W] {
	var registers = mapping.Limbs()
	//
	switch {
	case m.IsStatic():
		// A static ROM carries its (constant) contents, so these must be split
		// alongside the registers: each cell value is subdivided into its limbs.
		return descriptor.NewMemory(m.Name(), registers, m.Kind(), splitStaticContents(mapping, m))
	case m.IsWriteOnly(), m.IsReadOnly(), m.IsReadWrite():
		// Non-static memories (write-once, read-only, and read-write RAM —
		// including paged) carry no constant contents; only their registers are
		// split.  The associated read/write bytecodes have their address and data
		// registers split separately (see splitRegisters for ReadWrite).
		return descriptor.NewMemory(m.Name(), registers, m.Kind(), nil)
	default:
		panic(fmt.Sprintf("unknown memory \"%s\"", m.Name()))
	}
}

// splitStaticContents subdivides the constant contents of a static ROM to match
// its split (limb) registers.  The flat contents hold one value per data
// register for each row (address lines are not stored), so each cell value is
// split into the limbs of its register — most-significant limb first, matching
// both the geometry's data-register order (mapping.Limbs()) and the limb order
// produced for memory reads by split.ApplyLimbsMap.
func splitStaticContents[W word.Word[W]](mapping descriptor.LimbsMap[W], m *descriptor.Memory[W]) []W {
	var (
		contents = m.StaticContents()
		limbsMap = mapping.LimbsMap()
		// Identify the data (output) registers, in declaration order.
		dataIds []RegisterId
	)
	//
	for i, r := range m.Registers() {
		if r.IsOutput() {
			dataIds = append(dataIds, util.Cast[RegisterId](uint(i)))
		}
	}
	// A ROW occupies one value per data register; with no data registers there
	// is nothing to split.
	if len(dataIds) == 0 {
		return contents
	}
	//
	var out []W
	//
	for row := 0; row+len(dataIds) <= len(contents); row += len(dataIds) {
		for j, id := range dataIds {
			out = append(out, splitCell(contents[row+j], mapping.LimbIds(id), limbsMap)...)
		}
	}
	//
	return out
}

// splitCell subdivides a single ROM cell value into its limb values, returned
// most-significant limb first (the order in which limb registers appear in the
// module geometry).  limbIds are least-significant first (as returned by
// LimbIds), so the least-significant slices are taken first and the result is
// reversed.
func splitCell[W word.Word[W]](value W, limbIds []RegisterId, limbsMap descriptor.RegisterMap[W]) []W {
	var (
		acc   = value
		limbs = make([]W, len(limbIds))
	)
	//
	for i, id := range limbIds {
		var width = limbsMap.Register(id).Bitwidth().Unwrap()
		// Least-significant limb first; reverse into geometry (MSB-first) order.
		limbs[len(limbIds)-1-i] = acc.Slice(width)
		acc = acc.Shr64(uint64(width))
	}
	//
	return limbs
}

func splitFunction[W word.Word[W]](mapping descriptor.LimbsMap[W], m *descriptor.Function[W]) descriptor.Module[W] {
	var (
		alloc = split.NewAllocator(mapping.LimbsMap())
		code  = splitBytecodeVector(mapping, alloc, m.Vectors())
	)
	//
	return descriptor.NewFunction(m.Name(), alloc.Registers(), m.IsNative(), code)
}

func splitBytecodeVector[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc split.Allocator[W],
	code []BytecodeVector[W]) []BytecodeVector[W] {
	//
	var ncode = make([]BytecodeVector[W], len(code))
	//
	for i, c := range code {
		ncode[i] = splitBytecode(mapping, alloc, c)
	}
	//
	return ncode
}

func splitBytecode[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], vec BytecodeVector[W],
) BytecodeVector[W] {
	var insns []Bytecode[W]
	//
	for _, c := range vec.Bytecodes {
		switch c := c.(type) {
		// =======================================================
		// Base bytecodes
		// =======================================================
		case *bytecode.Call:
			insns = append(insns, splitRegisters(limbsMap, c))
		case *bytecode.Cat:
			insns = append(insns, split.Concat(limbsMap, alloc, c)...)
		case *bytecode.Debug:
			insns = append(insns, &bytecode.Debug{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)})
		case *bytecode.Fail:
			insns = append(insns, &bytecode.Fail{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)})
		case *bytecode.Hint:
			// Each operand (argument / return) is split into the limbs of its
			// constituent registers, preserving the per-operand grouping so the
			// hint's executor can still reconstruct each value.
			insns = append(insns, &bytecode.Hint{
				Op:      c.Op,
				Targets: splitRegisterVectors(limbsMap, c.Targets),
				Sources: splitRegisterVectors(limbsMap, c.Sources),
			})
		case *bytecode.Jmp:
			insns = append(insns, c)
		case *bytecode.ReadWrite:
			insns = append(insns, splitRegisters(limbsMap, c))
		case *bytecode.Ret:
			insns = append(insns, c)
		case *bytecode.Skip:
			insns = append(insns, c)
		case *bytecode.SkipIf:
			insns = append(insns, splitRegisters(limbsMap, c))

		// =======================================================
		// Arithmetic bytecodes
		// =======================================================
		case *bytecode.Arith[W]:
			switch c.Op {
			case bytecode.OP_ADD:
				insns = append(insns, split.Addition(limbsMap, alloc, c)...)
			case bytecode.OP_SUB:
				insns = append(insns, split.Subtraction(limbsMap, alloc, c)...)
			case bytecode.OP_MUL:
				insns = append(insns, split.Multiplication(limbsMap, alloc, c)...)
			default:
				panic(fmt.Sprintf("unsupported arithmetic operation (%d)", c.Op))
			}

		// =======================================================
		// Bitwise bytecodes
		// =======================================================
		case *bytecode.Bitwise:
			switch c.Op {
			case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
				insns = append(insns, split.Bitwise(limbsMap, alloc, c)...)
			default:
				panic("todo: split bitwise operations")
			}

		// =======================================================
		// Misc bytecodes
		// =======================================================
		case *bytecode.DivRem:
			panic("todo: split div/rem operations")
		case *bytecode.FieldArith[W]:
			panic("todo: split field arithmetic operations")
		case *bytecode.Switch[W]:
			panic("todo: split field arithmetic operations")

		default:
			// NOTE: checkcast does not technically need to be supported because
			// the cast insertion phase runs after register splitting.  However,
			// it should be noted that splitting checkcast is pretty simple.
			panic(fmt.Sprintf("unsupported bytecode (%T)", c))
		}
	}
	//
	return bytecode.NewVector(insns...)
}

func splitRegisters[W word.Word[W]](limbsMap descriptor.LimbsMap[W], insn Bytecode[W]) Bytecode[W] {
	switch c := insn.(type) {
	case *bytecode.Call:
		args := split.ApplyLimbsMap(limbsMap, c.Arguments...)
		rets := split.ApplyLimbsMap(limbsMap, c.Returns...)
		//
		return bytecode.CallFun(c.Target, c.Flags, args, rets)
	case *bytecode.ReadWrite:
		addr := split.ApplyLimbsMap(limbsMap, c.Address...)
		data := split.ApplyLimbsMap(limbsMap, c.Data...)
		//
		if c.Write {
			return bytecode.NewMemWrite(c.Id, addr, data)
		}
		//
		return bytecode.NewMemRead(c.Id, addr, data)
	case *bytecode.SkipIf:
		left := split.ApplyLimbsMap(limbsMap, c.Left.Registers()...)
		right := split.ApplyLimbsMap(limbsMap, c.Right.Registers()...)
		// Construct vectored form of skip_if
		return &bytecode.SkipIf{Op: c.Op, Left: bytecode.NewRegisterVector(left...),
			Right: bytecode.NewRegisterVector(right...), Skip: c.Skip}
	default:
		panic("unsupported instruction")
	}
}

// splitRegisterVectors splits each register vector (e.g. a debug / fail formatted
// argument) into the limbs of its constituent registers.
func splitRegisterVectors[W any](limbsMap descriptor.LimbsMap[W],
	vecs []bytecode.RegisterVector) []bytecode.RegisterVector {
	var nvecs = make([]bytecode.RegisterVector, len(vecs))
	//
	for i, v := range vecs {
		nvecs[i] = bytecode.NewRegisterVector(split.ApplyLimbsMap(limbsMap, v.Registers()...)...)
	}
	//
	return nvecs
}
