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
package field

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// Hint represents a non-deterministic register assignment.  The listed target
// registers are defined (written) by the prover without any polynomial
// constraint; correctness is validated by subsequent arithmetic checks.
type Hint struct {
	Targets []register.Id
	Sources []register.Id
}

// OpCode implementation for Instruction interface
func (p *Hint) OpCode() opcode.OpCode {
	return opcode.HINT_DIVISION
}

// IsWord implementation for instruction.Word interface
func (p *Hint) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *Hint) IsField() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *Hint) Uses() []register.Id {
	return p.Sources
}

// Definitions implementation for Instruction interface
func (p *Hint) Definitions() []register.Id {
	return p.Targets
}

// MicroValidate implementation for Instruction interface
func (p *Hint) MicroValidate(_ uint, _ field.Config, _ base.SystemMap) []error {
	return nil
}

// String implementation for Instruction interface
func (p *Hint) String(mapping base.SystemMap) string {
	return fmt.Sprintf("%s = hint(%s)",
		base.RegistersToString(mapping, p.Targets...),
		base.RegistersToString(mapping, p.Sources...))
}
