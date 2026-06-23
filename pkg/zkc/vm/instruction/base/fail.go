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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// Fail aborts execution of the machine, signalling an unrecoverable error
// (a "machine panic").  No further instructions are executed and the
// surrounding call stack is not unwound; the executor returns an error to its
// caller.  An optional sequence of formatted chunks describes the error
// message: each chunk holds some literal text plus, optionally, a format
// directive paired with a register holding the value to format.  Fail is one
// of the three control-flow terminators (along with Jmp and Return) recognised
// by the vectoriser as ending a basic block.
type Fail struct {
	Chunks []FormattedChunk
}

// OpCode implementation for Instruction interface
func (p *Fail) OpCode() opcode.OpCode {
	return opcode.FAIL
}

// IsWord implementation for instruction.Word interface
func (p *Fail) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *Fail) IsField() bool {
	return true
}

// Uses implementation for Instruction interface.
func (p *Fail) Uses() []register.Id {
	var uses []register.Id
	//
	for _, c := range p.Chunks {
		if c.Format.HasFormat() {
			uses = append(uses, c.Argument.Registers()...)
		}
	}
	//
	return uses
}

// Definitions implementation for Instruction interface.
func (p *Fail) Definitions() []register.Id {
	return nil
}

func (p *Fail) String(mapping SystemMap) string {
	if len(p.Chunks) == 0 {
		return "fail"
	}
	//
	var (
		tBuilder  strings.Builder
		builder   strings.Builder
		firstTime = true
	)
	//
	for _, c := range p.Chunks {
		tBuilder.WriteString(util.EscapeFormattedText(c.Text))
		//
		if c.Format.HasFormat() {
			tBuilder.WriteString(c.Format.String())

			if !firstTime {
				builder.WriteString(", ")
			}
			//
			firstTime = false
			//
			builder.WriteString(c.Argument.String(mapping))
		}
	}
	//
	if builder.Len() == 0 {
		return fmt.Sprintf("fail \"%s\"", tBuilder.String())
	}
	//
	return fmt.Sprintf("fail \"%s\", %s", tBuilder.String(), builder.String())
}

// Validate implementation for Instruction interface.
func (p *Fail) Validate(_ field.Config, _ SystemMap) []error {
	return nil
}

// MicroValidate implementation for Instruction interface.
func (p *Fail) MicroValidate(_ uint, _ field.Config, _ SystemMap) []error {
	return nil
}
