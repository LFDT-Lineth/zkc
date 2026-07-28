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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// Predicate abstracts the notion of a function which identifies something.
type Predicate[T any] = func(T) bool

// Array provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type Array[T any] interface {
	fmt.Stringer
	// Return the number of bits required to store an element of this array.
	BitWidth() uint
	// Clone this array producing a mutable copy
	Clone() MutArray[T]
	// Get returns the element at the given index in this array.
	Get(uint) T
	// Returns the number of elements in this array.
	Len() uint
}

// MutArray provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type MutArray[T any] interface {
	Array[T]
	// Append new element onto the end of array producing an updated array.
	// This updates the array in place, and will panic if the given value is not
	// representable in the array.
	Append(T)
	// Set the element at the given index in this array, overwriting the
	// original value. This updates the array in place, and will panic if the
	// given value is not representable in the array.
	Set(uint, T)
	// ToLegacy (efficiently) constructs an equivalent legacy array from this
	// array.  This should be considered a destructive operation and, hence,
	// once done this array should no longer be used.
	ToLegacy() array.MutArray[T]
}
