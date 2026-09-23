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
package mirc

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Expr is a wrapper around a corset expression which provides the
// necessary interface.
type Expr[F field.Element[F]] struct {
	expr    mir.Term[F]
	logical mir.LogicalTerm[F]
}

// AsLogical extracts a logical constraint from this expression.
func (p Expr[F]) AsLogical() mir.LogicalTerm[F] {
	return p.logical.Simplify()
}

// Add constructs a sum between this expression and zero or more
func (p Expr[F]) Add(exprs ...Expr[F]) Expr[F] {
	args := unwrapSplitMirExpr(p, exprs...)
	return Expr[F]{term.Sum(args...), nil}
}

// And constructs a conjunction between this expression and zero or more
// expressions.
func (p Expr[F]) And(exprs ...Expr[F]) Expr[F] {
	args := unwrapSplitMirLogicals(p, exprs...)
	return Expr[F]{nil, term.Conjunction(args...)}
}

// Equals constructs an equality between two expressions.
func (p Expr[F]) Equals(rhs Expr[F]) Expr[F] {
	if p.expr == nil {
		panic("invalid left argument")
	} else if rhs.expr == nil {
		panic("invalid right argument")
	}
	//
	logical := term.Equals[F, mir.LogicalTerm[F]](p.expr, rhs.expr)
	//
	return Expr[F]{nil, logical}
}

// Then constructs an implication between two expressions.
func (p Expr[F]) Then(trueBranch Expr[F]) Expr[F] {
	logical := term.IfThenElse(p.logical, trueBranch.logical, nil)
	return Expr[F]{nil, logical}
}

// ThenElse constructs an if-then-else expression with this expression
// acting as the condition.
func (p Expr[F]) ThenElse(trueBranch Expr[F], falseBranch Expr[F]) Expr[F] {
	logical := term.IfThenElse(p.logical, trueBranch.logical, falseBranch.logical)
	return Expr[F]{nil, logical}
}

// Multiply constructs a product between this expression and zero or more
// expressions.
func (p Expr[F]) Multiply(exprs ...Expr[F]) Expr[F] {
	args := unwrapSplitMirExpr(p, exprs...)
	return Expr[F]{term.Product(args...), nil}
}

// Subtract constructs a difference between this expression and zero or more
// expressions.
func (p Expr[F]) Subtract(exprs ...Expr[F]) Expr[F] {
	args := unwrapSplitMirExpr(p, exprs...)
	return Expr[F]{term.Subtract(args...), nil}
}

// NotEquals constructs a non-equality between two expressions.
func (p Expr[F]) NotEquals(rhs Expr[F]) Expr[F] {
	logical := term.NotEquals[F, mir.LogicalTerm[F]](p.expr, rhs.expr)
	return Expr[F]{nil, logical}
}

// Bool constructs a truth or falsehood
func (p Expr[F]) Bool(val bool) Expr[F] {
	if val {
		// empty conjunction is true
		return Expr[F]{nil, term.Conjunction[F, mir.LogicalTerm[F]]()}
	}
	// empty disjunction is false
	return Expr[F]{nil, term.Disjunction[F, mir.LogicalTerm[F]]()}
}

// BigInt constructs a constant expression from a big integer.
func (p Expr[F]) BigInt(number big.Int) Expr[F] {
	// Not power of 2
	var (
		num F
		n   big.Int
	)
	//
	if number.Sign() < 0 {
		n.Add(&number, num.Modulus())
	} else {
		n = number
	}
	//
	num = num.SetBytes(n.Bytes())
	//
	return Expr[F]{term.Const[F, mir.Term[F]](num), nil}
}

// Or constructs a disjunction between this expression and zero or more
// expressions.
func (p Expr[F]) Or(exprs ...Expr[F]) Expr[F] {
	args := unwrapSplitMirLogicals(p, exprs...)
	return Expr[F]{nil, term.Disjunction(args...)}
}

// Variable constructs a variable with a given shift.
func (p Expr[F]) Variable(index register.Id, bitwidth uint, shift int) Expr[F] {
	return Expr[F]{term.NewRegisterAccess[F, mir.Term[F]](index, bitwidth, shift), nil}
}

func (p Expr[F]) String(func(register.Id) string) string {
	if p.expr != nil {
		return p.expr.Lisp(false, nil).String(false)
	} else if p.logical != nil {
		return p.logical.Lisp(false, nil).String(false)
	} else {
		return "nil"
	}
}

func unwrapSplitMirExpr[F field.Element[F]](head Expr[F], tail ...Expr[F]) []mir.Term[F] {
	cexprs := make([]mir.Term[F], len(tail)+1)
	//
	cexprs[0] = head.expr
	//
	for i, e := range tail {
		cexprs[i+1] = e.expr
		//
		if e.logical != nil {
			panic("logical expression encountered")
		}
	}
	//
	return cexprs
}

func unwrapSplitMirLogicals[F field.Element[F]](head Expr[F], tail ...Expr[F]) []mir.LogicalTerm[F] {
	cexprs := make([]mir.LogicalTerm[F], len(tail)+1)
	//
	cexprs[0] = head.logical
	//
	for i, e := range tail {
		cexprs[i+1] = e.logical
		//
		if e.expr != nil {
			panic("arithmetic expression encountered")
		}
	}
	//
	return cexprs
}
