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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

var biZERO *big.Int = big.NewInt(0)
var biONE *big.Int = big.NewInt(1)

// Division operator divides either a register (or constant) by another register
// (or constant) producing a quotient and a remainder.
type Division struct {
	// Target registers (grouped into limbs)
	Quotient, Remainder, Witness register.Vector
	// Dividend and right comparisons
	Dividend, Divisor VecExpr
}

// Clone this micro code.
func (p *Division) Clone() Code {
	return &Division{
		Quotient:  p.Quotient,
		Remainder: p.Remainder,
		Witness:   p.Witness,
		Dividend:  p.Dividend,
		Divisor:   p.Divisor,
	}
}

// MicroExecute implementation for Code interface.
func (p *Division) MicroExecute(state io.State) (uint, uint) {
	var (
		lhs  = p.Dividend.Eval(state)
		rhs  = p.Divisor.Eval(state)
		quot big.Int
		rem  big.Int
		wit  big.Int
	)
	// Check for division by zero
	if rhs.Cmp(biZERO) == 0 {
		return 0, io.FAIL
	}
	// Compute quotient / remainder / witness
	quot.Div(lhs, rhs)
	rem.Mod(lhs, rhs)
	wit.Sub(rhs, &rem)
	wit.Sub(&wit, biONE)
	// Write target registers
	state.StoreAcross(quot, p.Quotient.Registers()...)
	state.StoreAcross(rem, p.Remainder.Registers()...)
	state.StoreAcross(wit, p.Witness.Registers()...)
	// Continue to next instruction
	return 1, 0
}

// RegistersRead implementation for Code interface.
func (p *Division) RegistersRead() []io.RegisterId {
	var regs []io.RegisterId
	//
	if p.Dividend.HasFirst() {
		regs = append(regs, p.Dividend.First().Registers()...)
	}
	//
	if p.Divisor.HasFirst() {
		regs = append(regs, p.Divisor.First().Registers()...)
	}
	//
	return regs
}

// RegistersWritten implementation for Code interface.
func (p *Division) RegistersWritten() []io.RegisterId {
	var written []io.RegisterId
	//
	written = append(written, p.Quotient.Registers()...)
	written = append(written, p.Remainder.Registers()...)
	written = append(written, p.Witness.Registers()...)
	//
	return written
}

// Split implementation for Code interface.
func (p *Division) Split(mapping register.LimbsMap, _ agnostic.RegisterAllocator) []Code {
	//
	return []Code{&Division{
		Quotient:  p.Quotient.Split(mapping),
		Remainder: p.Remainder.Split(mapping),
		Witness:   p.Witness.Split(mapping),
		Dividend:  p.Dividend.Split(mapping),
		Divisor:   p.Divisor.Split(mapping),
	}}
}

func (p *Division) String(fn register.Map) string {
	var builder strings.Builder
	//
	builder.WriteString(p.Quotient.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.Remainder.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.Witness.String(fn))
	builder.WriteString(" = ")
	builder.WriteString(p.Dividend.String(fn))
	builder.WriteString(" / ")
	builder.WriteString(p.Divisor.String(fn))
	//
	return builder.String()
}

// Validate implementation for Code interface.
func (p *Division) Validate(fieldWidth uint, fn register.Map) error {
	var (
		qBits = p.Quotient.BitWidth(fn)
		rBits = p.Remainder.BitWidth(fn)
		wBits = p.Witness.BitWidth(fn)
		dBits = p.Dividend.Bitwidth(fn)
		vBits = p.Divisor.Bitwidth(fn)
	)
	// check
	if qBits < dBits {
		return fmt.Errorf("quotient bit overflow (u%d into u%d)", dBits, qBits)
	} else if rBits < vBits {
		return fmt.Errorf("remainder bit overflow (u%d into u%d)", vBits, rBits)
	} else if wBits < vBits {
		return fmt.Errorf("witness bit overflow (u%d into u%d)", vBits, wBits)
	}
	//
	return nil
}
