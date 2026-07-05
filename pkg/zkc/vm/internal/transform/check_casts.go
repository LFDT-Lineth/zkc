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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// InsertCheckCasts inserts the width-check (CHECKCAST) bytecodes required by a
// bytecode program.  Codegen emits "core" operations without casts; this pass
// adds, for each operation, the cast checks needed when a result (or a value
// crossing a module boundary) is written to a narrower register -- mirroring the
// width checks the slow word machine performs implicitly on every register
// write.  It is applied per function via bytecode.Vector.Map, which rebuilds the
// vector and rewrites any branch (skip) offsets to account for the inserted
// bytecodes.  Memories have no body and are returned unchanged.
//
// References to other modules (a call's callee, a memory access's memory) are
// resolved against the program's module signatures, so this pass must run on a
// complete program.
func InsertCheckCasts[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		modules = program.Modules()
		out     = make([]descriptor.Module[W], len(modules))
	)
	//
	for i, m := range modules {
		if fn, ok := m.(*descriptor.Function[W]); ok {
			out[i] = insertFunctionCasts(fn, modules)
		} else {
			out[i] = m
		}
	}
	//
	return descriptor.NewProgram(program.Field(), out...)
}

// insertFunctionCasts rewrites a single function's vectors, inserting cast checks
// around each operation that needs them.
func insertFunctionCasts[W word.Word[W]](fn *descriptor.Function[W], modules []descriptor.Module[W],
) *descriptor.Function[W] {
	var vectors = make([]bytecode.Vector[W], len(fn.Vectors()))
	//
	for i, vec := range fn.Vectors() {
		vectors[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return castPacket(b, fn, modules)
		})
	}
	//
	return descriptor.NewFunction(fn.Name(), fn.Registers(), fn.IsNative(), vectors)
}

// castPacket returns the given bytecode wrapped with the cast checks it requires.
// Operations that never need a cast (subtraction, field arithmetic, concat,
// control flow, ...) return just the operation itself.  The cast positions match
// the slow word machine: arithmetic / bitwise / div-rem cast their result after
// the operation; calls cast arguments before and returns after; memory writes
// cast the data before the write; memory reads cast the data after.
func castPacket[W word.Word[W]](b Bytecode[W], regmap descriptor.RegisterMap[W], modules []descriptor.Module[W],
) []Bytecode[W] {
	//
	switch b := b.(type) {
	case *bytecode.Arith[W]:
		return arithCasts(b, regmap)
	case *bytecode.Bitwise:
		// AND/OR/XOR cast their result down to the target width; SHL/SHR/NOT do not.
		switch b.Op {
		case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
			return prepend(b, checkCast(regmap, util.Some(uint(b.Bitwidth)), b.Target))
		default:
			return []Bytecode[W]{b}
		}
	case *bytecode.DivRem:
		// The operation width is that of the (uniform) operands, recovered from
		// the dividend register; native dividends have no fixed width (-> 0).
		var width util.Option[uint]
		if dividend := regmap.Register(b.Dividend); !dividend.IsNative() {
			width = dividend.Bitwidth()
		}
		//
		return prepend(b, checkCast(regmap, width, b.Target))
	case *bytecode.Call:
		callee := modules[b.Target]
		pre := addOutgoingCheckCasts(regmap, b.Arguments, callee.Inputs())
		post := addIncomingCheckCasts(regmap, callee.Outputs(), b.Returns)
		//
		return append(append(pre, b), post...)
	case *bytecode.ReadWrite:
		return readWriteCasts(b, regmap, modules)
	default:
		return []Bytecode[W]{b}
	}
}

// arithCasts emits the cast check for an integer add / multiply (subtraction
// never overflows its target and needs none).
func arithCasts[W word.Word[W]](b *bytecode.Arith[W], regmap descriptor.RegisterMap[W]) []Bytecode[W] {
	var (
		bits    util.Option[uint]
		sources = b.Source
	)
	//
	switch b.Op {
	case bytecode.OP_ADD:
		bits = descriptor.CalculateAddBitwidth(sources, b.Constant, regmap)
	case bytecode.OP_MUL:
		bits = descriptor.CalculateMulBitwidth(sources, b.Constant, regmap)
	case bytecode.OP_SUB:
		bits = descriptor.CalculateSubBitwidth(sources, b.Constant, regmap)
	default:
		panic("unknown arithmetic instruction")
	}
	//
	return prepend(b, checkCast(regmap, bits, b.Target...))
}

// readWriteCasts emits the cast checks for a memory read (incoming, after the
// read) or write (outgoing, before the write), resolving the memory's data
// registers from the program's module signatures.
func readWriteCasts[W word.Word[W]](b *bytecode.ReadWrite, regmap descriptor.RegisterMap[W],
	modules []descriptor.Module[W]) []Bytecode[W] {
	var (
		mem  = modules[b.Id].(*descriptor.Memory[W])
		data = mem.Outputs()
	)
	//
	if b.Write {
		// Write: cast outgoing data registers before the write.
		return append(addOutgoingCheckCasts(regmap, b.Data, data), b)
	}
	// Read: cast incoming data registers after the read.
	return prepend(b, addIncomingCheckCasts(regmap, data, b.Data))
}

// prepend returns the operation followed by its (possibly empty) trailing cast
// checks.
func prepend[W word.Word[W]](op Bytecode[W], casts []Bytecode[W]) []Bytecode[W] {
	return append([]Bytecode[W]{op}, casts...)
}

// AddIncomingCheckCasts emits a CHECKCAST for every target register which is
// narrower than the corresponding source register, where sources are values
// arriving in this frame from another module (e.g. a memory's data registers,
// or a callee's return registers).  This mirrors the width check the slow
// machine performs on every register write (frame.Store / frameCopyFrom).  The
// targets are resolved against the given register map (the frame's own
// registers).
func addIncomingCheckCasts[W word.Word[W]](regmap descriptor.RegisterMap[W], sources []descriptor.Register[W],
	targets []RegisterId) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, target := range targets {
		var (
			src = sources[i]
			dst = regmap.Register(target)
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Bitwidth().Unwrap() > dst.Bitwidth().Unwrap()) {
			width := util.Cast[uint16](dst.Bitwidth().Unwrap())
			codes = append(codes, bytecode.NewCheckCast(target, width))
		}
	}
	//
	return codes
}

// AddOutgoingCheckCasts emits a CHECKCAST for every source register in this
// frame which is wider than the register receiving its value in another module
// (e.g. a memory's data registers, or a callee's parameter registers).  This
// mirrors the width check the slow machine performs on memory writes
// (executeMemWrite) and call arguments (frameCopyTo).  The sources are resolved
// against the given register map (the frame's own registers).
func addOutgoingCheckCasts[W word.Word[W]](regmap descriptor.RegisterMap[W], sources []RegisterId,
	targets []descriptor.Register[W]) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, source := range sources {
		var (
			src = regmap.Register(source)
			dst = targets[i]
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Bitwidth().Unwrap() > dst.Bitwidth().Unwrap()) {
			width := util.Cast[uint16](dst.Bitwidth().Unwrap())
			codes = append(codes, bytecode.NewCheckCast(source, width))
		}
	}
	//
	return codes
}

// checkCast adds a checkcast instruction if the bitwidth of the right-hand side
// does not fit within the target register(s), resolving widths against the
// given register map.
func checkCast[W word.Word[W]](rmap descriptor.RegisterMap[W], rhs util.Option[uint], lhs ...RegisterId) []Bytecode[W] {
	var (
		last = len(lhs) - 1
		// Determine bitwidth of lhs
		bitwidth = descriptor.BitwidthOf(rmap, lhs...)
		codes    []Bytecode[W]
	)
	// Add case if either: (i) the rhs has no specific bitwidth; or (2) the
	// bitwidth of the rhs overflows the lhs.
	if bitwidth.HasValue() && (rhs.IsEmpty() || bitwidth.Unwrap() < rhs.Unwrap()) {
		var (
			last      = lhs[last]
			lastWidth = util.Cast[uint16](rmap.Register(last).Bitwidth().Unwrap())
		)
		// yes
		codes = append(codes, bytecode.NewCheckCast(last, lastWidth))
	}
	//
	return codes
}
