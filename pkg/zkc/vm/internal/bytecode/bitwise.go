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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bitwise computes a binary bitwise operation between a register and a second
// operand.  The operation is identified by Opcode, which is one of AND, OR or
// XOR.
type Bitwise[W word.Word[W]] struct {
	// Opcode selects the operation (AND, OR or XOR).
	Op Operation
	// Target receives the result.
	Target RegisterId
	// Left is the first operand register.
	Left RegisterId
	// Right is the second operand: either a register or (for AND/OR/XOR only)
	// a constant.  A NOT carries its single operand duplicated across Left and
	// Right, whilst for SHL/SHR the right operand (i.e. shift amount) is
	// always a register.
	Right Operand[W]
	// Bitwidth of operands
	Bitwidth uint16
}

// Uses implementation for Bytecode interface.  A constant right operand
// contributes no register uses; a NOT carries its single operand duplicated
// across Left and Right (see compileNot), so returning both is always correct.
func (p *Bitwise[W]) Uses() []RegisterId {
	if p.Right.IsConstant() {
		return []RegisterId{p.Left}
	}
	//
	return []RegisterId{p.Left, p.Right.AsRegister()}
}

// Definitions implementation for Bytecode interface.
func (p *Bitwise[W]) Definitions() []RegisterId {
	return []RegisterId{p.Target}
}

// Validate implementation for Bytecode interface.
func (p *Bitwise[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	var errors = validateOperands(env, p.Uses(), p.Definitions())
	//
	if p.Right.IsConstant() {
		// Only AND/OR/XOR support a constant operand: NOT is unary, whilst
		// shifts always read their amount from a register.
		switch p.Op {
		case OP_AND, OP_OR, OP_XOR:
			// permitted
		default:
			return append(errors, fmt.Errorf("constant operand invalid for %s", p.Op.Prefix()))
		}
		//
		if !p.Right.AsConstant().FitsWithin(uint(p.Bitwidth)) {
			errors = append(errors, fmt.Errorf("constant operand exceeds u%d", p.Bitwidth))
		}
	}
	//
	return errors
}

func (p *Bitwise[W]) String(env Environment[W]) string {
	var (
		tgt = RegisterToString(p.Target, env)
		lhs = RegisterToString(p.Left, env)
		rhs = p.Right.String(env)
	)
	//
	switch p.Op {
	case OP_AND:
		return fmt.Sprintf("and %s = %s & %s [u%d]", tgt, lhs, rhs, p.Bitwidth)
	case OP_OR:
		return fmt.Sprintf("or %s = %s | %s [u%d]", tgt, lhs, rhs, p.Bitwidth)
	case OP_XOR:
		return fmt.Sprintf("xor %s = %s ^ %s [u%d]", tgt, lhs, rhs, p.Bitwidth)
	case OP_NOT:
		return fmt.Sprintf("not %s = ~%s [u%d]", tgt, lhs, p.Bitwidth)
	case OP_SHL:
		return fmt.Sprintf("shl %s = %s << %s [u%d]", tgt, lhs, rhs, p.Bitwidth)
	case OP_SHR:
		return fmt.Sprintf("shr %s = %s >> %s [u%d]", tgt, lhs, rhs, p.Bitwidth)
	default:
		panic("unknown bitwise operator")
	}
	//
}
