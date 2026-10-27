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

// Range constructs an enumerator over the half-open range [start .. end).
func Range[T any](start uint, end uint) Enumerator[uint] {
	return &rangeEnumerator{start, end}
}

type rangeEnumerator struct {
	index uint
	end   uint
}

// HasNext checks whether or not there are any items remaining to visit.
//
//nolint:revive
func (p *rangeEnumerator) HasNext() bool {
	return p.index < p.end
}

// Count returns the number of items left in this enumeration.
//
//nolint:revive
func (p *rangeEnumerator) Count() uint {
	return p.end - p.index
}

// Nth returns the nth item in this iterator.  This will mutate the iterator.
func (p *rangeEnumerator) Nth(n uint) uint {
	next := p.index + n
	p.index = next + 1
	//
	return next
}

// Next returns the next item, and advance the iterator.
//
//nolint:revive
func (p *rangeEnumerator) Next() uint {
	next := p.index
	p.index++

	return next
}
