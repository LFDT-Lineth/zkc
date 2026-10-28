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
package enum

// Predicate abstracts the notion of a function which identifies something.
type Predicate[T any] = func(T) bool

// Enumerator abstracts the process of iterating over a sequence of elements.
type Enumerator[T any] interface {
	// Check whether or not there are any items remaining to visit.
	HasNext() bool

	// Get the next item, and advanced the iterator.
	Next() T

	// Get the nth item in this iterator, where 0 refers to the next items, 1 to
	// the item after that, etc.  This will mutate the iterator.
	Nth(uint) T

	// Count the number of items left.  Note, this does not modify the iterator.
	Count() uint
}

// Find provides a default implementation of Iterator.Find which can be used by
// other iterator implementations.
//
//nolint:revive
func Find[T any, S Enumerator[T]](iter S, predicate Predicate[T]) (uint, bool) {
	index := uint(0)

	for iter.HasNext() {
		if predicate(iter.Next()) {
			return index, true
		}

		index++
	}
	// Failed to find it
	return 0, false
}

// Nth provides a default implementation of Iterator.Nth which can be used by
// other iterator implementations.
//
//nolint:revive
func Nth[T any, S Enumerator[T]](iter S, n uint) T {
	index := uint(0)

	for iter.HasNext() {
		ith := iter.Next()
		if index == n {
			return ith
		}

		index++
	}
	// Issue!
	panic("iterator out-of-bounds")
}

// Count provides a default implementation of Iterator.Count which can be used by
// other iterator implementations.
//
//nolint:revive
func Count[T any, S Enumerator[T]](iter S) uint {
	count := uint(0)

	for iter.HasNext() {
		iter.Next()
		//
		count++
	}
	// Issue!
	return count
}

// Collect provides a default implementation of Iterator.Collect which can be used by
// other iterator implementations.
//
//nolint:revive
func Collect[T any, S Enumerator[T]](iter S) []T {
	var items = make([]T, 0)
	//
	for iter.HasNext() {
		items = append(items, iter.Next())
	}
	//
	return items
}
