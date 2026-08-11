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

package split

import (
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/lazy"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// partAdd "partial add" represents an addition created during splitting which
// contributes towards the overall addition.
type partAdd[W word.Word[W]] struct {
	targets  []RegisterId
	sources  []RegisterId
	constant W
}

// Addition splits an add instruction into one (or more) add instructions,
// potentially introducing one (or more) carry lines at the same time.  For
// example, consider splitting this instruction (where both x and y are u16):
//
// > x = y + 1
//
// Suppose now that x (resp. y) is split into two u8 registers, x1 and x0 (resp.
// y1 and y0).  Then, we end up with the following instructions:
//
// > c::x0 = y0 + 1
// >    x1 = y1 + c
//
// Where c is a newly introduced (u1) carry line.
func Addition[W word.Word[W]](mapping LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	var (
		// Split into the initial set of chunks.
		chunks, context = initialiseAddChunks(mapping, alloc, insn.Target, insn.Source, insn.Constant)
	)
	// Next, add carry lines as needed
	chunks = insertAddCarryLines(alloc, chunks)
	// Convert chunks into assignments
	return append(array.Map(chunks, addAssignment[W]), context...)
}

// initialiseLineaChunks splits the source registers (and constant) into
// least-significant-first chunks, then assigns target limbs to each chunk
// according to the number of bits the corresponding RHS can produce.
func initialiseAddChunks[W word.Word[W]](mapping LimbsMap[W], alloc Allocator[W],
	targets, sources []RegisterId, constant W) ([]partAdd[W], []Bytecode[W]) {
	//
	var (
		zero W
		// Extract register width
		regWidth = mapping.RegisterWidth()
		// Determine target limbs
		limbs = applyLimbsMapReversed(mapping, targets...)
		// Initialise register stack
		stack = RegisterStack[W]{limbs, alloc, nil}
		// Initialise limb "matrix"
		matrix = newLimbMatrix(sources, mapping)
		// Split the cosntant
		constants = descriptor.SplitConstant(constant, mapping.RegisterWidth())
		//
		nChunks, nConstants = len(matrix.chunks), len(constants)
		codes               []partAdd[W]
	)
	// Initialise partial additions
	for i := range max(nChunks, nConstants) {
		var (
			lhs = stack.SelectExact(regWidth)
			rhs = lazy.IfDefault(i < nChunks, lazy.Read(i, matrix.chunks))
			c   = lazy.IfDefault(i < nConstants, lazy.Read(i, constants))
		)
		//
		codes = append(codes, partAdd[W]{lhs, mapAddRhs(rhs), c})
	}
	// Handle overhangs.  This arises e.g. when assigning a small constant to a
	// wide target register.
	for stack.Size() > 0 {
		codes = append(codes, partAdd[W]{[]RegisterId{stack.Pop()}, nil, zero})
	}
	//
	return codes, stack.post
}

// insertAddCarryLines verifies that each addition chunk fits within its
// assigned target limbs, returning a copied chunk sequence.  It panics when a
// chunk would overflow and therefore still requires an inserted carry line.
func insertAddCarryLines[W word.Word[W]](alloc Allocator[W], chunks []partAdd[W]) []partAdd[W] {
	//
	for i := range len(chunks) {
		var (
			// Determine bitwidth of left-hand side
			lhs = descriptor.BitwidthOf(alloc, chunks[i].targets...).Unwrap()
			// Determine bitwidth of right-hand side
			rhs = addRhsBitwidth(chunks[i], alloc)
		)
		// Check whether carry required.  NOTE: a chunk whose target limbs are
		// exhausted has a zero-width left-hand side.  In such case, there is no
		// value in adding carry lines.  Furthermore, doing so doesn't make sense
		// since the carry lines would be as wide the rhs anyway.
		if lhs > 0 && lhs < rhs && i+1 < len(chunks) {
			var (
				bitwidth = rhs - lhs
				// allocate new carry line
				carry = alloc.Allocate("c", util.Some(bitwidth))
			)
			// insert carry line
			chunks[i].targets = append(chunks[i].targets, carry)
			chunks[i+1].sources = append(chunks[i+1].sources, carry)
		}
	}
	//
	return chunks
}

func addRhsBitwidth[W word.Word[W]](chunk partAdd[W], mapping descriptor.RegisterMap[W]) uint {
	return descriptor.CalculateAddBitwidth(chunk.sources, chunk.constant, mapping).Unwrap()
}

func mapAddRhs(rhs []util.Option[RegisterId]) []RegisterId {
	var res []RegisterId
	//
	for _, rhs := range rhs {
		if rhs.HasValue() {
			res = append(res, rhs.Unwrap())
		}
	}
	//
	return res
}

// addAssignment lowers a chunk back into a concrete unsigned-add instruction.
func addAssignment[W word.Word[W]](_ uint, chunk partAdd[W]) Bytecode[W] {
	// Done
	return bytecode.AddVecConst(chunk.targets, chunk.sources, chunk.constant)
}
