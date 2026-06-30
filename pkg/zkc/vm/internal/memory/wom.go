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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// WriteOnce (WOM) represents a form of memory where each cell can be
// written exactly once and, furthermore, cells must be written consecutively
// starting from zero.  Thus, a WOM can be viewed as an output stream (which is
// exactly what they are typically used for).
type WriteOnce[W util.Uinter64] struct {
	StaticArray[W]
	writtenToAddresses map[uint64]bool
}

// Write implementation for Memory interface.
func (p *WriteOnce[W]) Write(address uint64, value W) error {
	if _, ok := p.writtenToAddresses[address]; ok {
		return fmt.Errorf("address ≡ %x of WOM ≡ %s was already written to", address, p.Name())
	}
	// ensure sufficient space
	p.data = expand(p.data, address+1)
	//
	p.writtenToAddresses[address] = true
	p.data[address] = value
	//
	return nil
}

// Initialise implementation for Memory interface.  Resets the backing array
// and the set of written addresses, so write-once tracking starts fresh on
// every execution (Boot calls Initialise on each memory before running).
func (p *WriteOnce[W]) Initialise(contents []W) {
	p.StaticArray.Initialise(contents)
	p.writtenToAddresses = make(map[uint64]bool)
}

// Read implementation for Memory interface.
func (p *WriteOnce[W]) Read(address uint64) (W, error) {
	panic("unsupported operation for write-once memory")
}

// NewWriteOnce constructs an empty write-once memory.
func NewWriteOnce[W util.Uinter64](name string, public bool, geometry Geometry[W]) *WriteOnce[W] {
	var kind Kind
	//
	if public {
		kind = PUBLIC_WRITE_ONCE_MEMORY
	} else {
		kind = PRIVATE_WRITE_ONCE_MEMORY
	}
	//
	return &WriteOnce[W]{
		StaticArray:        NewStaticArray[W](name, kind, geometry),
		writtenToAddresses: make(map[uint64]bool),
	}
}
