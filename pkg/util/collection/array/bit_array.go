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
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// bitOne is the byte-level binary representation of 1.
var bitOne = []byte{1}

// NewBitArray constructs a new word array with a given capacity.
func NewBitArray[T word.Word[T]](height uint, value bool) *BitArray[T] {
	var (
		bytewidth = word.ByteWidth(height)
		elements  = make([]byte, bytewidth)
	)
	//
	if value {
		// Fill with 1s
		for i := range bytewidth {
			elements[i] = 0xff
		}
	}
	//
	return &BitArray[T]{elements, height}
}

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

// Bytes implementation for Array interface
func (p BitArray[T]) Bytes() uint {
	return uint(len(p.data))
}

// Len returns the number of elements in this word array.
func (p BitArray[T]) Len() uint {
	return p.height
}

// BitWidth returns the width (in bits) of elements in this array.
func (p BitArray[T]) BitWidth() uint {
	return 1
}

// Get returns the field element at the given index in this array.
func (p BitArray[T]) Get(index uint) T {
	var b T
	//
	if bit.LittleEndianRead(p.data, index) {
		return b.SetBytes(bitOne)
	}
	// Default is zero
	return b
}

// Pad returns a copy of this array with n copies of the given padding value
// prepended, and m copies appended.  The receiver is left unmodified.
func (p BitArray[T]) Pad(n uint) Array[T] {
	return NewPaddedArray(p).Pad(n)
}

func (p BitArray[T]) String() string {
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

// =================================================================================
// MutArray Implementation
// =================================================================================

// Append new word on this array
func (p *BitArray[T]) Append(val T) MutArray[T] {
	var (
		// if byte length is 0, the word represents 0.  otherwise, it must be 1.
		v = !val.IsZero()
		// determin expected height
		bytewidth = word.ByteWidth(p.height + 1)
	)
	// ensure sufficient space
	if uint(len(p.data)) < bytewidth {
		p.data = append(p.data, 0)
	}
	// write new bit
	bit.LittleEndianWrite(v, p.data, p.height)
	// increase height
	p.height++
	//
	return p
}

// AppendAll elements of the given bit array onto the this array, mutating it
// in place.
func (p *BitArray[T]) AppendAll(other BitArray[T]) {
	// Determine height of resulting array
	var (
		nsize = word.ByteWidth(p.height + other.height)
		n     = nsize - uint(len(p.data))
	)
	//
	ndata := slices.Grow(p.data, int(n))
	// expand data length
	p.data = ndata[:nsize]
	// fast bit copy
	bit.LittleEndianCopy(other.data, 0, p.data, p.height, other.height)
	// update height
	p.height += other.height
}

// Build implementation for MutArray interface
func (p *BitArray[T]) Build() Array[T] {
	return p
}

// Height implementation of MutArray interface
func (p *BitArray[T]) Height() uint {
	return p.Len()
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *BitArray[T]) Set(index uint, word T) {
	// if byte length is 0, the word represents 0.  otherwise, it must be 1.
	var val = !word.IsZero()
	//
	bit.LittleEndianWrite(val, p.data, index)
}
