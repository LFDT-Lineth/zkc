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

// partSub "partial sub" represents a subtraction created during splitting which
// contributes towards the overall subtraction.
type partSub[W word.Word[W]] struct {
	targets  []RegisterId
	sources  []RegisterId
	constant W
}

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
	var pre []Bytecode[W]
	//
	if !requiresSplitting(mapping, insn) {
		return remapSubtraction(mapping, insn)
	} else if requiresBreakup(insn) {
		pre, insn, mapping = breakupSubtraction(mapping, alloc, insn)
	}
	//
	return append(pre, splitSubtraction(mapping, alloc, insn)...)
}

// Check whether this subtraction actually requires splitting or not.  This is
// important since, if it doesn't need splitting, then it doesn't need to be
// broken up.  A subtraction only requires splitting if it contains a source
// register or constant which must be split, or its right-hand side exceeds the
// target bandwidth.
func requiresSplitting[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.Arith[W]) bool {
	var bitwidth = descriptor.BitwidthOf(mapping, insn.Target...).Unwrap()
	//
	for _, source := range insn.Source {
		if len(mapping.LimbIds(source)) > 1 {
			return true
		}
	}
	// Check whether constant must be split (or not)
	return !insn.Constant.FitsWithin(mapping.RegisterWidth()) && bitwidth < mapping.BandWidth()
}

func requiresBreakup[W word.Word[W]](insn *bytecode.Arith[W]) bool {
	var hasConstant = insn.Constant.Cmp64(0) != 0
	//
	return len(insn.Source) > 2 || (len(insn.Source) == 2 && hasConstant)
}

// Remap a subtraction which does not require real splitting.
func remapSubtraction[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.Arith[W]) []Bytecode[W] {
	var (
		targets = applyLimbsMapReversed(mapping, insn.Target...)
		sources = applyLimbsMapReversed(mapping, insn.Source...)
	)
	//
	return []Bytecode[W]{
		bytecode.NewArith(bytecode.OP_SUB, targets, sources, insn.Constant),
	}
}

// Break up a subtraction with more than two operands into an addition, followed
// by a subtraction.  For example, consider the following:
//
// a = b - c - d
//
// Then this would be broken up into:
//
// t = c + d
// a = b - t
func breakupSubtraction[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) ([]Bytecode[W], *bytecode.Arith[W], descriptor.LimbsMap[W]) {
	var (
		zero W
		// Determine bitwidth of temporary register
		bitwidth = descriptor.CalculateAddBitwidth(insn.Source[1:], insn.Constant, mapping).Unwrap()
		// Create "fake" register to use for addition
		tmpReg, nmapping = allocateTemporary(bitwidth, mapping, alloc)
		// Create addition for splitting
		fakeAdd = bytecode.NewArith(bytecode.OP_ADD, []bytecode.RegisterId{tmpReg}, insn.Source[1:], insn.Constant)
		// Split the addition
		pre = Addition(nmapping, alloc, fakeAdd)
		// create new subtraction
		sub = bytecode.NewArith(bytecode.OP_SUB, insn.Target, []bytecode.RegisterId{insn.Source[0], tmpReg}, zero)
	)
	//
	return pre, sub, nmapping
}

func splitSubtraction[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Arith[W],
) []Bytecode[W] {
	// NOTE: by breaking up subtractions, we guarantee here that there at most
	// two source limbs.
	util.Assert(len(insn.Source) <= 2, "internal failure")
	// Split into the initial set of chunks.
	var chunks, context = initialiseSubChunks(mapping, alloc, insn.Target, insn.Source, insn.Constant)
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
	return append(array.Map(chunks, subAssignment[W]), context...)
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

// initialiseLineaChunks splits the source registers (and constant) into
// least-significant-first chunks, then assigns target limbs to each chunk
// according to the number of bits the corresponding RHS can produce.
func initialiseSubChunks[W word.Word[W]](mapping LimbsMap[W], alloc Allocator[W],
	targets, sources []RegisterId, constant W) ([]partSub[W], []Bytecode[W]) {
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
		codes               []partSub[W]
	)
	// Initialise partial additions
	for i := range max(nChunks, nConstants) {
		var (
			lhs = stack.SelectExact(regWidth)
			rhs = lazy.IfDefault(i < nChunks, lazy.Read(i, matrix.chunks))
			c   = lazy.IfDefault(i < nConstants, lazy.Read(i, constants))
		)
		//
		codes = append(codes, partSub[W]{lhs, mapSubRhs(rhs, alloc), c})
	}
	// Handle overhangs.  This arises e.g. when assigning a small constant to a
	// wide target register.
	for stack.Size() > 0 {
		codes = append(codes, partSub[W]{[]RegisterId{stack.Pop()}, nil, zero})
	}
	//
	return codes, stack.post
}

func mapSubRhs[W word.Word[W]](rhs []util.Option[RegisterId], alloc Allocator[W]) []RegisterId {
	var res []RegisterId
	//
	for i, rhs := range rhs {
		if i == 0 && rhs.IsEmpty() {
			// allocate zero line
			res = append(res, alloc.ZeroRegister())
		} else if rhs.HasValue() {
			res = append(res, rhs.Unwrap())
		}
	}
	//
	return res
}

// insertSubBorrowLines verifies that each subtraction chunk fits within its
// assigned target limbs, returning an updated chunk sequence.  When a chunk's
// RHS produces more bits than its LHS can hold, a borrow register is allocated
// and spliced into the current chunk's LHS and the next chunk's RHS.  The final
// chunk is skipped since its overflow represents the top bits with no successor
// to absorb it.
func insertSubBorrowLines[W word.Word[W]](alloc Allocator[W], chunks []partSub[W]) []partSub[W] {
	//
	for i := range len(chunks) {
		// u1 borrow lines are always required for subtraction.
		if i+1 < len(chunks) {
			var (
				// allocate new borrow line
				borrow = alloc.Allocate("b", util.Some[uint](1))
			)
			// insert borrow line
			chunks[i].targets = append(chunks[i].targets, borrow)
			chunks[i+1].sources = append(chunks[i+1].sources, borrow)
		}
	}
	//
	return chunks
}

// subAssignment lowers a chunk back into a concrete unsigned-subtract
// instruction.
func subAssignment[W word.Word[W]](_ uint, chunk partSub[W]) Bytecode[W] {
	var zero W
	// Check for non-subtraction case
	if len(chunk.sources) == 0 {
		return bytecode.LoadConstVec(chunk.targets, zero)
	}
	// Done
	return bytecode.SubVecConst(chunk.targets, chunk.sources, chunk.constant)
}
