// Copyright Consensys Incorporated
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

// PaddedArray wraps an underlying array with a number of (front) padding rows,
// all of which hold the same padding value.  This avoids copying the
// underlying array when padding is applied.
type PaddedArray[T word.Word[T], S Array[T]] struct {
	data S
	// start identifies the first non-padding (i.e. real) row in this array.
	// Every row below start is padding and, therefore, holds the padding value.
	start uint
	// padding value to use
	padding T
}

// NewPaddedArray constructs a new word array with a given capacity.
func NewPaddedArray[T word.Word[T], S Array[T]](data S) PaddedArray[T, S] {
	var val T
	return PaddedArray[T, S]{data, 0, val}
}

// Bytes implementation for Array interface
func (p PaddedArray[W, T]) Bytes() uint {
	return p.data.Bytes()
}

// Len returns the number of elements in this word array.
func (p PaddedArray[T, S]) Len() uint {
	return p.data.Len() + p.start
}

// BitWidth returns the width (in bits) of elements in this array.  Since the
// padding value need not fit within the bitwidth of the underlying array, this
// is the maximum of the two.
func (p PaddedArray[T, S]) BitWidth() uint {
	if p.start == 0 {
		return p.data.BitWidth()
	}
	//
	return max(p.data.BitWidth(), uint(p.padding.BigInt().BitLen()))
}

// Get returns the field element at the given index in this array.
func (p PaddedArray[T, S]) Get(index uint) T {
	if index < p.start {
		return p.padding
	}
	//
	return p.data.Get(index - p.start)
}

// Pad returns a copy of this array with n copies of the given padding value
// prepended.  The receiver is left unmodified.
func (p PaddedArray[T, S]) Pad(n uint, val T) Array[T] {
	if p.start == 0 || p.padding.Equals(val) {
		return PaddedArray[T, S]{p.data, p.start + n, val}
	}
	// Existing padding uses a different value, hence must be retained beneath
	// the new padding.
	return PaddedArray[T, Array[T]]{p, n, val}
}

func (p PaddedArray[T, S]) String() string {
	return p.data.String()
}
