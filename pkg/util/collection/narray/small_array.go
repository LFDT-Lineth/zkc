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
package narray

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// SmallArray implements an array of elements simply using an underlying array.
type SmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]] struct {
	// The data stored in this column (as bytes).
	data []K
	// Bitwidth of each word in this array
	bitwidth uint
}

// NewSmallArray constructs a new word array with a given capacity.
func NewSmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]](height uint, bitwidth uint) *SmallArray[K, T] {
	var (
		elements = make([]K, height)
	)
	//
	return &SmallArray[K, T]{elements, bitwidth}
}

// Append new word on this array
func (p *SmallArray[K, T]) Append(word T) {
	p.data = append(p.data, K(word.Uint64()))
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

// Encode implementation for Array interface.  The natural encoding of a small
// array is its elements written as fixed-width, little endian values.
func (p *SmallArray[K, T]) Encode(buffer *bytes.Buffer) {
	if err := binary.Write(buffer, binary.LittleEndian, p.data); err != nil {
		// Unreachable, since writes to a bytes.Buffer cannot fail.
		panic(err)
	}
}

// Decode implementation for MutArray interface.  This reads a given number of
// fixed-width, little endian values (as produced by Encode).
func (p *SmallArray[K, T]) Decode(height uint, buffer *bytes.Buffer) error {
	data := make([]K, height)
	//
	if err := binary.Read(buffer, binary.LittleEndian, data); err != nil {
		return err
	}
	//
	p.data = data
	//
	return nil
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
func (p *SmallArray[K, T]) Set(index uint, word T) {
	p.data[index] = K(word.Uint64())
}

// SetRaw sets a raw value at the given index in this array, overwriting the
// original value.
func (p *SmallArray[K, T]) SetRaw(index uint, val K) {
	p.data[index] = val
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

// ToLegacy implementation for MutArray interface
func (p *SmallArray[K, T]) ToLegacy() array.MutArray[T] {
	return array.RawSmallArray[K, T](p.data, p.bitwidth)
}
