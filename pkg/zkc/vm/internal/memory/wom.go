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

// WriteOnce (WOM) represents a form of memory where each cell can be
// written exactly once and, furthermore, cells must be written consecutively
// starting from zero.  Thus, a WOM can be viewed as an output stream (which is
// exactly what they are typically used for).
type WriteOnce[W util.Uinter64] struct {
	StaticArray[W]
}

// Read implementation for Memory interface.
func (p *WriteOnce[W]) Read(frame []W, address []register.Id, data []register.Id) error {
	panic("unsupported operation for write-once memory")
}

// Contents implementation for Memory interface.
func (p *WriteOnce[W]) Contents() []W {
	return p.data
}

// NewWriteOnce constructs an empty write-once memory.
func NewWriteOnce[W util.Uinter64](name string, public bool, registers []register.Register) InputOutput[W] {
	var kind Kind
	//
	if public {
		kind = PUBLIC_WRITE_ONCE_MEMORY
	} else {
		kind = PRIVATE_WRITE_ONCE_MEMORY
	}
	//
	return &WriteOnce[W]{
		StaticArray: NewStaticArray[W](name, kind, registers),
	}
}
