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

// StaticArray implements an array of elements simply using an underlying array.
type StaticArray[T word.Word[T]] struct {
	// The data stored in this column (as bytes).
	data []T
	// Bitwidth of each word in this array
	bitwidth uint
}

// NewStaticArray constructs a new word array with a given capacity.
func NewStaticArray[T word.Word[T]](height uint, bitwidth uint) *StaticArray[T] {
	var (
		elements = make([]T, height)
	)
	//
	return &StaticArray[T]{elements, bitwidth}
}

// Append new word on this array
func (p *StaticArray[T]) Append(word T) MutArray[T] {
	return p.Pad(0, 1, word)
}

// Len returns the number of elements in this word array.
func (p *StaticArray[T]) Len() uint {
	//
	return uint(len(p.data))
}

// BitWidth returns the width (in bits) of elements in this array.
func (p *StaticArray[T]) BitWidth() uint {
	return p.bitwidth
}

// Get returns the field element at the given index in this array.
func (p *StaticArray[T]) Get(index uint) T {
	return p.data[index]
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *StaticArray[T]) Set(index uint, word T) MutArray[T] {
	p.data[index] = word
	return p
}

// Clone makes clones of this array producing an otherwise identical copy.
func (p *StaticArray[T]) Clone() MutArray[T] {
	// Allocate sufficient memory
	ndata := make([]T, uint(len(p.data)))
	// Copy over the data
	copy(ndata, p.data)
	//
	return &StaticArray[T]{ndata, p.bitwidth}
}

// Slice out a subregion of this array.
func (p *StaticArray[T]) Slice(start uint, end uint) Array[T] {
	return &StaticArray[T]{
		p.data[start:end], p.bitwidth,
	}
}

// Pad prepend array with n copies and append with m copies of the given padding
// value.
func (p *StaticArray[T]) Pad(n uint, m uint, padding T) MutArray[T] {
	var (
		// Determine new length
		l = n + m + p.Len()
		// Initialise new array
		data = make([]T, l)
	)
	// copy
	copy(data[n:], p.data)
	p.data = data
	// Front padding!
	for i := range n {
		p.Set(i, padding)
	}
	// Back padding!
	for i := l - m; i < l; i++ {
		p.Set(i, padding)
	}
	//
	return p
}

func (p *StaticArray[T]) String() string {
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
