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

import "github.com/LFDT-Lineth/zkc/pkg/util/word"

// Predicate abstracts the notion of a function which identifies something.
type Predicate[T any] = func(T) bool

// Array provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type Array[T any] interface {
	// Return the number of bits required to store an element of this array.
	BitWidth() uint
	// Clone this array producing a mutable copy
	Clone() MutArray[T]
	// Get returns the element at the given index in this array.
	Get(uint) T
	// Returns the number of elements in this array.
	Len() uint
	// Slice out a subregion of this array.
	Slice(uint, uint) Array[T]
}

// MutArray provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type MutArray[T any] interface {
	Array[T]
	// Append new element onto the end of array producing an updated array.  In
	// most cases, this updates the array in place and returns it.  In some
	// cases, it may force a change of representation (e.g. moving from a
	// constant array).
	Append(T) MutArray[T]
	// Set the element at the given index in this array, overwriting the
	// original value. In most cases, this updates the array in place and
	// returns it.  In some cases, it may force a change of representation (e.g.
	// moving from a constant array).
	Set(uint, T) MutArray[T]
	// Insert n copies of T at start of the array and m copies at the back. In
	// most cases, this updates the array in place and returns it.  In some
	// cases, it may force a change of representation (e.g. moving from a
	// constant array).
	Pad(uint, uint, T) MutArray[T]
}

// CloneArray converts a word array for one word geometry into a mutable array
// for another geometry.
func CloneArray[W1 word.Word[W1], W2 word.Word[W2]](arr Array[W1], builder Builder[W2]) MutArray[W2] {
	var res = builder.NewArray(arr.Len(), arr.BitWidth())
	//
	for i := range arr.Len() {
		var (
			w1 = arr.Get(i)
			w2 W2
		)
		// Convert words
		w2 = w2.SetBytes(w1.Bytes())
		// Assign into new array
		res = res.Set(i, w2)
	}
	// Done
	return res
}
