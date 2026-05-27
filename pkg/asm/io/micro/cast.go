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

	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/schema/agnostic"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/poly"
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

// Clone this micro code.
func (p *Cast) Clone() Code {
	return &Cast{
		p.CastWidth,
		slices.Clone(p.Targets),
		p.Source,
	}
}

// MicroExecute implementation for Code interface.
func (p *Cast) MicroExecute(state io.State) (uint, uint) {
	// Read rhs
	value := state.Load(p.Source)
	//
	if value.BitLen() > int(p.CastWidth) {
		return 0, io.FAIL
	}
	// Write value across targets
	state.StoreAcross(*value, p.Targets...)
	//
	return 1, 0
}

// RegistersRead implementation for Instruction interface.
func (p *Cast) RegistersRead() []io.RegisterId {
	return []io.RegisterId{p.Source}
}

// RegistersWritten implementation for Instruction interface.
func (p *Cast) RegistersWritten() []io.RegisterId {
	return p.Targets
}

// Split implementation for Code interface.
func (p *Cast) Split(mapping register.LimbsMap, _ agnostic.RegisterAllocator) []Code {
	// Split lhs & rhs
	var (
		lhs    = register.ApplyLimbsMap(mapping, p.Targets...)
		rhs    = mapping.LimbIds(p.Source)
		ncodes []Code
		biONE  big.Int = *big.NewInt(1)
		zero           = NewConstant64(0)
		nLhs           = uint(len(lhs))
		nRhs           = uint(len(rhs))
	)
	// construct assignments
	for i, lval := range lhs {
		var (
			monomial = poly.NewMonomial(biONE, rhs[i])
			result   agnostic.StaticPolynomial
		)
		// construct assignment
		ncodes = append(ncodes, &Assign{
			[]io.RegisterId{lval},
			result.Set(monomial),
		})
	}
	// construct safety checks (if applicable)
	if nLhs < nRhs {
		ncodes = append(ncodes, &SkipIf{
			Left:  register.NewVector(rhs[nLhs:nRhs]...),
			Right: zero.ToVec(),
			Skip:  1,
		})
		// skip over fail
		ncodes = append(ncodes, &Skip{1})
		// fail
		ncodes = append(ncodes, &Fail{})
	}
	//
	return ncodes
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
