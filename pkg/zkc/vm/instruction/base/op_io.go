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
package base

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// ============================================================================
// Opcode-Io-Regs-Regs instruction type
// ============================================================================

// OpIo represents an instruction operating on a target module identified
// by Id, with a list of return (output) registers and a list of argument
// (input) registers.  It is similar in shape to a function call, but
// additionally carries an opcode allowing the same structure to be reused for
// distinct operations (e.g. function calls and memory reads/writes).
type OpIo struct {
	// Op identifies the specific operation this instruction represents.
	Op opcode.OpCode
	// Module identifier for the target of the operation.
	Id uint
	// Argument registers providing the inputs to the operation.
	Arguments []register.Id
	// Return registers which receive the outputs of the operation.
	Returns []register.Id
}

// OpCode implementation for Instruction interface
func (p *OpIo) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *OpIo) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *OpIo) IsField() bool {
	return true
}

// Address is an alias to help identify which are the data lines for a memory
// operation.
func (p *OpIo) Address() []register.Id {
	return p.Arguments
}

// Data is an alias to help identify which are the data lines for a memory
// operation.
func (p *OpIo) Data() []register.Id {
	return p.Returns
}

// Uses implementation for Instruction interface
func (p *OpIo) Uses() []register.Id {
	// A memory write uses both the address and data registers.
	if p.Op == opcode.MEMORY_WRITE {
		var data set.AnySortedSet[register.Id]
		//
		for _, t := range p.Returns {
			data.Insert(t)
		}
		//
		for _, s := range p.Arguments {
			data.Insert(s)
		}
		//
		return data
	}
	//
	return p.Arguments
}

// Definitions implementation for Instruction interface
func (p *OpIo) Definitions() []register.Id {
	// A memory write does not define any registers.
	if p.Op == opcode.MEMORY_WRITE {
		return nil
	}
	//
	return p.Returns
}

// MicroValidate implementation for MicroInstruction interface.
func (p *OpIo) MicroValidate(_ uint, _ field.Config, _ SystemMap) []error {
	return nil
}

func (p *OpIo) String(mapping SystemMap) string {
	var builder strings.Builder
	//
	switch p.Op {
	//
	case opcode.CALL:
		//
		if len(p.Returns) > 0 {
			builder.WriteString(RegistersToString(mapping, p.Returns...))
			builder.WriteString(" = ")
		}
		//
		fmt.Fprintf(&builder, "%s(%s)", getModuleName(mapping, p.Id),
			RegistersToString(mapping, p.Arguments...))
	case opcode.MEMORY_READ:
		builder.WriteString(RegistersToString(mapping, p.Returns...))
		builder.WriteString(" = ")
		//
		fmt.Fprintf(&builder, "%s[%s]", getModuleName(mapping, p.Id),
			RegistersToString(mapping, p.Arguments...))
	case opcode.MEMORY_WRITE:
		fmt.Fprintf(&builder, "%s[%s] = %s", getModuleName(mapping, p.Id),
			RegistersToString(mapping, p.Arguments...),
			RegistersToString(mapping, p.Returns...))
	}
	//
	return builder.String()
}

func getModuleName(mapping SystemMap, id uint) string {
	if mapping == nil {
		return "??"
	}
	//
	return mapping.Module(id).Name()
}
