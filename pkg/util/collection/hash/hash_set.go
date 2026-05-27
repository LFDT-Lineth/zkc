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
package hash

import (
	"fmt"
	"strings"
)

// Set defines a generic set implementation backed by a map.  This is a true
// hashtable in that collisions are handle gracefully using buckets, rather than
// simply discarding them.
type Set[T Hasher[T]] struct {
	// items maps hashcodes to *buckets* of items.
	items map[uint64]hashSetBucket[T]
}

// NewSet creates a new HashSet with a given underlying capacity.
func NewSet[T Hasher[T]](size uint) *Set[T] {
	items := make(map[uint64]hashSetBucket[T], size)
	return &Set[T]{items}
}

// Size returns the number of unique items stored in this HashSet.
//
//nolint:revive
func (p *Set[T]) Size() uint {
	count := uint(0)
	for _, b := range p.items {
		count += b.size()
	}

	return count
}

// MaxBucket returns the size of the largest bucket.
//
//nolint:revive
func (p *Set[T]) MaxBucket() uint {
	m := uint(0)
	for _, b := range p.items {
		m = max(m, b.size())
	}

	return m
}

// Insert a new item into this map, returning true if it was already contained
// and false otherwise.
//
//nolint:revive
func (p *Set[T]) Insert(item T) bool {
	var b1 hashSetBucket[T]
	// Compute item's hashcode
	hash := item.Hash()
	// Lookup existing bucket
	b1 = p.items[hash]
	// Insert new item
	r := b1.insert(item)
	// Update map
	p.items[hash] = b1
	// Done
	return r
}

// Contains checks whether the given item is contained within this map, or not.
//
//nolint:revive
func (p *Set[T]) Contains(item T) bool {
	hash := item.Hash()

	if bucket, ok := p.items[hash]; ok {
		return bucket.contains(item)
	}

	return false
}

//nolint:revive
func (p *Set[T]) String() string {
	var r strings.Builder
	//
	first := true
	// Write opening brace
	r.WriteString("{")
	// Iterate all buckets
	for _, b := range p.items {
		// Iterate all items in bucket
		for _, i := range b.items {
			if !first {
				r.WriteString(",")
			}

			first = false

			fmt.Fprintf(&r, "%v", any(i))
		}
	}
	// Write closing brace
	r.WriteString("}")
	// Done
	return r.String()
}

// ============================================================================
// Bucket
// ============================================================================

type hashSetBucket[T Hasher[T]] struct {
	items []T
}

// Get the number of items in this bucket.
//
//nolint:revive
func (b *hashSetBucket[T]) size() uint {
	return uint(len(b.items))
}

// Insert a new item into this bucket
//
//nolint:revive
func (b *hashSetBucket[T]) insert(item T) bool {
	if b.contains(item) {
		// Item already present, so nothing to do.
		return true
	}
	// Append item
	b.items = append(b.items, item)
	// Item not present
	return false
}

// Check whether this bucket contains a given item, or not.
//
//nolint:revive
func (b *hashSetBucket[T]) contains(item T) bool {
	for _, i := range b.items {
		if item.Equals(i) {
			return true
		}
	}

	return false
}
