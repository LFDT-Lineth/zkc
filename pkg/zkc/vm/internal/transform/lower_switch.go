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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerSwitch rewrites Switch (multiway skip) bytecodes into equivalent
// sequences of SkipIf bytecodes.  Each dispatch case becomes two codes: a
// constant load of the case's value into a fresh register, followed by a
// conditional (EQ) skip against the dispatch register targeting the case's
// original destination.  Cases are tested in order, preserving the
// first-match-wins semantics of the multiway dispatch; when no case matches,
// control falls through exactly as before.
//
// NOTE: this transform must run before register splitting (which does not
// support Switch bytecodes).
func LowerSwitch[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerSwitchFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func lowerSwitchFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = lowerSwitchVector(vec, alloc)
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.IsNative(), nvecs)
}

// lowerSwitchVector expands every Switch bytecode within a vector, recalculating
// the skip offsets of all bytecodes (including the other skips in the vector)
// against the new layout.  Vector.Map cannot be used here: a switch case whose
// skip is smaller than the size of its own replacement packet would be
// misclassified as an internal skip and left unremapped.
func lowerSwitchVector[W word.Word[W]](vec BytecodeVector[W], registers split.Allocator[W]) BytecodeVector[W] {
	var (
		codes = vec.Bytecodes
		// mapping[i] holds the new offset of old bytecode i, with an end
		// sentinel supporting skips which target the end of the vector.
		mapping = make([]uint, len(codes)+1)
		offset  uint
	)
	// First pass: determine the new offset of every bytecode.
	for i, c := range codes {
		mapping[i] = offset
		//
		if sw, ok := c.(*bytecode.Switch[W]); ok {
			offset += 2 * uint(len(sw.Cases))
		} else {
			offset++
		}
	}
	//
	mapping[len(codes)] = offset
	// Second pass: emit lowered bytecodes with recalculated skips.
	ncodes := make([]Bytecode[W], 0, offset)
	//
	for i, c := range codes {
		switch c := c.(type) {
		case *bytecode.Switch[W]:
			ncodes = append(ncodes, lowerSwitchCode(uint(i), c, mapping, registers)...)
		case *bytecode.Skip[W]:
			ncodes = append(ncodes, &bytecode.Skip[W]{Skip: relocateSkip(uint(i), c.Skip, mapping)})
		case *bytecode.SkipIf[W]:
			ncodes = append(ncodes, &bytecode.SkipIf[W]{
				Skip: relocateSkip(uint(i), c.Skip, mapping), Left: c.Left, Right: c.Right, Op: c.Op})
		default:
			ncodes = append(ncodes, c)
		}
	}
	//
	return bytecode.NewVector(ncodes...)
}

// lowerSwitchCode expands a single Switch bytecode, located at (old) offset pc
// within its enclosing vector, into a first-match-wins chain of constant loads
// and conditional (EQ) skips:
//
//	c_0 = v_0
//	skip_if r == c_0 <case 0 target>
//	c_1 = v_1
//	skip_if r == c_1 <case 1 target>
//	...
//
// Each case needs a fresh register since, on the fall-through path, every load
// executes.  The register is sized to its case's value, which cannot overflow
// the dispatch register (see Switch.Validate).
func lowerSwitchCode[W word.Word[W]](pc uint, sw *bytecode.Switch[W], mapping []uint,
	registers split.Allocator[W]) []Bytecode[W] {
	//
	var (
		codes = make([]Bytecode[W], 0, 2*len(sw.Cases))
		width = registers.Register(sw.Source).Bitwidth()
	)
	//
	for j, cse := range sw.Cases {
		var (
			creg = registers.Allocate("", width)
			// New position of this case's conditional skip.
			position = mapping[pc] + uint(2*j) + 1
			// New position of this case's dispatch target.
			target = mapping[pc+uint(cse.Skip)+1]
		)
		//
		codes = append(codes,
			bytecode.LoadConst(creg, cse.Value),
			bytecode.NewSkipIf[W](bytecode.CONDITION_EQ, util.Cast[uint16](target-position-1), sw.Source, creg))
	}
	//
	return codes
}

// relocateSkip recalculates the skip offset of a (conditional or unconditional)
// skip located at old offset pc, such that it continues to identify the same
// target under the new layout described by mapping.
func relocateSkip(pc uint, skip uint16, mapping []uint) uint16 {
	var (
		// New position of the skip's target.
		target = mapping[pc+uint(skip)+1]
		// New position of the skip itself.
		position = mapping[pc]
	)
	//
	return util.Cast[uint16](target - position - 1)
}
