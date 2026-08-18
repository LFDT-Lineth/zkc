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
package ast

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Expr represents an arbitrary expression over the columns of a given context
// (or the parameters of an enclosing function).  Such expressions are pitched
// at a higher-level than those of the underlying constraint system.  For
// example, they can contain conditionals (i.e. if expressions) and
// normalisations, etc.  During the lowering process down to the underlying
// constraints level (AIR), such expressions are "compiled out" using various
// techniques (such as introducing computed columns where necessary).
type Expr interface {
	Node
	// Evaluates this expression as a constant (signed) value.  If this
	// expression is not constant, then nil is returned.
	AsConstant() *big.Int
	// Context returns the context for this expression.  Observe that the
	// expression must have been resolved for this to be defined (i.e. it may
	// panic if it has not been resolved yet).
	Context() Context
	// Return set of columns on which this declaration depends.
	Dependencies() []Symbol
}

// ============================================================================
// Addition
// ============================================================================

// Add represents the sum over zero or more expressions.
type Add struct{ Args []Expr }

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Add) AsConstant() *big.Int {
	fn := func(l *big.Int, r *big.Int) { l.Add(l, r) }
	return AsConstantOfExpressions(e.Args, fn)
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Add) Context() Context {
	ctx, _ := ContextOfExpressions(e.Args...)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Add) Lisp() sexp.SExp {
	return ListOfExpressions(sexp.NewSymbol("+"), e.Args)
}

// Dependencies needed to signal declaration.
func (e *Add) Dependencies() []Symbol {
	return DependenciesOfExpressions(e.Args)
}

// ============================================================================
// ArrayAccess
// ============================================================================

// ArrayAccess represents the a given value taken to a power.
type ArrayAccess struct {
	Name         file.Path
	Arg          Expr
	ArrayBinding Binding
}

// IsResolved checks whether this symbol has been resolved already, or not.
func (e *ArrayAccess) IsResolved() bool {
	return e.ArrayBinding != nil
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *ArrayAccess) AsConstant() *big.Int {
	return nil
}

// Path returns the given path of this symbol.
func (e *ArrayAccess) Path() *file.Path {
	return &e.Name
}

// Binding gets binding associated with this interface.  This will panic if this
// symbol is not yet resolved.
func (e *ArrayAccess) Binding() Binding {
	return e.ArrayBinding
}

// Type returns the type associated with this symbol.  If the type cannot be
// determined, then nil is returned.
func (e *ArrayAccess) Type() Type {
	if binding, ok := e.ArrayBinding.(*ColumnBinding); !ok {
		return nil
	} else if arr_t, ok := binding.DataType.(*ArrayType); ok {
		return arr_t.element
	}
	// Cannot be typed
	return nil
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *ArrayAccess) Context() Context {
	// Check the expected options.
	binding, ok := e.ArrayBinding.(*ColumnBinding)
	// Sanity check
	if ok {
		context := binding.Context()
		context = context.Join(e.Arg.Context())
		//
		return context
	}
	//
	panic("invalid column access")
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *ArrayAccess) Lisp() sexp.SExp {
	return sexp.NewArray([]sexp.SExp{
		sexp.NewSymbol(e.Name.String()),
		e.Arg.Lisp(),
	})
}

// Resolve this symbol by associating it with the binding associated with
// the definition of the symbol to which this refers.
func (e *ArrayAccess) Resolve(binding Binding) bool {
	if binding == nil {
		panic("empty binding")
	} else if e.ArrayBinding != nil {
		panic("already resolved")
	}
	//
	e.ArrayBinding = binding
	//
	return true
}

// Dependencies needed to signal declaration.
func (e *ArrayAccess) Dependencies() []Symbol {
	deps := e.Arg.Dependencies()
	return append(deps, e)
}

// ============================================================================
// Connective
// ============================================================================

// Connective represents a logical connective, such as logical AND / logical OR.
type Connective struct {
	Sign bool // true = OR, false = AND
	Args []Expr
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Connective) AsConstant() *big.Int {
	fn := func(l *big.Int, r *big.Int) { l.Mul(l, r) }
	return AsConstantOfExpressions(e.Args, fn)
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Connective) Context() Context {
	ctx, _ := ContextOfExpressions(e.Args...)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Connective) Lisp() sexp.SExp {
	var symbol = "∧"
	if e.Sign {
		symbol = "∨"
	}

	return ListOfExpressions(sexp.NewSymbol(symbol), e.Args)
}

// Dependencies needed to signal declaration.
func (e *Connective) Dependencies() []Symbol {
	return DependenciesOfExpressions(e.Args)
}

// ============================================================================
// Constants
// ============================================================================

// Constant represents a constant value within an expression.
type Constant struct{ Val big.Int }

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Constant) AsConstant() *big.Int {
	return &e.Val
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Constant) Context() Context {
	return VoidContext()
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Constant) Lisp() sexp.SExp {
	return sexp.NewSymbol(e.Val.String())
}

// Dependencies needed to signal declaration.
func (e *Constant) Dependencies() []Symbol {
	return nil
}

// ============================================================================
// Equality
// ============================================================================

const (
	// EQUALS indicates an equals (==) relationship
	EQUALS uint8 = 0
	// NOT_EQUALS indicates a not-equals (!=) relationship
	NOT_EQUALS uint8 = 1
)

// Equation represents either an equality (e.g. X==Y), a non-equality (X!=Y), or
// an inequality (X<=Y, X<Y, etc).
type Equation struct {
	// Indicates equality (true) or non-equality (false).
	Kind uint8
	// Left-Hand Side
	Lhs Expr
	// Right-Hand Side
	Rhs Expr
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Equation) AsConstant() *big.Int {
	lhs := e.Lhs.AsConstant()
	rhs := e.Lhs.AsConstant()
	//
	if lhs == nil || rhs == nil {
		return nil
	}
	// Determine relationship
	cmp := lhs.Cmp(rhs)
	//
	switch e.Kind {
	case EQUALS:
		if cmp == 0 {
			return big.NewInt(0)
		}
	case NOT_EQUALS:
		if cmp != 0 {
			return big.NewInt(0)
		}
	default:
		panic("unreachable")
	}
	// false
	return big.NewInt(1)
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Equation) Context() Context {
	ctx, _ := ContextOfExpressions(e.Lhs, e.Rhs)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Equation) Lisp() sexp.SExp {
	var symbol sexp.SExp
	//
	switch e.Kind {
	case EQUALS:
		symbol = sexp.NewSymbol("==")
	case NOT_EQUALS:
		symbol = sexp.NewSymbol("!=")
	default:
		panic("unreachable")
	}
	//
	return sexp.NewList([]sexp.SExp{
		symbol,
		e.Lhs.Lisp(),
		e.Rhs.Lisp()})
}

// Dependencies needed to signal declaration.
func (e *Equation) Dependencies() []Symbol {
	return DependenciesOfExpressions([]Expr{e.Lhs, e.Rhs})
}

// LeftHandSide returns the left-hand side of this condition.
func (e *Equation) LeftHandSide() Expr {
	return e.Lhs
}

// RightHandSide returns the right-hand side of this condition.
func (e *Equation) RightHandSide() Expr {
	return e.Rhs
}

// ============================================================================
// If
// ============================================================================

// If returns the (optional) true branch when the condition evaluates to zero, and
// the (optional false branch otherwise.
type If struct {
	// Elements contained within this list.
	Condition Expr
	// True branch (optional).
	TrueBranch Expr
	// False branch (optional).
	FalseBranch Expr
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *If) AsConstant() *big.Int {
	if condition := e.Condition.AsConstant(); condition != nil {
		// Determine whether condition holds true (or not).
		holds := condition.Cmp(big.NewInt(0)) == 0
		//
		if holds && e.TrueBranch != nil {
			return e.TrueBranch.AsConstant()
		} else if !holds && e.FalseBranch != nil {
			return e.FalseBranch.AsConstant()
		}
	}
	//
	return nil
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *If) Context() Context {
	ctx := e.Condition.Context()
	//
	if e.TrueBranch != nil {
		ctx = ctx.Join(e.TrueBranch.Context())
	}
	//
	if e.FalseBranch != nil {
		ctx = ctx.Join(e.FalseBranch.Context())
	}
	//
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *If) Lisp() sexp.SExp {
	if e.FalseBranch != nil {
		return sexp.NewList([]sexp.SExp{
			sexp.NewSymbol("if"),
			e.Condition.Lisp(),
			e.TrueBranch.Lisp(),
			e.FalseBranch.Lisp()})
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("if"),
		e.Condition.Lisp(),
		e.TrueBranch.Lisp()})
}

// Dependencies needed to signal declaration.
func (e *If) Dependencies() []Symbol {
	return DependenciesOfExpressions([]Expr{e.Condition, e.TrueBranch, e.FalseBranch})
}

// ============================================================================
// Multiplication
// ============================================================================

// Mul represents the product over zero or more expressions.
type Mul struct{ Args []Expr }

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Mul) AsConstant() *big.Int {
	fn := func(l *big.Int, r *big.Int) { l.Mul(l, r) }
	return AsConstantOfExpressions(e.Args, fn)
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Mul) Context() Context {
	ctx, _ := ContextOfExpressions(e.Args...)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Mul) Lisp() sexp.SExp {
	return ListOfExpressions(sexp.NewSymbol("*"), e.Args)
}

// Dependencies needed to signal declaration.
func (e *Mul) Dependencies() []Symbol {
	return DependenciesOfExpressions(e.Args)
}

// ============================================================================
// Not
// ============================================================================

// Not performs a logical negation on its argument.
type Not struct{ Arg Expr }

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Not) AsConstant() *big.Int {
	if arg := e.Arg.AsConstant(); arg != nil {
		if arg.Cmp(big.NewInt(0)) != 0 {
			// false => true
			return big.NewInt(0)
		}
		// true => false
		return big.NewInt(1)
	}
	//
	return nil
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Not) Context() Context {
	return e.Arg.Context()
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Not) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("¬"),
		e.Arg.Lisp()})
}

// Dependencies needed to signal declaration.
func (e *Not) Dependencies() []Symbol {
	return e.Arg.Dependencies()
}

// ============================================================================
// Subtraction
// ============================================================================

// Sub represents the subtraction over zero or more expressions.
type Sub struct{ Args []Expr }

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Sub) AsConstant() *big.Int {
	fn := func(l *big.Int, r *big.Int) { l.Sub(l, r) }
	return AsConstantOfExpressions(e.Args, fn)
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Sub) Context() Context {
	ctx, _ := ContextOfExpressions(e.Args...)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Sub) Lisp() sexp.SExp {
	return ListOfExpressions(sexp.NewSymbol("-"), e.Args)
}

// Dependencies needed to signal declaration.
func (e *Sub) Dependencies() []Symbol {
	return DependenciesOfExpressions(e.Args)
}

// ============================================================================
// Shift
// ============================================================================

// Shift represents the result of a given expression shifted by a certain
// amount.  In reality, the shift amount must be statically known.  However, it
// is represented here as an expression to allow for constants and the results
// of function invocations, etc to be used.  In all cases, these must still be
// eventually translated into constant values however.
type Shift struct {
	// The expression being shifted
	Arg Expr
	// The amount it is being shifted by.
	Shift Expr
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *Shift) AsConstant() *big.Int {
	// Observe the shift doesn't matter as, in the case that the argument is a
	// constant, then the shift has no effect anyway.
	return e.Arg.AsConstant()
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *Shift) Context() Context {
	ctx, _ := ContextOfExpressions(e.Arg, e.Shift)
	return ctx
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *Shift) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("shift"),
		e.Arg.Lisp(),
		e.Shift.Lisp()})
}

// Dependencies needed to signal declaration.
func (e *Shift) Dependencies() []Symbol {
	return DependenciesOfExpressions([]Expr{e.Arg, e.Shift})
}

// ============================================================================
// VariableAccess
// ============================================================================

// VariableAccess represents reading the value of a given local variable (such
// as a function parameter).
type VariableAccess struct {
	Name    file.Path
	binding Binding
}

// NewVariableAccess creates a new variable access with the given (optionally
// qualified) path, and which has a given initial binding (which can be nil).
func NewVariableAccess(path file.Path, binding Binding) *VariableAccess {
	return &VariableAccess{path, binding}
}

// AsConstant attempts to evaluate this expression as a constant (signed) value.
// If this expression is not constant, then nil is returned.
func (e *VariableAccess) AsConstant() *big.Int {
	if binding, ok := e.binding.(*ConstantBinding); ok {
		return binding.Value.AsConstant()
	}
	// not a constant
	return nil
}

// Path returns the given path of this symbol.
func (e *VariableAccess) Path() *file.Path {
	return &e.Name
}

// IsResolved checks whether this symbol has been resolved already, or not.
func (e *VariableAccess) IsResolved() bool {
	return e.binding != nil
}

// Resolve this symbol by associating it with the binding associated with
// the definition of the symbol to which this refers.
func (e *VariableAccess) Resolve(binding Binding) bool {
	if binding == nil {
		panic("empty binding")
	} else if e.binding != nil {
		panic("already resolved")
	}
	//
	e.binding = binding
	//
	return true
}

// Binding gets binding associated with this interface.  This returns nil if the
// access has not already been resolved.
func (e *VariableAccess) Binding() Binding {
	return e.binding
}

// Context returns the context for this expression.  Observe that the
// expression must have been resolved for this to be defined (i.e. it may
// panic if it has not been resolved yet).
func (e *VariableAccess) Context() Context {
	// Check the expected options.
	if binding, ok := e.binding.(*ColumnBinding); ok {
		return binding.Context()
	} else if _, ok := e.Binding().(*ConstantBinding); ok {
		return VoidContext()
	}
	//
	panic("invalid column access")
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.a
func (e *VariableAccess) Lisp() sexp.SExp {
	return sexp.NewSymbol(e.Name.String())
}

// Dependencies needed to signal declaration.
func (e *VariableAccess) Dependencies() []Symbol {
	return []Symbol{e}
}

// Type returns the type associated with this symbol.  If the type cannot be
// determined, then nil is returned.
func (e *VariableAccess) Type() Type {
	if binding, ok := e.binding.(*ColumnBinding); ok {
		return binding.DataType
	}
	// Cannot be typed
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// ContextOfExpressions returns the context for a set of zero or more
// expressions.  Observe that, if there the expressions have no context (i.e.
// they are all constants) then the void context is returned.  Likewise, if
// there are expressions with different contexts then the conflicted context
// will be returned.  Otherwise, the one consistent context will be returned.
func ContextOfExpressions[E Expr](exprs ...E) (Context, uint) {
	context := VoidContext()
	//
	for i, e := range exprs {
		context = context.Join(e.Context())
		//
		if context.IsConflicted() {
			return context, uint(i)
		}
	}
	//
	return context, uint(len(exprs))
}

// DependenciesOfExpressions determines the dependencies for a given set of zero
// or more expressions.
func DependenciesOfExpressions(exprs []Expr) []Symbol {
	var deps []Symbol
	//
	for _, e := range exprs {
		if e != nil {
			deps = append(deps, e.Dependencies()...)
		}
	}
	//
	return deps
}

// ListOfExpressions converts an array of one or more expressions into a list of
// corresponding lisp expressions.
func ListOfExpressions[E Expr](head sexp.SExp, exprs []E) *sexp.List {
	lisps := make([]sexp.SExp, len(exprs)+1)
	// Assign head
	lisps[0] = head
	//
	for i, e := range exprs {
		lisps[i+1] = e.Lisp()
	}
	//
	return sexp.NewList(lisps)
}

// AsConstantOfExpressions attempts to fold one or more expressions across a
// given operation (e.g. add, subtract, etc) to produce a constant value.  If
// any of the expressions are not themselves constant, then neither is the
// result.
func AsConstantOfExpressions(exprs []Expr, fn func(*big.Int, *big.Int)) *big.Int {
	var val big.Int
	//
	for i, arg := range exprs {
		c := arg.AsConstant()
		if c == nil {
			return nil
		} else if i == 0 {
			// Must clone c
			val.Set(c)
		} else {
			fn(&val, c)
		}
	}
	//
	return &val
}
