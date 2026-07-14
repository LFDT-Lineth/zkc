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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Memory describes the familiar notion of a "machine memory" which can be
// read-only, write-only or read-write.  Furthermore, memory can be static (i.e.
// its contents are fixed for all executions of a machine).
type Memory[W Word[W]] = descriptor.Memory[W]

// RuntimeMemory captures the notion of an executing memory, rather than a
// simple static descriptor.  That is, one whose current contents can be
// accessed.
type RuntimeMemory[W Word[W]] = interpreter.Memory[W]

// MemoryKind identifies the kind of a memory: its access mode (read-only,
// write-once or read-write), its visibility (public/private), and whether it is
// static or paged.  Used with NewBytecodeMemory.
type MemoryKind = descriptor.MemoryKind

// Memory kinds, re-exported for use with NewBytecodeMemory.
var (
	// PUBLIC_STATIC_MEMORY is a public static (compile-time initialised) read-only memory.
	PUBLIC_STATIC_MEMORY = descriptor.PUBLIC_STATIC_MEMORY
	// PRIVATE_STATIC_MEMORY is a private static (compile-time initialised) read-only memory.
	PRIVATE_STATIC_MEMORY = descriptor.PRIVATE_STATIC_MEMORY
	// PUBLIC_READ_ONLY_MEMORY is a public read-only memory.
	PUBLIC_READ_ONLY_MEMORY = descriptor.PUBLIC_READ_ONLY_MEMORY
	// PRIVATE_READ_ONLY_MEMORY is a private read-only memory.
	PRIVATE_READ_ONLY_MEMORY = descriptor.PRIVATE_READ_ONLY_MEMORY
	// PUBLIC_WRITE_ONCE_MEMORY is a public write-once memory.
	PUBLIC_WRITE_ONCE_MEMORY = descriptor.PUBLIC_WRITE_ONCE_MEMORY
	// PRIVATE_WRITE_ONCE_MEMORY is a private write-once memory.
	PRIVATE_WRITE_ONCE_MEMORY = descriptor.PRIVATE_WRITE_ONCE_MEMORY
	// READWRITE_MEMORY is a (private) random-access read-write memory.
	READWRITE_MEMORY = descriptor.READWRITE_MEMORY
	// PAGED_READWRITE_MEMORY is a (private) paged random-access read-write memory.
	PAGED_READWRITE_MEMORY = descriptor.PAGED_READWRITE_MEMORY
)

// NewBytecodeMemory constructs a memory (descriptor) module directly from its
// name, kind and registers.  Since a memory has no body, this descriptor is its
// final form (cf. NewBytecodeFunction).  The geometry is derived from the
// registers; init supplies the static contents and must be empty for non-static
// memories.
func NewBytecodeMemory[W word.Word[W]](name string, kind MemoryKind, registers []Register[W], init ...W,
) *Memory[W] {
	return descriptor.NewMemory(name, registers, kind, init)
}
