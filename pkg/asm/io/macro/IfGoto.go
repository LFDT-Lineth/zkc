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

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

const (
	// EQ indicates an equality condition
	EQ uint8 = 0
	// NEQ indicates a non-equality condition
	NEQ uint8 = 1
	// LT indicates a less-than condition
	LT uint8 = 2
	// GT indicates a greater-than condition
	GT uint8 = 3
	// LTEQ indicates a less-than-or-equals condition
	LTEQ uint8 = 4
	// GTEQ indicates a greater-than-or-equals condition
	GTEQ uint8 = 5
)

// IfGoto describes a conditional branch, which is either jeq ("Jump if equal") or
// jne ("Jump if not equal").  This has two variants: register-register; and,
// register-constant.  The latter is indiciated when the right register is
// marked as UNUSED.
type IfGoto struct {
	// Cond indicates the condition
	Cond uint8
	// Left-hand side
	Left expr.AtomicExpr
	// Right-hand side
	Right expr.AtomicExpr
	// Target identifies target PC
	Target uint
}

// Bind any labels contained within this instruction using the given label map.
func (p *IfGoto) Bind(labels []uint) {
	p.Target = labels[p.Target]
}

// Execute implementation for Instruction interface.
func (p *IfGoto) Execute(state io.State) uint {
	var (
		lhs   = p.Left.Eval(state.Internal())
		rhs   = p.Right.Eval(state.Internal())
		taken bool
	)
	//
	switch p.Cond {
	case EQ:
		taken = lhs.Cmp(&rhs) == 0
	case NEQ:
		taken = lhs.Cmp(&rhs) != 0
	case LT:
		taken = lhs.Cmp(&rhs) < 0
	case LTEQ:
		taken = lhs.Cmp(&rhs) <= 0
	case GT:
		taken = lhs.Cmp(&rhs) > 0
	case GTEQ:
		taken = lhs.Cmp(&rhs) >= 0
	default:
		panic("unreachable")
	}
	// Check if taken or not taken
	if taken {
		return p.Target
	}
	//
	return state.Pc() + 1
}

// Lower implementation for Instruction interface.
func (p *IfGoto) Lower(pc uint) micro.Instruction {
	var (
		codes []micro.Code
		lhs   register.Vector
		rhs   micro.VecExpr
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
			&micro.SkipIf{Left: lhs, Right: rhs, Skip: 1},
			&micro.Jmp{Target: p.Target},
			&micro.Jmp{Target: pc + 1},
		}
	case NEQ:
		codes = []micro.Code{
			&micro.SkipIf{Left: lhs, Right: rhs, Skip: 1},
			&micro.Jmp{Target: pc + 1},
			&micro.Jmp{Target: p.Target},
		}
	default:
		panic("unreachable")
	}
	//
	return micro.Instruction{Codes: codes}
}

// RegistersRead implementation for Instruction interface.
func (p *IfGoto) RegistersRead() []io.RegisterId {
	return expr.RegistersRead(p.Left, p.Right)
}

// RegistersWritten implementation for Instruction interface.
func (p *IfGoto) RegistersWritten() []io.RegisterId {
	return nil
}

func (p *IfGoto) String(fn register.Map) string {
	var (
		l  = p.Left.String(fn)
		r  = p.Right.String(fn)
		op string
	)
	//
	switch p.Cond {
	case EQ:
		op = "=="
	case NEQ:
		op = "!="
	case LT:
		op = "<"
	case LTEQ:
		op = "<="
	case GT:
		op = ">"
	case GTEQ:
		op = ">="
	default:
		panic("unreachable")
	}
	//
	return fmt.Sprintf("if %s%s%s goto %d", l, op, r, p.Target)
}

// Validate implementation for Instruction interface.
func (p *IfGoto) Validate(fieldWidth uint, fn register.Map) error {
	if p.Left.Equals(p.Right) {
		switch p.Cond {
		case EQ, LTEQ, GTEQ:
			return fmt.Errorf("always taken")
		default:
			return fmt.Errorf("never taken")
		}
	}
	// sanity check
	if _, lhsConst := p.Left.(*expr.Const); lhsConst {
		if _, rhsConst := p.Right.(*expr.Const); rhsConst {
			return fmt.Errorf("branch always (or never) taken")
		}
	}
	//
	return nil
}
