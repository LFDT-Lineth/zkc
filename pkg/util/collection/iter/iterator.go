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
package iter

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/enum"
)

// Predicate abstracts the notion of a function which identifies something.
type Predicate[T any] = func(T) bool

// Iterator is an adapter which sits on top of a BaseIterator and provides
// various useful and reusable functions.
type Iterator[T any] interface {
	enum.Enumerator[T]

	// Append another iterator onto the end of this iterator.  Thus, when all
	// items are visited in this iterator, iteration continues into the other.
	Append(Iterator[T]) Iterator[T]

	// Clone creates a copy of this iterator at the given cursor position.
	// Modifying the clone (i.e. by calling Next) iterator will not modify the
	// original.
	Clone() Iterator[T]

	// Collect allocates a new array containing all items of this iterator.
	// This drains the iterator.
	Collect() []T

	// Find returns the index of the first match for a given predicate, or
	// return false if no match is found.  This will mutate the iterator.
	Find(Predicate[T]) (uint, bool)

	// Count the number of items left.  Note, this does not modify the iterator.
	Count() uint
}
