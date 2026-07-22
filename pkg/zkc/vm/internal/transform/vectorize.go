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
	"math"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Vectorize a given program by merging as many bytecodes as possible into each
// (vector) bytecode.  The strategy is greedy: walking each function, we
// repeatedly try to absorb the target of a goto back into the vector containing
// that goto, effectively pulling a successor vector up into its predecessor
// until no further merging is legal.  For example, given two bytecodes "x = y"
// and "a = b", neither writes a register the other touches and so they can be
// combined into the single vector "x=y ; a=b" whose constituents execute "in
// parallel".
//
// The principal obstacle to merging is the appearance of *register conflicts*
// between bytecodes — that is, data hazards in the classical sense from computer
// architecture.  All three textbook hazards (RAW, WAW, WAR) arise here, where
// "earlier" and "later" refer to the position of two bytecodes within the same
// vector:
//
//   - RAW (Read-After-Write).  A later bytecode reads a register that an earlier
//     bytecode writes.  This is the "true" data dependency.  Within a vector it
//     is normally resolved by *register forwarding*: the later bytecode simply
//     observes the freshly-written value.  However, when the upstream write is
//     *conditional* — i.e. it occurs on some intra-vector control-flow paths but
//     not others — the value to forward is not well-defined and the merge is
//     rejected.  This is reported as a "conflicting read".
//
//   - WAW (Write-After-Write).  Two bytecodes in the same vector both write the
//     same register.  The resulting register value would be ambiguous, so the
//     merge is rejected.  This is reported as a "conflicting write", and is the
//     most common form of register conflict in practice.
//
//   - WAR (Write-After-Read).  A later bytecode writes a register that an earlier
//     bytecode reads.  This is *not* a hazard in this setting, because forwarding
//     flows strictly forward: the earlier read always observes the pre-vector
//     value, while the later write only takes effect once the whole vector
//     completes.  No check is required, and no merge is blocked on this account.
//
// Register forwarding is the mechanism that makes RAW dependencies tractable
// inside a vector.  When one bytecode writes a register, every subsequent
// bytecode in the same vector observes the freshly-written value rather than the
// value held at the start of the vector.
//
// In addition to data hazards, two further conditions block a merge:
//
//   - Other validation failures.  The merged vector must continue to satisfy
//     every well-formedness invariant for vectors.
//
//   - Back-edges.  A goto whose target would bring control back into the vector
//     being built (a loop) is left alone; otherwise the inliner would unfold it
//     indefinitely.
func Vectorize[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	modules := slices.Clone(program.Modules())
	//
	for i, m := range modules {
		if fn, ok := m.(*descriptor.Function[W]); ok {
			modules[i] = vectorizeFunction(fn)
		}
	}
	//
	return descriptor.NewProgram(program.Field(), modules...)
}

// vectorizeFunction applies the per-function vectorisation pass, returning a new
// function whose code is the merged-and-pruned result.
func vectorizeFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		original = fn.Vectors()
		n        = uint(len(original))
	)
	//
	if n == 0 {
		return fn
	}
	// Wrap every top-level vector and append a fall-through Jmp(pc+1) to those
	// that don't already terminate.  This makes inter-vector control-flow explicit
	// so that lastJump can drive the merge loop.
	prepared := prepareCode[W](original)
	//
	var (
		insns    = make([]BytecodeVector[W], n)
		visited  = make([]bool, n)
		worklist stack.Stack[uint]
	)

	visited[0] = true

	worklist.Push(0)
	// Vectorize bytecodes as much as possible.
	for !worklist.IsEmpty() {
		pc := worklist.Pop()
		insns[pc] = vectorizeInstruction[W](pc, prepared)
		markJumpTargets[W](insns[pc], visited, &worklist)
	}
	// Remove unreachable vectors and rebind jump targets.
	insns = pruneUnreachableInstructions[W](insns)
	//
	return descriptor.NewFunction(fn.Name(), fn.Registers(), fn.Kind(), insns)
}

// prepareCode appends a fall-through Jmp(pc+1) to any vector that does not
// already terminate (i.e. whose last code is not a Jmp / Ret / Fail).  Vectors
// are built afresh so that subsequent merge work cannot accidentally mutate the
// input function.
func prepareCode[W word.Word[W]](code []BytecodeVector[W]) []BytecodeVector[W] {
	var (
		n        = uint(len(code))
		prepared = make([]BytecodeVector[W], n)
	)
	//
	for pc, vec := range code {
		// Clone vector
		codes := slices.Clone(vec.Bytecodes)
		// Append fall-through Jmp if the vector doesn't already terminate.
		if !endsInTerminator[W](codes) && uint(pc)+1 < n {
			codes = append(codes, bytecode.Jump[W](bytecode.Address(uint(pc)+1)))
		}
		//
		prepared[pc] = bytecode.NewVector(codes...)
	}
	//
	return prepared
}

// endsInTerminator reports whether all paths through codes terminate without
// falling off the end: the last code must be a Jmp/Ret/Fail, AND no
// Skip/SkipIf/Switch anywhere in the vector has a skip target past the end (which
// would create a second exit path not visible from the last bytecode).
func endsInTerminator[W word.Word[W]](codes []Bytecode[W]) bool {
	n := uint(len(codes))
	//
	if n == 0 {
		return false
	}
	//
	switch codes[n-1].(type) {
	case *bytecode.Jmp[W], *bytecode.Ret[W], *bytecode.Fail[W]:
	default:
		return false
	}
	// Verify no skip-bytecode can reach past the end of the vector.
	for i, code := range codes {
		switch code := code.(type) {
		case *bytecode.Skip[W]:
			if uint(i)+uint(code.Skip)+1 >= n {
				return false
			}
		case *bytecode.SkipIf[W]:
			if uint(i)+uint(code.Skip)+1 >= n {
				return false
			}
		case *bytecode.Switch[W]:
			for _, dc := range code.Cases {
				if uint(i)+uint(dc.Skip)+1 >= n {
					return false
				}
			}
		}
	}
	//
	return true
}

// vectorizeInstruction greedily absorbs the targets of jumps in the vector at pc
// until no further merging is legal.
func vectorizeInstruction[W word.Word[W]](pc uint, code []BytecodeVector[W]) BytecodeVector[W] {
	var (
		vec     = code[pc]
		changed = true
		// externs maps a foreign vector's pc to the offset within the current
		// vector at which its codes were inlined, or MaxUint if it has not (yet)
		// been absorbed.
		externs []uint = array.FrontPad[uint](nil, uint(len(code)), math.MaxUint)
	)
	// Keep merging until a complete pass produces no change.
	for changed {
		changed = false
		//
		index, ok := lastJump[W](vec.Bytecodes, uint(len(vec.Bytecodes)))
		// Try the right-most non-conflicting jump.
		for ok {
			jmpTarget := uint(vec.Bytecodes[index].(*bytecode.Jmp[W]).Target)
			// Skip back-edges into ourselves and absorbs that would shift
			// backwards (which would otherwise unfold a loop).
			if offset := externs[jmpTarget]; offset > index && jmpTarget != pc {
				var (
					target = code[jmpTarget]
					nvec   BytecodeVector[W]
				)
				//
				if offset != math.MaxUint {
					// Already absorbed earlier in the same vector — replace the
					// Jmp with a Skip to the previously inlined codes.
					nvec = replaceJump[W](vec, index, offset)
				} else {
					// Splice the target's codes into the vector in place of the Jmp.
					nvec = inlineJump[W](vec, index, target.Bytecodes)
				}
				// Accept the merge only if it stays valid.
				if validateConflicts[W](nvec) == nil {
					if offset == math.MaxUint {
						updateMicroMap(externs, index, jmpTarget, uint(len(target.Bytecodes)))
					}
					//
					vec = nvec
					changed = true
					//
					break
				}
			}
			// Try the next jump leftward.
			index, ok = lastJump[W](vec.Bytecodes, index)
		}
	}
	//
	return vec
}

// lastJump returns the index of the right-most Jmp within codes[:n], or false if
// none exists.
func lastJump[W word.Word[W]](codes []Bytecode[W], n uint) (uint, bool) {
	for i := n; i > 0; {
		i--
		//
		if _, ok := codes[i].(*bytecode.Jmp[W]); ok {
			return i, true
		}
	}
	//
	return 0, false
}

// markJumpTargets pushes every reachable Jmp target in the vectorised vector onto
// the worklist for later processing.
func markJumpTargets[W word.Word[W]](vec BytecodeVector[W], visited []bool, worklist *stack.Stack[uint]) {
	index, found := lastJump[W](vec.Bytecodes, uint(len(vec.Bytecodes)))
	for found {
		target := uint(vec.Bytecodes[index].(*bytecode.Jmp[W]).Target)
		//
		if !visited[target] {
			visited[target] = true
			worklist.Push(target)
		}
		//
		index, found = lastJump[W](vec.Bytecodes, index)
	}
}

// updateMicroMap records that the codes belonging to target have just been
// inlined at offset within the current vector, then shifts other recorded offsets
// to account for the size delta.
func updateMicroMap(externs []uint, index uint, target uint, ncodes uint) {
	externs[target] = index
	//
	for i := range externs {
		if externs[i] != math.MaxUint && externs[i] > index {
			externs[i] += ncodes - 1
		}
	}
}

// replaceJump returns a copy of vec with the Jmp at jmpIndex replaced by a Skip
// targeting the supplied offset within the same vector.
func replaceJump[W word.Word[W]](vec BytecodeVector[W], jmpIndex uint, offset uint) BytecodeVector[W] {
	if offset <= jmpIndex {
		// Should be unreachable: the externs guard requires offset > jmpIndex.
		panic("cannot skip backwards")
	}
	//
	codes := slices.Clone(vec.Bytecodes)
	codes[jmpIndex] = bytecode.NewSkip[W](uint16(offset - jmpIndex - 1))
	//
	return bytecode.NewVector(codes...)
}

// inlineJump returns a new vector formed by replacing the Jmp at jmpIndex with
// the codes from the target vector.  Skip / SkipIf / Switch offsets in the
// surrounding codes are recomputed so they continue to identify the same
// successor after the splice.
func inlineJump[W word.Word[W]](vec BytecodeVector[W], jmpIndex uint, targetCodes []Bytecode[W]) BytecodeVector[W] {
	var (
		codes   = vec.Bytecodes
		mapping = make([]uint, len(codes))
		npc     int
	)
	// Compute the mapping from old code offsets to new code offsets.  The Jmp
	// itself disappears and is replaced by len(targetCodes) entries.
	for cc := uint(0); cc < uint(len(codes)); cc, npc = cc+1, npc+1 {
		mapping[cc] = uint(npc)
		//
		if cc == jmpIndex {
			// NOTE: -1 because the Jmp is overwritten by the first target code.
			npc += len(targetCodes) - 1
		}
	}
	//
	ncodes := make([]Bytecode[W], npc)
	//
	for cc, npc := uint(0), uint(0); cc < uint(len(codes)); cc++ {
		code := codes[cc]
		//
		switch c := code.(type) {
		case *bytecode.Jmp[W]:
			if cc == jmpIndex {
				// Splice in the target's codes (shared references — the originals
				// are not mutated downstream).
				for _, tc := range targetCodes {
					ncodes[npc] = tc
					npc++
				}
				//
				continue
			}
		case *bytecode.Skip[W]:
			target := mapping[cc+1+uint(c.Skip)]
			code = bytecode.NewSkip[W](uint16(target - npc - 1))
		case *bytecode.SkipIf[W]:
			target := mapping[cc+1+uint(c.Skip)]
			code = &bytecode.SkipIf[W]{
				Op:    c.Op,
				Left:  c.Left,
				Right: c.Right,
				Skip:  uint16(target - npc - 1),
			}
		case *bytecode.Switch[W]:
			// Each dispatch case skips to a code within this vector, so its offset
			// must be recomputed after the splice (exactly as for Skip / SkipIf above).
			ncases := make([]bytecode.SwitchCase[W], len(c.Cases))
			//
			for k, dc := range c.Cases {
				target := mapping[cc+1+uint(dc.Skip)]
				ncases[k] = bytecode.SwitchCase[W]{Value: dc.Value, Skip: uint16(target - npc - 1)}
			}
			//
			code = &bytecode.Switch[W]{Source: c.Source, Cases: ncases}
		}
		//
		ncodes[npc] = code
		npc++
	}
	//
	return bytecode.NewVector(ncodes...)
}

// pruneUnreachableInstructions removes any vectors never reached by the worklist
// (left empty) and rebinds the surviving Jmp targets so they reference the new
// compacted positions.  Jmps are replaced rather than mutated so that any shared
// references inside the vector graph are not disturbed.
func pruneUnreachableInstructions[W word.Word[W]](insns []BytecodeVector[W]) []BytecodeVector[W] {
	var (
		kept    []BytecodeVector[W]
		mapping = make([]uint, len(insns))
	)
	// Compact the slice, recording where each surviving vector lands.
	for i, insn := range insns {
		if len(insn.Bytecodes) == 0 {
			continue
		}
		//
		mapping[i] = uint(len(kept))
		kept = append(kept, insn)
	}
	// Rebind every Jmp.Target to its new position.
	for _, vec := range kept {
		for i, code := range vec.Bytecodes {
			if jmp, ok := code.(*bytecode.Jmp[W]); ok {
				vec.Bytecodes[i] = bytecode.Jump[W](bytecode.Address(mapping[jmp.Target]))
			}
		}
	}
	//
	return kept
}

// validateConflicts reports the first read/write hazard found within vec, or nil
// if none.  This considers only register conflicts (RAW with conditional writes,
// WAW), since vectorisation rejects merges only on those grounds — never on field
// bandwidth.
func validateConflicts[W word.Word[W]](vec BytecodeVector[W]) error {
	var (
		nCodes = uint(len(vec.Bytecodes))
		writes = vec.WriteMap()
	)
	//
	for i := range nCodes {
		var (
			ithState = writes.StateOf(i)
			ith      = vec.Bytecodes[i]
		)
		// RAW: reading a register whose upstream write is conditional inside the
		// vector — no single value to forward from.
		for _, r := range ith.Uses() {
			if rid := register.NewId(uint(r)); ithState.MaybeAssigned(rid) && !ithState.DefinitelyAssigned(rid) {
				return fmt.Errorf("conflicting read on register %d", r)
			}
		}
		// WAW: writing a register that may already have been written by an earlier
		// code in the vector.
		for _, r := range ith.Definitions() {
			if rid := register.NewId(uint(r)); ithState.MaybeAssigned(rid) {
				return fmt.Errorf("conflicting write on register %d", r)
			}
		}
	}
	//
	return nil
}
