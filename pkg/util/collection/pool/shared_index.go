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
	"math"
	"slices"
	"sync"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// SharedIndex represents a pool which stores words "as is", and does not attempt
// to compress them into shorter byte sequences.
type SharedIndex[T word.Word[T]] struct {
	// heap of bytes
	words []T
	// hash buckets
	buckets [][]uint32
	// mutex required to ensure thread safety.
	mux sync.RWMutex
}

var _ Pool[uint32, word.BigEndian] = &SharedIndex[word.BigEndian]{}

// NewSharedIndex constructs a new shared index
func NewSharedIndex[T word.Word[T]]() *SharedIndex[T] {
	var (
		empty T
		//
		p = &SharedIndex[T]{
			words:   nil,
			buckets: make([][]uint32, HEAP_POOL_INIT_BUCKETS),
		}
	)
	// Initialise first index
	p.Put(empty)
	//
	return p
}

// Clone implementation for SharedPool interface.
func (p *SharedIndex[T]) Clone() *SharedIndex[T] {
	var (
		words   = slices.Clone(p.words)
		buckets = make([][]uint32, len(p.buckets))
	)
	//
	for i := range len(p.buckets) {
		buckets[i] = slices.Clone(p.buckets[i])
	}
	//
	return &SharedIndex[T]{
		words:   words,
		buckets: buckets,
	}
}

// Localise implementation for SharedPool interface.
func (p *SharedIndex[T]) Localise() *LocalIndex[T] {
	return NewLocalIndex[T]()
}

// Get implementation for the Pool interface.
func (p *SharedIndex[T]) Get(index uint32) T {
	// Obtain read lock
	p.mux.RLock()
	// Determine length of word in heap
	word := p.words[index]
	// Release read lock
	p.mux.RUnlock()
	// Initialise word
	return word
}

// Put implementation for the Pool interface.  This is somewhat challenging
// because it must be thread safe.  Since we anticipate a large number of cache
// hits compared with cache misses, we employ a Read/Write lock.
func (p *SharedIndex[T]) Put(word T) uint32 {
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
		index = uint32(len(p.words))
		p.words = append(p.words, word)
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

// Check whether the hash map is exceed its loading factor and, if so, rehash.
func (p *SharedIndex[T]) rehashIfOverloaded() {
	load := (100 * len(p.words)) / len(p.buckets)
	//
	if load > HEAP_POOL_LOADING {
		// Force a rehash
		p.rehash()
	}
}

// Has checks whether a given word is stored in this heap, or not.
func (p *SharedIndex[T]) has(word T) (uint32, uint64) {
	hash := word.Hash() % uint64(len(p.buckets))
	// Attempt to lookup word
	for _, index := range p.buckets[hash] {
		if p.words[index].Equals(word) {
			return index, hash
		}
	}
	//
	return math.MaxUint32, hash
}

func (p *SharedIndex[T]) rehash() {
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
			hash := p.words[index].Hash() % n
			// Record index in relevant bucket
			p.buckets[hash] = append(p.buckets[hash], index)
		}
	}
}
