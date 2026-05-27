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
	"github.com/consensys/go-corset/pkg/util/collection/enum"
)

// ArrayIterator provides an iterator implementation for an Array.
type arrayIterator[T any] struct {
	items []T
	index uint
}

// NewArrayIterator construct an iterator over an array of items.
func NewArrayIterator[T any](items []T) Iterator[T] {
	return &arrayIterator[T]{items, 0}
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *arrayIterator[T]) HasNext() bool {
	return p.index < uint(len(p.items))
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *arrayIterator[T]) Next() T {
	next := p.items[p.index]
	p.index++

	return next
}

// Append another iterator onto the end of this iterator.  Thus, when all
// items are visited in this iterator, iteration continues into the other.
//
//nolint:revive
func (p *arrayIterator[T]) Append(iter Iterator[T]) Iterator[T] {
	return NewAppendIterator(p, iter)
}

// Clone creates a copy of this iterator at the given cursor position.
// Modifying the clone (i.e. by calling Next) iterator will not modify the
// original.
//
//nolint:revive
func (p *arrayIterator[T]) Clone() Iterator[T] {
	return &arrayIterator[T]{p.items, p.index}
}

// Collect allocates a new array containing all items of this iterator.
// This drains the iterator.
//
//nolint:revive
func (p *arrayIterator[T]) Collect() []T {
	items := make([]T, len(p.items))
	copy(items, p.items)

	return items
}

// Count returns the number of items left in the iterator
//
//nolint:revive
func (p *arrayIterator[T]) Count() uint {
	return uint(len(p.items)) - p.index
}

// Find returns the index of the first match for a given predicate, or
// return false if no match is found.
//
//nolint:revive
func (p *arrayIterator[T]) Find(predicate Predicate[T]) (uint, bool) {
	return enum.Find(p, predicate)
}

// Nth returns the nth item in this iterator
//
//nolint:revive
func (p *arrayIterator[T]) Nth(n uint) T {
	p.index = n
	return p.items[n]
}

// reverseArrayIterator iterates over a slice in reverse order (last element
// first).
type reverseArrayIterator[T any] struct {
	items []T
	index int // decrements from len-1 to -1
}

// NewReverseArrayIterator constructs an iterator that yields the elements of
// items in reverse order, starting from the last element.
func NewReverseArrayIterator[T any](items []T) Iterator[T] {
	return &reverseArrayIterator[T]{items, len(items) - 1}
}

//nolint:revive
func (p *reverseArrayIterator[T]) HasNext() bool {
	return p.index >= 0
}

//nolint:revive
func (p *reverseArrayIterator[T]) Next() T {
	item := p.items[p.index]
	p.index--

	return item
}

//nolint:revive
func (p *reverseArrayIterator[T]) Nth(n uint) T {
	return enum.Nth(p, n)
}

//nolint:revive
func (p *reverseArrayIterator[T]) Append(iter Iterator[T]) Iterator[T] {
	return NewAppendIterator(p, iter)
}

//nolint:revive
func (p *reverseArrayIterator[T]) Clone() Iterator[T] {
	return &reverseArrayIterator[T]{p.items, p.index}
}

//nolint:revive
func (p *reverseArrayIterator[T]) Collect() []T {
	result := make([]T, 0, p.index+1)
	for p.index >= 0 {
		result = append(result, p.items[p.index])
		p.index--
	}

	return result
}

//nolint:revive
func (p *reverseArrayIterator[T]) Count() uint {
	if p.index < 0 {
		return 0
	}

	return uint(p.index + 1)
}

//nolint:revive
func (p *reverseArrayIterator[T]) Find(predicate Predicate[T]) (uint, bool) {
	return enum.Find(p, predicate)
}
