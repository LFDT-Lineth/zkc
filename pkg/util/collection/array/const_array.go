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
	"unsafe"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// NewConstantArray constructs a new word array with a given capacity.
func NewConstantArray[T word.Word[T]](height uint, bitwidth uint, value T) *ConstantArray[T] {
	if !value.FitsWithin(bitwidth) {
		panic(fmt.Sprintf("invalid constant value (%s) for u%d", value.String(), bitwidth))
	}
	//
	return &ConstantArray[T]{height, bitwidth, value}
}

// =================================================================================
// Implementation
// =================================================================================

// ConstantArray implements an array of a constant value.
type ConstantArray[T word.Word[T]] struct {
	// Actual height of column
	height uint
	// Bitwidth of column
	bitwidth uint
	// Constant value in play
	value T
}

// Bytes implementation for Array interface
func (p ConstantArray[T]) Bytes() uint {
	var tmp T
	//
	return uint(unsafe.Sizeof(tmp))
}

// Len returns the number of elements in this word array.
func (p ConstantArray[T]) Len() uint {
	return p.height
}

// BitWidth returns the width (in bits) of elements in this array.
func (p ConstantArray[T]) BitWidth() uint {
	return p.bitwidth
}

// Get returns the field element at the given index in this array.
func (p ConstantArray[T]) Get(index uint) T {
	return p.value
}

// Pad implementation for MutArray interface.  The receiver is left
// unmodified.
func (p ConstantArray[T]) Pad(n uint) Array[T] {
	var zero T
	//
	if !p.value.Equals(zero) {
		return NewPaddedArray(p).Pad(n)
	}
	//
	return &ConstantArray[T]{p.height + n, p.bitwidth, p.value}
}

func (p ConstantArray[T]) String() string {
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
func (p *ConstantArray[T]) Append(word T) MutArray[T] {
	if word.Cmp(p.value) == 0 {
		p.height++
		return p
	}
	// Determine necessary bitwidth
	var (
		bitwidth = max(p.bitwidth, bitwidthOf(word))
		q        MutArray[T]
	)
	// Resize column
	switch {
	case bitwidth == 1:
		q = NewBitArray[T](p.height, p.value.Cmp64(1) == 0)
	case bitwidth <= 8:
		q = NewSmallArray[uint8, T](bitwidth, p.height, uint8(p.value.Uint64()))
	case bitwidth <= 16:
		q = NewSmallArray[uint16, T](bitwidth, p.height, uint16(p.value.Uint64()))
	case bitwidth <= 32:
		q = NewSmallArray[uint32, T](bitwidth, p.height, uint32(p.value.Uint64()))
	case bitwidth <= 64:
		q = NewSmallArray[uint64, T](bitwidth, p.height, p.value.Uint64())
	default:
		var arr = Fill(p.height, p.value)
		//
		return NewStaticArray[T](bitwidth, append(arr, word)...)
	}
	//
	return q.Append(word)
}

// AppendAll elements of the given bit array onto the this array, mutating it
// in place.
func (p *ConstantArray[T]) AppendAll(other ConstantArray[T]) {
	if p.value.Cmp(other.value) != 0 {
		panic(fmt.Sprintf("cannot append %s onto constant array for %s", p.value.String(), other.value.String()))
	}
	// NOTE: attempting to assign a constant register any value other than the
	// given constant cannot change the value stored in the register.  This just
	// means that a constraint somewhere should fail
	p.height += other.height
}

// Build implementation for the array.Builder interface.  This simply means that
// a static array is its own builder.
func (p *ConstantArray[T]) Build() Array[T] {
	return p
}

// Height implementation of MutArray interface
func (p *ConstantArray[T]) Height() uint {
	return p.Len()
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *ConstantArray[T]) Set(index uint, word T) {
	// NOTE: attempting to assign a constant register any value other than the
	// given constant cannot change the value stored in the register.  This just
	// means that a constraint somewhere should fail
}
