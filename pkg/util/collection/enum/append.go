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

// Append constructs an enumerator from an array of zero or more enumerators.
func Append[T any](enumerators ...Enumerator[T]) Enumerator[T] {
	var (
		count uint = 0
		enums []Enumerator[T]
	)
	// Determine space and eliminate any empty enumerators
	for _, e := range enumerators {
		if e.Count() != 0 {
			count += e.Count()
			enums = append(enums, e)
		}
	}
	// Done
	return &appendEnumerator[T]{count, enums}
}

type appendEnumerator[T any] struct {
	count uint
	enums []Enumerator[T]
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *appendEnumerator[E]) HasNext() bool {
	return p.count > 0
}

// Count returns the number of items left in this enumeration.
//
//nolint:revive
func (p *appendEnumerator[E]) Count() uint {
	return p.count
}

// Nth returns the nth item in this iterator.  This will mutate the iterator.
func (p *appendEnumerator[E]) Nth(n uint) E {
	// Decrement count (recalling that n=0 is one item)
	p.count -= n + 1
	//
	for n >= p.enums[0].Count() {
		// Skip everything left
		n -= p.enums[0].Count()
		// Drop enumerator
		p.enums = p.enums[1:]
	}
	//
	return p.enums[0].Nth(n)
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *appendEnumerator[E]) Next() E {
	next := p.enums[0].Next()
	// decremenmt count
	p.count -= 1
	//
	if !p.enums[0].HasNext() {
		p.enums = p.enums[1:]
	}
	//
	return next
}
