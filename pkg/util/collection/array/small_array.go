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

	"github.com/consensys/go-corset/pkg/util/word"
)

// SmallArray implements an array of elements simply using an underlying array.
type SmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]] struct {
	// The data stored in this column (as bytes).
	data []K
	// Bitwidth of each word in this array
	bitwidth uint
}

// NewSmallArray constructs a new word array with a given capacity.
func NewSmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]](height uint, bitwidth uint) SmallArray[K, T] {
	var (
		elements = make([]K, height)
	)
	//
	return SmallArray[K, T]{elements, bitwidth}
}

// Append new word on this array
func (p *SmallArray[K, T]) Append(word T) MutArray[T] {
	p.Pad(0, 1, word)
	//
	return p
}

// Len returns the number of elements in this word array.
func (p *SmallArray[K, T]) Len() uint {
	//
	return uint(len(p.data))
}

// BitWidth returns the width (in bits) of elements in this array.
func (p *SmallArray[K, T]) BitWidth() uint {
	return p.bitwidth
}

// Clone makes clones of this array producing an otherwise identical copy.
func (p *SmallArray[K, T]) Clone() MutArray[T] {
	// Allocate sufficient memory
	ndata := make([]K, uint(len(p.data)))
	// Copy over the data
	copy(ndata, p.data)
	//
	return &SmallArray[K, T]{ndata, p.bitwidth}
}

// Get returns the word at the given index in this array.
func (p *SmallArray[K, T]) Get(index uint) T {
	var val T
	//
	return val.SetUint64(uint64(p.data[index]))
}

// Set the word at the given index in this array, overwriting the
// original value.
func (p *SmallArray[K, T]) Set(index uint, word T) MutArray[T] {
	p.data[index] = K(word.Uint64())
	return p
}

// SetRaw sets a raw value at the given index in this array, overwriting the
// original value.
func (p *SmallArray[K, T]) SetRaw(index uint, val K) {
	p.data[index] = val
}

// Slice out a subregion of this array.
func (p *SmallArray[K, T]) Slice(start uint, end uint) Array[T] {
	return &SmallArray[K, T]{
		p.data[start:end], p.bitwidth,
	}
}

// Pad prepend array with n copies and append with m copies of the given padding
// value.
func (p *SmallArray[K, T]) Pad(n uint, m uint, padding T) MutArray[T] {
	var (
		// Determine new length
		l = n + m + p.Len()
		// Initialise new array
		data = make([]K, l)
		//
		val = K(padding.Uint64())
	)
	// copy
	copy(data[n:], p.data)
	p.data = data
	// Front padding!
	for i := range n {
		p.data[i] = val
	}
	// Back padding!
	for i := l - m; i < l; i++ {
		p.data[i] = val
	}
	//
	return p
}

func (p *SmallArray[K, T]) String() string {
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
