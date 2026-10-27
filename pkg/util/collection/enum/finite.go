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

// Finite constructs an enumerator over a finite number of items.
func Finite[T any](items ...T) Enumerator[T] {
	return &finiteEnumerator[T]{items}
}

type finiteEnumerator[T any] struct {
	items []T
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *finiteEnumerator[T]) HasNext() bool {
	return len(p.items) > 0
}

// Count returns the number of items left in this enumeration.
//
//nolint:revive
func (p *finiteEnumerator[T]) Count() uint {
	return uint(len(p.items))
}

// Nth returns the nth item in this iterator.  This will mutate the iterator.
func (p *finiteEnumerator[T]) Nth(n uint) T {
	p.items = p.items[n:]
	return p.Next()
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *finiteEnumerator[T]) Next() T {
	next := p.items[0]
	p.items = p.items[1:]

	return next
}
