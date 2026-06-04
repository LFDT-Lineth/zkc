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
package memory

import (
	"bytes"
	"encoding/gob"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// Geometry is responsible for translating multi-word addresses into a
// contiguous range within a flat data slice.  Memories in this VM address their
// contents using tuples of words (i.e. []W) rather than a single scalar,
// because a single field element may not be wide enough to express every valid
// address.  An AddressDecoder bridges that representation to the concrete
// (start, end) index pair needed to slice into the underlying []W array.
//
// The returned half-open interval [start, end) identifies the data words that
// correspond to the given address.  Both indices are measured in units of W
// (not bytes), so the caller can write data[start:end] directly.
//
// Implementations are free to impose whatever layout they require.  For
// example, a decoder for a memory that stores 4-word rows at consecutive
// indices might compute start = index*4 and end = index*4 + 4.
type Geometry[W util.Uinter64] struct {
	registers             []register.Register
	numInputs, numOutputs uint
}

// NewGeometry constructs a new geometry from a given set of registers.
func NewGeometry[W util.Uinter64](registers []register.Register) Geometry[W] {
	// Compute internal values
	inputs := array.CountMatching(registers,
		func(r register.Register) bool { return r.IsInput() })
	outputs := array.CountMatching(registers,
		func(r register.Register) bool { return r.IsOutput() })
	// Done
	return Geometry[W]{registers, inputs, outputs}
}

// AddressLines returns the number of address lines
func (p Geometry[W]) AddressLines() uint {
	return p.numInputs
}

// DataLines returns the number of data lines
func (p Geometry[W]) DataLines() uint {
	return p.numOutputs
}

// Registers returns the set of registers used for the address and data lines of
// this memory.
func (p Geometry[W]) Registers() []register.Register {
	return p.registers
}

// AddressRegisters returns the subset of registers used for this memory's
// address lines.
func (p Geometry[W]) AddressRegisters() []register.Register {
	return p.registers[:p.numInputs]
}

// DataRegisters returns the subset of registers used for this memory's
// data lines.
func (p Geometry[W]) DataRegisters() []register.Register {
	return p.registers[p.numInputs:]
}

// Decode maps address (a tuple of words representing a logical memory
// address) to the half-open index range [start, end) within the backing flat
// slice.  The length end-start always equals dataGeometry, i.e. the number of
// data words per row.
//
// The linear row index is computed by packing the address components
// big-endian: each component is shifted left by the total bit width of all
// subsequent components, then OR-ed in.  For a scalar address this reduces to
// index = address[0]; for a tuple (u8, u16) it gives index = address[0]<<16 |
// address[1].
func (p Geometry[W]) Decode(frame []W, address []register.Id) (start, end uint64) {
	var index uint64

	for i, r := range address {
		var bitwidth = uint64(p.registers[i].Width())

		index = (index << bitwidth) | frame[r.Unwrap()].Uint64()
	}

	start = index * uint64(p.numOutputs)

	return start, start + uint64(p.numOutputs)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *Geometry[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.registers); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Geometry[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.registers); err != nil {
		return err
	}
	// Recompute internal values
	p.numInputs = array.CountMatching(p.registers,
		func(r register.Register) bool { return r.IsInput() })
	p.numOutputs = array.CountMatching(p.registers,
		func(r register.Register) bool { return r.IsOutput() })
	//
	return nil
}
