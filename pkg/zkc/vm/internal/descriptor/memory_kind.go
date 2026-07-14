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
)

// MemoryKind provides relevant information about the underlying memory (e.g. whether
// it is read-only, or read-write, etc).
type MemoryKind struct {
	public, static, read, write, paged bool
}

// IsPublic indicates whether this is a public input or output.
func (p MemoryKind) IsPublic() bool {
	return p.public
}

// IsStatic indicates a static (read-only) memory.  That is a ROM which never
// changes across all executions of a given machine.
func (p MemoryKind) IsStatic() bool {
	return p.static
}

// IsReadOnly indicates a read-only memory (which may or may not be static).  A
// non-static read-only memory can change between different executions of a given machine.
func (p MemoryKind) IsReadOnly() bool {
	return p.read && !p.write
}

// IsWriteOnly represents a write-only memory where each element can only be
// written once.
func (p MemoryKind) IsWriteOnly() bool {
	return !p.read && p.write
}

// IsReadWrite represents the ubiquitous form of memory which supports arbitrary
// reads / writes.  Observe that RAM is always private.
func (p MemoryKind) IsReadWrite() bool {
	return p.read && p.write
}

// IsPaged indicates whether this read-write memory is paged (or not).
func (p MemoryKind) IsPaged() bool {
	return p.paged
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *MemoryKind) GobEncode() ([]byte, error) {
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
func (p *MemoryKind) GobDecode(data []byte) error {
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
	PUBLIC_STATIC_MEMORY = MemoryKind{true, true, true, false, false}
	// PRIVATE_STATIC_MEMORY represents a (private) static read-only memory.  That
	// is a ROM which never changes across all executions of a given machine.
	PRIVATE_STATIC_MEMORY = MemoryKind{false, true, true, false, false}
	// PUBLIC_READ_ONLY_MEMORY represents a (public) read-only memory which can
	// change between different executions of a given machine.
	PUBLIC_READ_ONLY_MEMORY = MemoryKind{true, false, true, false, false}
	// PRIVATE_READ_ONLY_MEMORY represents a (private) read-only memory which
	// can change between different executions of a given machine.
	PRIVATE_READ_ONLY_MEMORY = MemoryKind{false, false, true, false, false}
	// PUBLIC_WRITE_ONCE_MEMORY represents a (public) write-only memory which can only be
	// written once.
	PUBLIC_WRITE_ONCE_MEMORY = MemoryKind{true, false, false, true, false}
	// PRIVATE_WRITE_ONCE_MEMORY represents a (private) write-only memory which
	// can only be written once.
	PRIVATE_WRITE_ONCE_MEMORY = MemoryKind{false, false, false, true, false}
	// READWRITE_MEMORY represents the ubiquitous form of memory which supports
	// arbitrary reads / writes.  Observe that RAM is always private.  Also,
	// this variant is unpaged --- meaning it is suitable only for relatively
	// small RAMs.
	READWRITE_MEMORY = MemoryKind{false, false, true, true, false}
	// PAGED_READWRITE_MEMORY represents the ubiquitous form of memory which
	// supports arbitrary reads / writes.  Observe that RAM is always private.
	// This variant is paged --- meaning it is suitable for larger RAMs.
	PAGED_READWRITE_MEMORY = MemoryKind{false, false, true, true, true}
)
