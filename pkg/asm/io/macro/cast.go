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
package macro

import (
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/asm/io/micro"
	"github.com/consensys/go-corset/pkg/schema/register"
)

// Cast provides a construct for (safely) casting a register into a narrower set
// of registers.  For example, assignment a 16bit register into an 8bit
// register.  The cast is safe in the sense that it will cause an exception if
// the value assigned does not fit.
type Cast struct {
	CastWidth uint
	// Target registers for cast
	Targets []io.RegisterId
	// Source register for cast
	Source io.RegisterId
}

// NewCast constructs a new cast instruction.
func NewCast(targets []io.RegisterId, cast uint, source io.RegisterId) *Cast {
	return &Cast{cast, targets, source}
}

// Execute implementation for Instruction interface.
func (p *Cast) Execute(state io.State) uint {
	// Read rhs
	value := state.Load(p.Source)
	//
	if value.BitLen() > int(p.CastWidth) {
		return io.FAIL
	}
	// Write value across targets
	state.StoreAcross(*value, p.Targets...)
	//
	return state.Pc() + 1
}

// Lower implementation for Instruction interface.
func (p *Cast) Lower(pc uint) micro.Instruction {
	return micro.NewInstruction(
		&micro.Cast{CastWidth: p.CastWidth, Targets: p.Targets, Source: p.Source},
		&micro.Jmp{Target: pc + 1},
	)
}

// RegistersRead implementation for Instruction interface.
func (p *Cast) RegistersRead() []io.RegisterId {
	return []io.RegisterId{p.Source}
}

// RegistersWritten implementation for Instruction interface.
func (p *Cast) RegistersWritten() []io.RegisterId {
	return p.Targets
}

func (p *Cast) String(fn register.Map) string {
	var builder strings.Builder
	//
	builder.WriteString(io.RegistersReversedToString(p.Targets, fn.Registers()))
	fmt.Fprintf(&builder, " = (u%d)", p.CastWidth)
	builder.WriteString(fn.Register(p.Source).Name())
	//
	return builder.String()
}

// Validate implementation for Instruction interface.
func (p *Cast) Validate(fieldWidth uint, fn register.Map) error {
	var (
		regs     = fn.Registers()
		lhs_bits = sumTargetBits(p.Targets, regs)
	)
	// check
	if lhs_bits < p.CastWidth {
		return fmt.Errorf("invalid cast (u%d into u%d)", p.CastWidth, lhs_bits)
	}
	//
	return io.CheckTargetRegisters(p.Targets, regs)
}
