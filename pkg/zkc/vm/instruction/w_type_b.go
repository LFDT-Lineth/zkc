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
package instruction

import (
	"slices"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// ============================================================================
// Opcode-Register-Registers-Constant instruction type
// ============================================================================

// WordTypeB represents an instruction of the following form:
//
// t0 := r0 # ... # rn # c
//
// Here, t0 is the *target register*, whilst r0 .. rn are the source registers
// and c is a constant (which can be 0).  Finally, "#" represents whatever
// operation the given opcode indicates.
type WordTypeB[W word.Word[W]] struct {
	Op opcode.OpCode
	// Target register for assignment
	Target register.Id
	// Source registers for assignment
	Sources []register.Id
	// Constant for assignment
	Constant W
}

// NewWordTypeB constructs a new bitwise instruction
func NewWordTypeB[W word.Word[W]](op opcode.OpCode, target register.Id, sources []register.Id, constant W,
) *WordTypeB[W] {
	if !slices.Contains(opcode.TYPE_B_OPCODES, op) {
		panic("invalid bitwise operation")
	}
	//
	return &WordTypeB[W]{op, target, sources, constant}
}

// OpCode implementation for Instruction interface
func (p *WordTypeB[W]) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *WordTypeB[W]) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *WordTypeB[W]) Uses() []register.Id {
	return p.Sources
}

// Definitions implementation for Instruction interface
func (p *WordTypeB[W]) Definitions() []register.Id {
	return []register.Id{p.Target}
}

// MicroValidate implementation for MicroInstruction interface.
func (p *WordTypeB[W]) MicroValidate(_ uint, field field.Config, _ base.SystemMap) []error {
	return nil
}

func (p *WordTypeB[W]) String(mapping base.SystemMap) string {
	var (
		builder strings.Builder
		op      = bType2Operation(p.Op)
		zero    W
	)
	//
	builder.WriteString(base.RegistersToString(mapping, p.Target))
	builder.WriteString(" = ")
	//
	if p.Constant.Cmp(zero) == 0 && len(p.Sources) > 0 &&
		(p.Op == opcode.INT_DIV || p.Op == opcode.INT_REM ||
			p.Op == opcode.INT_ADDMOD_P || p.Op == opcode.INT_SUBMOD_P ||
			p.Op == opcode.BIT_AND || p.Op == opcode.BIT_NOT ||
			p.Op == opcode.BIT_OR || p.Op == opcode.BIT_XOR ||
			p.Op == opcode.BIT_SHL || p.Op == opcode.BIT_SHR) {
		//
		builder.WriteString(base.ExpressionToStringWithoutConst(op, p.Sources, mapping))
	} else {
		builder.WriteString(base.ExpressionToString(op, p.Sources, p.Constant, mapping))
	}
	//
	return builder.String()
}

func bType2Operation(op opcode.OpCode) string {
	switch op {
	case opcode.INT_DIV:
		return "/"
	case opcode.INT_REM:
		return "%"
	case opcode.INT_ADDMOD_P:
		return "+f"
	case opcode.INT_SUBMOD_P:
		return "-f"
	case opcode.INT_MULMOD_P:
		return "*f"
	case opcode.BIT_AND:
		return "&"
	case opcode.BIT_NOT:
		return "~"
	case opcode.BIT_OR:
		return "|"
	case opcode.BIT_XOR:
		return "^"
	case opcode.BIT_SHL:
		return "<<"
	case opcode.BIT_SHR:
		return ">>"
	default:
		panic("unknown type B instruction")
	}
}
