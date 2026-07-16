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
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// partCat "partial cat" represents an concatenation created during splitting which
// contributes towards the overall concatenation.
type partCat struct {
	targets []RegisterId
	sources []RegisterId
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
func Concat[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	insn *bytecode.Cat[W]) []Bytecode[W] {
	// Split into the initial set of chunks.
	var chunks, context = initialiseConcatChunks(mapping, alloc, insn.Targets, insn.Sources)
	// Next, add carry lines as needed
	chunks = insertConcatCarryLines(mapping.Field(), alloc, chunks)
	// Convert chunks into assignments
	return append(array.Map(chunks, concatAssignment[W]), context...)
}

// initialiseAddChunks splits the addition sources and constant into
// least-significant-first chunks, then assigns target limbs to each chunk
// according to the number of bits the corresponding RHS can produce.
func initialiseConcatChunks[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	targets, sources []RegisterId) ([]partCat, []Bytecode[W]) {
	//
	var (
		limbsMap = mapping.LimbsRegisterMap()
		// Split all source registers
		sourceLimbs = applyLimbsMapReversed(mapping, sources...)
		// Split all target registers
		targetLimbs = applyLimbsMapReversed(mapping, targets...)
		// Initialise target register targetStack
		targetStack = RegisterStack[W]{targetLimbs, alloc, nil}
		// Initialise source register stack
		sourceStack = RegisterStack[W]{sourceLimbs, alloc, nil}
		//
		codes []partCat
	)
	// Keep going whilst we still have source registers
	for sourceStack.Size() > 0 {
		var (
			rhs      = sourceStack.SelectUpto(mapping.BandWidth())
			bitwidth = concatRhsBitwidth(mapping.Field(), rhs, limbsMap)
			lhs      []RegisterId
		)
		// Check if last chunk
		if sourceStack.Size() == 0 {
			//In this case, we might as well select everything which is left.
			lhs = targetStack.SelectUpto(mapping.BandWidth() - bitwidth)
		} else {
			lhs = targetStack.SelectExact(bitwidth)
		}
		// allocate selected targets
		codes = append(codes, partCat{lhs, rhs})
	}
	// Handle cases where we have more targets than necessary.  This can arise
	// under normal circumstances, such as when assigning a small constant to a
	// wide target register.  In this case, we simple assign each target in this
	// "overhang" to zero.
	for targetStack.Size() > 0 {
		codes = append(codes, partCat{[]RegisterId{targetStack.Pop()}, nil})
	}
	// Assert that we never create bytecodes for source registers.
	util.Assert(len(sourceStack.post) == 0, "internal failure")
	//
	return codes, targetStack.post
}

// insertConcatCarryLines allocates carry registers for chunks whose RHS
// produces more bits than its LHS can hold, splicing each carry into the
// current chunk's LHS and the next chunk's RHS.  The final chunk is skipped
// since its overflow represents the top bits with no successor to absorb it.
func insertConcatCarryLines[W word.Word[W]](field field.Config, alloc Allocator[W], chunks []partCat) []partCat {
	//
	for i := range len(chunks) {
		var (
			// Determine bitwidth of left-hand side
			lhs = descriptor.BitwidthOf(alloc, chunks[i].targets...).Unwrap()
			// Determine bitwidth of right-hand side
			rhs = concatRhsBitwidth(field, chunks[i].sources, alloc)
		)
		// check whether carry required
		if lhs < rhs && i+1 < len(chunks) {
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
		}
	}
	//
	return chunks
}

func concatRhsBitwidth[W word.Word[W]](field field.Config, chunk []RegisterId, mapping descriptor.RegisterMap[W]) uint {
	var bitwidth uint
	// Handle native registers on the rhs
	if descriptor.HasNativeRegisterId(chunk, mapping) {
		return field.BandWidth
	}
	//
	for _, r := range chunk {
		var reg = mapping.Register(r)
		//
		bitwidth += reg.Bitwidth().Unwrap()
	}
	//
	return bitwidth
}

// addAssignment lowers a chunk back into a concrete unsigned-add instruction.
func concatAssignment[W word.Word[W]](_ uint, chunk partCat) Bytecode[W] {
	var zero W

	if len(chunk.sources) == 0 {
		return bytecode.LoadConstVec(chunk.targets, zero)
	}
	// Done
	return bytecode.Concat[W](chunk.targets, chunk.sources)
}
