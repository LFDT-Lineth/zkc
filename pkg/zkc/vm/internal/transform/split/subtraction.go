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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Subtraction splits a subtraction instruction into one (or more) subtraction
// instructions, potentially introducing one (or more) borrow lines at the same
// time.  For example, consider splitting this instruction (where both x and y
// are u16):
//
// > x = y - 1
//
// Suppose now that x (resp. y) is split into two u8 registers, x1 and x0 (resp.
// y1 and y0).  Then, we end up with the following instructions:
//
// > b::x0 = y0 - 1
// >    x1 = y1 - b
//
// Where b is a newly introduced (u1) borrow line.  The mechanism is identical
// to that of Addition: when a chunk's right-hand side produces more bits than
// its left-hand side can hold, a borrow register is allocated and spliced into
// the current chunk's LHS and the next chunk's RHS.
func Subtraction[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	// Split into the initial set of chunks.
	var chunks, context = initialiseLineaChunks(mapping, alloc, insn.Target, insn.Source, insn.Constant)
	// A zero assignment of the form "0 = a - b" (a zero-width target, two operands and
	// no constant) asserts a == b.  Split limb-wise, this is exactly asserting a_i == b_i
	// for every limb independently, which requires NO borrow chain — equal values have
	// equal limbs.  Threading borrows instead would be unsound-free but catastrophic for
	// width: a zero-width target cannot absorb any bits, so each limb's entire value is
	// forced into the borrow, and those borrows compound across limbs until they exceed
	// the field's register width (e.g. a u256 difference needs a u256-wide borrow).  So
	// for this case we leave the chunks as independent per-limb assertions.
	if !isZeroDifference(mapping, insn) {
		// Next, add borrow lines as needed
		chunks = insertSubBorrowLines(alloc, chunks)
	}
	// Convert chunks into assignments
	return append(MapChunks(chunks, subAssignment[W]), context...)
}

// isZeroDifference determines whether the given subtraction is a zero assignment of
// the form "0 = a - b": a zero-width target subtracting exactly two operands with no
// constant.  Such an assertion (a == b) decomposes into independent per-limb equalities
// and therefore needs no borrow chain (see Subtraction).
func isZeroDifference[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.Arith[W]) bool {
	if len(insn.Source) != 2 || insn.Constant.Cmp64(0) != 0 {
		return false
	}
	//
	var width uint
	//
	for _, t := range insn.Target {
		width += mapping.Register(t).Bitwidth().Unwrap()
	}
	//
	return width == 0
}

// insertSubBorrowLines verifies that each subtraction chunk fits within its
// assigned target limbs, returning an updated chunk sequence.  When a chunk's
// RHS produces more bits than its LHS can hold, a borrow register is allocated
// and spliced into the current chunk's LHS and the next chunk's RHS.  The final
// chunk is skipped since its overflow represents the top bits with no successor
// to absorb it.
func insertSubBorrowLines[W word.Word[W]](alloc Allocator[W], chunks Chunks[W]) Chunks[W] {
	//
	for i := range chunks.Len() {
		var (
			ith = chunks.Ith(i)
			lhs = ith.LhsBitwidth(alloc)
			rhs = subRhsBitwidth(ith, alloc)
		)
		// check whether borrow required
		if lhs < rhs && i+1 < chunks.Len() {
			var (
				bitwidth = rhs - lhs
				// allocate new borrow line
				borrow = alloc.Allocate("b", bitwidth)
			)
			// insert borrow line
			chunks.Apply(i, appendLhsLimb[W](borrow))
			chunks.Apply(i+1, appendRhsLimb[W](borrow))
		}
	}
	//
	return chunks
}

func subRhsBitwidth[W word.Word[W]](chunk Chunk[W], mapping descriptor.RegisterMap[W]) uint {
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

// subAssignment lowers a chunk back into a concrete unsigned-subtract
// instruction.
func subAssignment[W word.Word[W]](chunk Chunk[W]) Bytecode[W] {
	// Done
	return bytecode.SubVecConst(chunk.LeftHandSide, chunk.RightHandSide, chunk.Constant)
}
