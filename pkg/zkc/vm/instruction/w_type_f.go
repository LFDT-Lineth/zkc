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

// WordTypeF represents an instruction of the following form:
//
// t0 := r0 # ... # rn # c
//
// Here, t0 is the *target register*, whilst r0 .. rn are the source registers
// and c is a constant (which can be 0).  Finally, "#" represents whatever
// operation the given opcode indicates.
type WordTypeF[W word.Word[W]] struct {
	Op opcode.OpCode
	// // Bitwidth for the operation.  Observe that this does not have to match the
	// // widths of either the source or target operands.
	// Bitwidth uint
	// Target register for assignment
	Target register.Id
	// Source registers for assignment
	Sources []register.Id
	// Constant for assignment
	Constant W
}

// NewWordTypeF constructs a new bitwise instruction
func NewWordTypeF[W word.Word[W]](op opcode.OpCode, target register.Id, sources []register.Id, constant W,
) *WordTypeF[W] {
	if !slices.Contains(opcode.TYPE_F_OPCODES, op) {
		panic("invalid field operation")
	}
	//
	return &WordTypeF[W]{op, target, sources, constant}
}

// OpCode implementation for Instruction interface
func (p *WordTypeF[W]) OpCode() opcode.OpCode {
	return p.Op
}

// IsWord implementation for instruction.Word interface
func (p *WordTypeF[W]) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *WordTypeF[W]) Uses() []register.Id {
	return p.Sources
}

// Definitions implementation for Instruction interface
func (p *WordTypeF[W]) Definitions() []register.Id {
	return []register.Id{p.Target}
}

// MicroValidate implementation for MicroInstruction interface.
func (p *WordTypeF[W]) MicroValidate(_ uint, field field.Config, _ base.SystemMap) []error {
	return nil
}

func (p *WordTypeF[W]) String(mapping base.SystemMap) string {
	var (
		builder strings.Builder
		op      = fType2Operation(p.Op)
		zero    W
		one     W
	)
	//
	one = one.SetUint64(1)
	//
	builder.WriteString(base.RegistersToString(mapping, p.Target))
	builder.WriteString(" = ")
	//
	if p.Constant.Cmp(zero) == 0 && len(p.Sources) > 0 &&
		(p.Op == opcode.INT_ADDMOD_P || p.Op == opcode.INT_SUBMOD_P) {
		//
		builder.WriteString(base.ExpressionToStringWithoutConst(op, p.Sources, mapping))
	} else if p.Constant.Cmp(one) == 0 && len(p.Sources) > 0 && p.Op == opcode.INT_MULMOD_P {
		//
		builder.WriteString(base.ExpressionToStringWithoutConst(op, p.Sources, mapping))
	} else {
		builder.WriteString(base.ExpressionToString(op, p.Sources, p.Constant, mapping))
	}
	//
	return builder.String()
}

func fType2Operation(op opcode.OpCode) string {
	switch op {
	case opcode.INT_ADDMOD_P:
		return "⊕"
	case opcode.INT_SUBMOD_P:
		return "⊖"
	case opcode.INT_MULMOD_P:
		return "⊗"
	default:
		panic("unknown type F instruction")
	}
}
