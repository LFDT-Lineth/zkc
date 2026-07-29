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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
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
func FlattenCalls[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = flattenCallsFunction[W](fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func flattenCallsFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		// Decide, for each call / RAM access in this vector, which operands must be
		// snapshotted.  This needs the whole vector body (to know which registers
		// are written at or after each site), which the Map closure cannot see one
		// bytecode at a time.
		callSnapshot := flattenableArgs[W](vec.Bytecodes)
		rwSnapshot := flattenableRwOperands[W](vec.Bytecodes)
		//
		nvecs[i] = vec.Map(func(idx uint, ith Bytecode[W]) []Bytecode[W] {
			switch b := ith.(type) {
			case *bytecode.Call[W]:
				return flattenCall[W](b, callSnapshot[idx], alloc)
			case *bytecode.ReadWrite[W]:
				return flattenReadWrite[W](b, rwSnapshot[idx], alloc)
			}
			//
			return []Bytecode[W]{ith}
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), nvecs)
}

// flattenableArgs returns, for each call in the vector, the set of argument
// positions whose register is written at or after the call.  Starting the scan
// at the call itself captures both the call's own returns and any register
// rewritten by a later bytecode.
func flattenableArgs[W word.Word[W]](codes []Bytecode[W]) map[uint][]bool {
	snapshot := make(map[uint][]bool)
	//
	for i, code := range codes {
		call, ok := code.(*bytecode.Call[W])
		if !ok {
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
		args := make([]bool, len(call.Arguments))
		//
		for j, arg := range call.Arguments {
			args[j] = written[arg]
		}
		//
		snapshot[uint(i)] = args
	}
	//
	return snapshot
}

// flattenCall expands a call, prefixing it with a snapshot ("tmp = arg") for
// each flagged argument and rewriting the call to read those temporaries.
func flattenCall[W word.Word[W]](call *bytecode.Call[W], snapshot []bool,
	registers split.Allocator[W]) []Bytecode[W] {
	var (
		args  = slices.Clone(call.Arguments)
		insns []Bytecode[W]
	)
	//
	for i, arg := range args {
		if snapshot[i] {
			tmp := registers.Allocate("", registers.Register(arg).Bitwidth())
			insns = append(insns, bytecode.Assign[W](tmp, arg))
			args[i] = tmp
		}
	}
	// Append the (possibly rewritten) call, preserving its flags.
	return append(insns, &bytecode.Call[W]{
		Target:    call.Target,
		Arguments: args,
		Returns:   call.Returns,
	})
}

// rwSnapshot flags, per operand, which of a RAM access's lookup operands must be
// snapshotted because their register is (re)written at or after the access in
// the same vector.
type rwSnapshot struct {
	address, data, stamp []bool
}

// flattenableRwOperands returns, for each read-write memory access in the vector,
// the operand positions whose register is written at or after the access.  The
// caller->RAM lookup reads an access's address / value / stamp columns at the
// access's row, so an operand rewritten in that same row (e.g. a loop counter
// used as the address, or the working stamp incremented in place) would be read
// post-write and mismatch the RAM table.  Snapshotting fixes this, exactly as for
// call arguments.  A read's data registers are its outputs (defined by the
// access itself, and legitimately read post-write), so they are never flagged.
func flattenableRwOperands[W word.Word[W]](codes []Bytecode[W]) map[uint]rwSnapshot {
	snapshot := make(map[uint]rwSnapshot)
	//
	for i, code := range codes {
		rw, ok := code.(*bytecode.ReadWrite[W])
		if !ok || len(rw.Stamp) == 0 {
			continue
		}
		// Registers written from this access onwards.
		written := make(map[bytecode.RegisterId]bool)
		//
		for _, later := range codes[i:] {
			for _, r := range later.Definitions() {
				written[r] = true
			}
		}
		//
		snapshot[uint(i)] = rwSnapshot{
			address: flagWritten(rw.Address, written),
			data:    flagWritten(rw.Data, written),
			stamp:   flagWritten(rw.Stamp, written),
		}
	}
	//
	return snapshot
}

// flagWritten reports, per register, whether it appears in the written set.
func flagWritten(regs []bytecode.RegisterId, written map[bytecode.RegisterId]bool) []bool {
	flags := make([]bool, len(regs))
	//
	for i, r := range regs {
		flags[i] = written[r]
	}
	//
	return flags
}

// flattenReadWrite expands a RAM access, prefixing it with a snapshot
// ("tmp = operand") for each flagged address / stamp operand (and, for a write,
// value operand), and rewriting the access to read those temporaries.  A read's
// data registers are its outputs and are left untouched.
func flattenReadWrite[W word.Word[W]](rw *bytecode.ReadWrite[W], snapshot rwSnapshot,
	registers split.Allocator[W]) []Bytecode[W] {
	//
	var (
		insns   []Bytecode[W]
		address = snapshotOperands[W](rw.Address, snapshot.address, &insns, registers)
		stamp   = snapshotOperands[W](rw.Stamp, snapshot.stamp, &insns, registers)
		data    = slices.Clone(rw.Data)
	)
	// A write's data are inputs (the value written) and may need snapshotting; a
	// read's data are outputs (the value read) and must not be touched.
	if rw.Write {
		data = snapshotOperands[W](rw.Data, snapshot.data, &insns, registers)
	}
	//
	return append(insns, &bytecode.ReadWrite[W]{
		Write:   rw.Write,
		Id:      rw.Id,
		Address: address,
		Data:    data,
		Stamp:   stamp,
	})
}

// snapshotOperands returns a copy of operands in which each flagged register is
// replaced by a fresh temporary, appending the corresponding "tmp = operand"
// snapshot instruction to insns.
func snapshotOperands[W word.Word[W]](operands []bytecode.RegisterId, flags []bool,
	insns *[]Bytecode[W], registers split.Allocator[W]) []bytecode.RegisterId {
	//
	out := slices.Clone(operands)
	// flags may be shorter than (or absent for) operands of an access that needs
	// no snapshotting (e.g. a stamp-less ROM/WOM read); treat missing as false.
	for i, r := range out {
		if i < len(flags) && flags[i] {
			tmp := registers.Allocate("", registers.Register(r).Bitwidth())
			*insns = append(*insns, bytecode.Assign[W](tmp, r))
			out[i] = tmp
		}
	}
	//
	return out
}
