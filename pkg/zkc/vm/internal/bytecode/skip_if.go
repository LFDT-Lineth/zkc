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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
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
type SkipIf struct {
	Skip  uint16
	Left  RegVec
	Right RegVec
	Op    Cond
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *SkipIf) Clone() Patched {
	var c = *p
	return &c
}

// Uses implementation for Bytecode interface.  A conditional skip reads both
// operand vectors being compared.
func (p *SkipIf) Uses() []RegisterId {
	return append(p.Left.Registers(), p.Right.Registers()...)
}

// Definitions implementation for Bytecode interface.
func (p *SkipIf) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.  A conditional skip is
// well-formed only when its left operand vector is at least as wide as its
// right (mirroring base.SkipIf.MicroValidate): a narrower left operand could
// not faithfully hold the value it is compared against.  When either vector
// involves a native register (and hence has no fixed width) no check applies.
func (p *SkipIf) Validate(_ uint, _ FieldConfig, env Environment) []error {
	var (
		errors []error
		lw     = vectorBitwidth(p.Left, env)
		rw     = vectorBitwidth(p.Right, env)
	)
	//
	if lw.HasValue() && rw.HasValue() && lw.Unwrap() < rw.Unwrap() {
		errors = append(errors, fmt.Errorf("bit overflow (u%d into u%d)", lw.Unwrap(), rw.Unwrap()))
	}
	//
	return errors
}

// vectorBitwidth returns the total bitwidth of a register vector under the
// given environment, or the empty option when any constituent register is
// native (and hence has no fixed bitwidth).
func vectorBitwidth(v RegVec, env Environment) util.Option[uint] {
	var total uint
	//
	for _, r := range v.Registers() {
		bw := env.Register(r).Bitwidth()
		//
		if !bw.HasValue() {
			return util.None[uint]()
		}
		//
		total += bw.Unwrap()
	}
	//
	return util.Some(total)
}

func (p *SkipIf) String(env Environment) string {
	var (
		ops  string
		src0 = RegisterVectorToString(p.Left, env)
		src1 = RegisterVectorToString(p.Right, env)
	)
	//
	switch p.Op {
	case opcode.EQ:
		ops = "=="
	case opcode.NEQ:
		ops = "!="
	case opcode.LT:
		ops = "<"
	case opcode.LTEQ:
		ops = "<="
	case opcode.GT:
		ops = ">"
	case opcode.GTEQ:
		ops = ">="
	default:
		ops = "??"
	}
	//
	return fmt.Sprintf("skip_if %s %s %s %d", src0, ops, src1, p.Skip)
}
