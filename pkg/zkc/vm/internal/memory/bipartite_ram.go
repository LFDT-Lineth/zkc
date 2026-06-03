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
)

// HALF_START is the smallest absolute word position belonging to the upper
// partition: positions in [0,HALF_START) go to the lower partition, whilst
// positions in [HALF_START,...] go to the upper partition.  This is fixed at the
// midpoint of the uint64 address space, regardless of the actual address-tuple
// width, so that the partition decision is a simple constant comparison.
const HALF_START uint64 = ^uint64(0) / 2

// TOP_POS is the largest absolute word position.  The upper partition is
// indexed from the top, so upper[j] corresponds to absolute position
// TOP_POS-j.  Like HALF_START, this is fixed at the top of the uint64 address
// space, regardless of the actual address-tuple width.
const TOP_POS uint64 = ^uint64(0)

// BiPartiteRandomAccess provides a read/write implementation of Memory optimised for
// representing the kind of split heap/stack memory found in typical compute
// architectures (e.g. RISC-V).  Here, memory is partitioned in two: the lower
// partition and the upper partition.  Here, the lower partition represents
// memory locations starting from the least addressable location (i.e. address
// 0), whilst upper represents memory locations upto the maximal addressable
// location.  We can view memory as follows:
//
// +-----------------+ ................. +-----------------+
// | lower partition |  (unallocated)    | upper partiaion |
// +-----------------+ ................. + ----------------+
//
//	0                                       n
//
// Here, n represents the largest addressable location (i.e. n==2^64-1). In
// between the two partions is a chunk of currently unallocated memory.  Thus,
// we see that as locations are read / written the two partitions move towards
// each other.  For simplicity we simply assume that any read / write to
// location l where l < n/2 is for the lower partiion, other its for the upper
// partition.
type BiPartiteRandomAccess[W util.Uinter64] struct {
	kind     Kind
	geometry Geometry[W]
	name     string
	// Lower and upper partitions
	lower, upper []W
}

// NewBiPartiteRandomAccess constructs a new bipartite random access memory.
func NewBiPartiteRandomAccess[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	return &BiPartiteRandomAccess[W]{
		kind:     RANDOM_ACCESS_MEMORY,
		geometry: NewGeometry[W](registers),
		name:     name,
	}
}

// Kind implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) Kind() Kind {
	return p.kind
}

// IsPublic implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) IsPublic() bool {
	return p.kind.IsPublic()
}

// IsStatic implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) IsStatic() bool {
	return p.kind.IsStatic()
}

// IsReadOnly implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) IsReadOnly() bool {
	return p.kind.IsReadOnly()
}

// IsWriteOnly implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) IsWriteOnly() bool {
	return p.kind.IsWriteOnly()
}

// IsReadWrite implementation for memory interface.
func (p *BiPartiteRandomAccess[W]) IsReadWrite() bool {
	return p.kind.IsReadWrite()
}

// Name implementation for Memory interface.
func (p *BiPartiteRandomAccess[W]) Name() string {
	return p.name
}

// IsNative implementation for Module interface.  Memory modules are never
// native.
func (p *BiPartiteRandomAccess[W]) IsNative() bool {
	return false
}

// Geometry implementation for Memory interface.
func (p *BiPartiteRandomAccess[W]) Geometry() Geometry[W] {
	return p.geometry
}

// Initialise implementation for Memory interface.  The provided contents
// populate the lower partition; the upper partition is cleared.
func (p *BiPartiteRandomAccess[W]) Initialise(contents []W) {
	p.lower = contents
	p.upper = nil
}

// Read implementation for Memory interface.
func (p *BiPartiteRandomAccess[W]) Read(address uint64) (W, error) {
	var val W
	//
	if address < HALF_START {
		if address < uint64(len(p.lower)) {
			val = p.lower[address]
		}
	} else {
		var idx = TOP_POS - address
		//
		if idx < uint64(len(p.upper)) {
			val = p.upper[idx]
		}
	}
	//
	return val, nil
}

// Write implementation for Memory interface.
func (p *BiPartiteRandomAccess[W]) Write(address uint64, value W) error {
	if address < HALF_START {
		// extend lower partition if needed
		p.lower = expand(p.lower, address+1)
		// copy over values
		p.lower[address] = value
	} else {
		var needed = TOP_POS - address + 1
		// extend upper partition if needed
		p.upper = expand(p.upper, needed)
		//
		p.upper[TOP_POS-address] = value
	}
	//
	return nil
}

// HasRegister implementation for vm.Module interface.
func (p *BiPartiteRandomAccess[W]) HasRegister(name string) (register.Id, bool) {
	for i, r := range p.geometry.registers {
		if r.Name() == name {
			return register.NewId(uint(i)), true
		}
	}
	// Failed
	return register.UnusedId(), false
}

// Register implementation for vm.Module interface.
func (p *BiPartiteRandomAccess[W]) Register(id register.Id) register.Register {
	return p.geometry.registers[id.Unwrap()]
}

// Registers implementation for vm.Module interface.
func (p *BiPartiteRandomAccess[W]) Registers() []register.Register {
	return p.geometry.registers
}

// Width implementation for Module interface.
func (p *BiPartiteRandomAccess[W]) Width() uint {
	return uint(len(p.geometry.registers))
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *BiPartiteRandomAccess[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(&p.kind); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.geometry); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.name); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.lower); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.upper); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *BiPartiteRandomAccess[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.kind); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.geometry); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.name); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.lower); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.upper); err != nil {
		return err
	}
	//
	return nil
}
