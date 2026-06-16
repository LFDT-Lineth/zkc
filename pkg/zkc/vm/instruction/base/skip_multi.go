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
	"math/bits"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// DispatchCase is a single (value, skip) entry of a multiway skip's dispatch
// array: when the source register holds Value, control skips forward by Skip
// micro-codes.
type DispatchCase struct {
	// Value compared against the source register.
	Value uint
	// Skip taken (number of micro-codes) when the source register equals Value.
	Skip uint
}

// SkipMulti performs a multi-way conditional skip based on a dispatch array.
// The value of the source register is compared, in order, against each case's
// Value; on the first match, control skips forward by that case's Skip.  If no
// case matches, control falls through to the following micro-code (i.e. as if
// skip were zero).  This generalises SkipIf (a single, two-register comparison)
// to an N-way register-vs-constant dispatch, analogous to a jump table.
type SkipMulti struct {
	// Source register whose value selects the dispatch target.
	Source register.Id
	// Cases gives the dispatch array of (value, skip) pairs, tried in order.
	Cases []DispatchCase
}

// IsWord implementation for instruction.Word interface
func (p *SkipMulti) IsWord() bool {
	return true
}

// IsField implementation for instruction.Field interface
func (p *SkipMulti) IsField() bool {
	return true
}

// OpCode implementation for Instruction interface
func (p *SkipMulti) OpCode() opcode.OpCode {
	return opcode.SKIP_MULTI
}

// Uses implementation for Instruction interface
func (p *SkipMulti) Uses() []register.Id {
	return []register.Id{p.Source}
}

// Definitions implementation for Instruction interface
func (p *SkipMulti) Definitions() []register.Id {
	return nil
}

func (p *SkipMulti) String(mapping SystemMap) string {
	var builder strings.Builder
	//
	builder.WriteString("skipm ")
	builder.WriteString(register.NewVector(p.Source).String(mapping))
	builder.WriteString(" [")
	//
	for i, c := range p.Cases {
		if i != 0 {
			builder.WriteString(", ")
		}
		//
		fmt.Fprintf(&builder, "%d->%d", c.Value, c.Skip)
	}
	//
	builder.WriteString("]")
	//
	return builder.String()
}

// MicroValidate implementation for Instruction interface.
func (p *SkipMulti) MicroValidate(_ uint, _ field.Config, fn SystemMap) []error {
	var (
		errors []error
		reg    = fn.Register(p.Source)
		seen   = make(map[uint]bool)
	)
	//
	for _, c := range p.Cases {
		// Detect duplicate dispatch values: since the first match wins, a
		// duplicate is unreachable and almost certainly a mistake.
		if seen[c.Value] {
			errors = append(errors, fmt.Errorf("duplicate dispatch value %d", c.Value))
		}
		//
		seen[c.Value] = true
		// A native register holds arbitrary-width values, so no constant can
		// overflow it; otherwise the value must fit within the register width.
		if !reg.IsNative() && uint(bits.Len(c.Value)) > reg.Width() {
			errors = append(errors, fmt.Errorf("dispatch value %d overflows u%d", c.Value, reg.Width()))
		}
	}
	//
	return errors
}
