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

// Bitwise computes a binary bitwise operation between two registers.  The
// operation is identified by Opcode, which is one of AND, OR or XOR.
type Bitwise[W word.Word[W]] struct {
	// Opcode selects the operation (AND, OR or XOR).
	Op Operation
	// Target receives the result.
	Target RegisterId
	// Left and Right are the operand registers.
	Left, Right RegisterId
	//
	Bitwidth uint16
}

// Uses implementation for Bytecode interface.  A NOT carries its single operand
// duplicated across Left and Right (see compileNot), so returning both is
// always correct.
func (p *Bitwise[W]) Uses() []RegisterId {
	return []RegisterId{p.Left, p.Right}
}

// Definitions implementation for Bytecode interface.
func (p *Bitwise[W]) Definitions() []RegisterId {
	return []RegisterId{p.Target}
}

// Validate implementation for Bytecode interface.
func (p *Bitwise[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *Bitwise[W]) String(env Environment[W]) string {
	var (
		tgt = RegisterToString(p.Target, env)
		lhs = RegisterToString(p.Left, env)
		rhs = RegisterToString(p.Right, env)
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
