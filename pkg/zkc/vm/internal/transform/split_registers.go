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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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
		out[i] = splitModule(mapping, mods, ith)
	}
	//
	return descriptor.NewProgram(out...)
}

func splitModule[W word.Word[W]](mapping descriptor.LimbsMap[W], mods []descriptor.Module[W],
	m descriptor.Module[W]) descriptor.Module[W] {
	//
	switch m := m.(type) {
	case *descriptor.Function[W]:
		return splitFunction(mapping, mods, m)
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

func splitFunction[W word.Word[W]](mapping descriptor.LimbsMap[W], mods []descriptor.Module[W],
	m *descriptor.Function[W]) descriptor.Module[W] {
	var (
		alloc = split.NewAllocator(mapping.LimbsMap())
		code  = splitBytecodeVector(mapping, mods, alloc, m.Vectors())
	)
	//
	return descriptor.NewFunction(m.Name(), alloc.Registers(), m.IsNative(), code)
}

func splitBytecodeVector[W word.Word[W]](mapping descriptor.LimbsMap[W], mods []descriptor.Module[W],
	alloc split.Allocator[W], code []BytecodeVector[W]) []BytecodeVector[W] {
	//
	var ncode = make([]BytecodeVector[W], len(code))
	//
	for i, c := range code {
		ncode[i] = splitBytecode(mapping, mods, alloc, c)
	}
	//
	return ncode
}

func splitBytecode[W word.Word[W]](limbsMap descriptor.LimbsMap[W], mods []descriptor.Module[W],
	alloc split.Allocator[W], vec BytecodeVector[W]) BytecodeVector[W] {
	// NOTE: Vector.Map rewrites skip offsets to account for instructions which
	// split into more than one instruction.
	return vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
		switch c := insn.(type) {
		// =======================================================
		// Base bytecodes
		// =======================================================
		case *bytecode.Call:
			return splitCall(limbsMap, alloc, mods, c)
		case *bytecode.Cat:
			return split.Concat(limbsMap, alloc, c)
		case *bytecode.Debug:
			return []Bytecode[W]{&bytecode.Debug{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)}}
		case *bytecode.Fail:
			return []Bytecode[W]{&bytecode.Fail{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)}}
		case *bytecode.Hint:
			// Each operand (argument / return) is split into the limbs of its
			// constituent registers, preserving the per-operand grouping so the
			// hint's executor can still reconstruct each value.
			return []Bytecode[W]{&bytecode.Hint{
				Op:      c.Op,
				Targets: splitRegisterVectors(limbsMap, c.Targets),
				Sources: splitRegisterVectors(limbsMap, c.Sources),
			}}
		case *bytecode.Jmp:
			return []Bytecode[W]{c}
		case *bytecode.ReadWrite:
			if c.Write {
				return splitWrite(limbsMap, alloc, mods, c)
			}
			//
			return splitRead(limbsMap, alloc, mods, c)
		case *bytecode.Ret:
			return []Bytecode[W]{c}
		case *bytecode.Skip:
			return []Bytecode[W]{c}
		case *bytecode.SkipIf:
			return []Bytecode[W]{splitSkipIf(limbsMap, c)}

		// =======================================================
		// Arithmetic bytecodes
		// =======================================================
		case *bytecode.Arith[W]:
			switch c.Op {
			case bytecode.OP_ADD:
				return split.Addition(limbsMap, alloc, c)
			case bytecode.OP_SUB:
				return split.Subtraction(limbsMap, alloc, c)
			case bytecode.OP_MUL:
				return split.Multiplication(limbsMap, alloc, c)
			default:
				panic(fmt.Sprintf("unsupported arithmetic operation (%d)", c.Op))
			}

		// =======================================================
		// Bitwise bytecodes
		// =======================================================
		case *bytecode.Bitwise:
			switch c.Op {
			case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
				return split.Bitwise(limbsMap, alloc, c)
			default:
				panic("todo: split shift operations")
			}

		// =======================================================
		// Misc bytecodes
		// =======================================================
		case *bytecode.DivRem:
			// NOTE: only relevant for splitting fast mode (i.e. non-lowered)
			// bytecode.
			panic("todo: split div/rem operations")
		case *bytecode.FieldArith[W]:
			panic("todo: split field arithmetic operations")
		case *bytecode.Switch[W]:
			// NOTE: only relevant for splitting fast mode (i.e. non-lowered)
			// bytecode.
			panic("todo: split switch bytecode")

		default:
			// NOTE: checkcast does not technically need to be supported because
			// the cast insertion phase runs after register splitting.  However,
			// it should be noted that splitting checkcast is pretty simple.
			panic(fmt.Sprintf("unsupported bytecode (%T)", c))
		}
	})
}

// splitCall splits the registers referenced by a call.  Unlike a purely local
// instruction, a call transfers values across a module boundary: its arguments
// flow out into the callee's inputs and its returns flow back in from the
// callee's outputs.  Each argument / return is therefore reconciled against the
// width of the corresponding callee parameter / result (see splitBoundary),
// which may introduce additional width-check / zero-extension bytecodes either
// side of the call.
func splitCall[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.Call) []Bytecode[W] {
	//
	var (
		callee = mods[c.Target]
		// Arguments are outgoing (this frame -> callee inputs); returns are
		// incoming (callee outputs -> this frame).
		args, pre  = alignArgsReturns(limbsMap, alloc, c.Arguments, callee.Inputs(), argAlignment)
		rets, post = alignArgsReturns(limbsMap, alloc, c.Returns, callee.Outputs(), retAlignment)
	)
	//
	return join(pre, bytecode.CallFun(c.Target, c.Flags, args, rets), post)
}

func splitRead[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.ReadWrite) []Bytecode[W] {
	//
	var (
		mem = mods[c.Id]
		// Address registers correspond to the memory's inputs, data registers to
		// its outputs.
		addr, pre1 = alignArgsReturns(limbsMap, alloc, c.Address, mem.Inputs(), argAlignment)
		data, pre2 = alignArgsReturns(limbsMap, alloc, c.Data, mem.Outputs(), argAlignment)
	)
	//
	return join(append(pre1, pre2...), bytecode.NewMemRead(c.Id, addr, data), nil)
}

func splitWrite[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.ReadWrite) []Bytecode[W] {
	//
	var (
		mem = mods[c.Id]
		// Address registers correspond to the memory's inputs, data registers to
		// its outputs.
		addr, pre  = alignArgsReturns(limbsMap, alloc, c.Address, mem.Inputs(), argAlignment)
		data, post = alignArgsReturns(limbsMap, alloc, c.Data, mem.Outputs(), retAlignment)
	)
	//
	return join(pre, bytecode.NewMemWrite(c.Id, addr, data), post)
}

func splitSkipIf[W word.Word[W]](limbsMap descriptor.LimbsMap[W], c *bytecode.SkipIf) Bytecode[W] {
	// Both operands are frame-local registers of equal width, so each is simply
	// mapped onto its limbs.
	left := split.ApplyLimbsMap(limbsMap, c.Left.Registers()...)
	right := split.ApplyLimbsMap(limbsMap, c.Right.Registers()...)
	// Construct vectored form of skip_if
	return &bytecode.SkipIf{Op: c.Op, Left: bytecode.NewRegisterVector(left...),
		Right: bytecode.NewRegisterVector(right...), Skip: c.Skip}
}

// Argument alignment is concerned with ensuring the number of arguments matches
// the number of declared parameters.  For example, consider this case:
//
// fn f(x:u16) { g(x as u32) }
// fn g(y:u32) { ... }
//
// Splitting to a maximum of u16 registers, then the following bytecode is
// produced:
//
// fn f(x:u16) { g(0,x) }
// fn g(y'1:u16,y'0:u16) { ... }
//
// Conversly, consider the opposite:
//
// fn f(x:u32) { g(x as u16) }
// fn g(y:u16) { ... }
//
// Again, splitting to a maximum of u16 registers, then the following bytecode
// is produced:
//
// fn f(x'1:u16,x'0:u16) { g(x'0); check x'1 == 0 }
// fn g(y:u16) { ... }
func argAlignment[W word.Word[W]](local []RegisterId, remote []descriptor.Register[W], alloc split.Allocator[W],
) ([]RegisterId, []Bytecode[W]) {
	//
	var (
		context []Bytecode[W]
		m, n    = len(local), len(remote)
	)
	//
	if m < n {
		var zreg = alloc.ZeroRegister()
		// Less locals than remotes.  In this case, pad locals with zero
		// register.
		for i := m; i < n; i++ {
			local = append(local, zreg)
		}
	} else if m > n {
		// More locals than remotes.  In this case, ensure all surplus locals
		// are zero.
		for _, s := range local[n:] {
			context = append(context, bytecode.NewCheckCast(s, 0))
		}
		// discard surplus
		local = local[:n]
	}
	//
	return local, context
}

// Argument alignment is concerned with ensuring the number of arguments matches
// the number of declared parameters.  For example, consider this case:
//
// fn f() -> (r:u16) { r = g() as u16 }
// fn g() -> (p:u32) { ... }
//
// Splitting to a maximum of u16 registers, then the following bytecode is
// produced:
//
// fn f() -> (r:u16) { 0, r = g() }
// fn g() -> (p'1:u16, p'0:u16) { ... }
//
// Conversly, consider the opposite:
//
// fn f() -> (r:u32) { r = g() as u32 }
// fn g() -> (p:u16) { ... }
//
// Again, splitting to a maximum of u16 registers, then the following bytecode
// is produced:
//
// fn f() -> (r'1:u16, r'0) { r'0 = g(); r'1 = 0 }
// fn g() -> (p:u16) { ... }
func retAlignment[W word.Word[W]](local []RegisterId, remote []descriptor.Register[W], alloc split.Allocator[W],
) ([]RegisterId, []Bytecode[W]) {
	var (
		context []Bytecode[W]
		m, n    = len(local), len(remote)
		zero    W
	)
	//
	if m < n {
		var zreg = alloc.ZeroRegister()
		// Less locals than remotes.  In this case, pad locals with zero
		// register.
		for i := m; i < n; i++ {
			local = append(local, zreg)
		}
	} else if m > n {
		// More locals than remotes.  In this case, surplus locals are assigned
		// zero.
		for _, s := range local[n:] {
			context = append(context, bytecode.LoadConst(s, zero))
		}
		// discard surplus
		local = local[:n]
	}
	//
	return local, context
}

// Align the arguments (resp. returns) for a function call.  This means ensuring
// that the number of arguments (resp. returns) matches the number of parameters
// (resp. return values).  This can have different sizes as a result of casting.
func alignArgsReturns[W word.Word[W]](
	limbsMap descriptor.LimbsMap[W],
	alloc split.Allocator[W],
	locals []RegisterId,
	remotes []descriptor.Register[W],
	splitter func([]RegisterId, []descriptor.Register[W], split.Allocator[W]) ([]RegisterId, []Bytecode[W]),
) (boundary []RegisterId, extras []Bytecode[W]) {
	//
	var regWidth = limbsMap.Field().RegisterWidth
	//
	for i, local := range locals {
		var (
			// Local limbs, least-significant first.
			ithLocals = limbsMap.LimbIds(local)
			// Number of limbs the corresponding remote register splits into.
			ithRemotes = descriptor.SplitIntoLimbs(regWidth, remotes[i])
			//
			limbs, extra = splitter(ithLocals, ithRemotes, alloc)
		)
		// Present the retained limbs in declaration (most-significant-first) order.
		boundary = append(boundary, array.Reverse(limbs)...)
		// Include any require bytecodes
		extras = append(extras, extra...)
	}
	//
	return boundary, extras
}

// join sandwiches an instruction between its preceding and following bytecodes.
func join[W word.Word[W]](pre []Bytecode[W], insn Bytecode[W], post []Bytecode[W]) []Bytecode[W] {
	return append(append(pre, insn), post...)
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
