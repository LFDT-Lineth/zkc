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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
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
	return descriptor.NewProgram(out...)
}

// insertFunctionCasts rewrites a single function's vectors, inserting cast checks
// around each operation that needs them.
func insertFunctionCasts[W word.Word[W]](fn *descriptor.Function[W], modules []descriptor.Module[W],
) *descriptor.Function[W] {
	var (
		// Build a register map over this function's (schema) registers, as needed
		// by the cast helpers to resolve register widths.
		regmap  = register.ArrayMap(trace.ModuleName{Multiplier: 1}, decompileRegisters(fn.Registers())...)
		vectors = make([]bytecode.Vector[W], len(fn.Vectors()))
	)
	//
	for i, vec := range fn.Vectors() {
		vectors[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return castPacket(b, regmap, modules)
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
func castPacket[W word.Word[W]](b Bytecode[W], regmap register.Map, modules []descriptor.Module[W]) []Bytecode[W] {
	switch b := b.(type) {
	case *bytecode.Arith[W]:
		return arithCasts(b, regmap)
	case *bytecode.Bitwise:
		// AND/OR/XOR cast their result down to the target width; SHL/SHR/NOT do not.
		switch b.Op {
		case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
			return prepend[W](b, BitwidthCheckCast[W](regmap, toId(b.Target), uint(b.Bitwidth)))
		default:
			return []Bytecode[W]{b}
		}
	case *bytecode.DivRem:
		// The operation width is that of the (uniform) operands, recovered from
		// the dividend register; native dividends have no fixed width (-> 0).
		var width uint
		if dividend := regmap.Register(toId(b.Dividend)); !dividend.IsNative() {
			width = dividend.Width()
		}
		//
		return prepend[W](b, BitwidthCheckCast[W](regmap, toId(b.Target), width))
	case *bytecode.Call:
		callee := modules[b.Target]
		pre := AddOutgoingCheckCasts[W](regmap, toIds(b.Arguments), callee.Inputs())
		post := AddIncomingCheckCasts[W](regmap, callee.Outputs(), toIds(b.Returns))
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
func arithCasts[W word.Word[W]](b *bytecode.Arith[W], regmap register.Map) []Bytecode[W] {
	var acc func(*big.Int, *big.Int)
	//
	switch b.Op {
	case bytecode.OP_ADD:
		acc = bitwidthAdd
	case bytecode.OP_MUL:
		acc = bitwidthMul
	default:
		// Subtraction (and any other arithmetic) needs no cast.
		return []Bytecode[W]{b}
	}
	//
	var (
		target = register.NewVector(toIds(b.Target)...)
		bits   = CalculateBitwidth(b.Constant, toIds(b.Source), regmap, acc)
	)
	//
	return prepend[W](b, AddCheckCast[W](regmap, target, bits))
}

// readWriteCasts emits the cast checks for a memory read (incoming, after the
// read) or write (outgoing, before the write), resolving the memory's data
// registers from the program's module signatures.
func readWriteCasts[W word.Word[W]](b *bytecode.ReadWrite, regmap register.Map, modules []descriptor.Module[W],
) []Bytecode[W] {
	var (
		mem  = modules[b.Id].(*descriptor.Memory[W])
		data = mem.Outputs()
	)
	//
	if b.Write {
		// Write: cast outgoing data registers before the write.
		return append(AddOutgoingCheckCasts[W](regmap, toIds(b.Data), data), b)
	}
	// Read: cast incoming data registers after the read.
	return prepend[W](b, AddIncomingCheckCasts[W](regmap, data, toIds(b.Data)))
}

// prepend returns the operation followed by its (possibly empty) trailing cast
// checks.
func prepend[W word.Word[W]](op Bytecode[W], casts []Bytecode[W]) []Bytecode[W] {
	return append([]Bytecode[W]{op}, casts...)
}

// bitwidthAdd / bitwidthMul are the accumulators passed to CalculateBitwidth:
// addition folds source widths additively (INT_ADD), multiplication
// multiplicatively (INT_MUL).
func bitwidthAdd(lhs, rhs *big.Int) { lhs.Add(lhs, rhs) }
func bitwidthMul(lhs, rhs *big.Int) { lhs.Mul(lhs, rhs) }
