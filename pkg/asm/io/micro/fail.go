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
package micro

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Fail signals a return from the enclosing function.
type Fail struct {
	// dummy is included to force Fail structs to be stored in the heap.
	//nolint
	dummy uint
}

// Clone this micro code.
func (p *Fail) Clone() Code {
	return p
}

// MicroExecute a given micro-code, using a given local state.  This may update
// the register values, and returns either the number of micro-codes to "skip
// over" when executing the enclosing instruction or, if skip==0, a destination
// program counter (which can signal return of enclosing function).
func (p *Fail) MicroExecute(state io.State) (uint, uint) {
	return 0, io.FAIL
}

// RegistersRead returns the set of registers read by this instruction.
func (p *Fail) RegistersRead() []io.RegisterId {
	return nil
}

// RegistersWritten returns the set of registers written by this instruction.
func (p *Fail) RegistersWritten() []io.RegisterId {
	return nil
}

// Split this micro code using registers of arbirary width into one or more
// micro codes using registers of a fixed maximum width.
func (p *Fail) Split(mapping register.LimbsMap, env agnostic.RegisterAllocator) []Code {
	return []Code{p}
}

func (p *Fail) String(fn register.Map) string {
	return "fail"
}

// Validate checks whether or not this instruction is correctly balanced.
func (p *Fail) Validate(fieldWidth uint, fn register.Map) error {
	return nil
}
