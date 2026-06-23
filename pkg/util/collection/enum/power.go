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
package enum

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
)

// Power returns an iterator which enumerates all arrays of size n
// over the given set of elements.  For example, if n==2 and elems contained two
// elements A and B, then this will return [[A,A],[A,B],[B,A],[B,B]].
func Power[E any](n uint, elems []E) Enumerator[[]E] {
	// Determine size of the space.
	remaining := math.PowUint64(uint64(len(elems)), uint64(n))
	//
	return &enumerator[E]{uint64(n), 0, remaining, elems}
}

type enumerator[E any] struct {
	nitems     uint64
	index, end uint64
	elements   []E
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *enumerator[E]) HasNext() bool {
	return p.index < p.end
}

// Count returns the number of items left in this enumeration.
//
//nolint:revive
func (p *enumerator[E]) Count() uint {
	return uint(p.end - p.index)
}

// Nth returns the nth item in this iterator.  This will mutate the iterator.
func (p *enumerator[E]) Nth(n uint) []E {
	next := p.index + uint64(n)
	p.index = next + 1
	//
	return extract(next, p.nitems, p.elements)
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *enumerator[E]) Next() []E {
	next := p.index
	p.index++

	return extract(next, p.nitems, p.elements)
}

// Extract the specific permutation mapped to a given index.
func extract[E any](index uint64, n uint64, elems []E) []E {
	var (
		m  = uint64(len(elems))
		rs = make([]E, n)
	)
	// Copy over elements
	for i := 0; i < len(rs); i++ {
		rs[i] = elems[index%m]
		index = index / m
	}
	//
	return rs
}
