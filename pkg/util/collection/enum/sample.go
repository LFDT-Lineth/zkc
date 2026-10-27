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

import (
	"math/rand/v2"
	"slices"
)

// ExhaustiveSample constructs a sampling enumerator from a given enumerator.
// Specifically, the sampling enumerator samples exactly n items from the given
// enumerator (assuming it held at least n items).  Otherwise, if n is larger or
// equal to the number of elements in the original numerator it simply returns
// that.
func ExhaustiveSample[T any](n uint, enum Enumerator[T]) Enumerator[T] {
	if n >= enum.Count() {
		return enum
	}
	//
	return &exhaustiveSamplingEnumerator[T]{n, enum}
}

type exhaustiveSamplingEnumerator[T any] struct {
	// Number of items left to sample
	count uint
	//
	enum Enumerator[T]
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *exhaustiveSamplingEnumerator[E]) HasNext() bool {
	return p.count > 0
}

// Count returns the number of items left in this enumeration.
//
//nolint:revive
func (p *exhaustiveSamplingEnumerator[E]) Count() uint {
	return p.count
}

// Nth returns the nth item in this iterator.  This will mutate the iterator.
func (p *exhaustiveSamplingEnumerator[E]) Nth(n uint) E {
	return Nth(p, n)
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *exhaustiveSamplingEnumerator[E]) Next() E {
	for p.enum.HasNext() {
		n := p.enum.Next()
		//
		if choose(p.count, p.enum.Count()+1) {
			p.count--
			return n
		}
	}
	//
	panic("unreachable")
}

// choosing n from m items
func choose(n, m uint) bool {
	return rand.UintN(m) < n
}

// FastSample does as it says on the tin.  Its fast, but not super precise.  For
// example, duplicates are possible.
func FastSample[T any](n uint, enum Enumerator[T]) Enumerator[T] {
	//
	var (
		count       = enum.Count()
		indices     = make([]uint, n)
		samples []T = make([]T, n)
		last    uint
	)
	//
	if n >= count {
		return enum
	}
	//
	for i := range n {
		indices[i] = rand.UintN(count)
	}
	// Sort selected indices
	slices.Sort(indices)
	//
	for i, v := range indices {
		samples[i] = enum.Nth(v - last)
		last = v
	}
	//
	return Finite(samples...)
}
