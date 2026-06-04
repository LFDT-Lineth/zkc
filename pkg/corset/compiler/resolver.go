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
	"math"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
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
func ResolveCircuit[M schema.ModuleView](srcmap *source.Maps[ast.Node], circuit *ast.Circuit,
	externs ...M) (*ModuleScope, []SyntaxError) {
	// Construct top-level scope
	scope := NewModuleScope(true)
	// Define natives
	for _, i := range NATIVE_SIGNATURES {
		scope.Define(&i)
	}
	// Define intrinsics
	for _, i := range INTRINSICS {
		scope.Define(&i)
	}
	// Initialise externs
	DeclareExterns(scope, externs...)
	// Register modules
	for _, m := range circuit.Modules {
		scope.Declare(m.Name, extractSelector(nil), true)
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
	// Input columns must be allocated before assignemts, since the hir.Schema
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
	// First, initialise any perspectives as submodules of the given scope.  Its
	// slightly frustrating that we have to do this separately, but the
	// non-lexical nature of perspectives forces our hand.
	for _, d := range decls {
		if def, ok := d.(*ast.DefPerspective); ok {
			// Attempt to declare the perspective.  Note, we don't need to check
			// whether or not this succeeds here as, if it fails, this will be
			// caught below.
			scope.Declare(def.Name(), extractSelector(def.Selector), true)
		}
	}
	// Second, initialise all symbol (e.g. column) definitions.
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
	// Third, intialise aliases
	if errors := r.initialiseAliasesInModule(scope, decls); len(errors) > 0 {
		return errors
	}
	//
	return errors
}

// This is really broken.  The problem is that we need to translate the selector
// expression within the translator.  But, setting that all up is not
// straightforward.  This should be done in the future!
func extractSelector(selector ast.Expr) util.Option[string] {
	if selector == nil {
		return util.None[string]()
	}
	//
	if e, ok := selector.(*ast.VariableAccess); ok && e.Name.Depth() == 1 {
		return util.Some(e.Name.Get(0))
	}
	// FIXME: #630
	panic("unsupported selector")
}

// Initialise all alias declarations in the given module scope.  This means
// declaring them within the module scope, whilst also supporting aliases of
// aliases, etc.  Since the order of aliases is unspecified, this means we have
// to iterate the alias declarations until a fixed point is reached.  Once that
// is done, if there are any aliases left unallocated then they indicate errors.
func (r *resolver) initialiseAliasesInModule(scope *ModuleScope, decls []ast.Declaration) []SyntaxError {
	// Apply any aliases
	errors := make([]SyntaxError, 0)
	visited := make(map[string]ast.Declaration)
	changed := true
	// Iterate aliases to fixed point (i.e. until no new aliases discovered)
	for changed {
		changed = false
		// Look for all aliases
		for _, d := range decls {
			if a, ok := d.(*ast.DefAliases); ok {
				for i, alias := range a.Aliases {
					symbol := a.Symbols[i]
					if _, ok := visited[alias.Name]; !ok {
						// Attempt to make the alias
						if change := scope.Alias(alias.Name, symbol); change {
							visited[alias.Name] = d
							changed = true
						}
					}
				}
			}
		}
	}
	// Check for any aliases which remain incomplete
	for _, decl := range decls {
		if a, ok := decl.(*ast.DefAliases); ok {
			for i, alias := range a.Aliases {
				symbol := a.Symbols[i]
				// Check whether it already exists (or not)
				if d, ok := visited[alias.Name]; ok && d == decl {
					continue
				} else if scope.Binding(alias.Name, symbol.Arity()) != nil {
					err := r.srcmap.SyntaxError(alias, "symbol already exists")
					errors = append(errors, *err)
				} else {
					err := r.srcmap.SyntaxError(symbol, "unknown symbol")
					errors = append(errors, *err)
				}
			}
		}
	}
	// Done
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
// which depends upon an interleaved column.  Until the interleaved column is
// finalised, its type won't be available and, hence, we cannot type the
// function.
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
	// DefConstraints require special handling because they can be associated
	// with a perspective.  Perspectives are challenging here because they are
	// effectively non-lexical scopes, which is not a good fit for the module
	// tree structure used.
	if dc, ok := decl.(*ast.DefConstraint); ok && dc.Perspective != nil {
		if dc.Perspective.IsResolved() || scope.Bind(dc.Perspective) {
			// Temporarily enter the perspective for the purposes of resolving
			// symbols within this declaration.
			scope = scope.Enter(dc.Perspective.Name())
		}
	}
	//
	for iter := decl.Dependencies(); iter.HasNext(); {
		symbol := iter.Next()
		// Attempt to resolve
		if !symbol.IsResolved() && !scope.Bind(symbol) {
			// try to report more useful error
			errors = append(errors, r.constructUnknownSymbolError(symbol, scope))
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
	case *ast.DefCall:
		return r.finaliseDefCallInModule(scope, d)
	case *ast.DefComputed:
		return r.finaliseDefComputedInModule(d)
	case *ast.DefConst:
		return r.finaliseDefConstInModule(scope, d)
	case *ast.DefConstraint:
		return r.finaliseDefConstraintInModule(scope, d)
	case *ast.DefFun:
		return r.finaliseDefFunInModule(scope, d)
	case *ast.DefInRange:
		return r.finaliseDefInRangeInModule(scope, d)
	case *ast.DefInterleaved:
		return r.finaliseDefInterleavedInModule(scope, d)
	case *ast.DefLookup:
		return r.finaliseDefLookupInModule(scope, d)
	case *ast.DefPermutation:
		return r.finaliseDefPermutationInModule(scope, d)
	case *ast.DefPerspective:
		return r.finaliseDefPerspectiveInModule(scope, d)
	case *ast.DefProperty:
		return r.finaliseDefPropertyInModule(scope, d)
	case *ast.DefSorted:
		return r.finaliseDefSortedInModule(scope, d)
	case *ast.DefComputedColumn:
		return r.finaliseDefComputedColumnInModule(scope, d)
	}
	//
	return nil
}

func (r *resolver) finaliseDefCallInModule(enclosing Scope, decl *ast.DefCall) []SyntaxError {
	var (
		scope = NewLocalScope(enclosing, false, false, false)
	)
	// Resolve return expressions
	errs1 := r.finaliseExpressionsInModule(scope, decl.Returns)
	// Resolve argument expressions
	errs2 := r.finaliseExpressionsInModule(scope, decl.Arguments)
	// Combine errors
	errors := append(errs1, errs2...)
	// Resolve selector (if applicable)
	if decl.Selector.HasValue() {
		errs3 := r.finaliseExpressionInModule(scope, decl.Selector.Unwrap())
		errors = append(errors, errs3...)
	}
	//
	return errors
}

func (r *resolver) finaliseDefComputedInModule(decl *ast.DefComputed) []SyntaxError {
	var (
		errors    []SyntaxError
		arguments []NativeColumn = make([]NativeColumn, len(decl.Sources))
		binding   *NativeDefinition
	)
	// Initialise arguments
	for i := 0; i < len(decl.Sources); i++ {
		// FIXME: sanity check that these things make sense.
		ith := decl.Sources[i].Binding().(*ast.ColumnBinding)
		arguments[i] = NativeColumn{ith.DataType, ith.Multiplier}
	}
	// Extract binding
	binding = decl.Function.Binding().(*NativeDefinition)
	//
	if binding.arity != uint(len(arguments)) {
		msg := fmt.Sprintf("incorrect number of arguments (found %d)", len(arguments))
		errors = append(errors, *r.srcmap.SyntaxError(decl.Function, msg))
	} else {
		// Apply definition to determine geometry of assignment
		assignments := binding.Apply(arguments)
		//
		if len(assignments) > len(decl.Targets) {
			msg := fmt.Sprintf("not enough target columns (expected %d)", len(assignments))
			errors = append(errors, *r.srcmap.SyntaxError(decl.Function, msg))
		} else if len(assignments) < len(decl.Targets) {
			msg := fmt.Sprintf("too many target columns (expected %d)", len(assignments))
			errors = append(errors, *r.srcmap.SyntaxError(decl.Function, msg))
		} else {
			// Finalise each target column
			for i := 0; i < len(decl.Targets); i++ {
				// Finalise ith target column
				var (
					ith_multiplier = assignments[i].multiplier
					ith_datatype   = assignments[i].datatype
					binding        = decl.Targets[i].Binding().(*ast.ColumnBinding)
				)
				// Finalise (if not already)
				if !binding.IsFinalised() {
					// Finalise column binding
					binding.Finalise(ith_multiplier, ith_datatype)
				}
				// Check data type
				if !ith_datatype.SubtypeOf(binding.DataType) {
					msg := fmt.Sprintf("incompatible type (%s)", ith_datatype.String())
					errors = append(errors, *r.srcmap.SyntaxError(decl.Targets[i], msg))
				}
				// Check multiplier
				if ith_multiplier != binding.Multiplier {
					errors = append(errors, *r.srcmap.SyntaxError(decl.Targets[i], "invalid length multiplier"))
				}
			}
			// Finalise declaration
			decl.Finalise()
		}
	}
	// Done
	return errors
}

// Finalise one or more constant definitions within a given module.
// Specifically, we need to check that the constant values provided are indeed
// constants.
func (r *resolver) finaliseDefConstInModule(enclosing Scope, decl *ast.DefConst) []SyntaxError {
	var errors []SyntaxError
	//
	for _, c := range decl.Constants {
		scope := NewLocalScope(enclosing, false, true, true)
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
	// Identifiery enclosing perspective (if applicable)
	if decl.Perspective != nil {
		// As before, we must temporarily enter the perspective here.
		perspective := decl.Perspective.Name()
		enclosing = enclosing.Enter(perspective)
	}
	// Construct scope in which to resolve constraint
	scope := NewLocalScope(enclosing, false, false, false)
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

// Finalise a vanishing constraint declaration after all symbols have been
// resolved. This involves: (a) checking the context is valid; (b) checking the
// expressions are well-typed.
func (r *resolver) finaliseDefComputedColumnInModule(enclosing *ModuleScope,
	decl *ast.DefComputedColumn) []SyntaxError {
	// Open definition
	enclosing.OpenDefinition(decl.Target)
	// Construct scope in which to resolve constraint
	scope := NewLocalScope(enclosing, false, false, false)
	// Resolve computation body
	computation_errors := r.finaliseExpressionInModule(scope, decl.Computation)
	//
	if len(computation_errors) == 0 {
		decl.Finalise()
	}
	// Close definition
	enclosing.CloseDefinition(decl.Target)
	// Done
	return computation_errors
}

// Finalise an interleaving assignment.  Since the assignment would already been
// initialised, all we need to do is determine the appropriate type and length
// multiplier for the interleaved column.  This can still result in an error,
// for example, if the multipliers between interleaved columns are incompatible,
// etc.
func (r *resolver) finaliseDefInterleavedInModule(enclosing *ModuleScope, decl *ast.DefInterleaved) []SyntaxError {
	var (
		// Length multiplier being determined
		length_multiplier uint
		// Column type being determined
		datatype ast.Type
		// Errors discovered
		errors []SyntaxError
	)
	//
	enclosing.OpenDefinition(decl.Target)
	// Determine type and length multiplier
	for _, source := range decl.Sources {
		// Lookup binding of column being interleaved.
		if binding, ok := source.Binding().(*ast.ColumnBinding); !ok {
			// Columns to be interleaved must have the same length multiplier.
			err := r.srcmap.SyntaxError(source, "invalid source column")
			errors = append(errors, *err)
		} else if !enclosing.IsVisible(source) {
			errors = append(errors, *r.srcmap.SyntaxError(source, "recursive definition"))
		} else if datatype == nil {
			length_multiplier = binding.Multiplier
			datatype = source.Type()
		} else if binding.Multiplier != length_multiplier {
			// Columns to be interleaved must have the same length multiplier.
			err := r.srcmap.SyntaxError(source, "incompatible length multiplier")
			errors = append(errors, *err)
		} else {
			// Combine datatypes.
			datatype = ast.LeastUpperBound(datatype, source.Type())
		}
	}
	// Finalise details only if no errors
	if len(errors) == 0 {
		// Determine actual length multiplier
		length_multiplier *= uint(len(decl.Sources))
		// Lookup existing declaration
		binding := decl.Target.Binding().(*ast.ColumnBinding)
		// Finalise (if not already)
		if !binding.IsFinalised() {
			// Finalise column binding
			binding.Finalise(length_multiplier, datatype)
		}
		// Check data type
		if !datatype.SubtypeOf(binding.DataType) {
			msg := fmt.Sprintf("incompatible type (%s)", datatype.String())
			errors = append(errors, *r.srcmap.SyntaxError(decl.Target, msg))
		}
		// Check multiplier
		if length_multiplier != binding.Multiplier {
			errors = append(errors, *r.srcmap.SyntaxError(decl.Target, "invalid length multiplier"))
		}
		// Finalise declaration
		decl.Finalise()
	}
	//
	enclosing.CloseDefinition(decl.Target)
	// Done
	return errors
}

// Finalise a permutation assignment after all symbols have been resolved.  This
// requires checking the contexts of all columns is consistent.
func (r *resolver) finaliseDefPermutationInModule(enclosing *ModuleScope, decl *ast.DefPermutation) []SyntaxError {
	var (
		multiplier uint = 0
		errors     []SyntaxError
		started    bool
	)
	//
	openDefinitions(enclosing, decl.Targets...)
	// Finalise each column in turn
	for i := 0; i < len(decl.Sources); i++ {
		ith := decl.Sources[i]
		// Lookup source of column being permuted
		if source, ok := ith.Binding().(*ast.ColumnBinding); !ok {
			errors = append(errors, *r.srcmap.SyntaxError(ith, "invalid source column"))
			return errors
		} else if !started && source.DataType.(*ast.IntType) == nil {
			errors = append(errors, *r.srcmap.SyntaxError(ith, "fixed-width type required"))
		} else if started && multiplier != source.Multiplier {
			// Problem
			errors = append(errors, *r.srcmap.SyntaxError(ith, "incompatible length multiplier"))
		} else if !enclosing.IsVisible(ith) {
			errors = append(errors, *r.srcmap.SyntaxError(ith, "recursive definition"))
		} else {
			// All good, finalise target column
			target := decl.Targets[i].Binding().(*ast.ColumnBinding)
			// Update with completed information
			target.Multiplier = source.Multiplier
			target.DataType = source.DataType
			multiplier = source.Multiplier
			started = true
		}
	}
	//
	closeDefinitions(enclosing, decl.Targets...)
	// Done
	return errors
}

// Resolve those variables appearing in the body of this property assertion.
func (r *resolver) finaliseDefPerspectiveInModule(enclosing Scope, decl *ast.DefPerspective) []SyntaxError {
	scope := NewLocalScope(enclosing, false, false, false)
	// Resolve assertion
	errors := r.finaliseExpressionInModule(scope, decl.Selector)
	// Error check
	if len(errors) == 0 {
		decl.Finalise()
	}
	// Done
	return errors
}

// Finalise a range constraint declaration after all symbols have been
// resolved. This involves: (a) checking the context is valid; (b) checking the
// expressions are well-typed.
func (r *resolver) finaliseDefInRangeInModule(enclosing Scope, decl *ast.DefInRange) []SyntaxError {
	var scope = NewLocalScope(enclosing, false, false, false)
	// Resolve property body
	errors := r.finaliseExpressionInModule(scope, decl.Expr)
	// Error check
	if len(errors) == 0 {
		decl.Finalise()
	}
	// Done
	return errors
}

// Finalise a function definition after all symbols have been resolved. This
// involves: (a) checking the context is valid for the body; (b) checking the
// body is well-typed; (c) for pure functions checking that no columns are
// accessed; (d) finally, resolving any parameters used within the body of this
// function.
func (r *resolver) finaliseDefFunInModule(enclosing *ModuleScope, decl *ast.DefFun) []SyntaxError {
	var scope = NewLocalScope(enclosing, true, decl.IsPure(), false)
	//
	enclosing.OpenDefinition(decl)
	// Declare parameters in local scope
	for _, p := range decl.Parameters() {
		scope.DeclareLocal(p.Binding.Name, &p.Binding)
	}
	// Resolve property body
	errors := r.finaliseExpressionInModule(scope, decl.Body())
	// Finalise declaration
	if len(errors) == 0 {
		decl.Finalise()
	}
	//
	enclosing.CloseDefinition(decl)
	// Done
	return errors
}

// Resolve those variables appearing in the body of this lookup constraint.
func (r *resolver) finaliseDefLookupInModule(enclosing Scope, decl *ast.DefLookup) []SyntaxError {
	var errors []SyntaxError
	// Resolve source expressions
	for i := range decl.Sources {
		var (
			scope = NewLocalScope(enclosing, true, false, false)
			errs  = r.finaliseExpressionsInModule(scope, decl.Sources[i])
		)

		errors = append(errors, errs...)
	}
	// Resolve all target expressions
	for i := range decl.Targets {
		var (
			scope = NewLocalScope(enclosing, true, false, false)
			errs  = r.finaliseExpressionsInModule(scope, decl.Targets[i])
		)

		errors = append(errors, errs...)
	}
	//
	return errors
}

// Resolve those variables appearing in the body of this property assertion.
func (r *resolver) finaliseDefPropertyInModule(enclosing Scope, decl *ast.DefProperty) []SyntaxError {
	scope := NewLocalScope(enclosing, false, false, false)
	// Resolve assertion
	return r.finaliseExpressionInModule(scope, decl.Assertion)
}

func (r *resolver) finaliseDefSortedInModule(enclosing Scope, decl *ast.DefSorted) []SyntaxError {
	var (
		scope = NewLocalScope(enclosing, false, false, false)
	)
	// Resolve source expressions
	errors := r.finaliseExpressionsInModule(scope, decl.Sources)
	// Resolve (optional) selector expression
	if decl.Selector.HasValue() {
		r.finaliseExpressionInModule(scope, decl.Selector.Unwrap())
	}
	// Sanity check length multipliers
	for _, e := range decl.Sources {
		// Sanity check multiplier has size 1
		if e.Context().Multiplier != 1 {
			errors = append(errors, *r.srcmap.SyntaxError(e, "interleaved column access not permitted"))
		}
	}
	// Error check
	if len(errors) == 0 {
		decl.Finalise()
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
	case *ast.Cast:
		return r.finaliseExpressionInModule(scope, v.Arg)
	case *ast.Connective:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Constant:
		return nil
	case *ast.Debug:
		return r.finaliseExpressionInModule(scope, v.Arg)
	case *ast.Equation:
		lhs_errs := r.finaliseExpressionInModule(scope, v.Lhs)
		rhs_errs := r.finaliseExpressionInModule(scope, v.Rhs)
		// combine errors
		return append(lhs_errs, rhs_errs...)
	case *ast.Exp:
		constscope := scope.NestedConstScope()
		arg_errs := r.finaliseExpressionInModule(scope, v.Arg)
		pow_errs := r.finaliseExpressionInModule(constscope, v.Pow)
		// combine errors
		return append(arg_errs, pow_errs...)
	case *ast.For:
		nestedscope := scope.NestedScope()
		// Declare local variable
		nestedscope.DeclareLocal(v.Binding.Name, &v.Binding)
		// Continue resolution
		return r.finaliseExpressionInModule(nestedscope, v.Body)
	case *ast.If:
		return r.finaliseExpressionsInModule(scope, []ast.Expr{v.Condition, v.TrueBranch, v.FalseBranch})
	case *ast.Invoke:
		return r.finaliseInvokeInModule(scope, v)
	case *ast.Let:
		return r.finaliseLetInModule(scope, v)
	case *ast.List:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Mul:
		return r.finaliseExpressionsInModule(scope, v.Args)
	case *ast.Normalise:
		return r.finaliseExpressionInModule(scope, v.Arg)
	case *ast.Not:
		return r.finaliseExpressionInModule(scope, v.Arg)
	case *ast.Reduce:
		return r.finaliseReduceInModule(scope, v)
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
	case *ast.Concat:
		return r.finaliseExpressionsInModule(scope, v.Args)
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

// Resolve a specific invocation contained within some expression which, in
// turn, is contained within some module.  Note, qualified accesses are only
// permitted in a global context.
func (r *resolver) finaliseInvokeInModule(scope LocalScope, expr *ast.Invoke) []SyntaxError {
	var errors []SyntaxError
	// Lookup the corresponding function definition.
	if !expr.Name.IsResolved() && !scope.Bind(expr.Name) {
		return append(errors, *r.srcmap.SyntaxError(expr.Name, "unknown function"))
	} else if !scope.IsVisible(expr.Name) {
		return r.srcmap.SyntaxErrors(expr, "recursion not permitted here")
	}
	// Resolve arguments
	errors = r.finaliseExpressionsInModule(scope, expr.Args)
	// Following must be true if we get here.
	binding := expr.Name.Binding().(ast.FunctionBinding)
	// Check purity
	if scope.IsPure() && !binding.IsPure() {
		errors = append(errors, *r.srcmap.SyntaxError(expr.Name, "not permitted in pure context"))
	}
	// Check provide correct number of arguments
	if binding.Signature() == nil {
		// NOTE: this should only be possible for native definitions which, at
		// the time of writing, cannot be called from arbitrary expressions.
		errors = append(errors, *r.srcmap.SyntaxError(expr.Name, "native invocation not permitted"))
	} else if binding.Signature().Arity() != uint(len(expr.Args)) {
		msg := fmt.Sprintf("incorrect number of arguments (found %d)", len(expr.Args))
		errors = append(errors, *r.srcmap.SyntaxError(expr, msg))
	}
	//
	return errors
}

func (r *resolver) finaliseLetInModule(scope LocalScope, expr *ast.Let) []SyntaxError {
	nestedscope := scope.NestedScope()
	// Declare assigned variable(s)
	for i, letvar := range expr.Vars {
		nestedscope.DeclareLocal(letvar.Name, &expr.Vars[i])
	}
	// Finalise assigned expressions
	args_errs := r.finaliseExpressionsInModule(scope, expr.Args)
	// Finalise body
	body_errs := r.finaliseExpressionInModule(nestedscope, expr.Body)
	//
	return append(args_errs, body_errs...)
}

// Resolve a specific invocation contained within some expression which, in
// turn, is contained within some module.  Note, qualified accesses are only
// permitted in a global context.
func (r *resolver) finaliseReduceInModule(scope LocalScope, expr *ast.Reduce) []SyntaxError {
	// Resolve arguments
	errors := r.finaliseExpressionInModule(scope, expr.Arg)
	// Lookup the corresponding function definition.
	if !expr.Name.IsResolved() && !scope.Bind(expr.Name) {
		errors = append(errors, *r.srcmap.SyntaxError(expr, "unknown function"))
	} else {
		// Following must be true if we get here.
		binding := expr.Name.Binding().(ast.FunctionBinding)

		if scope.IsPure() && !binding.IsPure() {
			errors = append(errors, *r.srcmap.SyntaxError(expr, "not permitted in pure context"))
		}
	}
	// Done
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
	} else if binding, ok := expr.Binding().(*ast.ConstantBinding); ok {
		// Constant
		if binding.Extern && scope.IsConstant() {
			return r.srcmap.SyntaxErrors(expr, "not permitted in const context")
		}
		//
		return nil
	} else if _, ok := expr.Binding().(*ast.LocalVariableBinding); ok {
		// Parameter, for or let variable
		return nil
	} else if _, ok := expr.Binding().(ast.FunctionBinding); ok {
		// Function doesn't makes sense here.
		return r.srcmap.SyntaxErrors(expr, "refers to a function")
	}
	// Should be unreachable.
	return r.srcmap.SyntaxErrors(expr, "unknown symbol kind")
}

// The purpose of this function is to construct a much more useful error message
// than the default "unknown symbol".  For example, if we have use a function
// but given an incorrect number of arguments, then we want to know this.
func (r *resolver) constructUnknownSymbolError(symbol ast.Symbol, scope Scope) SyntaxError {
	name := symbol.Path().Tail()
	parent := symbol.Path().Parent()
	//
	if symbol.Arity().HasValue() {
		var (
			aboveArity int = math.MaxInt
			belowArity int = math.MinInt
			belowCount     = 0
			aboveCount     = 0
			arity          = symbol.Arity().Unwrap()
		)
		//
		for _, bid := range scope.Bindings(*parent) {
			if bid.name == name && bid.arity.HasValue() {
				bidArity := bid.arity.Unwrap()
				//
				if bidArity < arity {
					belowArity = max(belowArity, int(bidArity))
					belowCount++
				} else if bidArity > arity {
					aboveArity = min(aboveArity, int(bidArity))
					aboveCount++
				}
			}
		}
		// Report useful error if we found something.
		if belowCount > 0 || aboveCount > 0 {
			var (
				str      string
				belowStr = fmt.Sprintf("%d", belowArity)
				aboveStr = fmt.Sprintf("%d", aboveArity)
			)
			//
			if belowCount > 1 {
				belowStr = fmt.Sprintf("%s (or less)", belowStr)
			}
			//
			if aboveCount > 1 {
				aboveStr = fmt.Sprintf("%s (or more)", aboveStr)
			}
			// Determine best error
			if belowCount > 0 && aboveCount > 0 {
				str = fmt.Sprintf("%s or %s", belowStr, aboveStr)
			} else if aboveArity != math.MaxInt {
				str = aboveStr
			} else if belowArity != math.MinInt {
				str = belowStr
			}
			//
			msg := fmt.Sprintf("found %d arguments, expected %s", arity, str)
			//
			return *r.srcmap.SyntaxError(symbol, msg)
		}
	}
	// Fall back on default.  We actually could do better here by trying to find
	// the closest match.
	return *r.srcmap.SyntaxError(symbol, "unknown symbol")
}

func openDefinitions[T ast.SymbolDefinition](scope *ModuleScope, defs ...T) {
	for _, def := range defs {
		scope.OpenDefinition(def)
	}
}

func closeDefinitions[T ast.SymbolDefinition](scope *ModuleScope, defs ...T) {
	for _, def := range defs {
		scope.CloseDefinition(def)
	}
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
