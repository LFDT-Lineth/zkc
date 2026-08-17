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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FlattenLookupAccess snapshots, into a fresh temporary, each call (or memory access) argument whose
// register is also written at or after the call within the same vector.
//
// The lookup gluing a call to its callee reads the argument and return columns
// at the call's row.  If an argument register is also written elsewhere in the
// call's vector, that column holds the final value rather than the argument
// actually passed, so we snapshot the argument into a fresh temporary first and
// let the call read that instead.  Two situations require this:
//
//   - the argument is also a return of the call (e.g. "x = f(x)"); or
//   - the argument coincides with a register written by a later instruction in
//     the vector, e.g. the destination of the enclosing assignment in
//     "x = f(x) + 1" (lowered to "$t = f(x); x = $t + 1").
//
// We could avoid the temporary, but it would imply a lookup with a row shift,
// which makes the prover's life harder.  This pass is therefore only meaningful
// when generating arithmetic constraints (it is not required by the vm) and
// must run after vectorisation, so the writes which would corrupt the argument
// column share the call's vector.
func FlattenLookupAccess[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = flattenLookupAccessFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), out...)
}

func flattenLookupAccessFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		// Decide, for each call in this vector, which arguments must be
		// snapshotted.  This needs the whole vector body (to know which registers
		// are written at or after each call), which the Map closure cannot see one
		// bytecode at a time.
		snapshot := flattenableArgs(vec.Bytecodes)
		//
		nvecs[i] = vec.Map(func(idx uint, ith Bytecode[W]) []Bytecode[W] {
			if flags, ok := snapshot[idx]; ok {
				return flattenLookupAccess(ith, flags, alloc)
			}
			//
			return []Bytecode[W]{ith}
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

// flattenableArgs returns, for each call in the vector, the set of argument
// positions whose register is written at or after the call.  Starting the scan
// at the call itself captures both the call's own returns and any register
// rewritten by a later bytecode.
func flattenableArgs[W word.Word[W]](codes []Bytecode[W]) map[uint][]bool {
	snapshot := make(map[uint][]bool)
	//
	for i, code := range codes {
		uses := lookupUses(code)
		if uses == nil {
			continue
		}
		// Collect the registers written from this call onwards.
		written := make(map[bytecode.RegisterId]bool)
		//
		for _, later := range codes[i:] {
			for _, r := range later.Definitions() {
				written[r] = true
			}
		}
		// Flag each argument coinciding with such a write.
		args := make([]bool, len(uses))
		//
		for j, use := range uses {
			args[j] = written[use]
		}
		//
		snapshot[uint(i)] = args
	}
	//
	return snapshot
}

// flattenLookupAccess expands a call, prefixing it with a snapshot ("tmp = arg") for
// each flagged argument and rewriting the call to read those temporaries.
func flattenLookupAccess[W word.Word[W]](code Bytecode[W], snapshot []bool,
	registers split.Allocator[W]) []Bytecode[W] {
	insns, uses := snapshotUses(lookupUses(code), snapshot, registers)
	//
	switch c := code.(type) {
	case *bytecode.Call[W]:
		// Append the (possibly rewritten) call, preserving its flags.
		return append(insns, &bytecode.Call[W]{
			Target:    c.Target,
			Arguments: uses,
			Returns:   c.Returns,
		})
	case *bytecode.ReadWrite[W]:
		// Uses() order is Address ++ (Data if write) ++ Stamp, so reslice the
		// (possibly snapshotted) operands back into their fields.
		address := uses[:len(c.Address)]
		rest := uses[len(c.Address):]
		//
		if c.Write {
			return append(insns, bytecode.NewMemWrite[W](c.Id, address, rest[:len(c.Data)], rest[len(c.Data):]))
		}
		// A read's data registers are outputs (absent from uses) and stay
		// untouched.
		return append(insns, bytecode.NewMemRead[W](c.Id, address, c.Data, rest))
	default:
		panic(fmt.Sprintf("unexpected bytecode (%T)", code))
	}
}

func snapshotUses[W word.Word[W]](uses []bytecode.RegisterId, snapshot []bool,
	registers split.Allocator[W]) ([]Bytecode[W], []bytecode.RegisterId) {
	var (
		ids   = slices.Clone(uses)
		insns []Bytecode[W]
	)
	//
	for i, use := range ids {
		if snapshot[i] {
			tmp := registers.Allocate("", registers.Register(use).Bitwidth())
			insns = append(insns, bytecode.Assign[W](tmp, use))
			ids[i] = tmp
		}
	}
	//
	return insns, ids
}

func lookupUses[W word.Word[W]](code Bytecode[W]) []bytecode.RegisterId {
	switch c := code.(type) {
	case *bytecode.Call[W]:
		return c.Arguments
	case *bytecode.ReadWrite[W]:
		return c.Uses()
	default:
		return nil
	}
}
