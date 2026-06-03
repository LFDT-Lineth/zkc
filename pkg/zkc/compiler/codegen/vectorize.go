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
package codegen

import (
	"fmt"
	"math"
	"slices"

	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/stack"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// Vectorize a given function by merging as many instructions as possible into
// each (vector) instruction.  The strategy is greedy: walking the function,
// we repeatedly try to absorb the target of a goto back into the instruction
// containing that goto, effectively pulling a successor instruction up into
// its predecessor until no further merging is legal.  For example, given two
// instructions "x = y" and "a = b", neither writes a register the other
// touches and so they can be combined into the single vector instruction
// "x=y ; a=b" whose constituents execute "in parallel".
//
// The principal obstacle to merging is the appearance of *register conflicts*
// between instructions — that is, data hazards in the classical sense from
// computer architecture.  All three textbook hazards (RAW, WAW, WAR) arise
// here, where "earlier" and "later" refer to the position of two instructions
// within the same vector instruction:
//
//   - RAW (Read-After-Write).  A later instruction reads a register that an
//     earlier instruction writes.  This is the "true" data dependency.
//     Within a vector instruction it is normally resolved by *register
//     forwarding* (described below): the later instruction simply observes
//     the freshly-written value.  However, when the upstream write is
//     *conditional* — i.e. it occurs on some intra-instruction control-flow
//     paths but not others — the value to forward is not well-defined and
//     the merge is rejected.  This is reported as a "conflicting read".
//
//   - WAW (Write-After-Write).  Two instructions in the same vector
//     instruction both write the same register.  The resulting register
//     value would be ambiguous, so the merge is rejected.  This is reported
//     as a "conflicting write", and is the most common form of register
//     conflict in practice.
//
//   - WAR (Write-After-Read).  A later instruction writes a register that an
//     earlier instruction reads.  This is *not* a hazard in this setting,
//     because forwarding flows strictly forward: the earlier read always
//     observes the pre-instruction value, while the later write only takes
//     effect once the whole vector instruction completes.  No check is
//     required, and no merge is blocked on this account.
//
// Register forwarding is the mechanism that makes RAW dependencies tractable
// inside a vector instruction.  When one instruction writes a register, every
// subsequent instruction in the same vector observes the freshly-written
// value rather than the value held at the start of the vector instruction.
// Forwarding is precisely what makes vectorisation useful (a downstream
// instruction can immediately consume an upstream instruction's result) but
// it is also what gives rise to RAW conflicts: if the upstream write only
// happens on some intra-instruction paths there is no single well-defined
// source to forward from.
//
// In addition to data hazards, two further conditions block a merge:
//
//   - Other validation failures.  The merged instruction must continue to
//     satisfy every well-formedness invariant for vector instructions (e.g.
//     balance, no field overflow, allocated temporaries).
//
//   - Back-edges.  A goto whose target would bring control back into the
//     instruction being built (a loop) is left alone; otherwise the inliner
//     would unfold it indefinitely.
//
// NOTE: this stage is assumed to run after flattening has taken place and,
// hence, only needs to deal with unstructured control-flow (i.e. not
// block-level control flow).
func Vectorize(modules []vm.Module, _ source.Maps[any]) {
	for i, m := range modules {
		if fn, ok := m.(*Function); ok {
			modules[i] = vectorizeFunction(fn, modules)
		}
	}
}

// vectorizeFunction applies the per-function vectorisation pass, returning a
// new function whose code is the merged-and-pruned result.  This mirrors
// vectorizeFunction in pkg/asm/lower.go.
func vectorizeFunction(fn *Function, modules []vm.Module) *Function {
	var (
		original = fn.Code()
		n        = uint(len(original))
	)
	//
	if n == 0 {
		return fn
	}
	// Wrap every top-level instruction in a Vector and append a fall-through
	// Jmp(pc+1) to those that don't already terminate.  This is the
	// counterpart of macro→micro lowering in zkasm: it makes inter-instruction
	// control-flow explicit so that LastJump can drive the merge loop.
	prepared := prepareCode(original)
	// Build a system map for register-conflict reporting.
	mapping := instruction.NewSystemMap(fn.RegisterMap(), modules)
	//
	var (
		insns    = make([]VectorInstruction, n)
		visited  = make([]bool, n)
		worklist stack.Stack[uint]
	)

	visited[0] = true

	worklist.Push(0)
	// Vectorize instructions as much as possible.
	for !worklist.IsEmpty() {
		pc := worklist.Pop()
		insns[pc] = vectorizeInstruction(pc, prepared, mapping)
		markJumpTargets(insns[pc], visited, &worklist)
	}
	// Remove unreachable instructions and rebind jump targets.
	insns = pruneUnreachableInstructions(insns)
	//
	return vm.NewFunction(fn.Name(), fn.IsNative(), fn.Registers(), insns)
}

// prepareCode wraps every top-level instruction in a Vector and appends a
// fall-through Jmp(pc+1) to any vector that does not already terminate (i.e.
// whose last code is not a Jmp / Return / Fail).  Vectors are built afresh so
// that subsequent merge work cannot accidentally mutate the input function.
func prepareCode(code []VectorInstruction) []VectorInstruction {
	var (
		n        = uint(len(code))
		prepared = make([]VectorInstruction, n)
	)
	//
	for pc, insn := range code {
		// Clone vector instruction
		codes := slices.Clone(insn.Codes)
		// Append fall-through Jmp if the vector doesn't already terminate.
		if !endsInTerminator(codes) && uint(pc)+1 < n {
			codes = append(codes, instruction.NewJump(uint(pc)+1))
		}
		//
		prepared[pc] = VectorInstruction{Codes: codes}
	}
	//
	return prepared
}

// endsInTerminator reports whether all micro-paths through codes terminate
// without falling off the end: the last code must be a Jmp/Return/Fail, AND
// no Skip/SkipIf anywhere in the vector has a skip target past the end (which
// would create a second exit path not visible from the last instruction).
func endsInTerminator(codes []Instruction) bool {
	n := uint(len(codes))
	//
	if n == 0 {
		return false
	}
	//
	switch codes[n-1].OpCode() {
	case opcode.JUMP, opcode.RETURN, opcode.FAIL:
	default:
		return false
	}
	// Verify no skip-instruction can reach past the end of the vector.
	for i, code := range codes {
		switch code := code.(type) {
		case *instruction.Skip:
			if uint(i)+code.Skip+1 >= n {
				return false
			}
		case *instruction.SkipIf:
			if uint(i)+code.Skip+1 >= n {
				return false
			}
		case *instruction.SkipIfConst[vm.Uint]:
			if uint(i)+code.Skip+1 >= n {
				return false
			}
		}
	}
	//
	return true
}

// vectorizeInstruction greedily absorbs the targets of jumps in the vector at
// pc until no further merging is legal.  Mirrors vectorizeInstruction from
// pkg/asm/lower.go.
func vectorizeInstruction(pc uint, code []VectorInstruction, mapping instruction.SystemMap) VectorInstruction {
	var (
		vec     = code[pc]
		changed = true
		// externs maps a foreign instruction's pc to the offset within the
		// current vector at which its codes were inlined, or MaxUint if it
		// has not (yet) been absorbed.
		externs []uint = array.FrontPad[uint](nil, uint(len(code)), math.MaxUint)
	)
	// Keep merging until a complete pass produces no change.
	for changed {
		changed = false
		//
		index, ok := lastJump(vec.Codes, uint(len(vec.Codes)))
		// Try the right-most non-conflicting jump.
		for ok {
			jmpTarget := vec.Codes[index].(*instruction.Jump).Immediate
			// Skip back-edges into ourselves and absorbs that would shift
			// backwards (which would otherwise unfold a loop).
			if offset := externs[jmpTarget]; offset > index && jmpTarget != pc {
				var (
					target = code[jmpTarget]
					nvec   VectorInstruction
				)
				//
				if offset != math.MaxUint {
					// Already absorbed earlier in the same vector — replace
					// the Jmp with a Skip to the previously inlined codes.
					nvec = replaceJump(vec, index, offset)
				} else {
					// Splice the target's codes into the vector in place of
					// the Jmp.
					nvec = inlineJump(vec, index, target.Codes)
				}
				// Accept the merge only if it stays valid.
				if validateConflicts(nvec, mapping) == nil {
					if offset == math.MaxUint {
						updateMicroMap(externs, index, jmpTarget, uint(len(target.Codes)))
					}
					//
					vec = nvec
					changed = true
					//
					break
				}
			}
			// Try the next jump leftward.
			index, ok = lastJump(vec.Codes, index)
		}
	}
	//
	return vec
}

// lastJump returns the index of the right-most Jmp within codes[:n], or false
// if none exists.
func lastJump(codes []Instruction, n uint) (uint, bool) {
	for i := n; i > 0; {
		i--
		//
		if codes[i].OpCode() == opcode.JUMP {
			return i, true
		}
	}
	//
	return 0, false
}

// markJumpTargets pushes every reachable Jmp target in the vectorised
// instruction onto the worklist for later processing.
func markJumpTargets(vec VectorInstruction, visited []bool, worklist *stack.Stack[uint],
) {
	//
	index, found := lastJump(vec.Codes, uint(len(vec.Codes)))
	for found {
		target := vec.Codes[index].(*instruction.Jump).Immediate
		//
		if !visited[target] {
			visited[target] = true
			worklist.Push(target)
		}
		//
		index, found = lastJump(vec.Codes, index)
	}
}

// updateMicroMap records that the codes belonging to target have just been
// inlined at offset within the current vector, then shifts other recorded
// offsets to account for the size delta.
func updateMicroMap(externs []uint, index uint, target uint, ncodes uint) {
	externs[target] = index
	//
	for i := range externs {
		if externs[i] != math.MaxUint && externs[i] > index {
			externs[i] += ncodes - 1
		}
	}
}

// replaceJump returns a copy of vec with the Jmp at jmpIndex replaced by a
// Skip targeting the supplied micro offset within the same vector.
func replaceJump(vec VectorInstruction, jmpIndex uint, offset uint) VectorInstruction {
	if offset <= jmpIndex {
		// Should be unreachable: the externs guard requires offset > jmpIndex.
		panic("cannot skip backwards")
	}
	//
	codes := slices.Clone(vec.Codes)
	codes[jmpIndex] = &instruction.Skip{Skip: offset - jmpIndex - 1}
	//
	return VectorInstruction{Codes: codes}
}

// inlineJump returns a new vector formed by replacing the Jmp at jmpIndex
// with the codes from the target instruction.  Skip and SkipIf offsets in the
// surrounding codes are recomputed so they continue to identify the same
// successor after the splice.
func inlineJump(vec VectorInstruction, jmpIndex uint, targetCodes []Instruction) VectorInstruction {
	var (
		codes   = vec.Codes
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
	ncodes := make([]Instruction, npc)
	//
	for cc, npc := uint(0), uint(0); cc < uint(len(codes)); cc++ {
		code := codes[cc]
		//
		switch c := code.(type) {
		case *instruction.Jump:
			if cc == jmpIndex {
				// Splice in the target's codes (shared references — the
				// originals are not mutated downstream).
				for _, tc := range targetCodes {
					ncodes[npc] = tc
					npc++
				}
				//
				continue
			}
		case *instruction.Skip:
			target := mapping[cc+1+c.Skip]
			code = &instruction.Skip{Skip: target - npc - 1}
		case *instruction.SkipIf:
			target := mapping[cc+1+c.Skip]
			code = &instruction.SkipIf{
				Cond:  c.Cond,
				Left:  c.Left,
				Right: c.Right,
				Skip:  target - npc - 1,
			}
		case *instruction.SkipIfConst[vm.Uint]:
			target := mapping[cc+1+c.Skip]
			code = &instruction.SkipIfConst[vm.Uint]{
				Cond:  c.Cond,
				Left:  c.Left,
				Right: c.Right,
				Skip:  target - npc - 1,
			}
		}
		//
		ncodes[npc] = code
		npc++
	}
	//
	return VectorInstruction{Codes: ncodes}
}

// pruneUnreachableInstructions removes any instructions never reached by the
// worklist (left as nil) and rebinds the surviving Jmp targets so they
// reference the new compacted positions.  Jmps are replaced rather than
// mutated so that any shared references inside the vector graph are not
// disturbed.
func pruneUnreachableInstructions(insns []VectorInstruction) []VectorInstruction {
	var (
		kept    []VectorInstruction
		mapping = make([]uint, len(insns))
	)
	// Compact the slice, recording where each surviving instruction lands.
	for i, insn := range insns {
		if insn.IsEmpty() {
			continue
		}
		//
		mapping[i] = uint(len(kept))
		kept = append(kept, insn)
	}
	// Rebind every Jmp.Target to its new position.
	for _, vec := range kept {
		for i, code := range vec.Codes {
			if code.OpCode() == opcode.JUMP {
				// Determine original jump target
				var jmpTarget = code.(*instruction.Jump).Immediate
				// construct replacement jump
				vec.Codes[i] = instruction.NewJump(mapping[jmpTarget])
			}
		}
	}
	//
	return kept
}

// validateConflicts reports the first read/write hazard found within vec, or
// nil if none.  This is a stripped-down version of Vector.Validate that
// considers only register conflicts (RAW with conditional writes, WAW), since
// vectorisation rejects merges only on those grounds — never on field
// bandwidth.
func validateConflicts(vec VectorInstruction, mapping instruction.SystemMap) error {
	var (
		nCodes = uint(len(vec.Codes))
		writes = vec.WriteMap()
	)
	//
	for i := range nCodes {
		var (
			ithState = writes.StateOf(i)
			ith      = vec.Codes[i]
		)
		// RAW: reading a register whose upstream write is conditional inside
		// the vector — no single value to forward from.
		for _, r := range ith.Uses() {
			if ithState.MaybeAssigned(r) && !ithState.DefinitelyAssigned(r) {
				return fmt.Errorf("conflicting read on register %q",
					mapping.Register(r).Name())
			}
		}
		// WAW: writing a register that may already have been written by an
		// earlier code in the vector.
		for _, r := range ith.Definitions() {
			if ithState.MaybeAssigned(r) {
				return fmt.Errorf("conflicting write on register %q",
					mapping.Register(r).Name())
			}
		}
	}
	//
	return nil
}
