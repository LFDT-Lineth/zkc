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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// stampWidth is the default bit-width of a timestamp register.
//
// TODO: source this per-memory from the (not-yet-implemented) stamp-width
// syntax "memory data(u16; addr:u16) -> ..." (issue #1807) rather than using a
// single global default.
const stampWidth uint = 32

// ThreadTimestamps threads a per-memory timestamp through every function which
// (transitively) accesses a read-write memory.  For each such function and each
// read-write memory M it accesses, it:
//
//   - adds an input register "M$stamp" and an output register "M$stamp'" (the
//     stamp flowing in and back out); these are placed first in the inputs /
//     outputs.  The entry function "main" is special: it takes no parameters, so
//     it gets no stamp in/out and instead seeds each working stamp to zero.
//   - forwards the stamp across every call to another accessing function (by
//     prepending it to the call's arguments and returns);
//   - sets the stamp operand of each memory access to the current working stamp
//     and increments the working stamp once per access ("stamp = stamp + 1").
//
// This is required only for tracing and constraint generation (the run-time
// memory maintains its own clock), so it is applied on the constraint path only
// and, crucially, before vectorisation / register splitting so the wide stamp
// registers and their increments are split into limbs and range-checked like any
// other register.
func ThreadTimestamps[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		out  = slices.Clone(mods)
	)
	//
	for i, mod := range mods {
		fn, ok := mod.(*descriptor.Function[W])
		if !ok {
			continue
		}
		//
		effects := rwMemoryEffects(mods, fn)
		if len(effects) == 0 {
			continue
		}
		//
		out[i] = threadFunction(mods, fn, effects)
	}
	//
	return descriptor.NewProgram(program.Field(), out...)
}

// rwMemoryEffects returns the module ids of the read-write memories declared as
// effects of the given function, preserving declaration order.  Effects naming a
// read-only / write-once memory (which need no timestamp) are dropped.
func rwMemoryEffects[W word.Word[W]](mods []descriptor.Module[W],
	fn *descriptor.Function[W]) []descriptor.ModuleId {
	//
	var out []descriptor.ModuleId
	//
	for _, id := range fn.Effects() {
		if mem, ok := mods[id].(*descriptor.Memory[W]); ok && mem.IsReadWrite() {
			out = append(out, id)
		}
	}
	//
	return out
}

// threadFunction rewrites a single function to thread a timestamp for each of
// the given read-write memory effects.
func threadFunction[W word.Word[W]](mods []descriptor.Module[W], fn *descriptor.Function[W],
	effects []descriptor.ModuleId) *descriptor.Function[W] {
	//
	var (
		isMain   = fn.Name() == "main"
		oldRegs  = fn.Registers()
		ni       = fn.NumInputs()
		no       = fn.NumOutputs()
		k        = uint(len(effects))
		padding  W
		newRegs  []descriptor.Register[W]
		stampIn  = map[descriptor.ModuleId]bytecode.RegisterId{}
		stampOut = map[descriptor.ModuleId]bytecode.RegisterId{}
		cur      = map[descriptor.ModuleId]bytecode.RegisterId{}
	)
	// New register layout: [stamp-ins, old inputs, stamp-outs, old outputs, old
	// computed, working stamps].  main has no stamp in/out.
	if !isMain {
		for _, e := range effects {
			stampIn[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.INPUT_REGISTER,
				memName(mods, e)+"$stamp", util.Some(stampWidth), padding))
		}
	}
	//
	newRegs = append(newRegs, oldRegs[:ni]...)
	//
	if !isMain {
		for _, e := range effects {
			stampOut[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.OUTPUT_REGISTER,
				memName(mods, e)+"$stamp'", util.Some(stampWidth), padding))
			// The stamp-out return doubles as the working register: seeded from
			// the stamp-in on entry and incremented in place at each access, so it
			// already holds the final value at every return.
			cur[e] = stampOut[e]
		}
	}
	//
	newRegs = append(newRegs, oldRegs[ni:ni+no]...)
	newRegs = append(newRegs, oldRegs[ni+no:]...)
	// main has no stamp-in / stamp-out (it takes no parameters), so it threads
	// through a fresh computed working register per effect, seeded to zero.
	if isMain {
		for _, e := range effects {
			cur[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.COMPUTED_REGISTER,
				memName(mods, e)+"$ts", util.Some(stampWidth), padding))
		}
	}
	// Old->new register id remap.  Inserting k stamp inputs shifts old inputs by
	// k; inserting k stamp outputs shifts old outputs and computed by a further k.
	// main inserts none, so the map is the identity there.
	sub := make([]bytecode.RegisterId, len(oldRegs))
	//
	for x := range oldRegs {
		id := bytecode.RegisterId(x)
		//
		switch {
		case isMain:
			sub[x] = id
		case uint(x) < ni:
			sub[x] = id + bytecode.RegisterId(k)
		default:
			sub[x] = id + bytecode.RegisterId(2*k)
		}
	}
	// Rewrite the body: remap every register, then thread the stamp.
	var (
		one     W
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
	)
	//
	one = one.SetUint64(1)
	//
	for vi, vec := range vectors {
		nvecs[vi] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			insn = substituteRegisters[W](insn, sub)
			//
			var codes []Bytecode[W]
			//
			switch b := insn.(type) {
			case *bytecode.ReadWrite[W]:
				if c, ok := cur[b.Id]; ok {
					b.Stamp = []bytecode.RegisterId{c}
					codes = append(codes, b)
					// One increment per access, independent of the number of
					// address / data lanes the access carries.
					codes = append(codes, bytecode.AddConst[W](c, []bytecode.RegisterId{c}, one))
				} else {
					codes = append(codes, b)
				}
			case *bytecode.Call[W]:
				codes = append(codes, threadCall[W](mods, b, cur))
			case *bytecode.Jmp[W]:
				// The seed preamble (prepended below) shifts every body vector down
				// by one, so absolute jump targets move by one.
				codes = append(codes, bytecode.Jump[W](b.Target+1))
			default:
				// Everything else (including Ret) is unchanged: the stamp-out
				// register already holds the current value, so no return-copy is
				// needed.
				codes = append(codes, insn)
			}
			//
			return codes
		})
	}
	// Prepend a preamble vector seeding the working stamps.  Keeping the seed on
	// its own row ensures the stamp register is written at most once per row (the
	// seed and the per-access increments never share a vector).
	nvecs = append([]BytecodeVector[W]{
		bytecode.NewVector[W](seedStamps[W](effects, cur, stampIn, isMain)...),
	}, nvecs...)
	//
	return descriptor.NewFunction(fn.Name(), newRegs, fn.Kind(), nvecs)
}

// threadCall rewrites a call so that, for each read-write memory the callee
// accesses, the caller's current working stamp is passed in and the updated
// stamp received back.  Stamps are prepended (matching the callee's stamp-first
// signature).  A call to a non-accessing callee is returned unchanged.
func threadCall[W word.Word[W]](mods []descriptor.Module[W], call *bytecode.Call[W],
	cur map[descriptor.ModuleId]bytecode.RegisterId) Bytecode[W] {
	//
	callee, ok := mods[call.Target].(*descriptor.Function[W])
	if !ok {
		return call
	}
	//
	effects := rwMemoryEffects(mods, callee)
	if len(effects) == 0 {
		return call
	}
	//
	stamps := make([]bytecode.RegisterId, len(effects))
	//
	for i, e := range effects {
		c, ok := cur[e]
		if !ok {
			// Guaranteed by the type-checker: callee effects are a subset of the
			// caller's, so the caller always has a working stamp for e.
			panic(fmt.Sprintf("caller lacks working stamp for memory %d", e))
		}
		//
		stamps[i] = c
	}
	//
	args := append(slices.Clone(stamps), call.Arguments...)
	returns := append(slices.Clone(stamps), call.Returns...)
	//
	return bytecode.CallFun[W](call.Target, args, returns)
}

// seedStamps returns the instructions initialising each working stamp at the
// start of a function: a copy from the stamp-in parameter, or (for main) a load
// of one.
//
// main seeds the stamp at ONE so the first memory access carries timestamp 1
// (timestamp 0 is reserved for the initial state of an untouched cell).  The
// caller->RAM lookup reads the access's stamp operand, which FlattenCalls
// snapshots to a temporary before the in-place increment, so it reads the
// PRE-increment value: access k then carries stamp 1+k, matching the
// interpreter's clock (which ticks before each access, so access k sees clock
// 1+k).  The RAM consistency constraint TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 +
// TIMESTAMP_DELTA then holds for the first access to any address (TIMESTAMP_READ
// = 0, TIMESTAMP_DELTA >= 0).
func seedStamps[W word.Word[W]](effects []descriptor.ModuleId,
	cur, stampIn map[descriptor.ModuleId]bytecode.RegisterId, isMain bool) []Bytecode[W] {
	//
	var (
		codes []Bytecode[W]
		one   W
	)
	//
	one = one.SetUint64(1)
	//
	for _, e := range effects {
		if isMain {
			codes = append(codes, bytecode.LoadConst[W](cur[e], one))
		} else {
			codes = append(codes, bytecode.Concat[W](
				[]bytecode.RegisterId{cur[e]}, []bytecode.RegisterId{stampIn[e]}))
		}
	}
	//
	return codes
}

// memName returns the name of the module (memory) with the given id.
func memName[W word.Word[W]](mods []descriptor.Module[W], id descriptor.ModuleId) string {
	return mods[id].Name()
}
