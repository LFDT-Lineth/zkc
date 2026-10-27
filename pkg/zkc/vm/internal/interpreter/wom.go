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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// WriteOnce (WOM) represents a form of memory where each cell holds at most
// one value: a write to a cell which already holds a different value fails,
// whilst a write which agrees with the existing value is permitted (and has no
// effect).  This matches exactly what the constraints enforce, where every
// write is bound (by lookup) to a single table row per address, since the
// constraints cannot distinguish a repeated (agreeing) write from the original.
//
// Agreeing rewrites also arise naturally during sharded tracing, where the
// complete (final) output is replicated into every shard's checkpoint (see
// Restore) such that each shard's trace carries the whole output table.  The
// writes a shard then performs land on cells which already hold their final
// value.
type WriteOnce[W word.Word[W]] struct {
	StaticArray[W, W]
}

// Write implementation for Memory interface.  This fails if the cell already
// holds a different value.
func (p *WriteOnce[W]) Write(address uint64, value W) error {
	if address < uint64(len(p.data)) && p.data[address].Cmp(value) != 0 {
		return fmt.Errorf("address %x already written for write-only memory %s", address, p.descriptor.Name())
	}
	// Ensure sufficient space
	p.data = expand(p.data, address+1)
	// Write data
	p.data[address] = value
	//
	return nil
}

// Initialise implementation for Memory interface.  Resets the backing array so
// write-once tracking starts fresh on every execution (Boot calls Initialise on
// each memory before running).
func (p *WriteOnce[W]) Initialise(contents []W) {
	p.StaticArray.Initialise(contents)
}

// Read implementation for Memory interface.
func (p *WriteOnce[W]) Read(address uint64) (W, error) {
	panic("unsupported operation for write-once memory")
}

// Checkpoint implementation for memory interface
func (p *WriteOnce[W]) Checkpoint(mid uint16, field word.Config) checkpoint.Memory {
	var (
		bytes = Pack(field, p.descriptor.DataRegisters(), p.data)
		page  = checkpoint.NewPage(0, bytes)
	)
	//
	return checkpoint.NewMemory(mid, 0, page)
}

// Restore implementation for memory interface.  Restored cells may be rewritten
// by the resumed execution provided the values agree (see Write).
func (p *WriteOnce[W]) Restore(m checkpoint.Memory, field word.Config) {
	var pages = m.Pages()
	// Sanity check
	util.Assert(len(pages) == 1, "write once memory requires one page")
	// Unpack data
	p.data = Unpack(field, p.descriptor.DataRegisters(), pages[0].Bytes())
}

// NewWriteOnce constructs an empty write-once memory.
func NewWriteOnce[W word.Word[W]](descriptor descriptor.Memory[W]) *WriteOnce[W] {
	return &WriteOnce[W]{
		StaticArray: NewStaticArray[W, W](descriptor),
	}
}
