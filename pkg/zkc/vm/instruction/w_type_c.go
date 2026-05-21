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
package instruction

import (
	"fmt"
	"math"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// WordTypeC represents a truncating cast instruction of the following form:
//
//	t := (uN)s
//
// Here, t is the target register, s is the source register, and N is the cast
// bit width.  The N low-order bits of s are retained and written to t.
type WordTypeC struct {
	// Target register for assignment
	Target register.Id
	// Source register
	Source register.Id
	// Width is the target bit width for truncation, where MaxUint signals field
	// cast.
	Width uint
}

// OpCode implementation for Instruction interface
func (p *WordTypeC) OpCode() opcode.OpCode {
	if p.Width == math.MaxUint {
		return opcode.INT_CASTMOD_P
	}
	//
	return opcode.INT_CAST
}

// IsWord implementation for instruction.Word interface
func (p *WordTypeC) IsWord() bool {
	return true
}

// Uses implementation for Instruction interface.
func (p *WordTypeC) Uses() []register.Id {
	return []register.Id{p.Source}
}

// Definitions implementation for Instruction interface.
func (p *WordTypeC) Definitions() []register.Id {
	return []register.Id{p.Target}
}

// String implementation for Instruction interface.
func (p *WordTypeC) String(mapping base.SystemMap) string {
	var builder strings.Builder
	//
	builder.WriteString(base.RegistersToString(mapping, p.Target))
	//
	if p.Width != math.MaxUint {
		fmt.Fprintf(&builder, " = (u%d) ", p.Width)
	} else {
		fmt.Fprintf(&builder, " = (𝔽) ")
	}
	//
	builder.WriteString(base.RegistersToString(mapping, p.Source))
	//
	return builder.String()
}

// MicroValidate implementation for MicroInstruction interface.
func (p *WordTypeC) MicroValidate(_ uint, _ field.Config, _ base.SystemMap) []error {
	return nil
}
