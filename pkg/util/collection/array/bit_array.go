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

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/word"
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

// NewBitArray constructs a new word array with a given capacity.
func NewBitArray[T word.Word[T]](height uint) BitArray[T] {
	var (
		bytewidth = word.ByteWidth(height)
		elements  = make([]byte, bytewidth)
	)
	//
	return BitArray[T]{elements, height}
}

// Len returns the number of elements in this word array.
func (p *BitArray[T]) Len() uint {
	return p.height
}

// Append new word on this array
func (p *BitArray[T]) Append(word T) MutArray[T] {
	p.Pad(0, 1, word)
	return p
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
func (p *BitArray[T]) Pad(n uint, m uint, padding T) MutArray[T] {
	// Front padding
	if n > 0 {
		p.insertBits(n, padding)
	}
	// Back padding
	if m > 0 {
		p.appendBits(m, padding)
	}
	//
	return p
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
		data      = make([]byte, bytewidth)
	)
	// copy
	bit.LittleEndianCopy(p.data, 0, data, n, p.height)
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
		data      = make([]byte, bytewidth)
	)
	// copy
	copy(data, p.data)
	p.data = data
	// assign
	for i := p.height; i < height; i++ {
		p.Set(i, padding)
	}
	// done
	p.height = height
}
