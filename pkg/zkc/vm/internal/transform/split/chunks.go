// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.
//
// SPDX-License-Identifier: Apache-2.0

package split

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ChunkMutator represents a map function which takes a chunk, changes it in some
// way, and returns the updated chunk.
type ChunkMutator[W word.Word[W]] func(Chunk[W]) Chunk[W]

// ChunkMapper represents a map function which takes a chunk and maps it into
// some other datatype.
type ChunkMapper[W word.Word[W], T any] func(Chunk[W]) T

// Chunk represents an intermediate assignment of the form xn::x0 =
// f(c,y0,...,yn).  Chunks eventually become real assignments.
type Chunk[W word.Word[W]] struct {
	LeftHandSide  []RegisterId
	RightHandSide []RegisterId
	Constant      W
}

// LhsBitwidth returns the number of bits which the left-hand side of the chunk
// can hold.  None is returned if the lhs is a native register.
func (p Chunk[W]) LhsBitwidth(mapping descriptor.RegisterMap[W]) uint {
	var bitwidth uint
	// Determine maximum expressible value
	for _, r := range p.LeftHandSide {
		reg := mapping.Register(r)
		// Check for native registers
		if reg.IsNative() {
			panic("cannot split field element")
		}
		// Accumulate bitwidth
		bitwidth += reg.Bitwidth().Unwrap()
	}
	//
	return bitwidth
}

// Chunks encapsulates an array of chunks
type Chunks[W word.Word[W]] struct {
	chunks []Chunk[W]
}

// Append appends a new chunk (created using the given mutator on an empty
// chunk) to the end of this array.
func (p *Chunks[W]) Append(fn ChunkMutator[W]) {
	p.chunks = append(p.chunks, fn(Chunk[W]{}))
}

// Apply a given function to a given chunk, whilst allocating empty chunks as
// needed to ensure the given chunk exists.
func (p *Chunks[W]) Apply(chunk uint, fn ChunkMutator[W]) {
	// Ensure enough chunks
	p.chunks = array.BackPad(p.chunks, chunk+1, Chunk[W]{})
	// Allocate limb
	p.chunks[chunk] = fn(p.chunks[chunk])
}

// Ith returns the ith chunk in this array, creating empty chunks as needed to
// satisfy this request.
func (p *Chunks[W]) Ith(chunk uint) Chunk[W] {
	// Ensure enough chunks
	p.chunks = array.BackPad(p.chunks, chunk+1, Chunk[W]{})
	// Return chunk
	return p.chunks[chunk]
}

// Len returns the number of chunks in this array.
func (p *Chunks[W]) Len() uint {
	return uint(len(p.chunks))
}

// String returns a compact debugging representation of the chunk sequence.
func (p *Chunks[W]) String() string {
	var builder strings.Builder
	//
	builder.WriteString("[")
	//
	for i, c := range p.chunks {
		if i != 0 {
			builder.WriteString(";")
		}
		//
		builder.WriteString(chunkToString(c))
	}
	//
	builder.WriteString("]")
	//
	return builder.String()
}

// MapChunks maps each chunk to a given type T using the provided mapper.
func MapChunks[W word.Word[W], T any](chunks Chunks[W], fn ChunkMapper[W, T]) []T {
	var items = make([]T, chunks.Len())
	//
	for i, c := range chunks.chunks {
		items[i] = fn(c)
	}
	//
	return items
}

// chunkToString formats a single chunk using register numbers and a
// hexadecimal constant for debugging output.
func chunkToString[W word.Word[W]](p Chunk[W]) string {
	var builder strings.Builder
	//
	builder.WriteString("(")
	// Write lhs
	for i, r := range array.Reverse(p.LeftHandSide) {
		if i != 0 {
			builder.WriteString("::")
		}
		//
		fmt.Fprintf(&builder, "r%d", r)
	}
	//
	builder.WriteString(":=")
	// Write rhs
	for _, r := range p.RightHandSide {
		fmt.Fprintf(&builder, "r%d", r)
		builder.WriteString(",")
	}
	//
	builder.WriteString("0x")
	builder.WriteString(p.Constant.Text(16))
	builder.WriteString(")")
	//
	return builder.String()
}

// maxValueOf determines the maximum unsigned integer value expressible in a
// given number of bits.
func maxValueOf(bitwidth util.Option[uint]) *big.Int {
	if bitwidth.IsEmpty() {
		panic("cannot split field element")
	}
	//
	var val = big.NewInt(1)
	// NOTE: safe cast given check above
	val.Lsh(val, bitwidth.Unwrap())
	//
	val.Sub(val, big.NewInt(1))
	//
	return val
}

// setLhsLimbs returns a mutator which replaces the left-hand side limbs of a
// chunk while preserving its right-hand side and constant.
func setLhsLimbs[W word.Word[W]](lhs ...RegisterId) ChunkMutator[W] {
	return func(c Chunk[W]) Chunk[W] {
		return Chunk[W]{
			LeftHandSide:  lhs,
			RightHandSide: c.RightHandSide,
			Constant:      c.Constant,
		}
	}
}

// setLhsLimbs returns a mutator which replaces the left-hand side limbs of a
// chunk while preserving its right-hand side and constant.
func setRhsLimbs[W word.Word[W]](rhs ...RegisterId) ChunkMutator[W] {
	return func(c Chunk[W]) Chunk[W] {
		return Chunk[W]{
			LeftHandSide:  c.LeftHandSide,
			RightHandSide: rhs,
			Constant:      c.Constant,
		}
	}
}

// appendRhsLimb returns a mutator which appends one source limb to a chunk's
// right-hand side while preserving its targets and constant.
func appendRhsLimb[W word.Word[W]](limb RegisterId) ChunkMutator[W] {
	return func(c Chunk[W]) Chunk[W] {
		return Chunk[W]{
			LeftHandSide:  c.LeftHandSide,
			RightHandSide: append(c.RightHandSide, limb),
			Constant:      c.Constant,
		}
	}
}

// appendLhsLimb returns a mutator which appends one target limb to a chunk's
// left-hand side whilst preserving the rest.
func appendLhsLimb[W word.Word[W]](limb RegisterId) ChunkMutator[W] {
	return func(c Chunk[W]) Chunk[W] {
		return Chunk[W]{
			LeftHandSide:  append(c.LeftHandSide, limb),
			RightHandSide: c.RightHandSide,
			Constant:      c.Constant,
		}
	}
}

// setRhsConstant returns a mutator which replaces a chunk's constant while
// preserving its target and source limbs.
func setRhsConstant[W word.Word[W]](constant W) ChunkMutator[W] {
	return func(c Chunk[W]) Chunk[W] {
		return Chunk[W]{
			LeftHandSide:  c.LeftHandSide,
			RightHandSide: c.RightHandSide,
			Constant:      constant,
		}
	}
}
