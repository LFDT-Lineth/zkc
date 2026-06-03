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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// Skip microcode performs an unconditional skip over a given number of codes.
type Skip struct {
	// Skip
	Skip uint
}

// OpCode implementation for Instruction interface
func (p *Skip) OpCode() opcode.OpCode {
	return opcode.SKIP
}

// IsWord implementation for instruction.Word interface
func (p *Skip) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *Skip) IsField() bool {
	return true
}

// Uses implementation for Instruction interface.
func (p *Skip) Uses() []register.Id {
	return nil
}

// Definitions implementation for Instruction interface.
func (p *Skip) Definitions() []register.Id {
	return nil
}

func (p *Skip) String(_ SystemMap) string {
	return fmt.Sprintf("skip %d", p.Skip)
}

// MicroValidate implementation for Instruction interface.
func (p *Skip) MicroValidate(_ uint, _ field.Config, _ SystemMap) []error {
	return nil
}
