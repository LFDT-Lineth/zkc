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
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

var biZERO *big.Int = big.NewInt(0)
var biONE *big.Int = big.NewInt(1)

// Division represents an Divisionment of the form:
//
// q,r,w = e1 / e1
//
// where e1/e2 is either a variable or constant, and the witness w is a proof
// that r < e1.
type Division struct {
	// Target registers
	Quotient  expr.RegAccess
	Remainder expr.RegAccess
	Witness   expr.RegAccess
	Sum       expr.RegAccess
	WitSum    expr.RegAccess
	// Dividend expression
	Dividend expr.AtomicExpr
	// Divisor expression
	Divisor expr.AtomicExpr
}

// Execute implementation for Instruction interface.
func (p *Division) Execute(state io.State) uint {
	var (
		lhs  = p.Dividend.Eval(state.Internal())
		rhs  = p.Divisor.Eval(state.Internal())
		quot big.Int
		rem  big.Int
		wit  big.Int
	)
	// Check for division by zero
	if rhs.Cmp(biZERO) == 0 {
		return io.FAIL
	}
	// Compute quotient / remainder / witntess
	quot.Div(&lhs, &rhs)
	rem.Mod(&lhs, &rhs)
	wit.Sub(&rhs, &rem)
	wit.Sub(&wit, biONE)
	// Write target registers
	state.Store(p.Quotient.Register, quot)
	state.Store(p.Remainder.Register, rem)
	state.Store(p.Witness.Register, wit)
	// Continue to next instruction
	return state.Pc() + 1
}

// Lower implementation for Instruction interface.
func (p *Division) Lower(pc uint) micro.Instruction {
	var one = big.NewInt(1)

	// NOTE: the micro division instruction is an unsafe computation which does
	// not generate any constraints.  Thus, our translation here must follow the
	// "compute & check" paradigm.  That is, witness variables (wit, sum,
	// witsum) are filled from the given assignments, whilst constraints are
	// generated from the given assertions.

	codes := []micro.Code{
		&micro.Division{
			Quotient:  register.NewVector(p.Quotient.Register),
			Remainder: register.NewVector(p.Remainder.Register),
			Witness:   register.NewVector(p.Witness.Register),
			Dividend:  p.Dividend.ToMicroExpr().ToVec(),
			Divisor:   p.Divisor.ToMicroExpr().ToVec(),
		},
		// sum := (quot * div) + rem
		&micro.Assign{
			Targets: []io.RegisterId{p.Sum.Register},
			Source:  p.Quotient.Polynomial().Mul(p.Divisor.Polynomial()).Add(p.Remainder.Polynomial()),
		},
		// witsum := rem + wit + 1
		&micro.Assign{
			Targets: []io.RegisterId{p.WitSum.Register},
			Source:  p.Remainder.Polynomial().Add(p.Witness.Polynomial()).AddScalar(one),
		},
		// [assertion 1] if sum != dividend goto fail
		&micro.SkipIf{Left: register.NewVector(p.Sum.Register), Right: p.Dividend.ToMicroExpr().ToVec(), Skip: 2},
		// [assertion 2] if witsum != divisor goto fail
		&micro.SkipIf{Left: register.NewVector(p.WitSum.Register), Right: p.Divisor.ToMicroExpr().ToVec(), Skip: 1},
		// Branch
		&micro.Jmp{Target: pc + 1},
		// fail
		&micro.Fail{},
	}
	//
	return micro.Instruction{Codes: codes}
}

// RegistersRead implementation for Instruction interface.
func (p *Division) RegistersRead() []io.RegisterId {
	return expr.RegistersRead(p.Dividend, p.Divisor)
}

// RegistersWritten implementation for Instruction interface.
func (p *Division) RegistersWritten() []io.RegisterId {
	return []io.RegisterId{p.Quotient.Register, p.Remainder.Register,
		p.Witness.Register, p.Sum.Register, p.WitSum.Register}
}

// String implementation for Instruction interface.
func (p *Division) String(fn register.Map) string {
	var builder strings.Builder
	//
	builder.WriteString(p.Quotient.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.Remainder.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.Witness.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.Sum.String(fn))
	builder.WriteString(", ")
	builder.WriteString(p.WitSum.String(fn))
	builder.WriteString(" = ")
	builder.WriteString(p.Dividend.String(fn))
	builder.WriteString(" / ")
	builder.WriteString(p.Divisor.String(fn))
	//
	return builder.String()
}

// Validate implementation for Instruction interface.
func (p *Division) Validate(fieldWidth uint, fn register.Map) error {
	var (
		qBits, _  = expr.BitWidth(&p.Quotient, fn)
		rBits, _  = expr.BitWidth(&p.Remainder, fn)
		wBits, _  = expr.BitWidth(&p.Witness, fn)
		sBits, _  = expr.BitWidth(&p.Sum, fn)
		wsBits, _ = expr.BitWidth(&p.WitSum, fn)
		dBits, _  = expr.BitWidth(p.Dividend, fn)
		vBits, _  = expr.BitWidth(p.Divisor, fn)
		// construct sum calculation to determine absolute bitwidth requirement.
		sumBits, _ = expr.BitWidth(sumCalc(&p.Quotient, &p.Remainder, p.Divisor), fn)
		// construct witsum calculation to determine absolute bitwidth requirement.
		witsumBits, _ = expr.BitWidth(witsumCalc(&p.Remainder, &p.Witness), fn)
	)
	// check
	if qBits < dBits {
		return fmt.Errorf("quotient bit overflow (u%d into u%d)", dBits, qBits)
	} else if rBits < vBits {
		return fmt.Errorf("remainder bit overflow (u%d into u%d)", vBits, rBits)
	} else if wBits < vBits {
		return fmt.Errorf("witness bit overflow (u%d into u%d)", vBits, wBits)
	} else if sBits < sumBits {
		return fmt.Errorf("sum bit overflow (u%d into u%d)", sumBits, sBits)
	} else if wsBits < witsumBits {
		return fmt.Errorf("witsum bit overflow (u%d into u%d)", witsumBits, wsBits)
	}
	//
	return nil
}

// construct the sum calculation
func sumCalc(quot, rem, divisor expr.Expr) expr.Expr {
	var quotDiv = []expr.Expr{quot, divisor}
	//
	return &expr.Add{
		Exprs: []expr.Expr{rem,
			&expr.Mul{Exprs: quotDiv}},
	}
}

func witsumCalc(rem, witness expr.Expr) expr.Expr {
	one := &expr.Const{Constant: *big.NewInt(1), Base: 10}
	//
	return &expr.Add{
		Exprs: []expr.Expr{one, rem, witness},
	}
}
