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
package compiler

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// SyntaxError defines the kind of errors that can be reported by this compiler.
// Syntax errors are always associated with some line in one of the original
// source files.  For simplicity, we reuse existing notion of syntax error from
// the S-Expression library.
type SyntaxError = source.SyntaxError

// TypeCheckCircuit performs a type checking pass over the circuit to ensure
// types are used correctly.  Additionally, this resolves some ambiguities
// arising from the possibility of overloading function calls, etc.
func TypeCheckCircuit(srcmap *source.Maps[ast.Node],
	circuit *ast.Circuit) []SyntaxError {
	// Construct fresh typeChecker
	p := typeChecker{srcmap}
	// typeCheck all declarations
	return p.typeCheckDeclarations(circuit)
}

// typeChecker performs typeChecking prior to final translation. Specifically,
// it expands all invocations and reductions.  Thus, final
// translation is greatly simplified after this step.
type typeChecker struct {
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
}

// typeCheck all assignment or constraint declarations in the circuit.
func (p *typeChecker) typeCheckDeclarations(circuit *ast.Circuit) []SyntaxError {
	errors := p.typeCheckDeclarationsInModule(circuit.Declarations)
	// typeCheck each module
	for _, m := range circuit.Modules {
		errs := p.typeCheckDeclarationsInModule(m.Declarations)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// typeCheck all assignment or constraint declarations in a given module within
// the circuit.
func (p *typeChecker) typeCheckDeclarationsInModule(decls []ast.Declaration) []SyntaxError {
	var errors []SyntaxError
	//
	for _, d := range decls {
		errs := p.typeCheckDeclaration(d)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// typeCheck an assignment or constraint declaration which occurs within a
// given module.
func (p *typeChecker) typeCheckDeclaration(decl ast.Declaration) []SyntaxError {
	var errors []SyntaxError
	//
	switch d := decl.(type) {
	case *ast.DefAliases:
		// ignore
	case *ast.DefColumns:
		// ignore
	case *ast.DefConst:
		errors = p.typeCheckDefConstInModule(d)
	case *ast.DefConstraint:
		errors = p.typeCheckDefConstraint(d)
	case *ast.DefFun:
		errors = p.typeCheckDefFunInModule(d)
	case *ast.DefInRange:
		errors = p.typeCheckDefInRange(d)
	case *ast.DefLookup:
		errors = p.typeCheckDefLookup(d)
	case *ast.DefPerspective:
		errors = p.typeCheckDefPerspective(d)
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// ast.Type check one or more constant definitions within a given module.
func (p *typeChecker) typeCheckDefConstInModule(decl *ast.DefConst) []SyntaxError {
	var errors []SyntaxError
	//
	for _, c := range decl.Constants {
		// Resolve constant body
		_, errs := p.typeCheckExpressionInModule(ast.UINT_TYPE, c.ConstBinding.Value, true)
		// Accumulate errors
		errors = append(errors, errs...)
	}
	//
	return errors
}

// typeCheck a "defconstraint" declaration.
func (p *typeChecker) typeCheckDefConstraint(decl *ast.DefConstraint) []SyntaxError {
	// FIXME: eventually, the guard should be a BOOLEAN_TYPE in order to
	// force a suitable interpretation.
	//
	// typeCheck (optional) guard
	_, guard_errors := p.typeCheckOptionalExpressionInModule(ast.UINT_TYPE, decl.Guard, true)
	// typeCheck constraint body
	_, constraint_errors := p.typeCheckExpressionInModule(ast.BOOL_TYPE, decl.Constraint, false)
	// Combine errors
	return append(constraint_errors, guard_errors...)
}

// ast.Type check the body of a function.
func (p *typeChecker) typeCheckDefFunInModule(decl *ast.DefFun) []SyntaxError {
	var (
		functional bool
		ret        ast.Type = decl.Return()
	)
	//
	if ret != nil {
		_, functional = ret.(*ast.BoolType)
	}
	// Resolve body and check return
	_, errors := p.typeCheckExpressionInModule(decl.Return(), decl.Body(), functional)
	// Done
	return errors
}

// typeCheck a "deflookup" declaration.
func (p *typeChecker) typeCheckDefLookup(decl *ast.DefLookup) []SyntaxError {
	var (
		errors   []SyntaxError
		srcTypes []ast.Type
		dstTypes []ast.Type
	)
	// Determine source column types
	for i := range decl.Sources {
		srcTypes = ast.LeastUpperBounds(srcTypes, typesOfLookupColumns(decl.Sources[i]))
	}
	// Determine target column types
	for i := range decl.Targets {
		dstTypes = ast.LeastUpperBounds(dstTypes, typesOfLookupColumns(decl.Targets[i]))
	}
	// Check the types (if checking is enabled and no other upstream errors)
	if decl.Checked && len(srcTypes) == len(dstTypes) {
		for i := range srcTypes {
			if !srcTypes[i].SubtypeOf(dstTypes[i]) {
				msg := fmt.Sprintf("expected %s, found %s", dstTypes[i].String(), srcTypes[i].String())
				err := p.srcmap.SyntaxError(decl.Sources[0][i], msg)
				errors = append(errors, *err)
			}
		}
	}
	// Combine errors
	return errors
}

// typesOfLookupColumns determines the type of each column on one side of a
// lookup.  Since these are column accesses (rather than arbitrary expressions)
// their types are already known.  As with expressions, nil is returned if any
// type is unknown, which indicates an upstream error (e.g. an unresolved
// symbol) has already been reported.
func typesOfLookupColumns(columns []ast.TypedSymbol) []ast.Type {
	types := make([]ast.Type, len(columns))
	//
	for i, column := range columns {
		if column == nil || column.Type() == nil {
			return nil
		}
		//
		types[i] = column.Type()
	}
	//
	return types
}

// typeCheck a "definrange" declaration.
func (p *typeChecker) typeCheckDefInRange(decl *ast.DefInRange) []SyntaxError {
	// typeCheck constraint body
	_, errors := p.typeCheckExpressionInModule(ast.UINT_TYPE, decl.Expr, true)
	// Done
	return errors
}

// typeCheck a "defperspective" declaration.
func (p *typeChecker) typeCheckDefPerspective(decl *ast.DefPerspective) []SyntaxError {
	// FIXME: eventually, the selector should be a BOOLEAN_TYPE in order to
	// force a suitable interpretation.
	//
	// typeCheck selector expression
	_, errors := p.typeCheckExpressionInModule(ast.UINT_TYPE, decl.Selector, true)
	// Combine errors
	return errors
}

// typeCheck an optional expression in a given context.  That is an expression
// which maybe nil (i.e. doesn't exist).  In such case, nil is returned (i.e.
// without any errors).
func (p *typeChecker) typeCheckOptionalExpressionInModule(expected ast.Type, expr ast.Expr,
	functional bool) (ast.Type, []SyntaxError) {
	//
	if expr != nil {
		return p.typeCheckExpressionInModule(expected, expr, functional)
	}
	//
	return nil, nil
}

// typeCheck a sequence of zero or more expressions enclosed in a given module.
// All expressions are expected to be non-voidable (see below for more on
// voidability).
func (p *typeChecker) typeCheckExpressionsInModule(expected ast.Type, exprs []ast.Expr,
	functional bool) ([]ast.Type, []SyntaxError) {
	errors := []SyntaxError{}
	types := make([]ast.Type, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		var errs []SyntaxError
		//
		if e == nil {
			continue
		}
		//
		types[i], errs = p.typeCheckExpressionInModule(expected, e, functional)
		errors = append(errors, errs...)
		// Sanity check what we got back
		if types[i] == nil {
			return nil, errors
		}
	}
	//
	return types, errors
}

// typeCheck an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for column access, function
// invocations, etc).
func (p *typeChecker) typeCheckExpressionInModule(expected ast.Type, expr ast.Expr,
	functional bool) (ast.Type, []SyntaxError) {
	var (
		result ast.Type
		types  []ast.Type
		errors []SyntaxError
	)
	//
	switch e := expr.(type) {
	case *ast.ArrayAccess:
		result, errors = p.typeCheckArrayAccessInModule(e)
	case *ast.Add:
		types, errors = p.typeCheckExpressionsInModule(ast.UINT_TYPE, e.Args, true)
		result = typeOfSum(types...)
	case *ast.Cast:
		actual, errs := p.typeCheckExpressionInModule(nil, e.Arg, functional)
		// Check safe casts
		if len(errs) == 0 && actual != nil && !e.Unsafe && expected != nil && !actual.SubtypeOf(expected) {
			msg := fmt.Sprintf("expected type %s, found %s", expected.String(), actual.String())
			return nil, p.srcmap.SyntaxErrors(expr, msg)
		}
		// Discard actual type in favour of coerced type
		result, errors = e.Type, errs
	case *ast.Connective:
		_, errors = p.typeCheckExpressionsInModule(ast.BOOL_TYPE, e.Args, true)
		result = ast.BOOL_TYPE
	case *ast.Constant:
		result = ast.NewIntType(math.NewInterval(e.Val, e.Val))
	case *ast.Equation:
		_, errs1 := p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Lhs, true)
		_, errs2 := p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Rhs, true)
		// Done
		result, errors = ast.BOOL_TYPE, append(errs1, errs2...)
	case *ast.Exp:
		_, errs1 := p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Arg, true)
		_, errs2 := p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Pow, true)
		// Done
		result, errors = ast.UINT_TYPE, append(errs1, errs2...)
	case *ast.If:
		result, errors = p.typeCheckIfInModule(expected, e, functional)
	case *ast.Invoke:
		result, errors = p.typeCheckInvokeInModule(expected, e, functional)
	case *ast.Mul:
		types, errors = p.typeCheckExpressionsInModule(ast.UINT_TYPE, e.Args, true)
		result = typeOfProduct(types...)
	case *ast.Normalise:
		_, errors = p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Arg, true)
		// Normalise guaranteed to return either 0 or 1.
		result = ast.NewUintType(1)
	case *ast.Not:
		_, errors = p.typeCheckExpressionInModule(ast.BOOL_TYPE, e.Arg, true)
		result = ast.BOOL_TYPE
	case *ast.Shift:
		res, arg_errs := p.typeCheckExpressionInModule(nil, e.Arg, functional)
		_, shf_errs := p.typeCheckExpressionInModule(ast.UINT_TYPE, e.Shift, functional)
		// combine errors
		result, errors = res, append(arg_errs, shf_errs...)
	case *ast.Sub:
		types, errors = p.typeCheckExpressionsInModule(ast.UINT_TYPE, e.Args, true)
		result = typeOfSubtraction(types...)
	case *ast.VariableAccess:
		result, errors = p.typeCheckVariableInModule(e)
	case *ast.Concat:
		ts, errors := p.typeCheckExpressionsInModule(ast.UINT_TYPE, e.Args, true)
		//
		return typeOfConcat(ts...), errors
	default:
		msg := fmt.Sprintf("unknown expression encountered during typing (%s)", reflect.TypeOf(expr).String())
		return nil, p.srcmap.SyntaxErrors(expr, msg)
	}
	// Error check
	if expected != nil && result != nil && !result.SubtypeOf(expected) {
		msg := fmt.Sprintf("expected %s, found %s", expected.String(), result.String())
		return nil, p.srcmap.SyntaxErrors(expr, msg)
	}
	//
	return result, errors
}

// ast.Type check an array access expression.  The main thing is to check that the
// column being accessed was originally defined as an array column.
func (p *typeChecker) typeCheckArrayAccessInModule(expr *ast.ArrayAccess) (ast.Type, []SyntaxError) {
	// ast.Type check index expression
	_, errs := p.typeCheckExpressionInModule(ast.UINT_TYPE, expr.Arg, true)
	// NOTE: following cast safe because resolver already checked them.
	if binding, ok := expr.Binding().(*ast.ColumnBinding); !ok || !expr.IsResolved() {
		// NOTE: we don't return an error here, since this case would have already
		// been caught by the resolver and we don't want to double up on errors.
		return nil, nil
	} else if arr_t, ok := binding.DataType.(*ast.ArrayType); !ok {
		return nil, append(errs, *p.srcmap.SyntaxError(expr, "expected array column"))
	} else {
		return arr_t.Element(), errs
	}
}

// ast.Type an if condition contained within some expression which, in turn, is
// contained within some module.  An important step occurrs here where, based on
// the semantics of the condition, this is inferred as an "if-zero" or an
// "if-notzero".
func (p *typeChecker) typeCheckIfInModule(expected ast.Type, expr *ast.If, functional bool) (ast.Type, []SyntaxError) {
	// Check condition
	_, errors := p.typeCheckExpressionInModule(ast.BOOL_TYPE, expr.Condition, true)
	// Check true branch
	res_t, errs := p.typeCheckExpressionInModule(expected, expr.TrueBranch, functional)
	errors = append(errors, errs...)
	//
	if expr.FalseBranch != nil {
		rhs_t, errs2 := p.typeCheckExpressionInModule(expected, expr.FalseBranch, functional)
		errors = append(errors, errs2...)
		// Join result types
		res_t = ast.LeastUpperBound(res_t, rhs_t)
	} else if functional {
		return nil, append(errors, *p.srcmap.SyntaxError(expr, "false branch required in functional context"))
	}
	// sanity check
	if len(errors) > 0 {
		return nil, errors
	}
	// success
	return res_t, nil
}

func (p *typeChecker) typeCheckInvokeInModule(expected ast.Type, expr *ast.Invoke,
	functional bool) (ast.Type, []SyntaxError) {
	var (
		ret    ast.Type
		errors []SyntaxError
	)
	//
	if binding, ok := expr.Name.Binding().(ast.FunctionBinding); ok {
		// Sanity check this is not an invocation on a native definition (which,
		// currently, do not have signatures).
		if sig := binding.Signature(); sig != nil {
			//
			for i := uint(0); i != sig.Arity(); i++ {
				_, errs := p.typeCheckExpressionInModule(sig.Parameter(i), expr.Args[i], functional)
				errors = append(errors, errs...)
			}
			// Check whether return type given (or not).
			if len(errors) > 0 {
				return nil, errors
			} else if ret = sig.Return(); ret != nil {
				return ret, errors
			}
			// TODO: this is potentially expensive, and it would likely be good if we
			// could avoid it.
			body := sig.Apply(expr.Args, p.srcmap)
			// Dig out the type
			ret, errors = p.typeCheckExpressionInModule(nil, body, functional)
			//
			if len(errors) > 0 {
				return nil, errors
			} else if expected != nil && ret != nil && !ret.SubtypeOf(expected) {
				msg := fmt.Sprintf("expected %s, found %s", expected.String(), ret.String())
				return nil, p.srcmap.SyntaxErrors(expr, msg)
			}
			//
			return ret, nil
		}
	}
	// No need to report an error here, as one would already have been reported
	// during resolution.
	return nil, nil
}

func (p *typeChecker) typeCheckVariableInModule(expr *ast.VariableAccess) (ast.Type, []SyntaxError) {
	// Check what we've got.
	if !expr.IsResolved() {
		//
	} else if binding, ok := expr.Binding().(*ast.ColumnBinding); ok {
		return binding.DataType, nil
	} else if binding, ok := expr.Binding().(*ast.ConstantBinding); ok {
		// Constant
		return p.typeCheckExpressionInModule(binding.DataType, binding.Value, true)
	} else if binding, ok := expr.Binding().(*ast.LocalVariableBinding); ok {
		// Parameter, for or let variable
		return binding.DataType, nil
	}
	// NOTE: we don't return an error here, since this case would have already
	// been caught by the resolver and we don't want to double up on errors.
	return nil, nil
}

// Calculate the actual return type for a given set of input values with the
// given types.
func typeOfSum(types ...ast.Type) ast.Type {
	var values math.Interval
	//
	for i, t := range types {
		if t == ast.UINT_TYPE {
			return t
		}
		//
		it := t.(*ast.IntType)
		vals := it.Values()
		//
		if i == 0 {
			values.Set(vals)
		} else {
			values.Add(vals)
		}
	}
	//
	return ast.NewIntType(values)
}

// Calculate the actual return type for a given set of input values with the
// given types.
func typeOfSubtraction(types ...ast.Type) ast.Type {
	var values math.Interval
	//
	for i, t := range types {
		if t == ast.UINT_TYPE {
			return t
		}
		//
		it := t.(*ast.IntType)
		vals := it.Values()
		//
		if i == 0 {
			values.Set(vals)
		} else {
			values.Sub(vals)
		}
	}
	//
	return ast.NewIntType(values)
}

// Calculate the actual return type for a given set of input values with the
// given types.
func typeOfProduct(types ...ast.Type) ast.Type {
	var values math.Interval
	//
	for i, t := range types {
		if t == ast.UINT_TYPE {
			return t
		}
		//
		it := t.(*ast.IntType)
		vals := it.Values()
		//
		if i == 0 {
			values.Set(vals)
		} else {
			values.Mul(vals)
		}
	}
	//
	return ast.NewIntType(values)
}

func typeOfConcat(types ...ast.Type) ast.Type {
	var (
		width uint
		zero  big.Int
		one   = big.NewInt(1)
	)
	//
	for _, t := range types {
		it, ok := t.(*ast.IntType)
		// sanity check what we've got
		if !ok || !t.HasUnderlying() {
			// Unknown
			return ast.UINT_TYPE
		}
		// append bitwidth
		width += it.BitWidth()
	}
	//
	var bound = big.NewInt(2)
	// Determine bound for static type check
	bound.Exp(bound, big.NewInt(int64(width)), nil)
	// Subtract 1 because interval is inclusive.
	bound.Sub(bound, one)
	//
	return ast.NewIntType(math.NewInterval(zero, *bound))
}
