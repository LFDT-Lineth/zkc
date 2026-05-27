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
	"github.com/consensys/go-corset/pkg/asm/io/macro/expr"
	"github.com/consensys/go-corset/pkg/asm/io/micro"
	"github.com/consensys/go-corset/pkg/schema/agnostic"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/poly"
)

// Call represents a function call providing one or more arguments and accepting
// zero or more values in return.  A function call requires a "bus" to read and
// write its arguments / returns.  A bus is a set of dedicated registers
// providing an I/O communication channel to a given peripheral (in this case,
// another function).
type Call struct {
	// Bus identifies the relevant IoBus for this instruction.
	IoBus io.Bus
	// Target registers for addition
	Targets []io.RegisterId
	// Source expressions (i.e. arguments) for call
	Sources []Expr
}

// NewCall constructs a new call instruction.
func NewCall(bus io.Bus, targets []io.RegisterId, sources []Expr) *Call {
	return &Call{bus, targets, sources}
}

// Bus returns information about the bus.  Observe that prior to Link being
// called, this will return an unlinked bus.
func (p *Call) Bus() io.Bus {
	return p.IoBus
}

// Execute implementation for Instruction interface.
func (p *Call) Execute(state io.State) uint {
	n := len(p.Targets) - 1
	// Determine read address by evaluating source expressions
	address := expr.Eval(state.Internal(), p.Sources)
	// Set bus address lines
	state.StoreN(p.IoBus.Address(), address)
	// Perform I/O read
	state.In(p.IoBus)
	// Load bus data lines
	values := state.LoadN(p.IoBus.Data())
	// Write back results (in reverse order)
	for i, dst := range p.Targets {
		state.Store(dst, values[n-i])
	}
	//
	return state.Pc() + 1
}

// Link links the bus.  Observe that this can only be called once on any
// given instruction.
func (p *Call) Link(bus io.Bus) {
	if !p.IoBus.IsUnlinked() {
		panic("bus already linked")
	}
	//
	p.IoBus = bus
}

// Lower implementation for Instruction interface.
func (p *Call) Lower(pc uint) micro.Instruction {
	var (
		code    []micro.Code
		address = p.IoBus.Address()
		data    = p.IoBus.Data()
	)
	// Write address lines
	for i, input := range p.Sources {
		source := input.Polynomial()
		insn := &micro.Assign{Targets: []io.RegisterId{address[i]}, Source: source}
		code = append(code, insn)
	}
	// For read / write on bus
	code = append(code, micro.NewIoRead(p.IoBus))
	//
	// Read output lines
	for i, output := range p.Targets {
		var (
			source agnostic.StaticPolynomial
			// NOTE: targets stored in reverse order
			j = len(p.Targets) - i - 1
		)

		source = source.Set(poly.NewMonomial(one, data[j]))
		insn := &micro.Assign{Targets: []io.RegisterId{output}, Source: source}
		code = append(code, insn)
	}
	// Append final branch
	code = append(code, &micro.Jmp{Target: pc + 1})
	// Done
	return micro.NewInstruction(code...)
}

// RegistersRead implementation for Instruction interface.
func (p *Call) RegistersRead() []io.RegisterId {
	return expr.RegistersRead(p.Sources...)
}

// RegistersWritten implementation for Instruction interface.
func (p *Call) RegistersWritten() []io.RegisterId {
	return p.Targets
}

func (p *Call) String(fn register.Map) string {
	var (
		builder strings.Builder
		regs    = fn.Registers()
	)
	//
	builder.WriteString(io.RegistersReversedToString(p.Targets, regs))
	fmt.Fprintf(&builder, " = %s(", p.IoBus.Name)
	//
	for i, e := range p.Sources {
		if i != 0 {
			builder.WriteString(", ")
		}
		//
		builder.WriteString(e.String(fn))
	}
	//
	builder.WriteString(")")
	//
	return builder.String()
}

// Validate implementation for Instruction interface.
func (p *Call) Validate(fieldWidth uint, fn register.Map) error {
	// Check bus is assigned
	if p.IoBus.IsUnlinked() {
		return fmt.Errorf("unknown function")
	}
	// Sanity check arguments and returns
	busInputs := p.IoBus.Address()
	busOutputs := p.IoBus.Data()
	//
	if len(busInputs) != len(p.Sources) {
		return fmt.Errorf("incorrect arguments (found %d expected %d)", len(p.Sources), len(busInputs))
	} else if len(busOutputs) != len(p.Targets) {
		return fmt.Errorf("incorrect returns (found %d expected %d)", len(p.Targets), len(busOutputs))
	}
	// Check arguments
	for i, src := range p.Sources {
		src_w, signed := expr.BitWidth(src, fn)
		bus_w := fn.Register(busInputs[i]).Width()
		//
		if src_w > bus_w {
			return fmt.Errorf("incorrect width for argument %d (found %d expected %d)", i+1, src_w, bus_w)
		} else if signed {
			return fmt.Errorf("signed arguments not yet supported (i.e. arguments which can be negative)")
		}
	}
	// Check returns
	for i, rtn := range p.Targets {
		// NOTE: targets stored in reverse order
		j := len(p.Targets) - i
		rtn_w := fn.Register(rtn).Width()
		bus_w := fn.Register(busOutputs[j-1]).Width()
		//
		if rtn_w < bus_w {
			return fmt.Errorf("incorrect width for return %d (found %d expected %d)", j, rtn_w, bus_w)
		}
	}
	// Done
	return nil
}
