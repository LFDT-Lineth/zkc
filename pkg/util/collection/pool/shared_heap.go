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
package pool

import (
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// HEAP_POOL_INIT_BUCKETS determines the initial number of buckets to use for
// any instance.  Since we are geared towards large pools, we set this figure
// quite high.
const HEAP_POOL_INIT_BUCKETS = 128

// HEAP_POOL_LOADING determines the loading point, overwhich rehashing will
// occur.  This is currently set to 75% capacity forces a rehashing.
const HEAP_POOL_LOADING = 75

// SharedHeap maintains a heap of bytes representing the words which is
// thread safe and is protected by an RWMutex.
type SharedHeap[T word.DynamicWord[T]] struct {
	// heap of bytes
	heap []byte
	// byte lengths for each chunk in the pool
	lengths []uint16
	// hash buckets
	buckets [][]uint32
	// count of words stored
	count uint
	// mutex required to ensure thread safety.
	mux sync.RWMutex
}

// NewSharedHeap constructs a new thread-safe heap
func NewSharedHeap[T word.DynamicWord[T]]() *SharedHeap[T] {
	// zero-sized word
	var (
		empty T
		p     = &SharedHeap[T]{
			lengths: []uint16{0},
			buckets: make([][]uint32, HEAP_POOL_INIT_BUCKETS),
			heap:    nil,
			count:   0,
		}
	)
	// Ensure address of this object is 0.
	p.Put(empty)
	p.heap = []byte{0}
	//
	return p
}

// Size returns the number of distinct entries in this heap.
func (p *SharedHeap[T]) Size() uint {
	return p.count
}

// Clone implementation for SharedPool interface.
func (p *SharedHeap[T]) Clone() *SharedHeap[T] {
	var (
		heap    = make([]byte, len(p.heap))
		lengths = make([]uint16, len(p.lengths))
		buckets = make([][]uint32, len(p.buckets))
	)
	//
	copy(heap, p.heap)
	copy(lengths, p.lengths)
	//
	for i := range len(p.buckets) {
		buckets[i] = slices.Clone(p.buckets[i])
	}
	//
	return &SharedHeap[T]{
		heap:    heap,
		lengths: lengths,
		buckets: buckets,
		count:   p.count,
	}
}

// Localise implementation for SharedPool interface.
func (p *SharedHeap[T]) Localise() *LocalHeap[T] {
	return &LocalHeap[T]{
		heap:    p.heap,
		lengths: p.lengths,
		buckets: p.buckets,
		count:   p.count,
	}
}

// Get implementation for the Pool interface.
func (p *SharedHeap[T]) Get(index uint32) T {
	// Obtain read lock
	p.mux.RLock()
	// Determine length of word in heap
	word := p.innerGet(index)
	// Release read lock
	p.mux.RUnlock()
	// Initialise word
	return word
}

// Put implementation for the Pool interface.  This is somewhat challenging
// because it must be thread safe.  Since we anticipate a large number of cache
// hits compared with cache misses, we employ a Read/Write lock.
func (p *SharedHeap[T]) Put(word T) uint32 {
	p.mux.RLock()
	// Check whether word already stored
	index, _ := p.has(word)
	// Release read lock
	p.mux.RUnlock()
	// Check whether we found it
	if index != math.MaxUint32 {
		// Yes, therefore return it.
		return index
	}
	// No, therefore begin critical section
	p.mux.Lock()
	// Recheck whether word stored in between read lock being released
	// (unlikely, but it is possible).
	index, hash := p.has(word)
	//
	if index == math.MaxUint32 {
		// Word still not present, so add it.
		index = p.alloc(word)
		// Record entry in relevant bucket
		p.buckets[hash] = append(p.buckets[hash], index)
		// Rehash (if necessary)
		p.rehashIfOverloaded()
	}
	// end critical section
	p.mux.Unlock()
	//
	return index
}

// Allocate a new word into the heap, returning its address.  This method is not
// threadsafe.
func (p *SharedHeap[T]) alloc(word T) uint32 {
	var (
		address = uint32(len(p.heap))
		// Determine length of word
		bytewidth = uint32(word.ByteWidth())
	)
	//
	if bytewidth > math.MaxUint16 {
		panic(fmt.Sprintf("word is too long (%d bytes)", bytewidth))
	}
	// Allocate space for new word
	for range bytewidth {
		p.heap = append(p.heap, 0)
		p.lengths = append(p.lengths, 0)
	}
	// Write word data
	word.PutBytes(p.heap[address : address+bytewidth])
	// Configure word length
	p.lengths[address] = uint16(bytewidth)
	// Record word
	p.count++
	// Done
	return address
}

// Check whether the hash map is exceed its loading factor and, if so, rehash.
func (p *SharedHeap[T]) rehashIfOverloaded() {
	load := (100 * p.count) / uint(len(p.buckets))
	//
	if load > HEAP_POOL_LOADING {
		// Force a rehash
		p.rehash()
	}
}

// Has checks whether a given word is stored in this heap, or not.
func (p *SharedHeap[T]) has(word T) (uint32, uint64) {
	hash := word.Hash() % uint64(len(p.buckets))
	// Attempt to lookup word
	for _, index := range p.buckets[hash] {
		if p.innerGet(index).Equals(word) {
			return index, hash
		}
	}
	//
	return math.MaxUint32, hash
}

// unsynchronized version of Get to be used when a lock is already acquired.
func (p *SharedHeap[T]) innerGet(index uint32) T {
	var (
		word T
		// Determine length of word in heap
		length = uint32(p.lengths[index])
		// Identify bytes of word in the heap
		bytes = p.heap[index : index+length]
	)
	// Initialise word
	return word.SetBytes(bytes)
}

func (p *SharedHeap[T]) rehash() {
	var (
		oldBuckets = p.buckets
		n          = uint64(len(oldBuckets) * 3)
	)
	// Double number of buckets
	p.buckets = make([][]uint32, n)
	// Rehash!
	for _, bucket := range oldBuckets {
		for _, index := range bucket {
			// Determine new hash
			hash := p.innerGet(index).Hash() % n
			// Record index in relevant bucket
			p.buckets[hash] = append(p.buckets[hash], index)
		}
	}
}
