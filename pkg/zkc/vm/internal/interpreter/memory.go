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
