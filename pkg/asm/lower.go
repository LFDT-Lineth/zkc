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
package asm

import (
	"math"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// LoweringConfig provides configuration options for configuring the lowering
// process.
type LoweringConfig struct {
	// Field determines necessary parameters for the underlying field.  This
	// includes: the maximum field bandwidth, which is number of bits the
	// underlying field can hold; and, the maximum register width.
	Field field.Config
	// Vectorize determines whether or not to enable vectorisation.  More
	// specifically, vectorisation attempts to combine multiple machine
	// instructions together into batches which can be "executed" concurrently,
	// roughly reminiscent of Very Long Instruction Word (VLIW) architectures.
	Vectorize bool
}

func lowerComponent(vectorize bool, f MacroComponent) MicroFunction {
	switch f := f.(type) {
	case *MacroFunction:
		return lowerFunction(vectorize, *f)
	default:
		panic("unknown component")
	}
}

func lowerFunction(vectorize bool, f MacroFunction) MicroFunction {
	insns := make([]micro.Instruction, len(f.Code()))
	// Lower macro instructions to micro instructions.
	for pc, insn := range f.Code() {
		insns[pc] = insn.Lower(uint(pc))
	}
	// Sanity checks (for now)
	fn := io.NewFunction(f.Name(), f.IsPublic(), f.Registers(), f.Buses(), insns)
	// Apply vectorisation (if enabled).
	if vectorize {
		fn = vectorizeFunction(fn)
	}
	//
	return fn
}

// Vectorize a given function by merging as many micro-codes as possible into
// each (vector) micro-instruction.  The strategy is greedy: walking the
// function, we repeatedly try to absorb the target of a jump back into the
// instruction containing that jump, effectively pulling a successor
// instruction up into its predecessor until no further merging is legal.  For
// example, given two micro instructions "x = y" and "a = b", neither writes a
// register the other touches and so they can be combined into the single
// vector instruction "x=y ; a=b" whose codes execute "in parallel".
//
// The principal obstacle to merging is the appearance of *register conflicts*
// between micro-codes — that is, data hazards in the classical sense from
// computer architecture.  All three textbook hazards (RAW, WAW, WAR) arise
// here, where "earlier" and "later" refer to the position of two codes within
// the same vector instruction:
//
//   - RAW (Read-After-Write).  A later code reads a register that an earlier
//     code writes.  This is the "true" data dependency.  Within a vector
//     instruction it is normally resolved by *register forwarding* (described
//     below): the later code simply observes the freshly-written value.
//     However, when the upstream write is *conditional* — i.e. it occurs on
//     some intra-instruction control-flow paths but not others
//     (Writes.MaybeAssigned without Writes.DefinitelyAssigned) — the value to
//     forward is not well-defined and the merge is rejected.  This is what
//     Instruction.Validate reports as a "conflicting read".
//
//   - WAW (Write-After-Write).  Two codes in the same vector instruction both
//     write the same register.  The resulting register value would be
//     ambiguous, so the merge is rejected.  This is what Instruction.Validate
//     reports as a "conflicting write", and is the most common form of
//     register conflict in practice.
//
//   - WAR (Write-After-Read).  A later code writes a register that an earlier
//     code reads.  This is *not* a hazard in this setting, because forwarding
//     flows strictly forward: the earlier read always observes the
//     pre-instruction value, while the later write only takes effect once the
//     whole vector instruction completes.  No check is required, and no merge
//     is blocked on this account.
//
// Register forwarding is the mechanism that makes RAW dependencies tractable
// inside a vector instruction.  When one micro-code writes a register, every
// subsequent code in the same instruction observes the freshly-written value
// rather than the value held at the start of the instruction.  Forwarding is
// precisely what makes vectorisation useful (a downstream code can
// immediately consume an upstream code's result) but it is also what gives
// rise to RAW conflicts: if the upstream write only happens on some
// intra-instruction paths there is no single well-defined source to forward
// from.
//
// In addition to data hazards, two further conditions block a merge:
//
//   - Other validation failures.  The merged instruction must continue to
//     satisfy every micro-instruction invariant (balance, no field overflow,
//     allocated temporaries, etc.) as checked by Instruction.Validate.
//
//   - Back-edges.  A jump whose target would bring control back into the
//     instruction being built (a loop) is left alone; otherwise the inliner
//     would unfold it indefinitely.
//
// After greedy merging, any micro-instructions left unreachable from the
// entry point are pruned and remaining jump targets rewritten accordingly.
func vectorizeFunction(f MicroFunction) MicroFunction {
	var (
		insns    = make([]micro.Instruction, len(f.Code()))
		visited  = make([]bool, len(f.Code()))
		worklist stack.Stack[uint]
	)
	// Initialise worklist
	worklist.Push(0)
	// Vectorize instructions as much as possible.
	for !worklist.IsEmpty() {
		// Get next instruction to process
		pc := worklist.Pop()
		//
		insns[pc] = vectorizeInstruction(pc, f.Code(), &f)
		//
		markJumpTargets(insns[pc], visited, &worklist)
	}
	// Remove all uncreachable instructions and compact remainder.
	insns = pruneUnreachableInstructions(insns)
	//
	return io.NewFunction(f.Name(), f.IsPublic(), f.Registers(), f.Buses(), insns)
}

func markJumpTargets(insn micro.Instruction, visited []bool, worklist *stack.Stack[uint]) {
	// identify first jumpo
	index, ok := insn.LastJump(uint(len(insn.Codes)))
	//
	for ok {
		// Extract jump instruction
		jmp := insn.Codes[index].(*micro.Jmp)
		// Mark instruction (if not already visited)
		if !visited[jmp.Target] {
			visited[jmp.Target] = true
			worklist.Push(jmp.Target)
		}
		// continue to next
		index, ok = insn.LastJump(index)
	}
}

func vectorizeInstruction(pc uint, insns []micro.Instruction, mapping register.Map) micro.Instruction {
	var (
		insn    = insns[pc]
		changed = true
		// maps foreign instructions to their micro-offset (if they have one) or
		// MaxUint (if they don't).
		externs []uint = array.FrontPad[uint](nil, uint(len(insns)), math.MaxUint)
	)
	// Keep vectorizing until worklist empty.
	for changed {
		changed = false
		//
		index, ok := insn.LastJump(uint(len(insn.Codes)))
		// Identify rightmost jump target (if exists)
		for ok {
			// Extract jump instruction
			jmp := insn.Codes[index].(*micro.Jmp)
			// Extract target instruction
			target := insns[jmp.Target]
			// Check against loops
			if offset := externs[jmp.Target]; offset > index && jmp.Target != pc {
				var ninsn micro.Instruction
				// Inline instruction
				if offset != math.MaxUint {
					// no need to inline, as this instruction was previously inlined further down.
					ninsn = replaceJump(insn, index, offset)
				} else {
					ninsn = inlineJump(insn, index, target.Codes)
				}
				// Check whether instruction remains valid or not.  An
				// instruction might be invalid at this point if it contains a
				// conflicting write and/or breaks any internal invariants.
				if ninsn.Validate(math.MaxUint, mapping) == nil {
					// valid, so update micro mapping (if applicable)
					if offset == math.MaxUint {
						updateMicroMap(externs, index, jmp.Target, uint(len(target.Codes)))
					}
					//
					insn = ninsn
					changed = true
					// Done
					break
				}
			}
			// continue looking for non-conflicting rightmost branch
			index, ok = insn.LastJump(index)
		}
	}
	//
	return insn
}

// Update the micro map after an instruction with n micro-codes is inlined at a
// given index.
func updateMicroMap(externs []uint, index uint, jmpTarget uint, ncodes uint) {
	// update micro mapping
	externs[jmpTarget] = index
	//
	for i := 0; i < len(externs); i++ {
		if externs[i] != math.MaxUint && externs[i] > index {
			externs[i] += ncodes - 1
		}
	}
}

// Replace a jump at a given index with a skip to a given micro offset
func replaceJump(insn micro.Instruction, jmpIndex uint, offset uint) micro.Instruction {
	var (
		// Extract jump instruction
		codes = slices.Clone(insn.Codes)
		delta = offset - (jmpIndex + 1)
	)
	// Sanity check
	if offset <= jmpIndex {
		// Should be unreachable
		panic("cannot skip backwards")
	}
	//
	codes[jmpIndex] = &micro.Skip{Skip: delta}
	// Done
	return micro.Instruction{Codes: codes}
}

// Inline a jump instruction within this instruction.  This requires correctly
// updating internal code offsets for skip instructions, otherwise they could
// now skip over the wrong number of codes.
func inlineJump(insn micro.Instruction, jmpIndex uint, targetCodes []micro.Code) micro.Instruction {
	var (
		// Extract jump instruction
		codes   = insn.Codes
		mapping = make([]uint, len(codes))
		npc     int
	)
	// Determine length of final sequence, and construct an appropriate mapping
	// from code offsets in the original instruction to those in the new
	// instruction.
	for cc := uint(0); cc < uint(len(codes)); cc, npc = cc+1, npc+1 {
		mapping[cc] = uint(npc)
		// Check for insn being inlined.
		if cc == jmpIndex {
			// NOTE: -1 as will overwrite the jmp.
			npc += len(targetCodes) - 1
		}
	}
	// construct new sequence (to be filled out).
	ninsns := make([]micro.Code, npc)
	// fill out new sequence.
	for cc, npc := uint(0), uint(0); cc < uint(len(codes)); cc++ {
		code := codes[cc]
		//
		switch c := code.(type) {
		case *micro.Jmp:
			if cc == jmpIndex {
				// copy over target instructions
				for _, c := range targetCodes {
					ninsns[npc] = c.Clone()
					npc++
				}
				//
				continue
			}
		case *micro.Skip:
			// Determine absolute target
			target := mapping[cc+1+c.Skip]
			// Recalculate as relative offset
			code = &micro.Skip{Skip: target - npc - 1}
		case *micro.SkipIf:
			// Determine absolute target
			target := mapping[cc+1+c.Skip]
			// Recalculate as relative offset
			code = &micro.SkipIf{Left: c.Left, Right: c.Right, Skip: target - npc - 1}
		}
		//
		ninsns[npc] = code
		npc++
	}
	// Skip instructions may need updating here.
	return micro.Instruction{Codes: ninsns}
}

// Identify and remove all unreachable instructions.  A tricky aspect of this is
// that we must updating jump targets accordingly.
func pruneUnreachableInstructions(insns []micro.Instruction) []micro.Instruction {
	var (
		ninsns  []micro.Instruction
		mapping []uint = make([]uint, len(insns))
	)
	// Remove all unreachable
	for i, insn := range insns {
		if len(insn.Codes) != 0 {
			mapping[i] = uint(len(ninsns))
			ninsns = append(ninsns, insn)
		}
	}
	// Rebinding all existing jump targets
	for _, insn := range ninsns {
		for i := range insn.Codes {
			code := insn.Codes[i]
			if jmp, ok := code.(*micro.Jmp); ok {
				jmp.Target = mapping[jmp.Target]
				insn.Codes[i] = jmp
			}
		}
	}
	//
	return ninsns
}
