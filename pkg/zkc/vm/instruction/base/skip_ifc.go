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
	"math/big"

	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// SkipIfConst microcode performs a conditional skip over a given number of codes.
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
type SkipIfConst[W word.Word[W]] struct {
	Cond opcode.Condition
	// Left vector
	Left register.Vector
	// Right vector
	Right word.Vector[W]
	// Skip
	Skip uint
}

// IsWord implementation for instruction.Word interface
func (p *SkipIfConst[W]) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *SkipIfConst[W]) IsField() bool {
	return true
}

// OpCode implementation for Instruction interface
func (p *SkipIfConst[W]) OpCode() opcode.OpCode {
	return opcode.SKIP_IFC
}

// Uses implementation for Instruction interface
func (p *SkipIfConst[W]) Uses() []register.Id {
	return p.Left.Registers()
}

// Definitions implementation for Instruction interface
func (p *SkipIfConst[W]) Definitions() []io.RegisterId {
	return nil
}

func (p *SkipIfConst[W]) String(mapping SystemMap) string {
	var (
		l = p.Left.String(mapping)
		r = constant2str(p.Left, p.Right, mapping)
		o string
	)
	//
	switch p.Cond {
	case opcode.EQ:
		o = "=="
	case opcode.NEQ:
		o = "!="
	case opcode.LT:
		o = "<"
	case opcode.LTEQ:
		o = "<="
	case opcode.GT:
		o = ">"
	case opcode.GTEQ:
		o = ">="
	default:
		panic("unknown skip condition encountered")
	}
	//
	return fmt.Sprintf("skip_if %s %s %s %d", l, o, r, p.Skip)
}

// MicroValidate iumplementation for MicroInstruction interface
func (p *SkipIfConst[W]) MicroValidate(n uint, _ field.Config, fn SystemMap) []error {
	var (
		errors []error
	)
	//
	if p.Left.Len() != p.Right.Len() {
		errors = append(errors, fmt.Errorf("mismatched vectors (%d vs %d)", p.Left.Len(), p.Right.Len()))
	}
	//
	return errors
}

func constant2str[W word.Word[W]](l register.Vector, r word.Vector[W], mapping SystemMap) string {
	var (
		offset    uint
		registers = l.Registers()
		val       = big.NewInt(0)
	)
	//
	for i, w := range r.Words() {
		var (
			ith = mapping.Register(registers[i])
			v   big.Int
		)
		// shift left
		v.Lsh(w.BigInt(), offset)
		//
		val.Add(val, &v)
		//
		offset += ith.Width()
	}
	//
	return fmt.Sprintf("0x%s", val.Text(16))
}
