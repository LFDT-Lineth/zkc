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
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
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
func (p *StaticArray[T]) Append(word T) {
	p.data = append(p.data, word)
}

// AppendAll elements of the given array onto the this array, mutating it in
// place.
func (p *StaticArray[T]) AppendAll(other StaticArray[T]) {
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
func (p *StaticArray[T]) Set(index uint, word T) {
	p.data[index] = word
}

// Encode implementation for Array interface.  The natural encoding of a static
// array is its elements written as length-prefixed sequences of raw bytes.
func (p *StaticArray[T]) Encode(buffer *bytes.Buffer) {
	for _, w := range p.data {
		writeWordBytes(buffer, w.Bytes())
	}
}

// Decode implementation for MutArray interface.  This reads a given number of
// length-prefixed words (as produced by Encode).
func (p *StaticArray[T]) Decode(height uint, buffer *bytes.Buffer) error {
	data := make([]T, height)
	//
	for i := range data {
		bs, err := readWordBytes(buffer)
		//
		if err != nil {
			return err
		}
		//
		data[i] = data[i].SetBytes(bs)
	}
	//
	p.data = data
	//
	return nil
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

// Pad returns a copy of this array with n copies of the given padding value
// prepended, and m copies appended.  The receiver is left unmodified.
func (p *StaticArray[T]) Pad(n uint, m uint, padding T) MutArray[T] {
	var (
		ol = p.Len()
		// Determine new length
		l = n + ol + m
		// Allocate exactly, copying existing data directly into its final
		// position.
		data = make([]T, l)
	)
	//
	copy(data[n:], p.data)
	// Front padding!
	for i := range n {
		data[i] = padding
	}
	// Back padding!
	for i := l - m; i < l; i++ {
		data[i] = padding
	}
	//
	return &StaticArray[T]{data, p.bitwidth}
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
