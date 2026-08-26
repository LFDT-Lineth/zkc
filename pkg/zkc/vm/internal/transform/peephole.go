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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Rewrite is a purely local ("peephole") bytecode rewrite.  Given a single
// bytecode, it returns the sequence of zero or more bytecodes which replace it.
// Returning the bytecode unchanged (i.e. a single-element sequence holding its
// argument) signals that the rewrite does not apply.
//
// The allocator provides both the enclosing function's register mapping (e.g.
// for determining the width of an operand) and the means to allocate any
// temporary registers the replacement sequence requires.
type Rewrite[W word.Word[W]] func(Bytecode[W], Allocator[W]) []Bytecode[W]

// ApplyRewrite applies a given rewrite to every bytecode of every function in the
// program, returning the updated program.  Non-function modules (i.e. memories)
// contain no bytecodes and are passed through unchanged.
//
// Since the rewrite sees each bytecode in isolation, it cannot observe (nor
// preserve) anything about the surrounding vector.  Two consequences follow.
// Firstly, a rewrite may freely change the number of bytecodes in a vector:
// branch offsets are recalculated by bytecode.Vector.Map, including for
// bytecodes whose targets lie outside the replaced sequence.  Secondly, a
// rewrite must be sound in every context in which the bytecode can appear ---
// in particular, a rewrite which deletes a bytecode (i.e. returns nothing)
// must be sure it is a genuine no-op, since deleting a branch target (e.g. the
// bytecode following a skip) can invalidate the enclosing vector and, hence,
// panic during remapping.
func ApplyRewrite[W word.Word[W]](rewrite Rewrite[W], program descriptor.Program[W]) descriptor.Program[W] {
	var modules = slices.Clone(program.Modules())
	//
	for i, mod := range modules {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			modules[i] = peepholeFunction(rewrite, fn)
		}
	}
	//
	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), modules...)
}

// peepholeFunction applies a given rewrite to every bytecode in every vector of
// a single function, threading an allocator through so that any temporaries
// introduced by the rewrite are retained in the resulting function.
func peepholeFunction[W word.Word[W]](rewrite Rewrite[W], fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = make([]BytecodeVector[W], len(fn.Vectors()))
		alloc   = split.NewAllocator(fn)
	)
	//
	for i, vec := range fn.Vectors() {
		vectors[i] = vec.Map(func(_ uint, code Bytecode[W]) []Bytecode[W] {
			return rewrite(code, alloc)
		})
	}
	//
	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), vectors)
}
