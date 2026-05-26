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

// WordTypeA represents an instruction of the following form:
//
// tn::..::t0 := r0 # ... # rn + c
//
// Here, t0 is the *target register*, whilst r0 .. rn are the source registers
// and c is a constant (which can be 0).  Finally, "#" represents whatever
// operation the given opcode indicates.
type WordTypeA[W word.Word[W]] struct {
	Op opcode.OpCode
	// Target register for assignment
	Target register.Vector
	// Source registers for assignment
	Sources []register.Id
	// Constant for assignment
	Constant W
}

// NewWordTypeA constructs a new arithmetic instruction
func NewWordTypeA[W word.Word[W]](op opcode.OpCode, target register.Vector, sources []register.Id, constant W,
) *WordTypeA[W] {
	//
	if !slices.Contains(opcode.ARITH_OPCODES, op) {
		panic("invalid arithmetic operation")
	}
	//
	return &WordTypeA[W]{op, target, sources, constant}
}

// OpCode implementation for Instruction interface
func (p *WordTypeA[W]) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *WordTypeA[W]) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *WordTypeA[W]) Uses() []register.Id {
	return p.Sources
}

// Definitions implementation for Instruction interface
func (p *WordTypeA[W]) Definitions() []register.Id {
	return p.Target.Registers()
}

// MicroValidate implementation for MicroInstruction interface.
func (p *WordTypeA[W]) MicroValidate(_ uint, field field.Config, _ base.SystemMap) []error {
	return nil
}

func (p *WordTypeA[W]) String(mapping base.SystemMap) string {
	if p.Op == opcode.BIT_CONCAT {
		return bitconcat2str(p, mapping)
	} else {
		var (
			builder strings.Builder
			op      = aType2Operation(p.Op)
			zero    W
		)
		//
		builder.WriteString(p.Target.String(mapping))
		builder.WriteString(" = ")
		//
		if p.Constant.Cmp(zero) == 0 && len(p.Sources) > 0 &&
			(p.Op == opcode.INT_ADD || p.Op == opcode.INT_SUB) {
			//
			builder.WriteString(base.ExpressionToStringWithoutConst(op, p.Sources, mapping))
		} else {
			builder.WriteString(base.ExpressionToString(op, p.Sources, p.Constant, mapping))
		}
		//
		return builder.String()
	}
}

func aType2Operation(op opcode.OpCode) string {
	switch op {
	case opcode.INT_ADD:
		return "+"
	case opcode.INT_SUB:
		return "-"
	case opcode.INT_MUL:
		return "*"
	case opcode.BIT_CONCAT:
		return "::"
	default:
		panic("unknown type A instruction")
	}
}

func bitconcat2str[W word.Word[W]](p *WordTypeA[W], mapping base.SystemMap) string {
	var (
		zero    W
		builder strings.Builder
	)
	// Sanity check
	if p.Constant.Cmp(zero) != 0 {
		panic("constant given for bit concatenation")
	}
	//
	builder.WriteString(p.Target.String(mapping))
	builder.WriteString(" = ")
	//
	for i := len(p.Sources); i > 0; i-- {
		ith := base.RegistersToString(mapping, p.Sources[i-1])
		if i != len(p.Sources) {
			builder.WriteString("::")
		}
		//
		builder.WriteString(ith)
	}
	//
	return builder.String()
}
