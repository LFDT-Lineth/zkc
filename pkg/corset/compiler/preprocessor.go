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
	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// PreprocessCircuit performs preprocessing prior to final translation.
// Specifically, it expands all invocations and reductions.  Thus,
// final translation is greatly simplified after this step.
func PreprocessCircuit(debug bool, srcmap *source.Maps[ast.Node],
	circuit *ast.Circuit) []SyntaxError {
	// Construct fresh preprocessor
	p := preprocessor{debug, srcmap}
	// Preprocess all declarations
	return p.preprocessDeclarations(circuit)
}

// Preprocessor performs preprocessing prior to final translation. Specifically,
// it expands all invocations and reductions.  Thus, final
// translation is greatly simplified after this step.
type preprocessor struct {
	// Debug enables the use of debug constraints.
	debug bool
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
}

// preprocess all assignment or constraint declarations in the circuit.
func (p *preprocessor) preprocessDeclarations(circuit *ast.Circuit) []SyntaxError {
	errors := p.preprocessDeclarationsInModule(circuit.Declarations)
	// preprocess each module
	for _, m := range circuit.Modules {
		errs := p.preprocessDeclarationsInModule(m.Declarations)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// preprocess all assignment or constraint declarations in a given module within
// the circuit.
func (p *preprocessor) preprocessDeclarationsInModule(decls []ast.Declaration) []SyntaxError {
	var errors []SyntaxError
	//
	for _, d := range decls {
		errs := p.preprocessDeclaration(d)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// preprocess an assignment or constraint declaration which occurs within a
// given module.
func (p *preprocessor) preprocessDeclaration(decl ast.Declaration) []SyntaxError {
	var errors []SyntaxError
	//
	switch d := decl.(type) {
	case *ast.DefAliases:
		// ignore
	case *ast.DefColumns:
		// ignore
	case *ast.DefConst:
		// ignore
	case *ast.DefConstraint:
		errors = p.preprocessDefConstraint(d)
	case *ast.DefFun:
		errors = p.preprocessDefFun(d)
	case *ast.DefInRange:
		errors = p.preprocessDefInRange(d)
	case *ast.DefLookup:
		// Sources and targets are column accesses, hence there is nothing to
		// preprocess (e.g. no invocations or reductions to expand).
	case *ast.DefPerspective:
		errors = p.preprocessDefPerspective(d)
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// preprocess a "defconstraint" declaration.
func (p *preprocessor) preprocessDefConstraint(decl *ast.DefConstraint) []SyntaxError {
	var (
		constraint_errors []SyntaxError
		guard_errors      []SyntaxError
	)
	// preprocess constraint body
	decl.Constraint, constraint_errors = p.preprocessExpressionInModule(decl.Constraint)
	// preprocess (optional) guard
	decl.Guard, guard_errors = p.preprocessOptionalExpressionInModule(decl.Guard)
	// NOTE: decl.Constraint can be nil at this point.  This case is possible
	// when the constraint expression consists entirely of debug constraints,
	// and debug mode is not enabled.  Translation simply ignores such
	// constraints.
	// Combine errors
	return append(constraint_errors, guard_errors...)
}

// preprocess a "deflookup" declaration.
//
//nolint:staticcheck
func (p *preprocessor) preprocessDefFun(decl *ast.DefFun) []SyntaxError {
	var errors []SyntaxError
	//
	binding := decl.Binding().(*ast.DefunBinding)
	// preprocess function body
	binding.Body, errors = p.preprocessExpressionInModule(binding.Body)
	// Combine errors
	return errors
}

// preprocess a "definrange" declaration.
func (p *preprocessor) preprocessDefInRange(decl *ast.DefInRange) []SyntaxError {
	var errors []SyntaxError
	// preprocess constraint body
	decl.Expr, errors = p.preprocessExpressionInModule(decl.Expr)
	// Done
	return errors
}

// preprocess a "defperspective" declaration.
func (p *preprocessor) preprocessDefPerspective(decl *ast.DefPerspective) []SyntaxError {
	var errors []SyntaxError
	// preprocess selector expression
	decl.Selector, errors = p.preprocessExpressionInModule(decl.Selector)
	// Combine errors
	return errors
}

// preprocess an optional expression in a given context.  That is an expression
// which maybe nil (i.e. doesn't exist).  In such case, nil is returned (i.e.
// without any errors).
func (p *preprocessor) preprocessOptionalExpressionInModule(expr ast.Expr) (ast.Expr, []SyntaxError) {
	//
	if expr != nil {
		return p.preprocessExpressionInModule(expr)
	}

	return nil, nil
}

// preprocess a sequence of zero or more expressions enclosed in a given module.
// All expressions are expected to be non-voidable (see below for more on
// voidability).
func (p *preprocessor) preprocessExpressionsInModule(exprs []ast.Expr) ([]ast.Expr, []SyntaxError) {
	//
	errors := []SyntaxError{}
	nexprs := make([]ast.Expr, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		if e != nil {
			var errs []SyntaxError
			//
			nexprs[i], errs = p.preprocessExpressionInModule(e)
			errors = append(errors, errs...)
			// Check for non-voidability
			if nexprs[i] == nil {
				errors = append(errors, *p.srcmap.SyntaxError(e, "void expression not permitted here"))
			}
		}
	}
	//
	return nexprs, errors
}

// preprocess an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for column access, function
// invocations, etc).
func (p *preprocessor) preprocessExpressionInModule(expr ast.Expr) (ast.Expr, []SyntaxError) {
	var (
		nexpr  ast.Expr
		errors []SyntaxError
	)
	//
	switch e := expr.(type) {
	case *ast.ArrayAccess:
		arg, errs := p.preprocessExpressionInModule(e.Arg)
		nexpr, errors = &ast.ArrayAccess{Name: e.Name, Arg: arg, ArrayBinding: e.ArrayBinding}, errs
	case *ast.Add:
		args, errs := p.preprocessExpressionsInModule(e.Args)
		nexpr, errors = &ast.Add{Args: args}, errs
	case *ast.Cast:
		arg, errs := p.preprocessExpressionInModule(e.Arg)
		nexpr, errors = &ast.Cast{Arg: arg, Type: e.Type, Unsafe: e.Unsafe}, errs
	case *ast.Connective:
		args, errs := p.preprocessExpressionsInModule(e.Args)
		nexpr, errors = &ast.Connective{Sign: e.Sign, Args: args}, errs
	case *ast.Constant:
		return e, nil
	case *ast.Debug:
		if p.debug {
			return p.preprocessExpressionInModule(e.Arg)
		}
		// When debug is not enabled, return "void".
		return nil, nil
	case *ast.Equation:
		lhs, errs1 := p.preprocessExpressionInModule(e.Lhs)
		rhs, errs2 := p.preprocessExpressionInModule(e.Rhs)
		// Done
		nexpr, errors = &ast.Equation{Kind: e.Kind, Lhs: lhs, Rhs: rhs}, append(errs1, errs2...)
	case *ast.Exp:
		arg, errs1 := p.preprocessExpressionInModule(e.Arg)
		pow, errs2 := p.preprocessExpressionInModule(e.Pow)
		// Done
		nexpr, errors = &ast.Exp{Arg: arg, Pow: pow}, append(errs1, errs2...)
	case *ast.If:
		cond, errs1 := p.preprocessExpressionInModule(e.Condition)
		args, errs2 := p.preprocessExpressionsInModule([]ast.Expr{e.TrueBranch, e.FalseBranch})
		// Construct appropriate if form
		nexpr, errors = &ast.If{Condition: cond, TrueBranch: args[0], FalseBranch: args[1]}, append(errs1, errs2...)
	case *ast.Invoke:
		return p.preprocessInvokeInModule(e)
	case *ast.Mul:
		args, errs := p.preprocessExpressionsInModule(e.Args)
		nexpr, errors = &ast.Mul{Args: args}, errs
	case *ast.Normalise:
		arg, errs := p.preprocessExpressionInModule(e.Arg)
		nexpr, errors = &ast.Normalise{Arg: arg}, errs
	case *ast.Not:
		arg, errs := p.preprocessExpressionInModule(e.Arg)
		nexpr, errors = &ast.Not{Arg: arg}, errs
	case *ast.Sub:
		args, errs := p.preprocessExpressionsInModule(e.Args)
		nexpr, errors = &ast.Sub{Args: args}, errs
	case *ast.Shift:
		arg, errs1 := p.preprocessExpressionInModule(e.Arg)
		shift, errs2 := p.preprocessExpressionInModule(e.Shift)
		// Done
		nexpr, errors = &ast.Shift{Arg: arg, Shift: shift}, append(errs1, errs2...)
	case *ast.VariableAccess:
		return e, nil
	case *ast.Concat:
		args, errs := p.preprocessExpressionsInModule(e.Args)
		nexpr, errors = &ast.Concat{Args: args}, errs
	default:
		return nil, p.srcmap.SyntaxErrors(expr, "unknown expression encountered during preprocessing")
	}
	// Copy over source information
	p.srcmap.Copy(expr, nexpr)
	// Done
	return nexpr, errors
}

func (p *preprocessor) preprocessInvokeInModule(expr *ast.Invoke) (ast.Expr, []SyntaxError) {
	if binding, ok := expr.Name.Binding().(ast.FunctionBinding); ok {
		var (
			args   []ast.Expr = make([]ast.Expr, len(expr.Args))
			errors []SyntaxError
			errs   []SyntaxError
		)
		// Preprocess arguments prior to subsitution.
		for i, e := range expr.Args {
			args[i], errs = p.preprocessExpressionInModule(e)
			errors = append(errors, errs...)
		}
		// Substitute through body
		body := binding.Signature().Apply(args, p.srcmap)
		// Preprocess body
		body, errs = p.preprocessExpressionInModule(body)
		// Done
		return body, append(errors, errs...)
	}
	//
	return nil, p.srcmap.SyntaxErrors(expr, "unbound function")
}
