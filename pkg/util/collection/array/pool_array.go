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
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/pool"
	"github.com/consensys/go-corset/pkg/util/word"
)

// PoolArray implements an array of elements simply using an underlying array.
type PoolArray[K uint8 | uint16 | uint32, T word.Word[T], P pool.Pool[K, T]] struct {
	// pool of values
	pool P
	// indices into pool
	index []K
	// Bitwidth of words in this array
	bitwidth uint
}

// NewPoolArray constructs a new indexed array.
func NewPoolArray[K uint8 | uint16 | uint32, T word.Word[T], P pool.Pool[K, T]](height uint, bitwidth uint,
	pool P) *PoolArray[K, T, P] {
	//
	index := make([]K, height)
	//
	return &PoolArray[K, T, P]{pool, index, bitwidth}
}

// Append adds a new element to the end of this array
func (p *PoolArray[K, T, P]) Append(element T) MutArray[T] {
	var (
		zero K
		n    = uint(len(p.index))
	)
	// Add new element
	p.index = append(p.index, zero)
	// Set value of that element
	return p.Set(n, element)
}

// Clone makes clones of this array producing an otherwise identical copy.
func (p *PoolArray[K, T, P]) Clone() MutArray[T] {
	// Allocate sufficient memory
	nindex := make([]K, uint(len(p.index)))
	// Copy over the data
	copy(nindex, p.index)
	//
	return &PoolArray[K, T, P]{p.pool, nindex, p.bitwidth}
}

// Len returns the number of elements in this word array.
func (p *PoolArray[K, T, P]) Len() uint {
	return uint(len(p.index))
}

// BitWidth returns the width (in bits) of elements in this array.
func (p *PoolArray[K, T, P]) BitWidth() uint {
	return p.bitwidth
}

// Get returns the field element at the given index in this array.
func (p *PoolArray[K, T, P]) Get(index uint) T {
	return p.pool.Get(p.index[index])
}

// Pad implementation for MutArray interface.
func (p *PoolArray[K, T, P]) Pad(n uint, m uint, padding T) MutArray[T] {
	var (
		// Determine new length
		l = n + m + p.Len()
		// Initialise new array
		index = make([]K, l)
	)
	// copy
	copy(index[n:], p.index)
	p.index = index
	// Front padding!
	for i := range n {
		p.Set(i, padding)
	}
	// Back padding!
	for i := l - m; i < l; i++ {
		p.Set(i, padding)
	}
	//
	return p
}

// Set sets the field element at the given index in this array, overwriting the
// original value.
func (p *PoolArray[K, T, P]) Set(index uint, word T) MutArray[T] {
	//
	p.index[index] = p.pool.Put(word)
	//
	return p
}

// Slice out a subregion of this array.
func (p *PoolArray[K, T, P]) Slice(start uint, end uint) Array[T] {
	return &PoolArray[K, T, P]{
		p.pool,
		p.index[start:end],
		p.bitwidth,
	}
}

func (p *PoolArray[K, T, P]) String() string {
	var sb strings.Builder

	sb.WriteString("[")

	for i := range p.Len() {
		if i != 0 {
			sb.WriteString(",")
		}

		fmt.Fprintf(&sb, "%v", p.Get(i))
	}

	sb.WriteString("]")

	return sb.String()
}
