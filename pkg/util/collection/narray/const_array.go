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
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

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

// NewConstantArray constructs a new word array with a given capacity.
func NewConstantArray[T word.Word[T]](height uint, bitwidth uint, value T) *ConstantArray[T] {
	return &ConstantArray[T]{height, bitwidth, value}
}

// Append new word on this array
func (p *ConstantArray[T]) Append(word T) {
	// NOTE: attempting to assign a constant register any value other than the
	// given constant cannot change the value stored in the register.  This just
	// means that a constraint somewhere should fail
	p.height++
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

// Clone makes clones of this array producing an otherwise identical copy.
func (p *ConstantArray[T]) Clone() MutArray[T] {
	return &ConstantArray[T]{p.height, p.bitwidth, p.value}
}

// Encode implementation for Array interface.  The natural encoding of a
// constant array is simply its constant value, written once as a
// length-prefixed sequence of raw bytes.
func (p *ConstantArray[T]) Encode(buffer *bytes.Buffer) {
	writeWordBytes(buffer, p.value.Bytes())
}

// Decode implementation for MutArray interface.  This reads the constant value
// (as produced by Encode), and sets the array to hold the given number of
// copies of it.
func (p *ConstantArray[T]) Decode(height uint, buffer *bytes.Buffer) error {
	data, err := readWordBytes(buffer)
	//
	if err != nil {
		return err
	}
	//
	p.value = p.value.SetBytes(data)
	p.height = height
	//
	return nil
}

// Len returns the number of elements in this word array.
func (p *ConstantArray[T]) Len() uint {
	return p.height
}

// BitWidth returns the width (in bits) of elements in this array.
func (p *ConstantArray[T]) BitWidth() uint {
	return p.bitwidth
}

// Build implementation for the array.Builder interface.  This simply means that
// a static array is its own builder.
func (p *ConstantArray[T]) Build() Array[T] {
	return p
}

// Get returns the field element at the given index in this array.
func (p *ConstantArray[T]) Get(index uint) T {
	return p.value
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *ConstantArray[T]) Set(index uint, word T) {
	// NOTE: attempting to assign a constant register any value other than the
	// given constant cannot change the value stored in the register.  This just
	// means that a constraint somewhere should fail
}

// Pad implementation for MutArray interface.
func (p *ConstantArray[T]) Pad(n uint, m uint, padding T) {
	if !padding.Equals(p.value) {
		// NOTE: this can be implemented by changing the representation to
		// something which can be mutated.
		panic("unsupported operation")
	}
	//
	p.height += n + m
}

func (p *ConstantArray[T]) String() string {
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
