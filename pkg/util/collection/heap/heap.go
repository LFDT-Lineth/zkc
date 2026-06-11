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
package heap

import "slices"

// Heap represents an array of values which can increase or decrease in size,
// and is expected to do so a lot. The key requirement of a heap is that
// capacity is retained in order to prevent subsequent reallocation when the
// heap expands.
type Heap[T any] struct {
	data []T
}

// Alloc ensures space for exactly n elements on top of this heap.  This may
// result in allocation only if the underlying capacity is exhausted.
func (p *Heap[T]) Alloc(n uint) {
	var nsize = uint(len(p.data)) + n
	// grow data capacity (as necessary)
	data := slices.Grow(p.data, int(n))
	// expand data length
	p.data = data[:nsize]
}

// Push exactly one item onto the heap.  This allocates space for one item, and
// sets that item accordingly.
func (p *Heap[T]) Push(item T) {
	var offset = len(p.data)
	p.Alloc(1)
	p.data[offset] = item
}

// Pop exactly one off the heap.  This retrieves the top item on the heap, and
// the frees it.
func (p *Heap[T]) Pop() T {
	var (
		offset = len(p.data) - 1
		item   = p.data[offset]
	)
	//
	p.Free(1)
	//
	return item
}

// Clear resets the heap to an empty state whilst retaining any allocated
// capacity.
func (p *Heap[T]) Clear() {
	p.data = p.data[:0]
}

// Free simply reduces the size of the heap by the given amount of cells.
func (p *Heap[T]) Free(n uint) {
	var nsize = uint(len(p.data)) - n
	//
	p.data = p.data[:nsize]
}

// Get the value at the given offset in this heap.  This will panic if offset >=
// p.Size().
func (p *Heap[T]) Get(offset uint) T {
	return p.data[offset]
}

// Set the value at the given offset in this heap.  This will panic if offset >=
// p.Size().
func (p *Heap[T]) Set(offset uint, val T) {
	p.data[offset] = val
}

// Size returns the current size of this heap.
func (p *Heap[T]) Size() uint {
	return uint(len(p.data))
}

// Slice returns a slice of this heap.  Again, this will panic if end >=
// p.Size().
func (p *Heap[T]) Slice(start, end uint) []T {
	return p.data[start:end]
}

// SliceEnd returns a slice of this heap from the given start position upto the
// end.
func (p *Heap[T]) SliceEnd(start uint) []T {
	return p.data[start:]
}
