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
package set

import (
	"cmp"
	"math"
	"slices"
	"sort"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Comparable provides an interface which types used in a AnySortedSet must implement.
type Comparable[T any] interface {
	// Cmp returns < 0 if this is less than other, or 0 if they are equal, or >
	// 0 if this is greater than other.
	Cmp(other T) int
}

// Order provides a wrapper around primtive types for use with an AnySortedSet.
// This is mostly for testing purposes.
type Order[T cmp.Ordered] struct {
	Item T
}

// Cmp implementation for the Comparable interface.
func (lhs Order[T]) Cmp(rhs Order[T]) int {
	return cmp.Compare(lhs.Item, rhs.Item)
}

// AnySortedSet is an array of unique sorted values (i.e. no duplicates).
type AnySortedSet[T Comparable[T]] []T

// NewAnySortedSet creates a sorted set from a given array by first cloning that
// array, and then sorting it appropriately, etc.  This means the given array
// will not be mutated by this function, or any subsequent calls on the
// resulting set.
func NewAnySortedSet[T Comparable[T]](items ...T) *AnySortedSet[T] {
	var nitems AnySortedSet[T] = slices.Clone(items)
	//
	return RawAnySortedSet[T](nitems...)
}

// RawAnySortedSet creates a sort set from a given array without first cloning
// it.  That means the array may well be mutated by this function and/or
// subsequent calls to the resulting set.
func RawAnySortedSet[T Comparable[T]](items ...T) *AnySortedSet[T] {
	var nitems AnySortedSet[T] = items
	// Sort incoming data
	slices.SortFunc(nitems, func(a, b T) int {
		return a.Cmp(b)
	})
	// Remove duplicates
	nitems = array.RemoveMatchingIndexed(nitems, func(i int, ith T) bool {
		return i > 0 && nitems[i].Cmp(nitems[i-1]) == 0
	})
	//
	return &nitems
}

// ToArray extracts the underlying array from this sorted set.
func (p *AnySortedSet[T]) ToArray() []T {
	return *p
}

// Find returns the index of the matching element in this set, or it returns
// MaxUInt.
func (p *AnySortedSet[T]) Find(element T) uint {
	data := *p
	// Find index where element either does occur, or should occur.
	i := sort.Search(len(data), func(i int) bool {
		// element <= data[i]
		return element.Cmp(data[i]) <= 0
	})
	// Check whether item existed or not.
	if i < len(data) && data[i].Cmp(element) == 0 {
		return uint(i)
	}
	// not found
	return math.MaxUint
}

// Contains returns true if a given element is in the set.
//
//nolint:revive
func (p *AnySortedSet[T]) Contains(element T) bool {
	return p.Find(element) != math.MaxUint
}

// Insert an element into this sorted set.
//
//nolint:revive
func (p *AnySortedSet[T]) Insert(element T) {
	data := *p
	// Find index where element either does occur, or should occur.
	i := sort.Search(len(data), func(i int) bool {
		// element <= data[i]
		return element.Cmp(data[i]) <= 0
	})
	// Check whether item existed or not.
	if i >= len(data) || data[i].Cmp(element) != 0 {
		// No, item was not found
		ndata := make([]T, len(data)+1)
		copy(ndata, data[0:i])
		ndata[i] = element
		copy(ndata[i+1:], data[i:])
		*p = ndata
	}
}

// InsertSorted inserts all elements in a given sorted set into this set.
//
//nolint:revive
func (p *AnySortedSet[T]) InsertSorted(q *AnySortedSet[T]) {
	left := *p
	right := *q
	// Check containment
	n := anyCountDuplicates(left, right)
	// Check for total inclusion
	if n == len(right) {
		// Right set completedly included in left, so actually there is nothing
		// to do.
		return
	}
	// Allocate space
	ndata := make([]T, len(left)+len(right)-n)
	// Merge
	anyMergeSorted(ndata, left, right)
	// Finally copy over new data
	*p = ndata
}

// Remove an element from this sorted set.
//
//nolint:revive
func (p *AnySortedSet[T]) Remove(element T) bool {
	data := *p
	// Find index where element either does occur, or should occur.
	i := sort.Search(len(data), func(i int) bool {
		// element <= data[i]
		return element.Cmp(data[i]) <= 0
	})
	// Check whether item existed or not.
	if i < len(data) && data[i].Cmp(element) == 0 {
		// yes, therefore can remove
		*p = array.RemoveAt(data, uint(i))
		return true
	}
	// No, nothing removed
	return false
}

// Iter returns an iterator over the elements of this sorted set.
//
//nolint:revive
func (p *AnySortedSet[T]) Iter() iter.Iterator[T] {
	return iter.NewArrayIterator(*p)
}

// UnionAnySortedSets unions together a number of things which can be turn into a
// sorted set using a given mapping function.  At some level, this is a
// map/reduce function.
func UnionAnySortedSets[S any, T Comparable[T]](elems []S, fn func(S) *AnySortedSet[T]) *AnySortedSet[T] {
	if len(elems) == 0 {
		return NewAnySortedSet[T]()
	}
	// Map first element
	set := fn(elems[0])
	// Map/reduce the rest
	for i := 1; i < len(elems); i++ {
		// Map ith element
		ith := fn(elems[i])
		// Reduce
		set.InsertSorted(ith)
	}
	//
	return set
}

// Determine number of duplicate elements
func anyCountDuplicates[T Comparable[T]](left []T, right []T) int {
	// Check containment
	i := 0
	j := 0
	n := 0

	for i < len(left) && j < len(right) {
		if left[i].Cmp(right[j]) == 0 {
			i++
			j++
			n++ // duplicate detected
		} else if left[i].Cmp(right[j]) < 0 {
			i++
		} else {
			j++
		}
	}

	return n
}

// Merge two sets of sorted arrays (left and right) into a target array.  This
// assumes the target array is big enough.
func anyMergeSorted[T Comparable[T]](target []T, left []T, right []T) {
	i := 0
	j := 0
	k := 0
	// Merge overlap of both sets
	for ; i < len(left) && j < len(right); k++ {
		if left[i].Cmp(right[j]) == 0 {
			target[k] = left[i]
			i++
			j++
		} else if left[i].Cmp(right[j]) < 0 {
			target[k] = left[i]
			i++
		} else {
			target[k] = right[j]
			j++
		}
	}
	// Handle anything left
	if i < len(left) {
		copy(target[k:], left[i:])
	} else if j < len(right) {
		copy(target[k:], right[j:])
	}
}
