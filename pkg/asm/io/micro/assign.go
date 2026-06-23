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
package micro

import (
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

// Polynomial provides a useful alias which captures the fact that all
// polynomials in assembly are static.  That is, we never consider the
// possibility that registers can be "shifted".
type Polynomial = agnostic.StaticPolynomial

// Assign represents a generic assignment of the following form:
//
// tn, .., t0 := M0 + ... + Mm
//
// Here, t0 .. tn are the *target registers*, of which tn is the *most
// significant*.  These must be disjoint as we cannot assign simultaneously to
// the same register.  In contrast, the right hand side represent a polynomial
// (i.e. the sum of m monomials).  Observe that bitwidths represented by the
// left- and right-hand sides must match.  For example, consider this case:
//
// c, r0 := r1 + 1
//
// Suppose that r0 and r1 are 16bit registers, whilst c is a 1bit register. The
// result of r1 + 1 occupies 17bits, of which the first 16 are written to r0
// with the most significant (i.e. 16th) bit written to c.  Thus, in this
// particular example, c represents a carry flag.
type Assign struct {
	// Target registers for addition where the least significant come first.
	Targets []io.RegisterId
	// Source register for addition
	Source Polynomial
}

// Clone this micro code.
func (p *Assign) Clone() Code {
	//
	return &Assign{
		slices.Clone(p.Targets),
		p.Source.Clone(),
	}
}

// MicroExecute a given micro-code, using a given state.  This may update the
// register values, and returns either the number of micro-codes to "skip over"
// when executing the enclosing instruction or, if skip==0, a destination
// program counter (which can signal return of enclosing function).
func (p *Assign) MicroExecute(state io.State) (uint, uint) {
	var value big.Int
	// Sum evaluated terms
	for i := uint(0); i < p.Source.Len(); i++ {
		ith := evalMonomial(p.Source.Term(i), state)
		value.Add(&value, &ith)
	}
	// Write value
	state.StoreAcross(value, p.Targets...)
	//
	return 1, 0
}

// RegistersRead returns the set of registers read by this instruction.
func (p *Assign) RegistersRead() []io.RegisterId {
	return agnostic.RegistersRead(p.Source)
}

// RegistersWritten returns the set of registers written by this instruction.
func (p *Assign) RegistersWritten() []io.RegisterId {
	return p.Targets
}

func (p *Assign) String(fn register.Map) string {
	var (
		builder strings.Builder
		regs    = fn.Registers()
	)
	//
	builder.WriteString(io.RegistersReversedToString(p.Targets, regs))
	builder.WriteString(" = ")
	builder.WriteString(poly.String(p.Source, func(rid io.RegisterId) string {
		return regs[rid.Unwrap()].Name()
	}))
	//
	return builder.String()
}

// Split this micro code using registers of arbirary width into one or more
// micro codes using registers of a fixed maximum width.  Here, the environment
// maps registers in this instruction to their "limbs" (that is, registers after
// the split).  For example, consider (where x,y,z are 16bit registers and b a 1
// bit register):
//
// > b, x := y + z + 1
//
// Then, splitting to a maximum register width of 8bits yields the following:
//
// > b,x1,x0 := (256*y1+y0) + (256*z1+z0) + 1
//
// This is then factored as such:
//
// > b,x1,x0 := 256*(y1+z1) + (y0+z0+1)
//
// Thus, y0+z0+1 define all of the bits for x0 and some of the bits for x1.
func (p *Assign) Split(mapping register.LimbsMap, env agnostic.RegisterAllocator) []Code {
	var (
		// map target registers into corresponding limbs
		lhs = register.ApplyLimbsMap(mapping, p.Targets...)
		// map lhs registers into corresponding limbs
		rhs = SplitPolynomial(p.Source, mapping)
		// construct initial assignment
		assignment = agnostic.NewAssignment(lhs, rhs)
		// split into smaller assignments as needed
		assignments = assignment.Split(mapping.Field(), env)
		// codes to be filled out
		codes = make([]Code, len(assignments))
	)
	// Convert agnostic assignments back into micro assignments
	for i, a := range assignments {
		codes[i] = &Assign{a.LeftHandSide, a.RightHandSide}
	}
	// Done
	return codes
}

// Validate checks whether or not this instruction is correctly balanced.
func (p *Assign) Validate(fieldWidth uint, fn register.Map) error {
	var (
		regs = fn.Registers()
		// Determine number of bits required to hold the left-hand side.
		lhs_bits = sumTargetBits(p.Targets, regs)
		// Determine number of bits  required to hold the right-hand side.
		rhs_bits, _ = agnostic.WidthOfPolynomial(p.Source, agnostic.ArrayEnvironment(regs))
	)
	// check
	if lhs_bits < rhs_bits {
		return fmt.Errorf("bit overflow (u%d into u%d)", rhs_bits, lhs_bits)
	} else if rhs_bits > fieldWidth {
		return fmt.Errorf("field overflow (u%d into u%d field)", rhs_bits, fieldWidth)
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

func evalMonomial(term poly.Monomial[register.Id], state io.State) big.Int {
	var (
		acc   big.Int
		coeff big.Int = term.Coefficient()
	)
	// Initialise accumulator
	acc.Set(&coeff)
	//
	for j := uint(0); j < term.Len(); j++ {
		jth := state.Load(term.Nth(j))
		acc.Mul(&acc, jth)
	}
	//
	return acc
}
