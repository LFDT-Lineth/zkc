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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FlattenCalls snapshots, into a fresh temporary, each call argument whose
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
func FlattenCalls[W word.Word[W]](modules []Module) []Module {
	out := append([]Module{}, modules...)

	for i, mod := range out {
		if fn, ok := mod.(*WordFunction); ok {
			out[i] = flattenCallsFunction[W](fn)
		}
	}

	return out
}

func flattenCallsFunction[W word.Word[W]](fn *WordFunction) *WordFunction {
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
		alloc = register.NewAllocator[int](fn.RegisterMap())
	)

	for i, insn := range code {
		// Decide, for each call in this vector, which arguments must be
		// snapshotted.  This needs the whole vector body (to know which registers
		// are written at or after each call), which the Map closure cannot see one
		// instruction at a time.
		snapshot := flattenableArgs(insn.Codes)
		//
		ncode[i] = insn.Map(func(idx uint, ith WordInstruction) []WordInstruction {
			if call, ok := ith.(*instruction.Call); ok {
				return flattenCall[W](call, snapshot[idx], alloc)
			}
			//
			return []WordInstruction{ith}
		})
	}

	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), ncode)
}

// flattenableArgs returns, for each call in the vector, the set of argument
// positions whose register is written at or after the call.  Starting the scan
// at the call itself captures both the call's own returns and any register
// rewritten by a later instruction;
func flattenableArgs(codes []WordInstruction) map[uint][]bool {
	snapshot := make(map[uint][]bool)
	//
	for i, code := range codes {
		call, ok := code.(*instruction.Call)
		if !ok {
			continue
		}
		// Collect the registers written from this call onwards.
		written := make(map[uint]bool)
		//
		for _, later := range codes[i:] {
			for _, r := range later.Definitions() {
				written[r.Unwrap()] = true
			}
		}
		// Flag each argument coinciding with such a write.
		args := make([]bool, len(call.Arguments))
		//
		for j, arg := range call.Arguments {
			args[j] = written[arg.Unwrap()]
		}
		//
		snapshot[uint(i)] = args
	}
	//
	return snapshot
}

// flattenCall expands a call, prefixing it with a snapshot ("tmp = arg") for
// each flagged argument and rewriting the call to read those temporaries.
func flattenCall[W word.Word[W]](call *instruction.Call, snapshot []bool,
	registers RegisterAllocator) []WordInstruction {
	var (
		args  = append([]register.Id{}, call.Arguments...)
		insns []WordInstruction
	)
	//
	for i, arg := range args {
		if snapshot[i] {
			tmp := registers.Allocate("", registers.Register(arg).WidthOrNative())
			insns = append(insns, instruction.UintAssignV[W](register.NewVector(tmp), arg))
			args[i] = tmp
		}
	}
	// Append the (possibly rewritten) call.
	return append(insns, instruction.NewCall(call.Id, args, call.Returns))
}
