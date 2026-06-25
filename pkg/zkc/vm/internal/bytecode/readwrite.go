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
)

// ReadWrite instruction captures memory read/writes.  It records only whether
// the access is a read or a write; the kind of memory being accessed (ROM, RAM,
// etc.) is resolved from the enclosing environment when the instruction is
// encoded.
type ReadWrite struct {
	// Write distinguishes a memory write (true) from a memory read (false).
	Write bool
	// Identifies the memory being read or written.
	Id uint16
	// Address lines used to determine which data row to read.
	Address []RegisterId
	// Data lines identify where the data row is written.
	Data []RegisterId
}

// Uses implementation for Bytecode interface.  A read uses only its address
// registers, whereas a write uses both the address and data registers.
func (p *ReadWrite) Uses() []RegisterId {
	if p.Write {
		return append(slices.Clone(p.Address), p.Data...)
	}
	//
	return p.Address
}

// Definitions implementation for Bytecode interface.  A read defines its data
// registers, whereas a write defines nothing in the surrounding frame.
func (p *ReadWrite) Definitions() []RegisterId {
	if p.Write {
		return nil
	}
	//
	return p.Data
}

// Validate implementation for Bytecode interface.
func (p *ReadWrite) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *ReadWrite) String(env Environment) string {
	var (
		name    = "???"
		address = RegistersToString(p.Address, env, ",")
		data    = RegistersToString(p.Data, env, ",")
	)
	//
	if env != nil {
		name = env.Module(p.Id).Name()
	}
	//
	if p.Write {
		return fmt.Sprintf("write %s[%s] = %s", name, address, data)
	}
	//
	return fmt.Sprintf("read %s = %s[%s]", data, name, address)
}
