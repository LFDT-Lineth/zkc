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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
)

// Memory captures the familiar notion of a "machine memory" which can be
// read-only, write-only or read-write.  Furthermore, memory can be static (i.e.
// its contents are fixed for all executions of a machine).
type Memory[W util.Uinter64] = memory.Memory[W]

// ============================================================================
// Constructors
// ============================================================================

// NewStaticMemory constructs a static read-only memory pre-loaded with the
// given values.
func NewStaticMemory[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) Memory[W] {
	return memory.NewStatic[W](name, public, registers, init...)
}

// NewInputMemory constructs a new read-only memory initialised with a given set of values.
func NewInputMemory[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) Memory[W] {
	return memory.NewReadOnly[W](name, public, registers, init...)
}

// NewOutputMemory constructs an empty write-once memory.
func NewOutputMemory[W util.Uinter64](name string, public bool, registers []register.Register) Memory[W] {
	return memory.NewWriteOnce[W](name, public, registers)
}

// NewReadWriteMemory constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
func NewReadWriteMemory[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	return memory.NewRandomAccess[W](name, registers)
}

// NewPagedReadWriteMemory constructs an empty random-access memory which
// represents memory as an array of pages, grown on demand.  This is a
// read/write implementation of Memory suitable for larger memories, provided
// they do not use very high addresses.
func NewPagedReadWriteMemory[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	return memory.NewPagedRandomAccess[W](name, registers)
}
