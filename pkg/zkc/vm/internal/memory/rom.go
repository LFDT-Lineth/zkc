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
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
)

// ReadOnly (ROM) represents a form of memory that can only be read during
// a given execution, but never written.  Thus, its contents are unchanged
// across a given execution.  ROMs can be static or dynamic.  The latter
// represents those which are fixed across all executions of a given machine,
// whilst the latter represent those which can change between different
// executions.  To understand this, consider the two ways in which ROMs are
// used: as inputs, and as static reference tables.  Dynamic ROMs correspond
// with inputs to the machine where, for example, we might want to execute the
// same program with different input data.  In constrast, static ROMs correspond
// to fixed tables used within the program (e.g. in a hash function such as
// BLAKE or KECCAK, there are fixed lookup tables used as part of the program).
type ReadOnly[W util.Uinter64] struct {
	StaticArray[W]
}

// Write implementation for Memory interface.
func (p *ReadOnly[W]) Write(address uint64, value W) error {
	panic("unsupported operation for read-only memory")
}

// Contents implementation for Memory interface.
func (p *ReadOnly[W]) Contents() []W {
	return p.data
}

// NewReadOnly constructs a new read-only memory initialised with a given set of values.
func NewReadOnly[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) InputOutput[W] {
	//
	var kind Kind
	//
	if public {
		kind = PUBLIC_READ_ONLY_MEMORY
	} else {
		kind = PRIVATE_READ_ONLY_MEMORY
	}
	//
	return &ReadOnly[W]{
		StaticArray: NewStaticArray[W](name, kind, registers, init...),
	}
}
