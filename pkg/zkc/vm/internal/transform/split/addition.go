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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

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
func Addition[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	var (
		// Split into the initial set of chunks.
		chunks, context = initialiseLineaChunks(mapping, alloc, insn.Target, insn.Source, insn.Constant)
	)
	// Next, add carry lines as needed
	chunks = insertAddCarryLines(alloc, chunks)
	// Convert chunks into assignments
	return append(MapChunks(chunks, addAssignment[W]), context...)
}

// insertAddCarryLines verifies that each addition chunk fits within its
// assigned target limbs, returning a copied chunk sequence.  It panics when a
// chunk would overflow and therefore still requires an inserted carry line.
func insertAddCarryLines[W word.Word[W]](alloc Allocator[W], chunks Chunks[W]) Chunks[W] {
	//
	for i := range chunks.Len() {
		var (
			ith = chunks.Ith(i)
			lhs = ith.LhsBitwidth(alloc)
			rhs = addRhsBitwidth(ith, alloc)
		)
		// check whether carry required
		if lhs < rhs && i+1 < chunks.Len() {
			var (
				bitwidth = rhs - lhs
				// allocate new carry line
				carry = alloc.Allocate("c", util.Some(bitwidth))
			)
			// insert carry line
			chunks.Apply(i, appendLhsLimb[W](carry))
			chunks.Apply(i+1, appendRhsLimb[W](carry))
		}
	}
	//
	return chunks
}

func addRhsBitwidth[W word.Word[W]](chunk Chunk[W], mapping descriptor.RegisterMap[W]) uint {
	var rhsMaxVal big.Int
	// Initialise max value
	rhsMaxVal.Set(chunk.Constant.BigInt())
	// Determine maximum expressible value
	for _, r := range chunk.RightHandSide {
		reg := mapping.Register(r)
		// Accumulate maximum register value
		rhsMaxVal.Add(&rhsMaxVal, maxValueOf(reg.Bitwidth()))
	}
	//
	return uint(rhsMaxVal.BitLen())
}

// addAssignment lowers a chunk back into a concrete unsigned-add instruction.
func addAssignment[W word.Word[W]](chunk Chunk[W]) Bytecode[W] {
	// Done
	return bytecode.AddVecConst(chunk.LeftHandSide, chunk.RightHandSide, chunk.Constant)
}
