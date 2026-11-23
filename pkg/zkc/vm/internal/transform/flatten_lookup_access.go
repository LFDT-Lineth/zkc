// Copyright Consensys Incorporated
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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
)

// FlattenLookupAccess snapshots, into a fresh temporary, each call (or memory access) argument whose
// register may be written at or after the call on some path through it within the same vector.
//
// The lookup gluing a call to its callee reads the argument and return columns
// at the call's row.  If an argument register is also written elsewhere in the
// call's vector, that column holds the final value rather than the argument
// actually passed, so we snapshot the argument into a fresh temporary first and
// let the call read that instead.  Two situations require this:
//
//   - the argument is also a return of the call (e.g. "x = f(x)"); or
//   - the argument coincides with a register written by a later instruction on
//     some path through the call, e.g. the destination of the enclosing
//     assignment in "x = f(x) + 1" (lowered to "$t = f(x); x = $t + 1").
//
// A write lying on a path mutually exclusive with the call does not require a
// snapshot, since the row which executes that write does not execute the call.
//
// The target of a cast ("check r:uN") is treated likewise, since a cast is
// lowered into a (conditional) range lookup reading that target's column (see
// flattenCasts).
//
// We could avoid the temporary, but it would imply a lookup with a row shift,
// which makes the prover's life harder.  This pass is therefore only meaningful
// when generating arithmetic constraints (it is not required by the vm) and
// must run after vectorisation, so the writes which would corrupt the argument
// column share the call's vector.
func FlattenLookupAccess[W Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*Function[W]); ok {
			out[i] = flattenLookupAccessFunction(fn)
		}
	}

	return descriptor.NewProgram(program.IsTracing(), program.Field(), program.MaxStaticHeight(), out...)
}

func flattenLookupAccessFunction[W Word[W]](fn *Function[W]) *Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		// Snapshot the target of each cast which may be written later on the
		// same row.  This comes first, since it redirects subsequent reads of
		// that target (e.g. by a call) to the snapshot.
		vec = flattenCasts(vec, alloc)
		// Decide, for each call in this vector, which arguments must be
		// snapshotted.  This needs the whole vector body (to know which registers
		// are written on paths through each call), which the Map closure cannot
		// see one bytecode at a time.
		snapshot := flattenableArgs(vec, alloc)
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

// flattenableArgs returns, for each lookup in the vector requiring at least one
// snapshot, the set of argument positions to snapshot.  An argument is flagged
// when its register may be written at or after the lookup on some path through
// it.  Starting at the lookup itself captures its own returns (e.g. "x = f(x)")
// whilst, for later bytecodes, the reach map discards those lying on a mutually
// exclusive path, such as the else branch in:
//
//	skip_if c != 0 2; read y = data[x]; ret; x = 1; ret
//
// Here, the write to x can never follow the read on the same row, so the
// column for x always holds the address actually read.  Zero-width registers
// carry no data and, hence, are never flagged.
func flattenableArgs[W Word[W]](vec BytecodeVector[W], registers RegisterMap[W]) map[uint][]bool {
	var (
		codes    = vec.Bytecodes
		reaches  = vec.ReachMap()
		snapshot = make(map[uint][]bool)
	)
	//
	for i, code := range codes {
		uses := lookupUses(code)
		if uses == nil {
			continue
		}
		//
		var (
			args    = make([]bool, len(uses))
			flagged = false
		)
		//
		for k, use := range uses {
			if isWrittenFrom(codes, reaches, uint(i), use, registers) {
				args[k], flagged = true, true
			}
		}
		//
		if flagged {
			snapshot[uint(i)] = args
		}
	}
	//
	return snapshot
}

// isWrittenFrom determines whether the given register may be written by the
// bytecode at offset "from", or by any later bytecode on some path through it.
func isWrittenFrom[W Word[W]](codes []Bytecode[W], reaches dfa.Result[dfa.Reaches], from uint,
	reg bytecode.RegisterId, registers RegisterMap[W]) bool {
	//
	for j := from; j < uint(len(codes)); j++ {
		if isReachableFrom(reaches, from, j) && isDefinedIn(reg, codes[j], registers) {
			return true
		}
	}
	//
	return false
}

// isReachableFrom determines whether the bytecode at offset "to" lies on some
// path through the bytecode at offset "from" (which trivially holds for the
// bytecode itself).
func isReachableFrom(reaches dfa.Result[dfa.Reaches], from, to uint) bool {
	return to == from || reaches.StateOf(to).ReachableFrom(from)
}

// isDefinedIn determines whether the given register is written by the given
// bytecode.  Zero-width registers carry no data and, hence, are never
// considered written.
func isDefinedIn[W Word[W]](reg bytecode.RegisterId, code Bytecode[W],
	registers RegisterMap[W]) bool {
	//
	return !registers.Register(reg).IsZeroWidth() && slices.Contains(code.Definitions(), reg)
}

// flattenLookupAccess expands a call, prefixing it with a snapshot ("tmp = arg") for
// each flagged argument and rewriting the call to read those temporaries.
func flattenLookupAccess[W Word[W]](code Bytecode[W], snapshot []bool,
	registers split.Allocator[W]) []Bytecode[W] {
	insns, uses := snapshotUses(lookupUses(code), snapshot, registers)
	//
	switch c := code.(type) {
	case *bytecode.Call[W]:
		// Append the (possibly rewritten) call, preserving its flags.
		return append(insns, bytecode.NeverCallFun[W](c.Target, uses, c.Returns, c.Never))
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

func snapshotUses[W Word[W]](uses []bytecode.RegisterId, snapshot []bool,
	registers split.Allocator[W]) ([]Bytecode[W], []bytecode.RegisterId) {
	var (
		ids   = slices.Clone(uses)
		insns []Bytecode[W]
	)
	//
	for i, use := range ids {
		if snapshot[i] {
			bw := registers.Register(use).Bitwidth()
			tmp := registers.Allocate("fla", bw)
			// A native register has no width, so the snapshot is a field copy.
			insns = append(insns, bytecode.Copy[W](tmp, use, bw))

			ids[i] = tmp
		}
	}
	//
	return insns, ids
}

// flattenCasts snapshots the target of each cast ("check r:uN") which may be
// written at or after the cast on some path through it within the given vector.
// For example, in "check x:u8; x = f(x)" the column for x holds the value
// returned (rather than the value cast) on the row executing the cast.  Hence,
// this is rewritten as "tmp = x; check tmp:u8; x = f(tmp)".
//
// Observe the call now reads the snapshot.  This is necessary since a cast also
// narrows the bitwidth of its target for subsequent bytecodes (e.g. a call
// passing it to a narrower parameter), which now applies to the snapshot
// instead.  As such, subsequent reads of the target are redirected to the
// snapshot wherever both hold the same value (see flattenCast).
func flattenCasts[W Word[W]](vec BytecodeVector[W], registers split.Allocator[W]) BytecodeVector[W] {
	for {
		index, ok := flattenableCast(vec, registers)
		//
		if !ok {
			return vec
		}
		// Observe this cannot be flagged again, since its target is now a
		// snapshot (which is never written again).
		vec = flattenCast(vec, index, registers)
	}
}

// flattenableCast returns the offset of the first cast in the vector whose
// target may be written at or after the cast on some path through it (if any).
// As for a call argument, a write lying on a mutually exclusive path does not
// count, and neither does a zero-width register (see flattenableArgs).
func flattenableCast[W Word[W]](vec BytecodeVector[W], registers RegisterMap[W]) (uint, bool) {
	var (
		codes   = vec.Bytecodes
		hasCast = slices.ContainsFunc(codes, func(code Bytecode[W]) bool {
			_, ok := code.(*bytecode.CheckCast[W])
			return ok
		})
	)
	// Avoid constructing the reach map unless it is actually needed.
	if !hasCast {
		return 0, false
	}
	//
	var reaches = vec.ReachMap()
	//
	for i, code := range codes {
		if cast, ok := code.(*bytecode.CheckCast[W]); ok &&
			isWrittenFrom(codes, reaches, uint(i), cast.Target, registers) {
			return uint(i), true
		}
	}
	//
	return 0, false
}

// flattenCast replaces the cast at the given offset with a snapshot of its
// target ("tmp = r") followed by a cast of that snapshot ("check tmp:uN").
// Subsequent reads of the target are then redirected to the snapshot, provided:
// (1) the snapshot has definitely been taken at that point; and (2) the target
// has not been written (in this vector) at that point.  Together, these ensure
// the snapshot holds exactly the value which would otherwise have been read.
//
// A read forming part of a multi-register vector is left alone, since such
// vectors must remain consecutive.  This is safe as, again, both registers hold
// the same value and such reads place no (narrowed) bitwidth expectation on the
// registers they read.
func flattenCast[W Word[W]](vec BytecodeVector[W], index uint, registers split.Allocator[W]) BytecodeVector[W] {
	var (
		cast   = vec.Bytecodes[index].(*bytecode.CheckCast[W])
		target = cast.Target
		bw     = registers.Register(target).Bitwidth()
		tmp    = registers.Allocate("fla", bw)
	)
	// Cast the snapshot in place of the target.
	vec = vec.Map(func(i uint, ith Bytecode[W]) []Bytecode[W] {
		if i == index {
			return []Bytecode[W]{bytecode.Copy[W](tmp, target, bw), bytecode.NewCheckCast[W](tmp, cast.Bitwidth)}
		}
		//
		return []Bytecode[W]{ith}
	})
	//
	var (
		codes  = slices.Clone(vec.Bytecodes)
		writes = vec.WriteMap()
		defs   = make([]bytecode.RegisterId, len(registers.Registers()))
	)
	//
	for i := range defs {
		defs[i] = bytecode.RegisterId(i)
	}
	//
	uses := slices.Clone(defs)
	uses[target] = tmp
	// Redirect reads of the target to the snapshot.
	for i, code := range codes {
		var st = writes.StateOf(uint(i))
		//
		if st.DefinitelyAssigned(tmp) && !st.MaybeAssigned(target) && isRedirectable(code, target) {
			codes[i] = substituteOperands(code, defs, uses)
		}
	}
	//
	return bytecode.NewVector(codes...)
}

// isRedirectable determines whether the given bytecode reads the given
// register, and does not refer to it as part of a multi-register vector.
func isRedirectable[W Word[W]](code Bytecode[W], reg bytecode.RegisterId) bool {
	if !slices.Contains(code.Uses(), reg) {
		return false
	}
	//
	for _, rv := range extractRegisterVectors(code) {
		if rv.Len > 1 && rv.Base <= reg && uint(reg) < uint(rv.Base)+uint(rv.Len) {
			return false
		}
	}
	//
	return true
}

func lookupUses[W Word[W]](code Bytecode[W]) []bytecode.RegisterId {
	switch c := code.(type) {
	case *bytecode.Call[W]:
		return c.Arguments
	case *bytecode.ReadWrite[W]:
		return c.Uses()
	default:
		return nil
	}
}
