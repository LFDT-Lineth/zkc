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
package interpreter

import (
	"bytes"
	"encoding/gob"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// StaticArray is a memory implementation backed by a fixed-size []C, meaning
// that an out-of-bound read will panic. Reads are performed by delegating
// address decoding to a D (an AddressDecoder) which translates the incoming
// multi-word address tuple into a (start, end) index range, and then returning
// the corresponding sub-slice of the backing data.
//
// The type parameter W is the word type (e.g. a field element or big.Int), and
// C is the cell type stored in the backing slice.  For most memories C is just
// W; RAM stores TimestampedCell[W] so it can track per-cell access timestamps.
type StaticArray[W word.Word[W], C any] struct {
	descriptor descriptor.Memory[W]
	data       []C
}

// NewStaticArray constructs a new array initialised with a given set of values.
func NewStaticArray[W word.Word[W], C any](descriptor descriptor.Memory[W], init ...C) StaticArray[W, C] {
	return StaticArray[W, C]{descriptor, init}
}

// Descriptor implementation for memory interface.
func (p *StaticArray[W, C]) Descriptor() *descriptor.Memory[W] {
	return &p.descriptor
}

// Initialise implementation for Memory interface.
func (p *StaticArray[W, C]) Initialise(contents []C) {
	p.data = contents
}

// Read implementation for Memory interface.
func (p *StaticArray[W, C]) Read(address uint64) (C, error) {
	return p.data[address], nil
}

// Write implementation for Memory interface.
func (p *StaticArray[W, C]) Write(address uint64, value C) error {
	// ensure sufficient space
	p.data = expand(p.data, address+1)
	//
	p.data[address] = value
	//
	return nil
}

// Contents implementation for Memory interface.
func (p *StaticArray[W, C]) Contents() []C {
	return p.data
}

// Expand a slice to ensure it has at least length n.  If the slice already has
// at least n elements it is returned as-is.  Otherwise capacity is grown if
// needed (via slices.Grow, which uses the runtime's append-style growth
// heuristic) and the length is extended to n.
func expand[W any](data []W, n uint64) []W {
	m := uint64(len(data))
	if n > m {
		//
		return slices.Grow(data, int(n-m))[:n]
	}
	//
	return data
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *StaticArray[W, C]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(&p.descriptor); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.data); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *StaticArray[W, C]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.descriptor); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.data); err != nil {
		return err
	}
	//
	return nil
}
