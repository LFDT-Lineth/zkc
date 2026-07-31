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
package array

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// bitOne is the byte-level binary representation of 1.
var bitOne = []byte{1}

// =================================================================================
// Implementation
// =================================================================================

// BitArray implements an array of single bit words simply using an underlying
// array of packed bytes.  That is, where eight bits are packed into a single
// byte.
type BitArray[T word.Word[T]] struct {
	// The data stored in this column (as bytes).
	data []byte
	// Actual height of column
	height uint
}

// NewBitArray constructs a new bit array with a given capacity.
func NewBitArray[T word.Word[T]](height uint) BitArray[T] {
	var (
		bytewidth = word.ByteWidth(height)
		elements  = make([]byte, bytewidth)
	)
	//
	return BitArray[T]{elements, height}
}

// RawBitArray constructs a new bit array directly from raw data.
func RawBitArray[T word.Word[T]](data []byte, height uint) *BitArray[T] {
	return &BitArray[T]{data, height}
}

// Len returns the number of elements in this word array.
func (p *BitArray[T]) Len() uint {
	return p.height
}

// BitWidth returns the width (in bits) of elements in this array.
func (p *BitArray[T]) BitWidth() uint {
	return 1
}

// Clone makes clones of this array producing an otherwise identical copy.
func (p *BitArray[T]) Clone() MutArray[T] {
	// Allocate sufficient memory
	ndata := make([]byte, uint(len(p.data)))
	// Copy over the data
	copy(ndata, p.data)
	//
	return &BitArray[T]{ndata, p.height}
}

// Get returns the field element at the given index in this array.
func (p *BitArray[T]) Get(index uint) T {
	var b T
	//
	if bit.LittleEndianRead(p.data, index) {
		return b.SetBytes(bitOne)
	}
	// Default is zero
	return b
}

// Pad implementation for MutArray interface.
func (p *BitArray[T]) Pad(n uint, m uint, padding T) {
	// Front padding
	if n > 0 {
		p.insertBits(n, padding)
	}
	// Back padding
	if m > 0 {
		p.appendBits(m, padding)
	}
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *BitArray[T]) Set(index uint, word T) MutArray[T] {
	// if byte length is 0, the word represents 0.  otherwise, it must be 1.
	var val = !word.IsZero()
	//
	bit.LittleEndianWrite(val, p.data, index)
	//
	return p
}

// SetRaw sets a raw bit at the given index in this array, overwriting the
// original value.
func (p *BitArray[T]) SetRaw(index uint, val bool) {
	bit.LittleEndianWrite(val, p.data, index)
}

// Slice out a subregion of this array.
func (p *BitArray[T]) Slice(start uint, end uint) Array[T] {
	var (
		height    = end - start
		bytewidth = word.ByteWidth(height)
	)
	// Check for aligned slice (since this is a fast case).
	if start%8 == 0 {
		// Yes, easy case
		start = start / 8
		//
		return &BitArray[T]{p.data[start : start+bytewidth], height}
	}
	// No, hard case.  We'll just do a bitcopy for now.  In theory we could
	// improve performance by allowing BitArray to have a starting offset.  But,
	// the use cases for Slice() are very limited at this time, so no need.
	bytes := make([]byte, bytewidth)
	// Copy height bits over
	bit.LittleEndianCopy(p.data, start, bytes, 0, height)
	// Done
	return &BitArray[T]{bytes, height}
}

func (p *BitArray[T]) String() string {
	var sb strings.Builder

	sb.WriteString("[")

	for i := range p.Len() {
		if i != 0 {
			sb.WriteString(",")
		}

		fmt.Fprintf(&sb, "%v", p.Get(i))
	}

	sb.WriteString("]")

	return sb.String()
}

func (p *BitArray[T]) insertBits(n uint, padding T) {
	var (
		height    = p.height + n
		bytewidth = word.ByteWidth(height)
		data      = p.data
	)
	//
	if uint(cap(data)) < bytewidth {
		// Insufficient capacity: allocate exactly, copying existing bits
		// directly into their final position.
		data = make([]byte, bytewidth)
		bit.LittleEndianCopy(p.data, 0, data, n, p.height)
	} else {
		// Sufficient capacity: extend and shift in place.  Freshly exposed
		// bytes are zeroed so that bits beyond the new height read as zero
		// (the shift only ever moves zeros into them).
		oldwidth := uint(len(data))
		data = data[:bytewidth]
		clear(data[oldwidth:])
		//
		shiftBitsRight(data, p.height, n)
	}
	//
	p.data = data
	// assign
	for i := range n {
		p.Set(i, padding)
	}
	// done
	p.height = height
}

func (p *BitArray[T]) appendBits(n uint, padding T) {
	var (
		height    = p.height + n
		bytewidth = word.ByteWidth(height)
		data      = p.data
	)
	//
	if uint(cap(data)) < bytewidth {
		// Insufficient capacity: allocate exactly, copying existing data
		// directly into place.
		data = make([]byte, bytewidth)
		copy(data, p.data)
	} else {
		// Sufficient capacity: extend in place, zeroing freshly exposed bytes
		// so that bits beyond the new height read as zero.
		oldwidth := uint(len(data))
		data = data[:bytewidth]
		clear(data[oldwidth:])
	}
	//
	p.data = data
	// assign
	for i := p.height; i < height; i++ {
		p.Set(i, padding)
	}
	// done
	p.height = height
}

// shiftBitsRight shifts the first height bits of data right (i.e. towards
// higher bit offsets) by n positions, in place.  Bytes are processed from the
// most significant end backwards, so the overlapping source and destination
// regions are handled correctly (unlike bit.LittleEndianCopy, which copies
// forwards).  The vacated low n bits are left holding garbage, which callers
// are expected to overwrite.
func shiftBitsRight(data []byte, height uint, n uint) {
	if height == 0 || n == 0 {
		return
	}
	//
	var (
		// Whole-byte and residual components of the shift.
		k = n / 8
		s = n % 8
		// Last byte holding a shifted bit.
		last = (height + n - 1) / 8
	)
	// NOTE: when s == 0 the second term shifts by eight which, in Go, yields
	// zero — degenerating into a pure byte move.
	for j := last; j > k; j-- {
		data[j] = (data[j-k] << s) | (data[j-k-1] >> (8 - s))
	}
	// Lowest destination byte has no byte below it.
	data[k] = data[0] << s
}
