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
package split

import (
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// partCat "partial cat" represents an concatenation created during splitting which
// contributes towards the overall concatenation.
type partCat struct {
	targets []RegisterId
	sources []RegisterId
	// borrows indicates the most significant source of this chunk is a borrow
	// line written by the next chunk, which must therefore be executed first.
	borrows bool
}

// Concat splits a concatenation instruction into one (or more) instructions,
// potentially introducing one (or more) carry lines at the same time.  For
// example, consider splitting this function:
//
//	fn (y,z:u16) -> (x:32) {
//	  x = y::z
//	}
//
// Splitting into (max) u8 registers gives the following (assuming a bandwidth
// of u16):
//
//	fn (y1,y0,z1,z0:u8) -> (x3,x2,x1,x0:u8) {
//	  x1::x0 = z1::z0
//	  x3::x2 = y1::y0
//	}
//
// In this case, no carry lines were required.  But, now consider the follwing
// (more complex) example:
//
//	fn (y:u12,z:u20) -> (x:32) {
//	  x = y::z
//	}
//
// Again, splitting into (max) u8 registers gives the following  (again,
// assuming a bandwidth of u16):
//
//	fn (y1:u4,y0:u8, z2:u4,z1,z0:u8) -> (x3,x2,x1,x0:u8) {
//	  var c0,c1:u4
//	  c0::x0 = y1::y0
//	  c1::x1 = z0::c0
//	  x3::x2 = z1::c1
//	}
//
// Here, we can see that two carry registers, c0 and c1 have been introduced.
// Finally, a chunk's target limbs can be wider than its sources, as in this
// example (assuming u32 registers and a bandwidth of u48):
//
//	fn (x:u40, y:u8, z:u16) -> (a:u64) {
//	  a = x::y::z
//	}
//
// Splitting gives the following:
//
//	fn (x1:u8, x0:u32, y:u8, z:u16) -> (a1,a0:u32) {
//	  var b0:u8
//	  a1::b0 = x1::x0
//	  a0 = b0::y::z
//	}
//
// Here, the borrow b0 supplies the missing top bits of the first chunk from the
// bottom of the next.  Since b0 is written by the second chunk, it is executed
// first.
func Concat[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	insn *bytecode.Cat[W]) []Bytecode[W] {
	// Split into the initial set of chunks.
	var chunks, context = initialiseConcatChunks(mapping, alloc, insn.Targets, insn.Sources)
	// Next, add carry and borrow lines as needed
	chunks = insertConcatCarryLines(mapping, alloc, chunks)
	// Convert chunks into assignments
	return append(orderConcatAssignments(chunks, alloc), context...)
}

// orderConcatAssignments lowers chunks into assignments ordered such that every
// carry or borrow line is written before it is read.  Carries flow upwards from
// a chunk to the next, whilst borrows flow downwards.  Hence, each maximal run
// of chunks linked by borrows is emitted in reverse, whilst runs themselves are
// emitted in order.
func orderConcatAssignments[W word.Word[W]](chunks []partCat, alloc Allocator[W]) []Bytecode[W] {
	var codes = make([]Bytecode[W], 0, len(chunks))
	//
	for start := 0; start < len(chunks); {
		var end = start
		// Find end of run
		for chunks[end].borrows {
			end++
		}
		// Emit run in reverse
		for i := end; i >= start; i-- {
			codes = append(codes, concatAssignment(chunks[i], alloc)...)
		}
		//
		start = end + 1
	}
	//
	return codes
}

// initialiseConcatChunks splits the addition sources and constant into
// least-significant-first chunks, then assigns target limbs to each chunk
// according to the number of bits the corresponding RHS can produce.
func initialiseConcatChunks[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	targets, sources []RegisterId) ([]partCat, []Bytecode[W]) {
	//
	var (
		// Split all source registers
		sourceLimbs = applyLimbsMapReversed(mapping, sources...)
		// Split all target registers
		targetLimbs = applyLimbsMapReversed(mapping, targets...)
		// Initialise target register targetStack
		targetStack = RegisterStack[W]{targetLimbs, alloc, nil}
		// Initialise source register stack
		sourceStack = RegisterStack[W]{sourceLimbs, alloc, nil}
		// Offset overhang / underhang between chunks
		overhang, underhang uint
		//
		chunks []partCat
	)
	// Keep going whilst we still have source registers
	for sourceStack.Size() > 0 && targetStack.Size() > 0 {
		var (
			// Select source registers for this chunk.
			rhs = sourceStack.SelectUpto(mapping.BandWidth() - overhang)
			// Calculate bitwidth of selected source registers (including overhang)
			rhsBitwidth = descriptor.BitwidthOf(alloc, rhs...).Unwrap() + overhang
			// Select target registers for this chunk.
			lhs = targetStack.SelectUpto(mapping.BandWidth() - underhang)
			// Calculate bitwidth of selected target registers
			lhsBitwidth = descriptor.BitwidthOf(alloc, lhs...).Unwrap() + underhang
		)
		// Basic sanity checks
		util.Assert(len(lhs) > 0 || len(rhs) > 0, "chunking failure")
		util.Assert(mapping.BandWidth() >= rhsBitwidth, "rhs overflow")
		util.Assert(mapping.BandWidth() >= lhsBitwidth, "lhs overflow")
		// allocate selected targets
		chunks = append(chunks, partCat{targets: lhs, sources: rhs})
		// update overhang
		if rhsBitwidth > lhsBitwidth {
			overhang = rhsBitwidth - lhsBitwidth
			underhang = 0
		} else {
			overhang = 0
			underhang = lhsBitwidth - rhsBitwidth
		}
	}
	// Handle cases where we have more targets than necessary.  This can arise
	// under normal circumstances, such as when assigning a small constant to a
	// wide target register.  In this case, we simple assign each target in this
	// "overhang" to zero.
	for targetStack.Size() > 0 {
		chunks = append(chunks, partCat{targets: []RegisterId{targetStack.Pop()}})
	}
	// Handle cases where we have more sources than necessary (which can also
	// arise under normal circumstances).  In this case, we assign each source
	// to zero.
	for sourceStack.Size() > 0 {
		// Select upto bandwidth registers
		var chunk = sourceStack.SelectUpto(mapping.BandWidth() - overhang)
		// Combine into a single assignment to zero
		chunks = append(chunks, partCat{sources: chunk})
		// Reset overhang
		overhang = 0
	}
	// Assert that we never create bytecodes for source registers.
	util.Assert(len(sourceStack.post) == 0, "internal failure")
	//
	return chunks, targetStack.post
}

// insertConcatCarryLines resolves width mismatches between each chunk's LHS and
// RHS by threading an intermediate register into the next chunk.  When the RHS
// produces more bits than the LHS can hold, a carry takes the overflowing (most
// significant) bits of the RHS and passes them up as the least significant
// source of the next chunk.  When the LHS is wider than the RHS, a borrow fills
// the missing (most significant) bits of the RHS, and is written by the next
// chunk as its least significant target.  The last real chunk (and everything
// after it) is skipped, since there is no value in adding carry lines: overflow
// there is the discarded top bits, and a shortfall is zero-filled.
func insertConcatCarryLines[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], chunks []partCat,
) []partCat {
	var (
		nChunks = uint(len(chunks))
		// Count number of chunks for which a carry line makes sense.
		nRealChunks = determineRealChunks(chunks)
	)
	//
	for i := uint(0); i < nChunks; i++ {
		var (
			// Determine bitwidth of left-hand side
			lhs = descriptor.BitwidthOf(alloc, chunks[i].targets...).Unwrap()
			// Determine bitwidth of right-hand side
			rhs = descriptor.BitwidthOf(alloc, chunks[i].sources...).Unwrap()
		)
		// Check whether carry required.  NOTE: a chunk whose target limbs are
		// exhausted has a zero-width left-hand side.  In such case, there is no
		// value in adding carry lines.  Furthermore, doing so doesn't make
		// sense since the carry lines would be as wide the rhs anyway.
		if lhs > 0 && lhs < rhs && i+1 < nRealChunks {
			var (
				bitwidth = rhs - lhs
				// allocate new carry line
				carry = alloc.Allocate("c", util.Some(bitwidth))
			)
			// insert carry line.  Observe that, since the carry holds the
			// overflowing (most significant) bits of this chunk, it becomes
			// the least significant limb of the next chunk.
			chunks[i].targets = append(chunks[i].targets, carry)
			chunks[i+1].sources = array.Prepend(carry, chunks[i+1].sources)
			// Sanity check the next chunk still fits.  This holds because
			// initialiseConcatChunks reserved exactly this many bits
			// (the overhang) when selecting its sources.
			util.Assert(descriptor.BitwidthOf(alloc, chunks[i+1].sources...).Unwrap() <= mapping.BandWidth(),
				"carry overflow")
		} else if lhs > rhs && i+1 < nChunks && len(chunks[i+1].sources) > 0 {
			var (
				bitwidth = lhs - rhs
				// allocate new borrow line
				borrow = alloc.Allocate("b", util.Some(bitwidth))
			)
			// insert borrow line.  Observe that, since the borrow supplies the
			// missing (most significant) bits of this chunk, it is assigned
			// from the least significant bits of the next chunk.  The next
			// chunk must have sources, otherwise the borrow is always zero and
			// the shortfall is simply zero-filled instead.
			chunks[i].sources = append(chunks[i].sources, borrow)
			chunks[i].borrows = true
			chunks[i+1].targets = array.Prepend(borrow, chunks[i+1].targets)
			// Sanity check the next chunk still fits.  This holds because
			// initialiseConcatChunks reserved exactly this many bits
			// (the underhang) when selecting its targets.
			util.Assert(descriptor.BitwidthOf(alloc, chunks[i+1].targets...).Unwrap() <= mapping.BandWidth(),
				"borrow overflow")
		}
	}
	//
	return chunks
}

func determineRealChunks(chunks []partCat) (num uint) {
	var nChunks = uint(len(chunks))
	//
	for i := nChunks; i > 0; i-- {
		if len(chunks[i-1].targets) > 0 {
			return i
		}
	}
	// No trailing targetless chunks
	return nChunks
}

// concatAssignment lowers a chunk back into a concrete assignment instruction.
func concatAssignment[W word.Word[W]](chunk partCat, alloc Allocator[W]) (codes []Bytecode[W]) {
	var (
		zero      W
		targets   = chunk.targets
		noSources = len(chunk.sources) == 0
		noTargets = len(chunk.targets) == 0
	)

	if noSources && noTargets {
		return codes
	} else if noSources {
		return append(codes, bytecode.LoadConstVec(chunk.targets, zero))
	} else if noTargets {
		targets = []RegisterId{alloc.ZeroRegister()}
	}
	// Done
	return append(codes, bytecode.AssignV[W](targets, chunk.sources...))
}
