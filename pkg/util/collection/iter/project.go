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

type projectIterator[S, T any] struct {
	iter       Iterator[S]
	projection func(S) T
}

// NewProjectIterator construct an iterator that is the projection of another.
func NewProjectIterator[S, T any](iter Iterator[S], projection func(S) T) Iterator[T] {
	return &projectIterator[S, T]{iter, projection}
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *projectIterator[S, T]) HasNext() bool {
	return p.iter.HasNext()
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *projectIterator[S, T]) Next() T {
	return p.projection(p.iter.Next())
}

// Append another iterator onto the end of this iterator.  Thus, when all
// items are visited in this iterator, iteration continues into the other.
//
//nolint:revive
func (p *projectIterator[S, T]) Append(iter Iterator[T]) Iterator[T] {
	return NewAppendIterator(p, iter)
}

// Clone creates a copy of this iterator at the given cursor position.
// Modifying the clone (i.e. by calling Next) iterator will not modify the
// original.
//
//nolint:revive
func (p *projectIterator[S, T]) Clone() Iterator[T] {
	return NewProjectIterator(p.iter.Clone(), p.projection)
}

// Collect allocates a new array containing all items of this iterator. This drains the iterator.
//
//nolint:revive
func (p *projectIterator[S, T]) Collect() []T {
	items := make([]T, p.iter.Count())
	index := 0

	for i := p.iter; i.HasNext(); {
		items[index] = p.projection(i.Next())
		index++
	}

	return items
}

// Count returns the number of items left in the iterator
//
//nolint:revive
func (p *projectIterator[S, T]) Count() uint {
	return p.iter.Count()
}

// Find returns the index of the first match for a given predicate, or
// return false if no match is found.
//
//nolint:revive
func (p *projectIterator[S, T]) Find(predicate Predicate[T]) (uint, bool) {
	return p.iter.Find(func(item S) bool {
		return predicate(p.projection(item))
	})
}

// Nth returns the nth item in this iterator
//
//nolint:revive
func (p *projectIterator[S, T]) Nth(n uint) T {
	return p.projection(p.iter.Nth(n))
}
