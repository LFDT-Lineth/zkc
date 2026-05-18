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

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
)

// Memory represents (in many ways) the simplest form of memory
// which can be read or written without restrictions.  Initially, all locations
// of a RAM can be considered to hold zero.  Thus, reading a location which has
// not yet been written will return zero; otherwise, it will return the last
// value written.
type Memory[W util.Uinter64] interface {
	base.Module
	// Geometry defines the geometry of this RAM.
	Geometry() Geometry[W]
	// Kind returns the underlying kind of memory
	Kind() Kind
	// IsPublic indicates whether this is a public input or output.
	IsPublic() bool
	// IsStatic indicates a static (read-only) memory.  That is a ROM which never
	// changes across all executions of a given machine.
	IsStatic() bool
	// IsReadOnly indicates a read-only memory (which may or may not be static).  A
	// non-static read-only memory can change between different executions of a given machine.
	IsReadOnly() bool
	// IsWriteOnly represents a write-only memory where each element can only be
	// written once.
	IsWriteOnly() bool
	// IsReadWrite represents the ubiquitous form of memory which supports arbitrary
	// reads / writes.  Observe that RAM is always private.
	IsReadWrite() bool
	// Initialise this memory with the given contents.  This will overwrite any
	// existing contents.
	Initialise(contents []W)
	// Read (indirect) a given data-tuple from a given address-tuple. The
	// address tuple is formed from the "frame" (i.e. the register-file) using
	// the given register identifiers and, likewise, the target registers are
	// given in data.
	Read(frame []W, address []register.Id, data []register.Id) error
	// Write (indirect) a given data-tuple at a given address-tuple.  The
	// address tuple is formed from the "frame" (i.e. the register-file) using
	// the given register identifiers and, likewise, the source registers are
	// given in data.
	Write(frame []W, address []register.Id, data []register.Id) error
}

// InputOutput identifiers memory used to represent inputs or outputs.  The main
// purpose of this is to enable inspection of said memory to ensure e.g. the
// correct outputs are produced.
type InputOutput[W util.Uinter64] interface {
	Memory[W]
	// Contents returns the contents of this memory as an array.
	Contents() []W
}

// Kind provides relevant information about the underlying memory (e.g. whether
// it is read-only, or read-write, etc).
type Kind struct {
	public, static, read, write bool
}

// IsPublic indicates whether this is a public input or output.
func (p Kind) IsPublic() bool {
	return p.public
}

// IsStatic indicates a static (read-only) memory.  That is a ROM which never
// changes across all executions of a given machine.
func (p Kind) IsStatic() bool {
	return p.static
}

// IsReadOnly indicates a read-only memory (which may or may not be static).  A
// non-static read-only memory can change between different executions of a given machine.
func (p Kind) IsReadOnly() bool {
	return p.read && !p.write
}

// IsWriteOnly represents a write-only memory where each element can only be
// written once.
func (p Kind) IsWriteOnly() bool {
	return !p.read && p.write
}

// IsReadWrite represents the ubiquitous form of memory which supports arbitrary
// reads / writes.  Observe that RAM is always private.
func (p Kind) IsReadWrite() bool {
	return p.read && p.write
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *Kind) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.public); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.static); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.read); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.write); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Kind) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.public); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.static); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.read); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.write); err != nil {
		return err
	}
	//
	return nil
}

var (
	// PUBLIC_STATIC_MEMORY represents a (public) static read-only memory.  That
	// is a ROM which never changes across all executions of a given machine.
	PUBLIC_STATIC_MEMORY = Kind{true, true, true, false}
	// PRIVATE_STATIC_MEMORY represents a (private) static read-only memory.  That
	// is a ROM which never changes across all executions of a given machine.
	PRIVATE_STATIC_MEMORY = Kind{false, true, true, false}
	// PUBLIC_READ_ONLY_MEMORY represents a (public) read-only memory which can
	// change between different executions of a given machine.
	PUBLIC_READ_ONLY_MEMORY = Kind{true, false, true, false}
	// PRIVATE_READ_ONLY_MEMORY represents a (private) read-only memory which
	// can change between different executions of a given machine.
	PRIVATE_READ_ONLY_MEMORY = Kind{false, false, true, false}
	// PUBLIC_WRITE_ONCE_MEMORY represents a (public) write-only memory which can only be
	// written once.
	PUBLIC_WRITE_ONCE_MEMORY = Kind{true, false, false, true}
	// PRIVATE_WRITE_ONCE_MEMORY represents a (private) write-only memory which
	// can only be written once.
	PRIVATE_WRITE_ONCE_MEMORY = Kind{false, false, false, true}
	// RANDOM_ACCESS_MEMORY represents the ubiquitous form of memory which
	// supports arbitrary reads / writes.  Observe that RAM is always private.
	RANDOM_ACCESS_MEMORY = Kind{false, false, true, true}
)
