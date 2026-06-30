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
package vm

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Memory captures the familiar notion of a "machine memory" which can be
// read-only, write-only or read-write.  Furthermore, memory can be static (i.e.
// its contents are fixed for all executions of a machine).
type Memory[W util.Uinter64] = memory.Memory[W]

// MemoryKind identifies the kind of a memory: its access mode (read-only,
// write-once or read-write), its visibility (public/private), and whether it is
// static or paged.  Used with NewBytecodeMemory.
type MemoryKind = memory.Kind

// Memory kinds, re-exported for use with NewBytecodeMemory.
var (
	// PUBLIC_STATIC_MEMORY is a public static (compile-time initialised) read-only memory.
	PUBLIC_STATIC_MEMORY = memory.PUBLIC_STATIC_MEMORY
	// PRIVATE_STATIC_MEMORY is a private static (compile-time initialised) read-only memory.
	PRIVATE_STATIC_MEMORY = memory.PRIVATE_STATIC_MEMORY
	// PUBLIC_READ_ONLY_MEMORY is a public read-only memory.
	PUBLIC_READ_ONLY_MEMORY = memory.PUBLIC_READ_ONLY_MEMORY
	// PRIVATE_READ_ONLY_MEMORY is a private read-only memory.
	PRIVATE_READ_ONLY_MEMORY = memory.PRIVATE_READ_ONLY_MEMORY
	// PUBLIC_WRITE_ONCE_MEMORY is a public write-once memory.
	PUBLIC_WRITE_ONCE_MEMORY = memory.PUBLIC_WRITE_ONCE_MEMORY
	// PRIVATE_WRITE_ONCE_MEMORY is a private write-once memory.
	PRIVATE_WRITE_ONCE_MEMORY = memory.PRIVATE_WRITE_ONCE_MEMORY
	// READWRITE_MEMORY is a (private) random-access read-write memory.
	READWRITE_MEMORY = memory.READWRITE_MEMORY
	// PAGED_READWRITE_MEMORY is a (private) paged random-access read-write memory.
	PAGED_READWRITE_MEMORY = memory.PAGED_READWRITE_MEMORY
)

// NewBytecodeMemory constructs a memory (descriptor) module directly from its
// name, kind and registers.  Since a memory has no body, this descriptor is its
// final form (cf. NewBytecodeFunction).  The geometry is derived from the
// registers; init supplies the static contents and must be empty for non-static
// memories.
func NewBytecodeMemory[W word.Word[W]](name string, kind MemoryKind, registers []Register[W], init ...W,
) *BytecodeMemory[W] {
	return descriptor.NewMemory(name, registers, kind, init)
}

// ============================================================================
// Constructors
// ============================================================================

// NewStaticMemory constructs a static read-only memory pre-loaded with the
// given values.
func NewStaticMemory[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) Memory[W] {
	var geometry = memory.NewGeometry[W](registers)
	return memory.NewStatic[W](name, public, geometry, init...)
}

// NewInputMemory constructs a new read-only memory initialised with a given set of values.
func NewInputMemory[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) Memory[W] {
	var geometry = memory.NewGeometry[W](registers)
	return memory.NewReadOnly[W](name, public, geometry, init...)
}

// NewOutputMemory constructs an empty write-once memory.
func NewOutputMemory[W util.Uinter64](name string, public bool, registers []register.Register) Memory[W] {
	var geometry = memory.NewGeometry[W](registers)
	return memory.NewWriteOnce[W](name, public, geometry)
}

// NewReadWriteMemory constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
func NewReadWriteMemory[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	var geometry = memory.NewGeometry[W](registers)
	return memory.NewRandomAccess(name, geometry)
}

// NewPagedReadWriteMemory constructs an empty random-access memory which
// represents memory as an array of pages, grown on demand.  This is a
// read/write implementation of Memory suitable for larger memories, provided
// they do not use very high addresses.
func NewPagedReadWriteMemory[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	var geometry = memory.NewGeometry[W](registers)
	return memory.NewPagedRandomAccess(name, geometry)
}
