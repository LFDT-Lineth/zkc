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
package expr

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

const (
	// EQ indicates an equality condition
	EQ CmpOp = 0
	// NEQ indicates a non-equality condition
	NEQ CmpOp = 1
	// LT indicates a less-than condition
	LT CmpOp = 2
	// GT indicates a greater-than condition
	GT CmpOp = 3
	// LTEQ indicates a less-than-or-equals condition
	LTEQ CmpOp = 4
	// GTEQ indicates a greater-than-or-equals condition
	GTEQ CmpOp = 5
)

// CmpOp represents the set of possible operators for a comparison condition.
type CmpOp uint8

// Fieldable returns true for operators which are permitted on the field element
// type.
func (p CmpOp) Fieldable() bool {
	return p == EQ || p == NEQ
}

// Cmp represents a comparison, such as "==", ">=", etc.
type Cmp[S symbol.Symbol[S]] struct {
	// Operator indicates the condition
	Operator CmpOp
	// Left-hand side
	Left Expr[S]
	// Right-hand side
	Right Expr[S]
}

// NewCmp returns a freshly created comparison condition.
func NewCmp[S symbol.Symbol[S]](op CmpOp, lhs, rhs Expr[S]) *Cmp[S] {
	return &Cmp[S]{op, lhs, rhs}
}

// Negate implementation for Condition interface.
func (p *Cmp[S]) Negate() Condition[S] {
	var op CmpOp
	//
	switch p.Operator {
	case EQ:
		op = NEQ
	case NEQ:
		op = EQ
	case LT:
		op = GTEQ
	case LTEQ:
		op = GT
	case GT:
		op = LTEQ
	case GTEQ:
		op = LT
	default:
		panic("unreachable")
	}
	//
	return &Cmp[S]{op, p.Left, p.Right}
}

// ExternUses implementation for the Condition interface.
func (p *Cmp[S]) ExternUses() set.AnySortedSet[S] {
	panic("todo")
}

// LocalUses implementation for the Condition interface.
func (p *Cmp[S]) LocalUses() bit.Set {
	var reads bit.Set
	//
	reads.Union(p.Left.LocalUses())
	reads.Union(p.Right.LocalUses())
	//
	return reads
}

// SetType implementation for Expr interface
func (p *Cmp[S]) SetType(t data.Type[S]) {
	panic("unreachable")
}

// Type implementation for Expr interface
func (p *Cmp[S]) Type() data.Type[S] {
	panic("unreachable")
}

func (p *Cmp[S]) String(env variable.Map[S]) string {
	var (
		l  = p.Left.String(env)
		r  = p.Right.String(env)
		op string
	)
	//
	switch p.Operator {
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
	return fmt.Sprintf("%s%s%s", l, op, r)
}
