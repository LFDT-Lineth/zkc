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
package macro

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Fail signals an exceptional return from the enclosing function.
type Fail struct {
	// dummy is included to force Return structs to be stored in the heap.
	//nolint
	Dummy uint
}

// Execute implementation for Instruction interface.
func (p *Fail) Execute(state io.State) uint {
	return io.FAIL
}

// Lower implementation for Instruction interface.
func (p *Fail) Lower(pc uint) micro.Instruction {
	// Lowering here produces an instruction containing a single microcode.
	return micro.NewInstruction(&micro.Fail{})
}

// RegistersRead implementation for Instruction interface.
func (p *Fail) RegistersRead() []io.RegisterId {
	return nil
}

// RegistersWritten implementation for Instruction interface.
func (p *Fail) RegistersWritten() []io.RegisterId {
	return nil
}

func (p *Fail) String(fn register.Map) string {
	return "fail"
}

// Validate implementation for Instruction interface.
func (p *Fail) Validate(fieldWidth uint, fn register.Map) error {
	return nil
}
