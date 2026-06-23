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
package array

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Comparable interface which can be implemented by non-primitive types.
type Comparable[T any] interface {
	// Cmp returns < 0 if this is less than other, or 0 if they are equal, or >
	// 0 if this is greater than other.
	Cmp(other T) int
}

// Compare two slices of ordered elements.
func Compare[T Comparable[T]](lhs []T, rhs []T) int {
	c := cmp.Compare(len(lhs), len(rhs))
	//
	if c == 0 {
		for i := range lhs {
			c = lhs[i].Cmp(rhs[i])
			if c != 0 {
				break
			}
		}
	}
	//
	return c
}

// Expand ensures space for exactly n elements on top of this heap.  This may
// result in allocation only if the underlying capacity is exhausted.
func Expand[T any](data []T, n uint) []T {
	var nsize = uint(len(data)) + n
	// grow data capacity (as necessary)
	data = slices.Grow(data, int(n))
	// expand data length
	return data[:nsize]
}

// FrontPad pads an array upto a given length n with a given item.
// Specifically, new items are inserted at the front of the array.
func FrontPad[T any](slice []T, n uint, item T) []T {
	//
	if uint(len(slice)) < n {
		var (
			nslice = make([]T, n)
			delta  = n - uint(len(slice))
		)
		//
		copy(nslice[delta:], slice)
		// Pad out remainder
		for i := uint(0); i < delta; i++ {
			nslice[i] = item
		}
		//
		slice = nslice
	}
	//
	return slice
}

// BackPad pads an array upto a given length n with a given item.
// Specifically, new items are inserted at the end of the array.
func BackPad[T any](slice []T, n uint, item T) []T {
	//
	if uint(len(slice)) < n {
		var (
			nslice = make([]T, n)
			m      = uint(len(slice))
		)
		//
		copy(nslice, slice)
		// Pad out remainder
		for i := m; i < n; i++ {
			nslice[i] = item
		}
		//
		slice = nslice
	}
	//
	return slice
}

// Prepend creates a new slice containing the result of prepending the given
// item onto the end of the given slice.  Observe that, unlike the built-in
// append() function, this will never modify the given slice.
func Prepend[T any](item T, slice []T) []T {
	n := len(slice)
	// Make space for new slice
	nslice := make([]T, n+1)
	// Copy existing values
	copy(nslice[1:], slice)
	// Set first value
	nslice[0] = item
	// Done
	return nslice
}

// Append creates a new slice containing the result of appending the given item
// onto the end of the given slice.  Observe that, unlike the built-in append()
// function, this will never modify the given slice.
//
//nolint:revive
func Append[T any](slice []T, item T) []T {
	n := len(slice)
	// Make space for new slice
	nslice := make([]T, n+1)
	// Copy existing values
	copy(nslice[:n], slice)
	// Set last value
	nslice[n] = item
	// Done
	return nslice
}

// AppendAll creates a new slice containing the result of appending the given items
// onto the end of the given slice.  Observe that, unlike the built-in append()
// function, this will never modify the given slice.
//
//nolint:revive
func AppendAll[T any](lhs []T, rhs ...T) []T {
	n := len(lhs)
	m := len(rhs)
	// Make space for new slice
	nslice := make([]T, n+m)
	// Copy left values
	copy(nslice[:n], lhs)
	// Copy right values
	copy(nslice[n:], rhs)
	// Done
	return nslice
}

// CountUnique counts the number of unique items within a given slice.
func CountUnique[T cmp.Ordered](items []T) uint {
	// First sort them
	slices.Sort(items)
	//
	count := uint(0)
	//
	for i, v := range items {
		if i == 0 || items[i-1] != v {
			count++
		}
	}
	//
	return count
}

// CountMatching counts the number of items in the given array for which the
// predicate returns true.
func CountMatching[T any](items []T, predicate Predicate[T]) uint {
	var count = uint(0)
	//
	for _, item := range items {
		if predicate(item) {
			count++
		}
	}
	// Done
	return count
}

// ReplaceFirstOrPanic replaces the first occurrence of a given item (from) in an
// array with another item (to).  If not match is found, then this will panic.
// In otherwords, we are expecting a match.
func ReplaceFirstOrPanic[T comparable](columns []T, from T, to T) {
	for i, c := range columns {
		if c == from {
			// Success
			columns[i] = to
			return
		}
	}
	// Failure
	panic(fmt.Sprintf("invalid replace (item %s not found)", any(from)))
}

// FindMatching determines the index of first matching item in a given array, or
// returns max.MaxUint otherwise.
func FindMatching[T any](items []T, predicate Predicate[T]) uint {
	for i, item := range items {
		if predicate(item) {
			return uint(i)
		}
	}
	//
	return math.MaxUint
}

// Filter a given slice by retaing only those elements where the given predicate
// holds.
func Filter[T any](items []T, predicate Predicate[T]) (nitems []T) {
	for _, item := range items {
		if predicate(item) {
			nitems = append(nitems, item)
		}
	}
	//
	return nitems
}

// ContainsMatching checks whether a given array contains an item matching a given predicate.
func ContainsMatching[T any](items []T, predicate Predicate[T]) bool {
	for _, item := range items {
		if predicate(item) {
			return true
		}
	}
	//
	return false
}

// InsertAt constructs an identical slice, except with the element inserted at
// the given index.  If the index is beyond the bounds of the array, then the
// element is simply appended.
func InsertAt[T any](items []T, element T, index uint) []T {
	n := uint(len(items))
	//
	if index < n {
		first := items[:index]
		second := items[index:]
		items = make([]T, n+1)
		copy(items, first)
		copy(items[index+1:], second)
		items[index] = element
	} else {
		items = append(items, element)
	}
	//
	return items
}

// InsertAllAt constructs an identical slice, except with the given elements
// inserted at the given index.  If the index is beyond the bounds of the array,
// then the element is simply appended.
func InsertAllAt[T any](items []T, elements []T, index uint) []T {
	n := uint(len(items))
	m := uint(len(elements))
	//
	if index < n {
		first := items[:index]
		second := items[index:]
		items = make([]T, n+m)
		copy(items, first)
		copy(items[index:], elements)
		copy(items[index+m:], second)
	} else {
		items = append(items, elements...)
	}
	//
	return items
}

// RemoveAt constructs an identical slice, except with the element at the given
// index removed.  If the index is beyond the bounds of the array, then there is
// no change.
func RemoveAt[T any](items []T, index uint) []T {
	var (
		n      = uint(len(items))
		nitems []T
	)
	//
	if index >= n {
		return items
	}
	// Construct new array
	nitems = make([]T, n-1)
	copy(nitems, items[0:index])
	copy(nitems[index:], items[index+1:])
	//
	return nitems
}

// RemoveMatching removes all elements from an array matching the given item.
func RemoveMatching[T any](items []T, predicate Predicate[T]) []T {
	count := 0
	// Check how many matches we have
	for _, r := range items {
		if !predicate(r) {
			count++
		}
	}
	// Check for stuff to remove
	if count != len(items) {
		nitems := make([]T, count)
		j := 0
		// Remove items
		for i, r := range items {
			if !predicate(r) {
				nitems[j] = items[i]
				j++
			}
		}
		//
		items = nitems
	}
	//
	return items
}

// RemoveMatchingIndexed removes all elements from an array matching the given item.
func RemoveMatchingIndexed[T any](items []T, predicate func(int, T) bool) []T {
	count := 0
	// Check how many matches we have
	for i, r := range items {
		if !predicate(i, r) {
			count++
		}
	}
	// Check for stuff to remove
	if count != len(items) {
		nitems := make([]T, count)
		j := 0
		// Remove items
		for i, r := range items {
			if !predicate(i, r) {
				nitems[j] = items[i]
				j++
			}
		}
		//
		items = nitems
	}
	//
	return items
}

// Reverse reverses the contents of an array.
func Reverse[T any](items []T) []T {
	var (
		n      = len(items) - 1
		nitems = make([]T, len(items))
	)
	// Write in reverse order
	for i := range items {
		nitems[i] = items[n-i]
	}
	//
	return nitems
}

// ReverseInPlace reversees the items in an array in place.
func ReverseInPlace[T any](items []T) {
	var (
		j     = len(items) - 1
		pivot = len(items) >> 1
	)
	// Perform the reverse
	for i := 0; i < pivot; i, j = i+1, j-1 {
		ith := items[i]
		items[i] = items[j]
		items[j] = ith
	}
}

// Flatten flattens items from an array which expand into arrays of terms.
func Flatten[T any](items []T, fn func(T) []T) []T {
	for _, t := range items {
		if fn(t) != nil {
			return forceFlatten(items, fn)
		}
	}
	// no change
	return items
}
func forceFlatten[T any](items []T, fn func(T) []T) []T {
	nitems := make([]T, 0)
	//
	for _, t := range items {
		if ts := fn(t); ts != nil {
			nitems = append(nitems, ts...)
		} else {
			nitems = append(nitems, t)
		}
	}
	// no change
	return nitems
}

// FlatMap flattens items from an array which expand into arrays of terms.
func FlatMap[S, T any](items []S, fn func(S) []T) []T {
	nitems := make([]T, 0)
	//
	for _, t := range items {
		nitems = append(nitems, fn(t)...)
	}
	// no change
	return nitems
}

// Map an array from one type to another using a given mapping (or projection)
// function.
func Map[S, T any](items []S, mapping func(uint, S) T) []T {
	nitems := make([]T, len(items))
	//
	for i, t := range items {
		nitems[i] = mapping(uint(i), t)
	}
	//
	return nitems
}

// MergeSorted combines two sorted arrays whilst maintaining the sorted
// invariant.  You can think of this as first appending the second array to the
// first, then sorting the result.  In fact, it uses a merge sort which is
// slightly more efficient than resorting from scratch.
func MergeSorted[S Comparable[S]](lhs []S, rhs []S) []S {
	var (
		result    = make([]S, len(lhs)+len(rhs))
		li, ri, i int
	)
	//
	for li < len(lhs) && ri < len(rhs) {
		var (
			lth = lhs[li]
			rth = rhs[ri]
		)
		//
		if c := lth.Cmp(rth); c <= 0 {
			result[i] = lth
			li++
		} else {
			result[i] = rth
			ri++
		}
		//
		i++
	}
	// Include any stragglers
	copy(result[i:], lhs[li:])
	copy(result[i:], rhs[ri:])
	// Done
	return result
}

// ToString constructs a string representation for an array of values which
// themselves have String() functions.
func ToString[S fmt.Stringer](arr []S) string {
	var builder strings.Builder
	//
	builder.WriteString("[")
	//
	for i, v := range arr {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(v.String())
	}
	//
	builder.WriteString("]")
	//
	return builder.String()
}
