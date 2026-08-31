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
	"unsafe"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// NewSmallArray constructs a new word array with a given capacity.
func NewSmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]](bitwidth uint, height uint, value K,
) *SmallArray[K, T] {
	return &SmallArray[K, T]{Fill(height, value), bitwidth}
}

// =================================================================================
// Implementation
// =================================================================================

// SmallArray implements an array of elements simply using an underlying array.
type SmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]] struct {
	// The data stored in this column (as bytes).
	data []K
	// Bitwidth of each word in this array
	bitwidth uint
}

// Bytes implementation for Array interface
func (p SmallArray[K, T]) Bytes() uint {
	var (
		tmp K
		n   = uint(unsafe.Sizeof(tmp))
	)
	//
	return n * p.Len()
}

// Len returns the number of elements in this word array.
func (p SmallArray[K, T]) Len() uint {
	//
	return uint(len(p.data))
}

// BitWidth returns the width (in bits) of elements in this array.
func (p SmallArray[K, T]) BitWidth() uint {
	return p.bitwidth
}

// Get returns the word at the given index in this array.
func (p SmallArray[K, T]) Get(index uint) T {
	var val T
	//
	return val.SetUint64(uint64(p.data[index]))
}

// Pad returns a copy of this array with n copies of the given padding value
// prepended, and m copies appended.  The receiver is left unmodified.
func (p SmallArray[K, T]) Pad(n uint) Array[T] {
	return NewPaddedArray(p).Pad(n)
}

func (p SmallArray[K, T]) String() string {
	var sb strings.Builder

	sb.WriteString("[")

	for i := range p.Len() {
		if i != 0 {
			sb.WriteString(",")
		}

		fmt.Fprintf(&sb, "%v", p.data[i])
	}

	sb.WriteString("]")

	return sb.String()
}

// =================================================================================
// MutArray Implementation
// =================================================================================

// Append new word on this array
func (p *SmallArray[K, T]) Append(word T) MutArray[T] {
	p.data = append(p.data, K(word.Uint64()))
	//
	return p
}

// AppendAll elements of the given array onto the this array, mutating it in
// place.
func (p *SmallArray[K, T]) AppendAll(other SmallArray[K, T]) {
	// Determine height of resulting array
	var (
		nsize = uint(len(p.data) + len(other.data))
		n     = nsize - uint(len(p.data))
	)
	// sanity check
	if p.bitwidth != other.bitwidth {
		panic(fmt.Sprintf("incompatible array bitwidth (u%d vs u%d)", p.bitwidth, other.bitwidth))
	}
	// expand data length
	ndata := slices.Grow(p.data, int(n))[:nsize]
	// copy data
	copy(ndata[len(p.data):], other.data)
	// finalisex
	p.data = ndata
}

// Build implementation for the array.Builder interface.  This simply means that
// a static array is its own builder.
func (p *SmallArray[K, T]) Build() Array[T] {
	return p
}

// Height implementation of MutArray interface
func (p *SmallArray[K, T]) Height() uint {
	return p.Len()
}

// Set the word at the given index in this array, overwriting the
// original value.
func (p *SmallArray[K, T]) Set(index uint, word T) {
	p.data[index] = K(word.Uint64())
}
