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
	// Stamp lines identify the timestamp this access carries: empty before
	// timestamp threading (and always in fast mode), one register after it,
	// several limbs after register splitting.
	Stamp []RegisterId
}

// Uses implementation for Bytecode interface.  A read uses only its address
// registers, whereas a write uses both the address and data registers.  The
// timestamp operand (when present) is read by both.
func (p *ReadWrite[W]) Uses() []RegisterId {
	uses := slices.Clone(p.Address)
	//
	if p.Write {
		uses = append(uses, p.Data...)
	}
	//
	return append(uses, p.Stamp...)
}

// Definitions implementation for Bytecode interface.  A read defines its data
// registers, whereas a write defines nothing in the surrounding frame.
// Discarded data lines of a (static) read bind no register, so they are
// excluded.
func (p *ReadWrite[W]) Definitions() []RegisterId {
	if p.Write {
		return nil
	}
	//
	return boundRegisters(p.Data)
}

// Validate implementation for Bytecode interface.
func (p *ReadWrite[W]) Validate(env Environment[W]) ([]error, bool) {
	var (
		extra        error
		errors, safe = validateOperands(env, p.Address, p.Data, p.Stamp)
	)

	if module := env.Module(p.Id); module.IsEmpty() {
		extra = fmt.Errorf("memory target %d does not exist", p.Id)
	} else if memory := module.Unwrap(); !memory.IsMemory() {
		extra = fmt.Errorf("memory target %d (%s) is not a memory", p.Id, memory.Name())
	} else if p.Write && memory.IsReadOnly() {
		extra = fmt.Errorf("cannot write to read-only memory %s", memory.Name())
	} else if !p.Write && memory.IsWriteOnly() {
		extra = fmt.Errorf("cannot read from write-only memory %s", memory.Name())
	} else if len(p.Address) != int(memory.NumInputs()) {
		extra = fmt.Errorf("memory %s expects %d address registers (found %d)",
			memory.Name(), memory.NumInputs(), len(p.Address))
	} else if len(p.Data) != int(memory.NumOutputs()) {
		extra = fmt.Errorf("memory %s expects %d data registers (found %d)",
			memory.Name(), memory.NumOutputs(), len(p.Data))
	} else if slices.Contains(p.Address, DISCARD) || slices.Contains(p.Stamp, DISCARD) ||
		(p.Write && slices.Contains(p.Data, DISCARD)) {
		extra = fmt.Errorf("memory %s access discards an operand", memory.Name())
	}
	// Sanity check whether additional error arose
	if extra == nil {
		return errors, safe
	}
	//
	return append(errors, extra), false
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
