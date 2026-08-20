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
func SplitRegisters[W word.Word[W]](target field.Config, program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		//
		out = make([]descriptor.Module[W], len(mods))
	)
	//
	for i, ith := range mods {
		// construct limbs map for this module
		mapping := descriptor.NewLimbsMap(target, program.Field(), ith)
		// split the module
		out[i] = splitModule(mapping, mods, ith)
	}
	//
	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), out...)
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
		return descriptor.NewMemory(m.Name(), m.Kind(), m.TimestampWidth(), registers, splitStaticContents(mapping, m))
	case m.IsWriteOnly(), m.IsReadOnly(), m.IsReadWrite():
		// Non-static memories (write-once, read-only, and read-write RAM —
		// including paged) carry no constant contents; only their registers are
		// split.  The associated read/write bytecodes have their address and data
		// registers split separately (see splitRegisters for ReadWrite).
		return descriptor.NewMemory(m.Name(), m.Kind(), m.TimestampWidth(), registers, nil)
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
		limbsMap = mapping.LimbsRegisterMap()
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
			// Check for native field
			if descriptor.BitwidthOf(limbsMap, mapping.LimbIds(id)...).HasValue() {
				out = append(out, splitCell(contents[row+j], mapping.LimbIds(id), limbsMap)...)
			} else {
				// No splitting necessary
				out = append(out, contents[row+j])
			}
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
		alloc = split.NewAllocator(mapping.LimbsRegisterMap()).
			EnforceRegisterWidth(mapping.RegisterWidth())
		code = splitBytecodeVector(mapping, mods, alloc, m.Vectors())
	)
	//
	return descriptor.NewFunction(m.Name(), alloc.Registers(), m.Kind(), m.Effects(), code)
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
		case *bytecode.Call[W]:
			return splitCall(limbsMap, alloc, mods, c)
		case *bytecode.Cat[W]:
			return split.Concat(limbsMap, alloc, c)
		case *bytecode.Debug[W]:
			return []Bytecode[W]{&bytecode.Debug[W]{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)}}
		case *bytecode.Fail[W]:
			return []Bytecode[W]{&bytecode.Fail[W]{Chunks: c.Chunks, Sources: splitRegisterVectors(limbsMap, c.Sources)}}
		case *bytecode.Intrinsic[W]:
			// Each operand (argument / return) is split into the limbs of its
			// constituent registers, preserving the per-operand grouping so the
			// hint's executor can still reconstruct each value.  Constant
			// operands stay a single (unsplit) value, like arithmetic
			// immediates (see split.Operand).
			return []Bytecode[W]{&bytecode.Intrinsic[W]{
				Op:      c.Op,
				Targets: splitRegisterVectors(limbsMap, c.Targets),
				Sources: splitOperandVectors(limbsMap, c.Sources),
			}}
		case *bytecode.Jmp[W]:
			return []Bytecode[W]{c}
		case *bytecode.ReadWrite[W]:
			if c.Write {
				return splitWrite(limbsMap, alloc, mods, c)
			}
			//
			return splitRead(limbsMap, alloc, mods, c)
		case *bytecode.Ret[W]:
			return []Bytecode[W]{c}
		case *bytecode.Skip[W]:
			return []Bytecode[W]{c}
		case *bytecode.SkipIf[W]:
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
		case *bytecode.Bitwise[W]:
			switch c.Op {
			case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR, bytecode.OP_NOT:
				return split.Bitwise(limbsMap, alloc, c)
			case bytecode.OP_SHL, bytecode.OP_SHR:
				return split.Shift(limbsMap, c)
			default:
				panic(fmt.Sprintf("unsupported bitwise operation (%d)", c.Op))
			}

		// =======================================================
		// Misc bytecodes
		// =======================================================
		case *bytecode.DivRem[W]:
			// NOTE: only relevant for splitting fast mode (i.e. non-lowered)
			// bytecode.
			return split.DivRem(limbsMap, c)
		case *bytecode.FieldArith[W]:
			return []Bytecode[W]{splitFieldArith(limbsMap, c)}
		case *bytecode.UintToField[W]:
			// The native target stays a single register; the uint source splits
			// into limbs.
			return []Bytecode[W]{&bytecode.UintToField[W]{
				Target: split.ApplyLimbsMap(limbsMap, c.Target)[0],
				Source: split.ApplyLimbsMapReversed(limbsMap, c.Source...)}}
		case *bytecode.FieldToUint[W]:
			// The uint target splits into limbs; the native source stays a single
			// register.
			return []Bytecode[W]{&bytecode.FieldToUint[W]{
				Target: split.ApplyLimbsMapReversed(limbsMap, c.Target...),
				Source: split.ApplyLimbsMap(limbsMap, c.Source)[0]}}
		case *bytecode.Switch[W]:
			return split.Switch(limbsMap, c)
		case *bytecode.Dispatch[W]:
			return split.Dispatch(limbsMap, c)
		case *bytecode.CheckCast[W]:
			panic("CheckCast is not supposed to happen before splitting")
		default:
			panic(fmt.Sprintf("unsupported bytecode (%T)", c))
		}
	})
}

// splitFieldArith remaps the native target and sources of a field arithmetic
// instruction into the split register layout. Native registers remain single
// limbs, but their identifiers can move when earlier integer registers expand.
func splitFieldArith[W word.Word[W]](limbsMap descriptor.LimbsMap[W], c *bytecode.FieldArith[W]) Bytecode[W] {
	var (
		targets = split.ApplyLimbsMap(limbsMap, c.Target)
		sources = split.ApplyLimbsMap(limbsMap, c.Sources...)
	)
	// Field arithmetic is defined only over native registers, each of which
	// remains exactly one limb after register splitting.
	util.Assert(len(targets) == 1, "field arithmetic target has limbs")
	util.Assert(len(sources) == len(c.Sources), "field arithmetic source has limbs")
	//
	return bytecode.NewFieldArith(c.Op, targets[0], sources, c.Constant)
}

// splitCall splits the registers referenced by a call.  Unlike a purely local
// instruction, a call transfers values across a module boundary: its arguments
// flow out into the callee's inputs and its returns flow back in from the
// callee's outputs.  Each argument / return is therefore reconciled against the
// width of the corresponding callee parameter / result (see splitBoundary),
// which may introduce additional width-check / zero-extension bytecodes either
// side of the call.
func splitCall[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.Call[W]) []Bytecode[W] {
	//
	var (
		callee = mods[c.Target]
		// Arguments are outgoing (this frame -> callee inputs); returns are
		// incoming (callee outputs -> this frame).
		args, pre1, post1 = alignArgsReturns(limbsMap, alloc, c.Arguments, callee.Inputs(), argAlignment)
		rets, pre2, post2 = alignArgsReturns(limbsMap, alloc, c.Returns, callee.Outputs(), retAlignment)
		// Combine all together
		pre, post = append(pre1, pre2...), append(post1, post2...)
	)
	//
	return join(pre, bytecode.CallFun[W](c.Target, args, rets), post)
}

func splitRead[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.ReadWrite[W]) []Bytecode[W] {
	//
	var (
		mem = mods[c.Id]
		// Address registers correspond to the memory's inputs, data registers to
		// its outputs.
		addr, pre1, post1 = alignArgsReturns(limbsMap, alloc, c.Address, mem.Inputs(), argAlignment)
		data, pre2, post2 = alignArgsReturns(limbsMap, alloc, c.Data, mem.Outputs(), retAlignment)
		// The timestamp operand splits like any other caller register.
		stamp = split.ApplyLimbsMap(limbsMap, c.Stamp...)
		// Combine all together
		pre, post = append(pre1, pre2...), append(post1, post2...)
	)
	//
	return join(pre, bytecode.NewMemRead[W](c.Id, addr, data, stamp), post)
}

func splitWrite[W word.Word[W]](limbsMap descriptor.LimbsMap[W], alloc split.Allocator[W], mods []descriptor.Module[W],
	c *bytecode.ReadWrite[W]) []Bytecode[W] {
	//
	var (
		mem = mods[c.Id]
		// Address registers correspond to the memory's inputs, data registers to
		// its outputs.
		addr, pre1, post1 = alignArgsReturns(limbsMap, alloc, c.Address, mem.Inputs(), argAlignment)
		data, pre2, post2 = alignArgsReturns(limbsMap, alloc, c.Data, mem.Outputs(), retAlignment)
		// The timestamp operand splits like any other caller register.
		stamp = split.ApplyLimbsMap(limbsMap, c.Stamp...)
		// Combine all together
		pre, post = append(pre1, pre2...), append(post1, post2...)
	)
	//
	return join(pre, bytecode.NewMemWrite[W](c.Id, addr, data, stamp), post)
}

func splitSkipIf[W word.Word[W]](limbsMap descriptor.LimbsMap[W], c *bytecode.SkipIf[W]) Bytecode[W] {
	// Both operands are frame-local registers of equal width, so each is simply
	// mapped onto its limbs.
	var (
		left  = split.ApplyLimbsMap(limbsMap, c.Left.Registers()...)
		right bytecode.Operand[W]
	)
	// Split right-hand side according to what it is.
	if c.Right.IsRegisterVector() {
		limbs := split.ApplyLimbsMap(limbsMap, c.Right.AsRegisters()...)
		right = bytecode.NewRegisterOperand[W](limbs...)
	} else {
		// Split the constant
		constants := descriptor.SplitConstantReversed(c.Right.AsConstant(), limbsMap.RegisterWidth())
		right = bytecode.NewConstantOperand(constants...)
	}
	// Construct vectored form of skip_if
	return bytecode.NewSkipIf(c.Op, c.Skip,
		bytecode.NewRegisterVector(left...), right,
	)
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
) (nlocals []RegisterId, pre, post []Bytecode[W]) {
	//
	var (
		m, n = len(local), len(remote)
	)
	//
	if m < n && descriptor.HasNativeRegisterId(local, alloc) {
		panic("field-to-uint argument must be materialised before splitting")
	} else if m > n && descriptor.HasNativeRegister(remote) {
		panic("uint-to-field argument must be materialised before splitting")
	} else if m < n {
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
			pre = append(pre, bytecode.NewCheckCast[W](s, 0))
		}
		// discard surplus
		local = local[:n]
	}
	//
	return local, pre, post
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
) (nlocal []RegisterId, pre, post []Bytecode[W]) {
	var (
		m, n = len(local), len(remote)
		zero W
	)
	//
	if m < n && descriptor.HasNativeRegisterId(local, alloc) {
		panic("uint-to-field return must be materialised before splitting")
	} else if m > n && descriptor.HasNativeRegister(remote) {
		panic("field-to-uint return must be materialised before splitting")
	} else if m < n {
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
			post = append(post, bytecode.LoadConst(s, zero))
		}
		// discard surplus
		local = local[:n]
	}
	//
	return local, pre, post
}

type splitterFunc[W word.Word[W]] = func(local []RegisterId, remote []descriptor.Register[W], alloc split.Allocator[W],
) (nlocal []RegisterId, pre, post []Bytecode[W])

// Align the arguments (resp. returns) for a function call.  This means ensuring
// that the number of arguments (resp. returns) matches the number of parameters
// (resp. return values).  This can have different sizes as a result of casting.
func alignArgsReturns[W word.Word[W]](
	limbsMap descriptor.LimbsMap[W],
	alloc split.Allocator[W],
	locals []RegisterId,
	remotes []descriptor.Register[W],
	splitter splitterFunc[W],
) (boundary []RegisterId, pre, post []Bytecode[W]) {
	//
	var regWidth = limbsMap.RegisterWidth()
	//
	for i, local := range locals {
		var (
			// Local limbs, least-significant first.
			ithLocals = limbsMap.LimbIds(local)
			// Number of limbs the corresponding remote register splits into.
			ithRemotes = descriptor.SplitIntoLimbs(regWidth, remotes[i])
			//
			limbs, preExtra, postExtra = splitter(ithLocals, ithRemotes, alloc)
		)
		// Present the retained limbs in declaration (most-significant-first) order.
		boundary = append(boundary, array.Reverse(limbs)...)
		// Include any require bytecodes
		pre = append(pre, preExtra...)
		post = append(post, postExtra...)
	}
	//
	return boundary, pre, post
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

// splitOperandVectors splits each operand (e.g. an intrinsic argument) into
// limbs: register vectors into the limbs of their constituent registers,
// whilst constants stay a single (unsplit) value (see split.Operand).
func splitOperandVectors[W word.Word[W]](limbsMap descriptor.LimbsMap[W],
	ops []bytecode.Operand[W]) []bytecode.Operand[W] {
	//
	var nops = make([]bytecode.Operand[W], len(ops))
	//
	for i, o := range ops {
		nops[i], _ = split.Operand(limbsMap, o)
	}
	//
	return nops
}
