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

// SkipIf instruction performs a conditional skip over a given number of codes.
// This is a *vectored* instruction, meaning the condition compares two register
// *vectors*.  For evaluating the condition, the interpretation of a vector is
// that the least significant register has the least index in the vector.  Two
// compare two vectors "left" and "right" of equal length, we find the highest
// index i where left[i] != right[i].  If no such index exists, the vectors are
// equal. Otherwise, if left[i] < right[i] the left vector is "less than" the
// right, otherwise it is "greater than" the right.  Then, the skip is taken or
// not depending on the condition opcode.
//
// NOTE: currently their is an assumption that both vectors have the same
// length.  This assumption could be relaxed in the future.
type SkipIf[W word.Word[W]] struct {
	Skip  uint16
	Left  RegisterVector
	Right RegisterVector
	Op    Condition
}

// Uses implementation for Bytecode interface.  A conditional skip reads both
// operand vectors being compared.
func (p *SkipIf[W]) Uses() []RegisterId {
	return append(p.Left.Registers(), p.Right.Registers()...)
}

// Definitions implementation for Bytecode interface.
func (p *SkipIf[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *SkipIf[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	return validateOperands(env, p.Left.Registers(), p.Right.Registers())
}

func (p *SkipIf[W]) String(env Environment[W]) string {
	var (
		ops  string
		src0 = RegisterVectorToString(p.Left, env)
		src1 = RegisterVectorToString(p.Right, env)
	)
	//
	switch p.Op {
	case CONDITION_EQ:
		ops = "=="
	case CONDITION_NEQ:
		ops = "!="
	case CONDITION_LT:
		ops = "<"
	case CONDITION_LTEQ:
		ops = "<="
	case CONDITION_GT:
		ops = ">"
	case CONDITION_GTEQ:
		ops = ">="
	default:
		ops = "??"
	}
	//
	return fmt.Sprintf("skip_if %s %s %s %d", src0, ops, src1, p.Skip)
}
