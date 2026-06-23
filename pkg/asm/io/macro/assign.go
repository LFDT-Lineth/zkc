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
	"errors"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Assign represents a generic assignment of the following form:
//
// tn, .., t0 := e
//
// Here, t0 .. tn are the *target registers*, of which tn is the *most
// significant*.  These must be disjoint as we cannot assign simultaneously to
// the same register.  Likewise, e is the source expression.  For example,
// consider this case:
//
// c, r0 := r1 + 1
//
// Suppose that r0 and r1 are 16bit registers, whilst c is a 1bit register. The
// result of r1 + 1 occupies 17bits, of which the first 16 are written to r0
// with the most significant (i.e. 16th) bit written to c.  Thus, in this
// particular example, c represents a carry flag.
type Assign struct {
	// Target registers for assignment
	Targets []io.RegisterId
	// Source expresion for assignment
	Source Expr
}

// Execute implementation for Instruction interface.
func (p *Assign) Execute(state io.State) uint {
	value := p.Source.Eval(state.Internal())
	// Write value across targets
	state.StoreAcross(value, p.Targets...)
	//
	return state.Pc() + 1
}

// Lower implementation for Instruction interface.
func (p *Assign) Lower(pc uint) micro.Instruction {
	//
	code := &micro.Assign{
		Targets: p.Targets,
		Source:  p.Source.Polynomial(),
	}
	// Lowering here produces an instruction containing a single microcode.
	return micro.NewInstruction(code, &micro.Jmp{Target: pc + 1})
}

// RegistersRead implementation for Instruction interface.
func (p *Assign) RegistersRead() []io.RegisterId {
	return expr.RegistersRead(p.Source)
}

// RegistersWritten implementation for Instruction interface.
func (p *Assign) RegistersWritten() []io.RegisterId {
	return p.Targets
}

func (p *Assign) String(fn register.Map) string {
	var builder strings.Builder
	//
	builder.WriteString(io.RegistersReversedToString(p.Targets, fn.Registers()))
	builder.WriteString(" = ")
	builder.WriteString(p.Source.String(fn))
	//
	return builder.String()
}

// Validate implementation for Instruction interface.
func (p *Assign) Validate(fieldWidth uint, fn register.Map) error {
	var (
		regs             = fn.Registers()
		lhs_bits         = sumTargetBits(p.Targets, regs)
		rhs_bits, signed = expr.BitWidth(p.Source, fn)
	)
	// check
	if lhs_bits < rhs_bits {
		return fmt.Errorf("bit overflow (u%d into u%d)", rhs_bits, lhs_bits)
	} else if rhs_bits > fieldWidth {
		return fmt.Errorf("field overflow (u%d into u%d field)", rhs_bits, fieldWidth)
	} else if signed {
		// Sign bit required, so check there is one.
		if err := checkSignBit(p.Targets, regs); err != nil {
			return err
		}
	}
	//
	return io.CheckTargetRegisters(p.Targets, regs)
}

// Sum the total number of bits used by the given set of target registers.
func sumTargetBits(targets []io.RegisterId, regs []io.Register) uint {
	sum := uint(0)
	//
	for _, target := range targets {
		sum += regs[target.Unwrap()].Width()
	}
	//
	return sum
}

// the sign bit check is necessary to ensure there is always exactly one sign bit.
func checkSignBit(targets []io.RegisterId, regs []io.Register) error {
	var n = len(targets) - 1
	// Sanity check targets
	if n < 0 {
		return errors.New("malformed assignment")
	}
	// Determine width of sign bit
	signBitWidth := regs[targets[n].Unwrap()].Width()
	// Check it is a single bit
	if signBitWidth == 1 {
		return nil
	}
	// Problem, no alignment.
	return fmt.Errorf("missing sign bit (found u%d most significant bits)", signBitWidth)
}
