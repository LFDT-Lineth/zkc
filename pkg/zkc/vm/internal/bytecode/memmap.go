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
package bytecode

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

const (
	// STATIC_READONLY_MEMORY identifies the type of SROMs
	STATIC_READONLY_MEMORY MemoryKind = iota
	// READONLY_MEMORY identifies the type of ROMs
	READONLY_MEMORY
	// WRITEONCE_MEMORY identifies the type of WOMs
	WRITEONCE_MEMORY
	// READWRITE_MEMORY identifies the type of RAMs
	READWRITE_MEMORY
	// BIPARTITE_READWRITE_MEMORY identifies the type of BRAMs
	BIPARTITE_READWRITE_MEMORY
)

// MemoryKind identifies the different kinds of memory
type MemoryKind uint8

// MemoryId provides a notion of a memory-specific identifier.  This identifies
// the type of memory (i.e. based on its mode), along with corresponding index.
type MemoryId struct {
	mode  MemoryKind
	index uint16
}

// Map each memory module to a "memory-specific" identifier.  A memory specific
// identifier is essentially the index of that memory with respect to other
// memories of the same kind.
func buildMemoryMap[W word.Word[W]](modules ...Module) []MemoryId {
	var (
		memmap = make([]MemoryId, len(modules))
		//
		nsroms, nroms, nwoms, nsrams, nbrams uint
	)
	// construct memory map
	for i, m := range modules {
		switch m.(type) {
		case *memory.StaticReadOnly[W]:
			memmap[i] = MemoryId{STATIC_READONLY_MEMORY, uint16(nsroms)}
			nsroms++
		case *memory.ReadOnly[W]:
			memmap[i] = MemoryId{READONLY_MEMORY, uint16(nroms)}
			nroms++
		case *memory.WriteOnce[W]:
			memmap[i] = MemoryId{WRITEONCE_MEMORY, uint16(nwoms)}
			nwoms++
		case *memory.RandomAccess[W]:
			memmap[i] = MemoryId{READWRITE_MEMORY, uint16(nsrams)}
			nsrams++
		case *memory.BiPartiteRandomAccess[W]:
			memmap[i] = MemoryId{BIPARTITE_READWRITE_MEMORY, uint16(nbrams)}
			nbrams++
		}
	}
	// Sanity checks
	checkMemoryCount(nroms, "read only")
	checkMemoryCount(nsroms, "static read-only")
	checkMemoryCount(nwoms, "write once")
	checkMemoryCount(nsrams, "(small) random access")
	checkMemoryCount(nbrams, "(bipartite) random access")
	//
	return memmap
}

// Build the reverse memory map.  This maps memory-specific identifiers back to
// their original module-specific form.
func buildReverseMemoryMap[W word.Word[W]](modules ...Module) map[MemoryId]uint16 {
	var (
		rmap = make(map[MemoryId]uint16)
		//
		nsroms, nroms, nwoms, nsrams, nbrams uint
	)
	// construct memory map
	for i, m := range modules {
		switch m.(type) {
		case *memory.StaticReadOnly[W]:
			rmap[MemoryId{STATIC_READONLY_MEMORY, uint16(nsroms)}] = uint16(i)
			nsroms++
		case *memory.ReadOnly[W]:
			rmap[MemoryId{READONLY_MEMORY, uint16(nroms)}] = uint16(i)
			nroms++
		case *memory.WriteOnce[W]:
			rmap[MemoryId{WRITEONCE_MEMORY, uint16(nwoms)}] = uint16(i)
			nwoms++
		case *memory.RandomAccess[W]:
			rmap[MemoryId{READWRITE_MEMORY, uint16(nsrams)}] = uint16(i)
			nsrams++
		case *memory.BiPartiteRandomAccess[W]:
			rmap[MemoryId{BIPARTITE_READWRITE_MEMORY, uint16(nbrams)}] = uint16(i)
			nbrams++
		}
	}
	//
	return rmap
}

func checkMemoryCount(count uint, name string) {
	// NOTE: in reality, this should never be trigged.  But, it is included just
	// in case.
	if count > math.MaxUint16 {
		panic(fmt.Sprintf("too many %s memory modules (%d)", name, count))
	}
}
