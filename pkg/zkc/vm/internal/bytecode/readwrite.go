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
package bytecode

import (
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ReadWrite instruction captures memory read/writes.  It records only whether
// the access is a read or a write; the kind of memory being accessed (ROM, RAM,
// etc.) is resolved from the enclosing environment when the instruction is
// encoded.
type ReadWrite[W word.Word[W]] struct {
	// Write distinguishes a memory write (true) from a memory read (false).
	Write bool
	// Identifies the memory being read or written.
	Id uint16
	// Address lines used to determine which data row to read.
	Address []RegisterId
	// Data lines identify where the data row is written.
	Data []RegisterId
	// Stamp holds the timestamp operand of an access to a read-write memory
	// (i.e. the "s" in "mem[s; addr]").  It is empty for read-only / write-once
	// memories and before timestamp threading; the ThreadTimestamps transform
	// populates it.  A single register before register splitting; multiple limbs
	// afterwards.
	Stamp []RegisterId
}

// Uses implementation for Bytecode interface.  A read uses only its address
// registers, whereas a write uses both the address and data registers.
func (p *ReadWrite[W]) Uses() []RegisterId {
	uses := slices.Clone(p.Address)
	//
	if p.Write {
		uses = append(uses, p.Data...)
	}
	// The timestamp operand (present after timestamp threading) is read by both
	// reads and writes.
	return append(uses, p.Stamp...)
}

// Definitions implementation for Bytecode interface.  A read defines its data
// registers, whereas a write defines nothing in the surrounding frame.
func (p *ReadWrite[W]) Definitions() []RegisterId {
	if p.Write {
		return nil
	}
	//
	return p.Data
}

// Validate implementation for Bytecode interface.
func (p *ReadWrite[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errors := validateOperands(env, p.Address, p.Data)

	module := env.Module(p.Id)
	if module.IsEmpty() {
		return append(errors, fmt.Errorf("memory target %d does not exist", p.Id))
	}

	memory := module.Unwrap()
	if !memory.IsMemory() {
		return append(errors, fmt.Errorf("memory target %d (%s) is not a memory", p.Id, memory.Name()))
	}

	if p.Write && memory.IsReadOnly() {
		errors = append(errors, fmt.Errorf("cannot write to read-only memory %s", memory.Name()))
	} else if !p.Write && memory.IsWriteOnly() {
		errors = append(errors, fmt.Errorf("cannot read from write-only memory %s", memory.Name()))
	}

	if len(p.Address) != int(memory.NumInputs()) {
		errors = append(errors, fmt.Errorf("memory %s expects %d address registers (found %d)",
			memory.Name(), memory.NumInputs(), len(p.Address)))
	}

	if len(p.Data) != int(memory.NumOutputs()) {
		errors = append(errors, fmt.Errorf("memory %s expects %d data registers (found %d)",
			memory.Name(), memory.NumOutputs(), len(p.Data)))
	}

	return errors
}

func (p *ReadWrite[W]) String(env Environment[W]) string {
	var (
		name    = "???"
		address = RegistersToString(p.Address, env, ",")
		data    = RegistersToString(p.Data, env, ",")
	)
	//
	if env != nil {
		if module := env.Module(p.Id); module.HasValue() {
			name = module.Unwrap().Name()
		}
	}
	// Render the timestamp operand as "stamp; addr" when present.
	index := address
	if len(p.Stamp) != 0 {
		index = RegistersToString(p.Stamp, env, ",") + "; " + address
	}
	//
	if p.Write {
		return fmt.Sprintf("write %s[%s] = %s", name, index, data)
	}
	//
	return fmt.Sprintf("read %s = %s[%s]", data, name, index)
}
