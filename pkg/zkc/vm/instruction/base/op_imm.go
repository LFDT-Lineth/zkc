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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// ============================================================================
// Opcode-Immediate (OpImm) instruction type
// ============================================================================

// OpImm represents an instruction parameterised solely by an opcode and a uint
// immediate value.  This is used for control-flow instructions such as JUMP
// (where the immediate identifies the branch target), as well as RETURN
// (which ignores the immediate).
type OpImm struct {
	Op        opcode.OpCode
	Immediate uint
}

// OpCode implementation for Instruction interface
func (p *OpImm) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *OpImm) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *OpImm) IsField() bool {
	return true
}

// Uses implementation for Instruction interface.
func (p *OpImm) Uses() []register.Id {
	return nil
}

// Definitions implementation for Instruction interface.
func (p *OpImm) Definitions() []register.Id {
	return nil
}

func (p *OpImm) String(_ SystemMap) string {
	switch p.Op {
	case opcode.JUMP:
		return fmt.Sprintf("jmp %d", p.Immediate)
	case opcode.RETURN:
		return "ret"
	default:
		panic("unknown OpImm32 opcode")
	}
}

// MicroValidate implementation for MicroInstruction interface.
func (p *OpImm) MicroValidate(_ uint, _ field.Config, _ SystemMap) []error {
	return nil
}
