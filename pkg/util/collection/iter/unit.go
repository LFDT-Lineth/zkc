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
package iter

type unitIterator[T any] struct {
	item  T
	index uint
}

// NewUnitIterator construct an iterator over an array of items.
func NewUnitIterator[T any](item T) *unitIterator[T] {
	return &unitIterator[T]{item, 0}
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *unitIterator[T]) HasNext() bool {
	return p.index < 1
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *unitIterator[T]) Next() T {
	p.index++
	return p.item
}

// Append another iterator onto the end of this iterator.  Thus, when all
// items are visited in this iterator, iteration continues into the other.
//
//nolint:revive
func (p *unitIterator[T]) Append(iter Iterator[T]) Iterator[T] {
	return NewAppendIterator(p, iter)
}

// Clone creates a copy of this iterator at the given cursor position. Modifying
// the clone (i.e. by calling Next) iterator will not modify the original.
//
//nolint:revive
func (p *unitIterator[T]) Clone() Iterator[T] {
	return &unitIterator[T]{p.item, p.index}
}

// Collect allocates a new array containing all items of this iterator.
// This drains the iterator.
//
//nolint:revive
func (p *unitIterator[T]) Collect() []T {
	items := make([]T, 1)
	items[0] = p.item

	return items
}

// Count returns the number of items left in the iterator
//
//nolint:revive
func (p *unitIterator[T]) Count() uint {
	if p.index == 0 {
		return 1
	}
	// nothing left
	return 0
}

// Find returns the index of the first match for a given predicate, or
// return false if no match is found.
//
//nolint:revive
func (p *unitIterator[T]) Find(predicate Predicate[T]) (uint, bool) {
	if predicate(p.item) {
		// Success
		return 0, true
	}
	// Failed
	return 0, false
}

// Nth returns the nth item in this iterator
//
//nolint:revive
func (p *unitIterator[T]) Nth(n uint) T {
	return p.item
}
