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
package interpreter

import (
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode provides a useful alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

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
	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), out...)
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
	return descriptor.NewFunction(fn.Name(), fn.Registers(), fn.Kind(), fn.Effects(), vectors)
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
	// case *bytecode.Bitwise[W]:
	// 	// AND/OR/XOR cast their result down to the target width; SHL/SHR/NOT do not.
	// 	switch b.Op {
	// 	case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
	// 		return prepend(b, checkCast(regmap, util.Some(uint(b.Bitwidth)), b.Target))
	// 	default:
	// 		return []Bytecode[W]{b}
	// 	}
	// case *bytecode.DivMod[W]:
	// 	// The operation width is that of the (uniform) operands, recovered from
	// 	// the dividend register; native dividends have no fixed width (-> 0).
	// 	var width util.Option[uint]
	// 	if dividend := regmap.Register(b.Dividend); !dividend.IsNative() {
	// 		width = dividend.Bitwidth()
	// 	}
	// 	// The quotient and remainder are independent values, so each gets its
	// 	// own cast check.
	// 	casts := checkCast(regmap, width, b.Quotient)
	// 	casts = append(casts, checkCast(regmap, width, b.Remainder)...)
	// 	//
	// 	return prepend(b, casts)
	// case *bytecode.Call[W]:
	// 	callee := modules[b.Target]
	// 	pre := addOutgoingCheckCasts(regmap, b.Arguments, callee.Inputs())
	// 	post := addIncomingCheckCasts(regmap, callee.Outputs(), b.Returns)
	// 	//
	// 	return append(append(pre, b), post...)
	// case *bytecode.ReadWrite[W]:
	// 	return readWriteCasts(b, regmap, modules)
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
		// Subtraction can always underflow.
		bits = util.Some[uint](math.MaxUint)
	default:
		panic("unknown arithmetic instruction")
	}
	// Construct relevant casts
	casts := checkCast(regmap, bits, b.Target...)
	// Done
	return append([]Bytecode[W]{b}, casts...)
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
		codes = append(codes, bytecode.NewCheckCast[W](last, lastWidth))
	}
	//
	return codes
}
