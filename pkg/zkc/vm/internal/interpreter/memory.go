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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Memory represents (in many ways) the simplest form of memory
// which can be read or written without restrictions.  Initially, all locations
// of a RAM can be considered to hold zero.  Thus, reading a location which has
// not yet been written will return zero; otherwise, it will return the last
// value written.
type Memory[W word.Word[W]] interface {
	// Descriptor defines the layout (a.k.a. the geometry) of this memory.  This
	// is responsible for translating multi-word addresses into a contiguous
	// range within a flat data slice.  Memories in this VM address their
	// contents using tuples of words (i.e. []W) rather than a single scalar,
	// because a single field element may not be wide enough to express every
	// valid address.
	Descriptor() *descriptor.Memory[W]
	// Initialise this memory with the given contents.  This will overwrite any
	// existing contents. For WOM's it resets the seen addresses.
	Initialise(contents []W)
	// Read the value at a given physical address within this memory, possibly
	// producing an error (e.g. for an out-of-bounds access).
	Read(address uint64) (W, error)
	// Write to a given physical address within this memory, possibly
	// producing an error (e.g. for an out-of-bounds access).
	Write(address uint64, value W) error
	// Contents returns the contents of this memory as an array.
	Contents() []W
	//
	Checkpoint(mid uint16) checkpoint.Memory
	//
	Restore(checkpoint.Memory)
	// AccessLog returns the chronological log of reads / writes performed
	// against this memory, or nil if it does not record accesses (only
	// read-write memory records, and only when a recording log is installed
	// for trace generation).  The trace observer consumes this to materialise
	// the RAM trace.
	AccessLog() []AccessData[W]
}

// InputOutput identifiers memory used to represent inputs or outputs.  The main
// purpose of this is to enable inspection of said memory to ensure e.g. the
// correct outputs are produced.
type InputOutput[W word.Word[W]] interface {
	// Contents returns the contents of this memory as an array.
	Contents() []W
	// Descriptor defines the layout (a.k.a. the geometry) of this memory
	Descriptor() *descriptor.Memory[W]
	// Initialise this memory with the given contents.  This will overwrite any
	// existing contents.
	Initialise(contents []W)
}

// Pack a given set of words into a given set of bytes, according to a given set
// of descriptors.
func Pack[W word.Word[W]](regs []descriptor.Register[W], data []W) []byte {
	panic("todo")
}

// PackTimed packs a given set of (timestamped) words into a given set of bytes,
// according to a given set of descriptors.
func PackTimed[W word.Word[W]](regs []descriptor.Register[W], data []TimestampedCell[W]) ([]byte, []uint64) {
	panic("todo")
}

// Unpack a set of words from a given set of bytes.
func Unpack[W word.Word[W]](regs []descriptor.Register[W], data []byte) []W {
	panic("todo")
}

// UnpackTimed unpacks a set of (timestamped) words from a given set of bytes.
func UnpackTimed[W word.Word[W]](regs descriptor.Memory[W], data []byte, stamps []uint64) []TimestampedCell[W] {
	panic("todo")
}
