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

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/util"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// FormattedChunk represents a chunk of a printf string which consists of some
// text, followed by an optional argument format.
type FormattedChunk struct {
	Text     string
	Format   util.Format
	Argument register.Vector
}

// Debug performs an unconditional branch to a given target instructon.
type Debug struct {
	Chunks []FormattedChunk
}

// OpCode implementation for Instruction interface
func (p *Debug) OpCode() opcode.OpCode {
	return opcode.DEBUG
}

// IsWord implementation for instruction.Word interface
func (p *Debug) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *Debug) IsField() bool {
	return true
}

// Uses implementation for Instruction interface.
func (p *Debug) Uses() []register.Id {
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
func (p *Debug) Definitions() []register.Id {
	return nil
}

func (p *Debug) String(mapping SystemMap) string {
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
	return fmt.Sprintf("debug \"%s\", %s", tBuilder.String(), builder.String())
}

// Validate implementation for Instruction interface.
func (p *Debug) Validate(_ field.Config, _ SystemMap) []error {
	return nil
}

// MicroValidate implementation for Instruction interface.
func (p *Debug) MicroValidate(_ uint, _ field.Config, _ SystemMap) []error {
	return nil
}
