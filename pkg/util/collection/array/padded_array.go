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
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// PaddedArray implements an array of single bit words simply using an underlying
// array of packed bytes.  That is, where eight bits are packed into a single
// byte.
type PaddedArray[T word.Word[T], S Array[T]] struct {
	data S
	// start identifies the first non-padding (i.e. real) row in this array.
	// Every row below start is padding and, therefore, implicitly zero.
	start uint
}

// NewPaddedArray constructs a new word array with a given capacity.
func NewPaddedArray[T word.Word[T], S Array[T]](data S) PaddedArray[T, S] {
	return PaddedArray[T, S]{data, 0}
}

// Bytes implementation for Array interface
func (p PaddedArray[W, T]) Bytes() uint {
	return p.data.Bytes()
}

// Len returns the number of elements in this word array.
func (p PaddedArray[T, S]) Len() uint {
	return p.data.Len() + p.start
}

// BitWidth returns the width (in bits) of elements in this array.
func (p PaddedArray[T, S]) BitWidth() uint {
	return 1
}

// Get returns the field element at the given index in this array.
func (p PaddedArray[T, S]) Get(index uint) T {
	var b T
	//
	if index < p.start {
		return b
	}
	//
	return p.data.Get(index - p.start)
}

// Pad returns a copy of this array with n copies of the given padding value
// prepended, and m copies appended.  The receiver is left unmodified.
func (p PaddedArray[T, S]) Pad(n uint) Array[T] {
	return PaddedArray[T, S]{p.data, p.start + n}
}

func (p PaddedArray[T, S]) String() string {
	return p.data.String()
}
