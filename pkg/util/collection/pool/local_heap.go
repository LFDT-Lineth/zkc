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
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// LocalHeap maintains a heap of bytes representing the words which is *not* thread
// safe.
type LocalHeap[T word.DynamicWord[T]] struct {
	// heap of bytes
	heap []byte
	// byte lengths for each chunk in the pool
	lengths []uint16
	// hash buckets
	buckets [][]uint32
	// count of words stored
	count uint
}

// NewLocalHeap constructs a new thread-unsafe heap
func NewLocalHeap[T word.DynamicWord[T]]() *LocalHeap[T] {
	// zero-sized word
	var (
		empty T
		p     = &LocalHeap[T]{
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

// Clone implementation for SharedPool interface.
func (p *LocalHeap[T]) Clone() LocalHeap[T] {
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
	return LocalHeap[T]{
		heap:    heap,
		lengths: lengths,
		buckets: buckets,
		count:   p.count,
	}
}

// Get implementation for the Pool interface.
func (p *LocalHeap[T]) Get(index uint32) T {
	// Determine length of word in heap
	return p.innerGet(index)
}

// Put implementation for the Pool interface.  This is somewhat challenging
// because it must be thread safe.  Since we anticipate a large number of cache
// hits compared with cache misses, we employ a Read/Write lock.
func (p *LocalHeap[T]) Put(word T) uint32 {
	// Check whether word stored in heap already
	index, hash := p.has(word)
	//
	if index == math.MaxUint32 {
		// Word not present, so add it.
		index = p.alloc(word)
		// Record entry in relevant bucket
		p.buckets[hash] = append(p.buckets[hash], index)
		// Rehash (if necessary)
		p.rehashIfOverloaded()
	}
	//
	return index
}

// Bytes returns the raw bytes in this heap.
func (p *LocalHeap[T]) Bytes() []byte {
	return p.heap
}

// Size returns the number of distinct entries in this heap.
func (p *LocalHeap[T]) Size() uint {
	return p.count
}

// MarshalBinaryV3 converts this heap into a sequence of bytes.
func (p *LocalHeap[T]) MarshalBinaryV3() ([]byte, error) {
	var (
		buf     bytes.Buffer
		n       = len(p.lengths)
		lengths = make([]byte, len(p.lengths)*2)
	)
	// construct lengths (big endian)
	for i, l := range p.lengths {
		lengths[2*i] = byte(l >> 8)
		lengths[(2*i)+1] = byte(l & 0xff)
	}
	// write heap length
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(p.heap))); err != nil {
		return nil, err
	}
	// write lengths
	if m, err := buf.Write(lengths); err != nil {
		return nil, err
	} else if m != 2*n {
		return nil, fmt.Errorf("wrote insufficient bytes (%d v %d)", m, 2*n)
	}
	// write bytes
	if m, err := buf.Write(p.heap); err != nil {
		return nil, err
	} else if m != n {
		return nil, fmt.Errorf("wrote insufficient bytes (%d v %d)", m, n)
	}
	// Done
	return buf.Bytes(), nil
}

// UnmarshalBinaryV2 initialises this heap from a given set of data bytes. This
// should match exactly the encoding above.
func (p *LocalHeap[T]) UnmarshalBinaryV2(data []byte) error {
	var (
		buf = bytes.NewReader(data)
		n   uint32
	)
	// Read bytes length
	if err := binary.Read(buf, binary.BigEndian, &n); err != nil {
		return err
	}
	// Allocate space
	p.lengths = make([]uint16, n)
	p.heap = data[n+4 : n+n+4]
	// Reconstruct lengths
	for i, b := range data[4 : n+4] {
		p.lengths[i] = uint16(b)
	}
	// Reconstruct hash
	p.reconstruct()
	// Done
	return nil
}

// UnmarshalBinaryV3 initialises this heap from a given set of data bytes. This
// should match exactly the encoding above.
func (p *LocalHeap[T]) UnmarshalBinaryV3(data []byte) error {
	var (
		buf     = bytes.NewReader(data)
		lengths []byte
		n       uint32
	)
	// Read bytes length
	if err := binary.Read(buf, binary.BigEndian, &n); err != nil {
		return err
	}
	// Allocate space
	p.lengths = make([]uint16, n)
	p.heap = data[n+n+4 : n+n+n+4]
	// Reconstruct lengths
	lengths = data[4 : n+n+4]
	//
	for i := range p.lengths {
		msb := uint16(lengths[2*i])
		lsb := uint16(lengths[(2*i)+1])
		p.lengths[i] = (msb << 8) | lsb
	}
	// Reconstruct hash
	p.reconstruct()
	// Done
	return nil
}

// Allocate a new word into the heap, returning its address.  This method is not
// threadsafe.
func (p *LocalHeap[T]) alloc(word T) uint32 {
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
func (p *LocalHeap[T]) rehashIfOverloaded() {
	load := (100 * p.count) / uint(len(p.buckets))
	//
	if load > HEAP_POOL_LOADING {
		// Force a rehash
		p.rehash()
	}
}

// Has checks whether a given word is stored in this heap, or not.
func (p *LocalHeap[T]) has(word T) (uint32, uint64) {
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
func (p *LocalHeap[T]) innerGet(index uint32) T {
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

func (p *LocalHeap[T]) rehash() {
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

func (p *LocalHeap[T]) reconstruct() {
	var n = uint32(len(p.lengths))
	//
	p.count = 1
	// Determine the count
	for index := range n {
		// TODO: handle first word.
		if p.lengths[index] != 0 {
			p.count++
		}
	}
	//
	p.buckets = make([][]uint32, numOfBuckets(p.count))
	// Reconstruct hash
	for index := range n {
		if index == 0 || p.lengths[index] != 0 {
			v := p.innerGet(index)
			hash := v.Hash() % uint64(len(p.buckets))
			// Record index in relevant bucket
			p.buckets[hash] = append(p.buckets[hash], index)
		}
	}
}

func numOfBuckets(count uint) uint {
	var nBuckets = uint(HEAP_POOL_INIT_BUCKETS)
	//
	for {
		load := (100 * count) / nBuckets
		//
		if load <= HEAP_POOL_INIT_BUCKETS {
			return nBuckets
		}
		//
		nBuckets *= 3
	}
}
