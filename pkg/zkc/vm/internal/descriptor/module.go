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
package descriptor

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Module is the base descriptor for a unit of a compiled program.  It is a
// named collection of registers, partitioned into inputs, outputs and
// (internal) computed registers.  Both functions and memories are modules, and
// this interface captures the register-related structure common to them.
type Module[W word.Word[W]] interface {
	RegisterMap[W]
	// IsFunction indicates whether this module is a callable function.
	IsFunction() bool
	// IsMemory indicates whether this module supports memory accesses.
	IsMemory() bool
	// IsReadOnly indicates whether this module forbids writes.
	IsReadOnly() bool
	// IsWriteOnly indicates whether this module forbids reads.
	IsWriteOnly() bool
	// Inputs returns the set of input registers for this module.
	Inputs() []Register[W]
	// NumInputs returns the number of input registers for this module.
	NumInputs() uint
	// NumOutputs returns the number of output registers for this module.
	NumOutputs() uint
	// Outputs returns the set of output registers for this module.
	Outputs() []Register[W]
}

type moduleBase[W word.Word[W]] struct {
	// Unique name of this function.
	name string
	// Registers describes zero or more registers of a given width.  Each
	// register can be designated as an input / output or temporary.
	registers []Register[W]
	// Number of input registers
	numInputs uint
	// Number of output registers
	numOutputs uint
}

// New constructs a new function with the given components.
func newModuleBase[W word.Word[W]](name string, registers []Register[W]) moduleBase[W] {
	//
	var (
		numInputs  = array.CountMatching(registers, func(r Register[W]) bool { return r.IsInput() })
		numOutputs = array.CountMatching(registers, func(r Register[W]) bool { return r.IsOutput() })
	)
	// Check registers sorted as: inputs, outputs then internal.
	if !set.IsSorted(registers, func(r Register[W]) register.Type { return r.kind }) {
		panic("function registers ordered incorrectly")
	}
	// All good
	return moduleBase[W]{name, registers, numInputs, numOutputs}
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p *moduleBase[W]) HasRegister(name string) util.Option[RegisterId] {
	for i, r := range p.registers {
		if r.Name() == name {
			return util.Some(RegisterId(i))
		}
	}
	// Failed
	return util.None[RegisterId]()
}

// Inputs returns the set of input registers for this function.
func (p *moduleBase[W]) Inputs() []Register[W] {
	return p.registers[:p.numInputs]
}

// NumInputs returns the number of input registers for this function.
func (p *moduleBase[W]) NumInputs() uint {
	return p.numInputs
}

// NumOutputs returns the number of output registers for this function.
func (p *moduleBase[W]) NumOutputs() uint {
	return p.numOutputs
}

// Name returns the name of this function.
func (p *moduleBase[W]) Name() string {
	// Functions always have a multiplier of 1.
	return p.name
}

// Outputs returns the set of output registers for this function.
func (p *moduleBase[W]) Outputs() []Register[W] {
	return p.registers[p.numInputs : p.numInputs+p.numOutputs]
}

// Register returns the ith register used in this function.
func (p *moduleBase[W]) Register(id RegisterId) Register[W] {
	return p.registers[id]
}

// Registers returns the set of all registers used during execution of this
// function.
func (p *moduleBase[W]) Registers() []Register[W] {
	return p.registers
}

// Width returns the number of registers in this module.'
func (p *moduleBase[W]) Width() uint {
	return uint(len(p.registers))
}
