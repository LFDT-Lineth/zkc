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
package word

import (
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// Destruct represents an instruction of the following form:
//
// tn::t0 := r0
//
// Here, t0 .. tn are the *target registers*, of which tn is the *most
// significant*.  These must be disjoint as we cannot assign simultaneously to
// the same register.  Likewise, r0 is the source register which are.
type Destruct struct {
	// Target registers for assignment
	Targets []register.Id
	// Source register for assignment
	Source register.Id
}

// OpCode implementation for Instruction interface
func (p *Destruct) OpCode() opcode.OpCode {
	return opcode.BIT_DESTRUCT
}

// IsWord implementation for instruction.Word interface
func (p *Destruct) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface
func (p *Destruct) Uses() []register.Id {
	return []register.Id{p.Source}
}

// Definitions implementation for Instruction interface
func (p *Destruct) Definitions() []register.Id {
	return p.Targets
}

func (p *Destruct) String(mapping base.SystemMap) string {
	var builder strings.Builder
	//
	for i := len(p.Targets); i > 0; i-- {
		var rid = p.Targets[i-1]
		//
		if i != len(p.Targets) {
			builder.WriteString("::")
		}
		//
		builder.WriteString(base.RegistersToString(mapping, rid))
	}
	//
	builder.WriteString(" = ")
	builder.WriteString(base.RegistersToString(mapping, p.Source))
	//
	return builder.String()
}

// MicroValidate implementation for MicroInstruction interface.
func (p *Destruct) MicroValidate(_ uint, field field.Config, _ base.SystemMap) []error {
	return nil
}
