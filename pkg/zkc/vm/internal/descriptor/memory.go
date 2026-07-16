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
package descriptor

import (
	"bytes"
	"encoding/gob"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Memory is a module describing a memory region of a program, such as a ROM,
// RAM or write-once memory.  Alongside the registers exposed by every module,
// it records the memory's geometry and kind, together with access
// characteristics (public/private, static, read-only, write-only, read-write).
type Memory[W word.Word[W]] struct {
	moduleBase[W]
	kind     MemoryKind
	contents []W
}

// NewMemory creates a new memory module with the specified parameters. It
// initializes a memory region with a given name, registers, kind (ROM, RAM,
// WOM), geometry (size and layout), and initial contents (for static memories
// only).  NOTE: this will panic is a non-static memory is created with some
// contents.
func NewMemory[W word.Word[W]](name string, registers []Register[W], kind MemoryKind,
	contents []W) *Memory[W] {
	// Sanity check
	if !kind.IsStatic() && len(contents) > 0 {
		panic("unsupported contents for non-static memory")
	}
	//
	return &Memory[W]{newModuleBase(name, registers), kind, contents}
}

// AddressRegisters returns the set of registers making up the address lines of
// this memory (which is exactly the set of input registers).
func (p *Memory[W]) AddressRegisters() []Register[W] {
	return p.Inputs()
}

// DataRegisters returns the set of registers making up the data lines of this
// memory (which is exactly the set of output registers).
func (p *Memory[W]) DataRegisters() []Register[W] {
	return p.Outputs()
}

// Kind returns the underlying kind of memory (e.g. ROM, WOM, RAM, etc)
func (p *Memory[W]) Kind() MemoryKind {
	return p.kind
}

// IsPublic indicates whether this is a public input or output.
func (p *Memory[W]) IsPublic() bool {
	return p.kind.IsPublic()
}

// IsStatic indicates a static (read-only) memory.  That is a ROM which never
// changes across all executions of a given machine.
func (p *Memory[W]) IsStatic() bool {
	return p.kind.IsStatic()
}

// IsReadOnly indicates a read-only memory (which may or may not be static).  A
// non-static read-only memory can change between different executions of a given machine.
func (p *Memory[W]) IsReadOnly() bool {
	return p.kind.IsReadOnly()
}

// IsWriteOnly represents a write-only memory where each element can only be
// written once.
func (p *Memory[W]) IsWriteOnly() bool {
	return p.kind.IsWriteOnly()
}

// IsReadWrite represents the ubiquitous form of memory which supports arbitrary
// reads / writes.  Observe that RAM is always private.
func (p *Memory[W]) IsReadWrite() bool {
	return p.kind.IsReadWrite()
}

// IsPaged determines whether this memory is "paged" or not (i.e. divided into
// pages).
func (p *Memory[W]) IsPaged() bool {
	return p.kind.IsPaged()
}

// IsFunction identifies this module as a memory rather than a callable function.
func (p *Memory[W]) IsFunction() bool {
	return false
}

// IsMemory identifies this module as a memory.
func (p *Memory[W]) IsMemory() bool {
	return true
}

// StaticContents returns the static contents of this memory (if
// applicable).  This will panic if !Kind().IsStatic().
func (p *Memory[W]) StaticContents() []W {
	if p.IsStatic() {
		return p.contents
	}
	//
	panic("non-static memory has no contents")
}

// StaticDepth returns the number of rows described by this static memory.
// This will panic if !Kind().IsStatic().
func (p *Memory[W]) StaticDepth() uint {
	if p.IsStatic() {
		var (
			n = uint(len(p.contents))
			m = n / p.numOutputs
		)
		//
		if m*p.numOutputs != n {
			m++
		}
		//
		return m
	}
	//
	panic("non-static memory has no contents")
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode marshals this memory.  The embedded moduleBase holds unexported
// fields (as do the kind and contents), so an explicit encoding is required.
//
// nolint
func (p *Memory[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.name); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.registers); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.kind); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.contents); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Memory[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		name       string
		registers  []Register[W]
	)
	//
	if err := gobDecoder.Decode(&name); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&registers); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.kind); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.contents); err != nil {
		return err
	}
	// Reconstruct the module base (which recomputes the input / output counts).
	p.moduleBase = newModuleBase(name, registers)
	//
	return nil
}
