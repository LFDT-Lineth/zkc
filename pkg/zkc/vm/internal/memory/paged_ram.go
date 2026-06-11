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
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// PAGE_SIZE determines the number of words in a single page of a
// PagedRandomAccess memory.  This is fixed at 1M words.
const PAGE_SIZE uint64 = 1 << 20

// PagedRandomAccess provides a read/write implementation of Memory which
// represents memory as a single array of pages (each of PAGE_SIZE words).  The
// page table grows on demand as higher addresses are written, whilst
// individual pages are only allocated on their first write.  Reads of
// locations which have never been written simply return zero.  We can view
// memory as follows:
//
// +--------+--------+ ................. +
// | page 0 | page 1 |  (unallocated)
// +--------+--------+ ................. +
//
//	0        PAGE_SIZE
//
// Since the page table is indexed densely by address / PAGE_SIZE, this
// representation assumes programs do not access very high addresses.
type PagedRandomAccess[W util.Uinter64] struct {
	kind     Kind
	geometry Geometry[W]
	name     string
	// Pages of memory, allocated on demand.
	pages [][]W
}

// NewPagedRandomAccess constructs a new paged random access memory.
func NewPagedRandomAccess[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	return &PagedRandomAccess[W]{
		kind:     RANDOM_ACCESS_MEMORY,
		geometry: NewGeometry[W](registers),
		name:     name,
	}
}

// Kind implementation for memory interface.
func (p *PagedRandomAccess[W]) Kind() Kind {
	return p.kind
}

// IsPublic implementation for memory interface.
func (p *PagedRandomAccess[W]) IsPublic() bool {
	return p.kind.IsPublic()
}

// IsStatic implementation for memory interface.
func (p *PagedRandomAccess[W]) IsStatic() bool {
	return p.kind.IsStatic()
}

// IsReadOnly implementation for memory interface.
func (p *PagedRandomAccess[W]) IsReadOnly() bool {
	return p.kind.IsReadOnly()
}

// IsWriteOnly implementation for memory interface.
func (p *PagedRandomAccess[W]) IsWriteOnly() bool {
	return p.kind.IsWriteOnly()
}

// IsReadWrite implementation for memory interface.
func (p *PagedRandomAccess[W]) IsReadWrite() bool {
	return p.kind.IsReadWrite()
}

// Name implementation for Memory interface.
func (p *PagedRandomAccess[W]) Name() string {
	return p.name
}

// IsNative implementation for Module interface.  Memory modules are never
// native.
func (p *PagedRandomAccess[W]) IsNative() bool {
	return false
}

// Geometry implementation for Memory interface.
func (p *PagedRandomAccess[W]) Geometry() Geometry[W] {
	return p.geometry
}

// Initialise implementation for Memory interface.  The provided contents
// populate memory starting from address zero; all other locations are cleared.
func (p *PagedRandomAccess[W]) Initialise(contents []W) {
	p.pages = nil
	//
	for len(contents) > 0 {
		var (
			n    = min(uint64(len(contents)), PAGE_SIZE)
			page = make([]W, PAGE_SIZE)
		)
		//
		copy(page, contents[:n])
		p.pages = append(p.pages, page)
		contents = contents[n:]
	}
}

// Read implementation for Memory interface.
func (p *PagedRandomAccess[W]) Read(address uint64) (W, error) {
	var (
		val    W
		page   = address / PAGE_SIZE
		offset = address % PAGE_SIZE
	)
	//
	if page < uint64(len(p.pages)) && p.pages[page] != nil {
		val = p.pages[page][offset]
	}
	//
	return val, nil
}

// Write implementation for Memory interface.
func (p *PagedRandomAccess[W]) Write(address uint64, value W) error {
	var (
		page   = address / PAGE_SIZE
		offset = address % PAGE_SIZE
	)
	// extend page table if needed
	p.pages = expand(p.pages, page+1)
	// allocate page if needed
	if p.pages[page] == nil {
		p.pages[page] = make([]W, PAGE_SIZE)
	}
	//
	p.pages[page][offset] = value
	//
	return nil
}

// Contents implementation for Memory interface.
func (p *PagedRandomAccess[W]) Contents() []W {
	panic("unsupported operation")
}

// HasRegister implementation for vm.Module interface.
func (p *PagedRandomAccess[W]) HasRegister(name string) (register.Id, bool) {
	for i, r := range p.geometry.registers {
		if r.Name() == name {
			return register.NewId(uint(i)), true
		}
	}
	// Failed
	return register.UnusedId(), false
}

// Register implementation for vm.Module interface.
func (p *PagedRandomAccess[W]) Register(id register.Id) register.Register {
	return p.geometry.registers[id.Unwrap()]
}

// RegisterMap returns a register map view of the registers declared by this
// function.
func (p *PagedRandomAccess[W]) RegisterMap() register.Map {
	name := trace.ModuleName{Name: p.Name(), Multiplier: 1}
	return register.ArrayMap(name, p.Registers()...)
}

// Registers implementation for vm.Module interface.
func (p *PagedRandomAccess[W]) Registers() []register.Register {
	return p.geometry.registers
}

// Width implementation for Module interface.
func (p *PagedRandomAccess[W]) Width() uint {
	return uint(len(p.geometry.registers))
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *PagedRandomAccess[W]) GobEncode() ([]byte, error) {
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
	if err := gobEncoder.Encode(p.pages); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *PagedRandomAccess[W]) GobDecode(data []byte) error {
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
	if err := gobDecoder.Decode(&p.pages); err != nil {
		return err
	}
	//
	return nil
}
