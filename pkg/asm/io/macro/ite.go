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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// IfThenElse represents a ternary operation.
type IfThenElse struct {
	Targets []io.RegisterId
	// Cond indicates the condition
	Cond uint8
	// Left-hand side
	Left expr.AtomicExpr
	// Right-hand side
	Right expr.AtomicExpr
	// Then/Else branches
	Then, Else Expr
}

// Execute implementation for Instruction interface.
func (p *IfThenElse) Execute(state io.State) uint {
	var (
		lhs   = p.Left.Eval(state.Internal())
		rhs   = p.Right.Eval(state.Internal())
		value big.Int
		taken bool
	)
	// Check whether taken or not.
	switch p.Cond {
	case EQ:
		taken = lhs.Cmp(&rhs) == 0
	case NEQ:
		taken = lhs.Cmp(&rhs) != 0
	default:
		panic("unreachable")
	}
	//
	if taken {
		value = p.Then.Eval(state.Internal())
	} else {
		value = p.Else.Eval(state.Internal())
	}
	// Write value
	state.StoreAcross(value, p.Targets...)
	//
	return state.Pc() + 1
}

// Lower implementation for Instruction interface.
func (p *IfThenElse) Lower(pc uint) micro.Instruction {
	var (
		codes      []micro.Code
		thenBranch = p.Then.Polynomial()
		elseBranch = p.Else.Polynomial()
		lhs        register.Vector
		rhs        micro.VecExpr
	)
	// normalise left / right
	if c, ok := p.Left.(*expr.Const); ok {
		lhs = register.NewVector(p.Right.(*expr.RegAccess).Register)
		rhs = micro.NewConstant(c.Constant).ToVec()
	} else if c, ok := p.Right.(*expr.Const); ok {
		lhs = register.NewVector(p.Left.(*expr.RegAccess).Register)
		rhs = micro.NewConstant(c.Constant).ToVec()
	} else {
		lhs = register.NewVector(p.Left.(*expr.RegAccess).Register)
		rhs = p.Right.(*expr.RegAccess).ToMicroExpr().ToVec()
	}
	//
	switch p.Cond {
	case EQ:
		codes = []micro.Code{
			&micro.SkipIf{Left: lhs, Right: rhs, Skip: 2},
			// Then branch
			&micro.Assign{Targets: p.Targets, Source: thenBranch},
			&micro.Jmp{Target: pc + 1},
			// Else branch
			&micro.Assign{Targets: p.Targets, Source: elseBranch},
			&micro.Jmp{Target: pc + 1},
		}
	case NEQ:
		codes = []micro.Code{
			&micro.SkipIf{Left: lhs, Right: rhs, Skip: 2},
			// Then branch
			&micro.Assign{Targets: p.Targets, Source: elseBranch},
			&micro.Jmp{Target: pc + 1},
			// Else branch
			&micro.Assign{Targets: p.Targets, Source: thenBranch},
			&micro.Jmp{Target: pc + 1},
		}
	default:
		panic("unreachable")
	}
	//
	return micro.Instruction{Codes: codes}
}

// RegistersRead implementation for Instruction interface.
func (p *IfThenElse) RegistersRead() []io.RegisterId {
	return expr.RegistersRead(p.Left, p.Right, p.Then, p.Else)
}

// RegistersWritten implementation for Instruction interface.
func (p *IfThenElse) RegistersWritten() []io.RegisterId {
	return p.Targets
}

func (p *IfThenElse) String(fn register.Map) string {
	var (
		regs    = fn.Registers()
		targets = io.RegistersReversedToString(p.Targets, regs)
		left    = p.Left.String(fn)
		right   = p.Right.String(fn)
		tb      = p.Then.String(fn)
		fb      = p.Else.String(fn)
		op      string
	)
	//
	switch p.Cond {
	case EQ:
		op = "=="
	case NEQ:
		op = "!="
	default:
		panic("unreachable")
	}
	//
	return fmt.Sprintf("%s = %s%s%s ? %s : %s", targets, left, op, right, tb, fb)
}

// Validate implementation for Instruction interface.
func (p *IfThenElse) Validate(fieldWidth uint, fn register.Map) error {
	var (
		regs                  = fn.Registers()
		lhs_bits              = sumTargetBits(p.Targets, regs)
		then_bits, thenSigned = expr.BitWidth(p.Then, fn)
		else_bits, elseSigned = expr.BitWidth(p.Else, fn)
		rhs_bits              = max(then_bits, else_bits)
	)
	// check
	if lhs_bits < rhs_bits {
		return fmt.Errorf("bit overflow (u%d into u%d)", rhs_bits, lhs_bits)
	} else if rhs_bits > fieldWidth {
		return fmt.Errorf("field overflow (u%d into u%d field)", rhs_bits, fieldWidth)
	} else if thenSigned || elseSigned {
		return errors.New("signed exprtession not supported here")
	}
	//
	return io.CheckTargetRegisters(p.Targets, regs)
}
