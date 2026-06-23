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
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
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
type WordTypeB struct {
	Op opcode.OpCode
	// Bitwidth for the operation.  Observe that this does not have to match the
	// widths of either the source or target operands.
	Bitwidth uint
	// Target register for assignment
	Target register.Id
	// Source register (left) for assignment
	LeftSource register.Id
	// Source register (right) for assignment
	RightSource register.Id
}

// NewWordTypeB constructs a new bitwise instruction
func NewWordTypeB(op opcode.OpCode, bitwidth uint, target, lhs, rhs register.Id) *WordTypeB {
	if !slices.Contains(opcode.TYPE_B_OPCODES, op) {
		panic("invalid bitwise operation")
	}
	//
	return &WordTypeB{op, bitwidth, target, lhs, rhs}
}

// OpCode implementation for Instruction interface
func (p *WordTypeB) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *WordTypeB) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *WordTypeB) Uses() []register.Id {
	return []register.Id{p.LeftSource, p.RightSource}
}

// Definitions implementation for Instruction interface
func (p *WordTypeB) Definitions() []register.Id {
	return []register.Id{p.Target}
}

// MicroValidate implementation for MicroInstruction interface.
func (p *WordTypeB) MicroValidate(_ uint, field field.Config, _ base.SystemMap) []error {
	return nil
}

func (p *WordTypeB) String(mapping base.SystemMap) string {
	var (
		builder  strings.Builder
		op            = bType2Operation(p.Op)
		bitwidth uint = base.RegisterBitwidth(mapping, p.Target)
	)
	//
	builder.WriteString(base.RegistersToString(mapping, p.Target))
	builder.WriteString(" = ")
	builder.WriteString(base.ExpressionToStringWithoutConst(op, p.Uses(), mapping))
	//
	if bitwidth != p.Bitwidth {
		fmt.Fprintf(&builder, " [u%d] ", p.Bitwidth)
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
