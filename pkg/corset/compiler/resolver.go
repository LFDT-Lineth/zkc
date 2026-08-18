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
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// DeclPredicate is a shorthand notation.
type DeclPredicate = array.Predicate[ast.Declaration]

// ResolveCircuit resolves all symbols declared and used within a circuit,
// producing an environment which can subsequently be used to look up the
// relevant module or column identifiers.  This process can fail, of course, if
// a symbol (e.g. a column) is referred to which doesn't exist.  Likewise, if
// two modules or columns with identical names are declared in the same scope,
// etc.
func ResolveCircuit(srcmap *source.Maps[ast.Node], circuit *ast.Circuit) (*ModuleScope, []SyntaxError) {
	// Construct top-level scope
	scope := NewModuleScope(true)
	// Register modules
	for _, m := range circuit.Modules {
		scope.Declare(m.Name, util.None[string](), true)
	}
	// Construct resolver
	r := resolver{srcmap}
	// Initialise all columns
	if errs := r.initialiseDeclarations(scope, circuit); len(errs) > 0 {
		return nil, errs
	}
	// Finalise all columns / declarations
	if errs := r.resolveDeclarations(scope, circuit); len(errs) > 0 {
		return nil, errs
	}
	// Done
	return scope, nil
}

// Resolver packages up information necessary for resolving a circuit and
// checking that everything makes sense.
type resolver struct {
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
}

// Initialise all columns from their declaring constructs.
func (r *resolver) initialiseDeclarations(scope *ModuleScope, circuit *ast.Circuit) []SyntaxError {
	// Input columns must be allocated before assignemts, since the MIR schema
	// separates these out.
	errs := r.initialiseDeclarationsInModule(scope, circuit.Declarations)
	//
	for _, m := range circuit.Modules {
		// Process all declarations in the module
		merrs := r.initialiseDeclarationsInModule(scope.Enter(m.Name), m.Declarations)
		// Package up all errors
		errs = append(errs, merrs...)
	}
	//
	return errs
}

// Initialise all declarations in the given module scope.  That means allocating
// all bindings into the scope, whilst also ensuring that we never have two
// bindings for the same symbol, etc.  The key is that, at this stage, all
// bindings are potentially "non-finalised".  That means they may be missing key
// information which is yet to be determined (e.g. information about types, or
// contexts, etc).
func (r *resolver) initialiseDeclarationsInModule(scope *ModuleScope, decls []ast.Declaration) []SyntaxError {
	errors := make([]SyntaxError, 0)
	// Initialise all symbol (e.g. column) definitions.
	for _, d := range decls {
		for iter := d.Definitions(); iter.HasNext(); {
			def := iter.Next()
			// Attempt to declare symbol
			if !scope.Define(def) {
				msg := fmt.Sprintf("symbol %s already declared", def.Path())
				err := r.srcmap.SyntaxError(def, msg)
				errors = append(errors, *err)
			}
		}
	}
	//
	return errors
}

// Process all assignment, constraint and other declarations.  These are more
// complex than for input columns, since there can be dependencies between them.
// Thus, we cannot simply resolve them in one linear scan.
func (r *resolver) resolveDeclarations(scope *ModuleScope, circuit *ast.Circuit) []SyntaxError {
	state := NewGlobalResolution(circuit, *r.srcmap)
	// Continue iterating until nothing more can be done.  That way, we generate
	// the maximum possible number of error messages to report.
	for state.Continue() {
		// Marked start of a new iteration
		state.BeginIteration()
		// Finalise root module first.
		r.finaliseDeclarationsInModule(scope, circuit.Declarations, state.Enter(0))
		// Finalise nested modules
		for i, m := range circuit.Modules {
			// Process all declarations in the module
			r.finaliseDeclarationsInModule(scope.Enter(m.Name), m.Declarations, state.Enter(i+1))
		}
	}
	// Return any errors arising
	return state.Errors()
}

// Finalise a subset of declarations in a given module.  This requires an
// iterative process as we cannot finalise an arbitrary declaration until all of
// its dependencies have been themselves finalised.  For example, a function
// which depends upon another, not-yet-finalised function.  Until that function
// is finalised, its type won't be available and, hence, we cannot type the
// dependent function.
func (r *resolver) finaliseDeclarationsInModule(scope *ModuleScope, decls []ast.Declaration, state ModuleResolution) {
	for i, decl := range decls {
		// Check whether included and already finalised
		if !state.AlreadyFailed(i) && !decl.IsFinalised() {
			// No, so attempt to finalise
			ready, errs := r.declarationDependenciesAreFinalised(scope, decl)
			// Check what we found
			if ready && len(errs) == 0 {
				// Finalise declaration and handle errors
				errs = r.finaliseDeclaration(scope, decl)
				// Record that a new assignment is available.
				if len(errs) == 0 {
					// Mark this declaration as completed
					state.Completed(i)
				}
			}
			// If any errors arising, mark this declaration has having failed.
			if errs != nil {
				state.Failed(i, errs)
			}
		}
	}
}

// Check that a given set of symbols have been finalised.  This is important,
// since we cannot finalise a declaration until all of its dependencies have
// themselves been finalised.
func (r *resolver) declarationDependenciesAreFinalised(scope *ModuleScope,
	decl ast.Declaration) (bool, []SyntaxError) {
	var (
		errors    []SyntaxError
		finalised bool = true
	)
	//
	for iter := decl.Dependencies(); iter.HasNext(); {
		symbol := iter.Next()
		// Attempt to resolve
		if !symbol.IsResolved() && !scope.Bind(symbol) {
			errors = append(errors, *r.srcmap.SyntaxError(symbol, "unknown symbol"))
			// not finalised yet
			finalised = false
		} else {
			// Check whether this declaration defines this symbol (because if it
			// does, we cannot expect it to be finalised yet :)
			selfdefinition := decl.Defines(symbol)
			// Check whether this symbol is already finalised.
			symbol_finalised := symbol.Binding().IsFinalised()
			// Final check
			if !selfdefinition && !symbol_finalised {
				// Ok, not ready for finalisation yet.
				finalised = false
			}
		}
	}
	//
	return finalised, errors
}

// Finalise a declaration.
func (r *resolver) finaliseDeclaration(scope *ModuleScope, decl ast.Declaration) []SyntaxError {
	switch d := decl.(type) {
	case *ast.DefConst:
		return r.finaliseDefConstInModule(scope, d)
	case *ast.DefConstraint:
		return r.finaliseDefConstraintInModule(scope, d)
	case *ast.DefInRange:
		return r.finaliseDefInRangeInModule(scope, d)
	case *ast.DefLookup:
		return r.finaliseDefLookupInModule(scope, d)
	}
	//
	return nil
}

// Finalise one or more constant definitions within a given module.
// Specifically, we need to check that the constant values provided are indeed
// constants.
func (r *resolver) finaliseDefConstInModule(enclosing Scope, decl *ast.DefConst) []SyntaxError {
	var errors []SyntaxError
	//
	for _, c := range decl.Constants {
		scope := NewLocalScope(enclosing, false, true)
		// Resolve constant body
		errs := r.finaliseExpressionInModule(scope, c.ConstBinding.Value)
		// Accumulate errors
		errors = append(errors, errs...)
		//
		if len(errs) == 0 {
			// Check it is indeed constant!
			if constant := c.ConstBinding.Value.AsConstant(); constant != nil {
				datatype := c.ConstBinding.DataType
				result := ast.NewIntType(util_math.NewInterval(*constant, *constant))
				// Sanity check explicit type (if given)
				if datatype != nil && !result.SubtypeOf(datatype) {
					// error, constant value outside bounds of given type!
					errors = append(errors, *r.srcmap.SyntaxError(c, "constant out-of-bounds"))
					continue
				}
				// Finalise constant binding.  Note, no need to register a syntax
				// error for the error case, because it would have already been
				// accounted for during resolution.
				c.ConstBinding.Finalise()
			}
		}
	}
	//
	return errors
}

// Finalise a vanishing constraint declaration after all symbols have been
// resolved. This involves: (a) checking the context is valid; (b) checking the
// expressions are well-typed.
func (r *resolver) finaliseDefConstraintInModule(enclosing *ModuleScope, decl *ast.DefConstraint) []SyntaxError {
	var guard_errors []SyntaxError
	// Construct scope in which to resolve constraint
	scope := NewLocalScope(enclosing, false, false)
	// Resolve guard
	if decl.Guard != nil {
		guard_errors = r.finaliseExpressionInModule(scope, decl.Guard)
	}
	// Resolve constraint body
	constraint_errors := r.finaliseExpressionInModule(scope, decl.Constraint)
	//
	if len(guard_errors) == 0 && len(constraint_errors) == 0 {
		// Finalise declaration.
		decl.Finalise()
	}
	// Done
	return append(guard_errors, constraint_errors...)
}

// Finalise a range constraint declaration after all symbols have been
// resolved.  As for a lookup, this requires the constrained column resolves to
// an actual column (rather than, for example, a constant).
func (r *resolver) finaliseDefInRangeInModule(enclosing Scope, decl *ast.DefInRange) []SyntaxError {
	var scope = NewLocalScope(enclosing, true, false)
	// Resolve constrained column
	errors := r.finaliseColumnAccessInModule(scope, decl.Column)
	// Error check
	if len(errors) == 0 {
		decl.Finalise()
	}
	// Done
	return errors
}

// Resolve the columns accessed by this lookup constraint.  Observe that each
// source (and, likewise, each target) is resolved in its own scope, since each
// determines its own context.
func (r *resolver) finaliseDefLookupInModule(enclosing Scope, decl *ast.DefLookup) []SyntaxError {
	var errors []SyntaxError
	// Resolve source columns
	for i := range decl.Sources {
		errs := r.finaliseLookupColumnsInModule(enclosing, decl.SourceSelectors[i], decl.Sources[i])
		errors = append(errors, errs...)
	}
	// Resolve all target columns
	for i := range decl.Targets {
		errs := r.finaliseLookupColumnsInModule(enclosing, decl.TargetSelectors[i], decl.Targets[i])
		errors = append(errors, errs...)
	}
	//
	return errors
}

// Resolve one side of a lookup constraint (i.e. a set of source or target
// columns, along with their optional selector) within a single scope.  Sharing
// the scope ensures the columns (and selector) all reside in the same context.
func (r *resolver) finaliseLookupColumnsInModule(enclosing Scope, selector ast.TypedSymbol,
	columns []ast.TypedSymbol) []SyntaxError {
	//
	var (
		errors []SyntaxError
		scope  = NewLocalScope(enclosing, true, false)
	)
	// Resolve each column in turn
	for _, column := range columns {
		if column != nil {
			errors = append(errors, r.finaliseColumnAccessInModule(scope, column)...)
		}
	}
	// Resolve selector (when present).  NOTE: the selector is resolved last so
	// that, when its context conflicts with that of the columns it gates, the
	// selector is the access reported as conflicting.
	if selector != nil {
		errors = append(errors, r.finaliseColumnAccessInModule(scope, selector)...)
	}
	//
	return errors
}

// Resolve a single column access arising within a lookup or range constraint.
// Unlike an arbitrary expression, this must resolve to a column (e.g. it cannot
// be a constant or a function parameter).
func (r *resolver) finaliseColumnAccessInModule(scope LocalScope, symbol ast.TypedSymbol) []SyntaxError {
	var errors []SyntaxError
	// Resolve the underlying access
	switch s := symbol.(type) {
	case *ast.ArrayAccess:
		errors = r.finaliseArrayAccessInModule(scope, s)
	case *ast.VariableAccess:
		errors = r.finaliseVariableInModule(scope, s)
	default:
		return r.srcmap.SyntaxErrors(symbol, "invalid column access")
	}
	// Sanity check we ended up with a column, since nothing else can be looked
	// up (e.g. a constant cannot).
	if len(errors) == 0 {
		if _, ok := symbol.Binding().(*ast.ColumnBinding); !ok {
			return r.srcmap.SyntaxErrors(symbol, "expected column")
		}
	}
	//
	return errors
}

// Resolve a sequence of zero or more expressions within a given module.  This
// simply resolves each of the arguments in turn, collecting any errors arising.
func (r *resolver) finaliseExpressionsInModule(scope LocalScope, args []ast.Expr) []SyntaxError {
	var errors []SyntaxError
	// Visit each argument
	for _, arg := range args {
		if arg != nil {
			errs := r.finaliseExpressionInModule(scope, arg)
			errors = append(errors, errs...)
		}
	}
	// Done
	return errors
}

// Resolve any variable accesses with this expression (which is declared in a
// given module).  The enclosing module is required to resolve unqualified
// variable accesses.  As above, the goal is ensure variable refers to something
// that was declared and, more specifically, what kind of access it is (e.g.
// column access, constant access, etc).
//
//nolint:staticcheck
func (r *resolver) finaliseExpressionInModule(scope LocalScope, expr ast.Expr) []SyntaxError {
	switch v := expr.(type) {
	case *ast.ArrayAccess:
		return r.finaliseArrayAccessInModule(scope, v)
	case *ast.Add:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Connective:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Constant:
		return nil
	case *ast.Equation:
		lhs_errs := r.finaliseExpressionInModule(scope, v.Lhs)
		rhs_errs := r.finaliseExpressionInModule(scope, v.Rhs)
		// combine errors
		return append(lhs_errs, rhs_errs...)
	case *ast.If:
		return r.finaliseExpressionsInModule(scope, []ast.Expr{v.Condition, v.TrueBranch, v.FalseBranch})
	case *ast.Mul:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Not:
		return r.finaliseExpressionInModule(scope, v.Arg)
	case *ast.Shift:
		constscope := scope.NestedConstScope()
		arg_errs := r.finaliseExpressionInModule(scope, v.Arg)
		shf_errs := r.finaliseExpressionInModule(constscope, v.Shift)
		// combine errors
		return append(arg_errs, shf_errs...)
	case *ast.Sub:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.VariableAccess:
		return r.finaliseVariableInModule(scope, v)
	default:
		typeStr := reflect.TypeOf(expr).String()
		msg := fmt.Sprintf("unknown expression encountered during resolution (%s)", typeStr)

		return r.srcmap.SyntaxErrors(expr, msg)
	}
}

// Resolve a specific array access contained within some expression which, in
// turn, is contained within some module.
func (r *resolver) finaliseArrayAccessInModule(scope LocalScope, expr *ast.ArrayAccess) []SyntaxError {
	// Resolve argument
	errors := r.finaliseExpressionInModule(scope, expr.Arg)
	//
	if !expr.IsResolved() && !scope.Bind(expr) {
		errors = append(errors, *r.srcmap.SyntaxError(expr, "unknown array column"))
	} else if binding, ok := expr.Binding().(*ast.ColumnBinding); !ok {
		errors = append(errors, *r.srcmap.SyntaxError(expr, "unknown array column"))
	} else if !scope.FixContext(binding.Context()) {
		return r.srcmap.SyntaxErrors(expr, "conflicting context")
	}
	// All good
	return errors
}

// Resolve a specific variable access contained within some expression which, in
// turn, is contained within some module.  Note, qualified accesses are only
// permitted in a global context.
func (r *resolver) finaliseVariableInModule(scope LocalScope, expr *ast.VariableAccess) []SyntaxError {
	// Check whether this is a qualified access, or not.
	if !scope.IsGlobal() && !scope.IsWithin(*expr.Path()) {
		return r.srcmap.SyntaxErrors(expr, "qualified access not permitted here")
	} else if !scope.IsVisible(expr) {
		return r.srcmap.SyntaxErrors(expr, "recursion not permitted here")
	}
	// Symbol should be resolved at this point, but we'd better sanity check this.
	if !expr.IsResolved() && !scope.Bind(expr) {
		// Unable to resolve variable
		return r.srcmap.SyntaxErrors(expr, "unresolved symbol")
	}
	// Check what we've got.
	if binding, ok := expr.Binding().(*ast.ColumnBinding); ok {
		// For column bindings, we still need to sanity check the context is
		// compatible.
		if !scope.FixContext(binding.Context()) {
			return r.srcmap.SyntaxErrors(expr, "conflicting context")
		} else if scope.IsPure() {
			return r.srcmap.SyntaxErrors(expr, "not permitted in pure context")
		}
		//
		return nil
	} else if _, ok := expr.Binding().(*ast.ConstantBinding); ok {
		// Constant
		return nil
	}
	// Should be unreachable.
	return r.srcmap.SyntaxErrors(expr, "unknown symbol kind")
}

// GlobalResolution maintains detailed state about the ongoing attempt to
// resolve all declarations in a given circuit.
type GlobalResolution struct {
	// Stash of declarations for error reporting purposes
	decls [][]ast.Declaration
	// Source map for error reporting
	srcmap source.Maps[ast.Node]
	// Failed indicates which declarations for each module have failed (if any).
	// The purpose of this is to prevent attempts to refinalise a declaration,
	// as this then leads to (potentially many) duplicate error messages.
	failed [][]bool
	// Completed indicates which declarations for each module have completed
	// successfully.  The purpose of this is, in the event of a resolution
	// failure, to be able to find examples to report errors on.
	completed [][]bool
	// Counts declarations remaining to be completed.  The purpose of this is to
	// make it easy to tell when resolution is finished.
	uncompleted uint
	// Changed indicates whether or not any new declarations changed state (i.e.
	// went from unresolved to resolved) within current iteration.
	changed bool
	// Number of iterations remaining before we give up.
	count uint
	// Accumulate errors
	errors []SyntaxError
}

// NewGlobalResolution simply initialises an appropriate state object for the
// given circuit.
func NewGlobalResolution(circuit *ast.Circuit, srcmap source.Maps[ast.Node]) GlobalResolution {
	var (
		n = len(circuit.Modules) + 1
		// Construct initial state
		state = GlobalResolution{make([][]ast.Declaration, n), srcmap,
			make([][]bool, n), make([][]bool, n),
			0, true, 32, nil,
		}
	)
	// Initialise root module
	state.initialise(0, circuit.Declarations)
	// Initialise submodules
	for i, m := range circuit.Modules {
		state.initialise(i+1, m.Declarations)
	}
	// Initialise other modules
	return state
}

// BeginIteration signals that a new iteration is beginning.
func (p *GlobalResolution) BeginIteration() {
	p.changed = false
	p.count--
}

// Continue determines whether or not to continue onto another iteration.
func (p *GlobalResolution) Continue() bool {
	if p.changed && p.count == 0 {
		// Determine appropriate error
		p.giveUp()
	} else if !p.changed && p.uncompleted > 0 {
		// Resolution didn't finish for some reason.  This should not happened
		// in practice but, in reality, it can do.  For example, if there is a
		// bug in the resolution process somewhere (which might e.g. arise when
		// adding new declaration types).
		p.internalFailure()
	}
	//
	return p.changed && p.count > 0
}

// Errors simply returns any error messages arising.
func (p *GlobalResolution) Errors() []SyntaxError {
	return p.errors
}

// Enter returns the state for a given module.
func (p *GlobalResolution) Enter(index int) ModuleResolution {
	return ModuleResolution{index, p}
}

// GiveUp means we should not attempt any more iterations, as it seems like
// resolution is stuck in an infinite loop.  In theory, such infinite loops
// should not happen.  The goal of this is to ensure (in the unlikely event they
// do happen) a graceful failure.
func (p *GlobalResolution) giveUp() {
	if len(p.errors) == 0 {
		for i, cs := range p.completed {
			for j, completed := range cs {
				if !completed {
					err := p.srcmap.SyntaxError(p.decls[i][j], "unable to complete resolution")
					p.errors = append(p.errors, *err)

					return
				}
			}
		}
	}
}

// InternalFailure arises when we stop making progress towards completing
// resolution.  This should not happen in practice, but it could arise if there
// is a bug somewhere in the resolution mechanism.  For example, when adding new
// declaration types.  The goal is to report some kind error message, rather
// than just nothing.
func (p *GlobalResolution) internalFailure() {
	if len(p.errors) == 0 {
		for i, cs := range p.completed {
			for j, completed := range cs {
				decl := p.decls[i][j]
				//
				if !completed {
					for iter := decl.Dependencies(); iter.HasNext(); {
						symbol := iter.Next()
						// Check whether this dependency is a problem
						if symbol.Binding() != nil && !symbol.Binding().IsFinalised() {
							// Yes, so report error
							err := p.srcmap.SyntaxError(symbol, "unresolvable symbol")
							p.errors = append(p.errors, *err)
						}
					}
				}
			}
		}
	}
}

func (p *GlobalResolution) initialise(index int, decls []ast.Declaration) {
	p.decls[index] = decls
	p.completed[index] = make([]bool, len(decls))
	p.failed[index] = make([]bool, len(decls))
	//
	for i, d := range decls {
		if d.IsFinalised() {
			p.completed[index][i] = true
		} else {
			p.uncompleted++
		}
	}
}

// ModuleResolution provides a handy interface for resolving declarations within
// a given module.  It is really just a wrapper around the global resolution
// state.
type ModuleResolution struct {
	index int
	state *GlobalResolution
}

// AlreadyFailed can be used to determine whether a given declaration within the
// module already failed in a previous iteration.  This is useful to prevent
// reattempts to resolve the declaration (which would lead to duplicate errors,
// etc).
func (p *ModuleResolution) AlreadyFailed(decl int) bool {
	return p.state.failed[p.index][decl]
}

// Completed indicates a given declaration within the module has been resolved.
func (p *ModuleResolution) Completed(decl int) {
	p.state.completed[p.index][decl] = true
	p.state.uncompleted--
	p.state.changed = true
}

// Failed indicates a given declaration within the module has failed resolution
// and generated one or more errors.
func (p *ModuleResolution) Failed(decl int, errs []SyntaxError) {
	p.state.failed[p.index][decl] = true
	p.state.errors = append(p.state.errors, errs...)
}
