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
	"slices"
	"strings"

	"github.com/consensys/go-corset/pkg/asm"
	"github.com/consensys/go-corset/pkg/corset/ast"
	"github.com/consensys/go-corset/pkg/ir"
	"github.com/consensys/go-corset/pkg/ir/assignment"
	"github.com/consensys/go-corset/pkg/ir/hir"
	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/constraint/lookup"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/file"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/word"
)

// Config encapsulates various options which can affect compilation.
type Config struct {
	// Enable standard library
	Stdlib bool
	// Enable debug constraints
	Debug bool
	// Enable legacy register allocator
	Legacy bool
	// Enforce all types by default
	EnforceTypes bool
	// Enforce types for all limbs arising from splitting registers
	EnforceLimbTypes bool
	// Target field configuration.  This is only used to assist in reporting
	// errors which are specific to the given field configuration.
	Field field.Config
}

// SchemaBuilder is used within this translator for building the final mixed HIR
// schema.
type SchemaBuilder = ir.SchemaBuilder[word.BigEndian, hir.Constraint, hir.Term]

// ModuleBuilder is used within this translator for building the various modules
// which are contained within the mixed HIR schema.
type ModuleBuilder = ir.ModuleBuilder[word.BigEndian, hir.Constraint, hir.Term]

// TranslateCircuit translates the components of a Corset circuit and add them
// to the schema.  By the time we get to this point, all malformed source files
// should have been rejected already and the translation should go through
// easily.  Thus, whilst syntax errors can be returned here, this should never
// happen.  The mechanism is supported, however, to simplify development of new
// features, etc.
func TranslateCircuit(
	env Environment,
	srcmap *source.Maps[ast.Node],
	circuit *ast.Circuit,
	extern asm.MacroProgram,
	config Config) (asm.MacroHirProgram, []SyntaxError) {
	//
	builder := ir.NewSchemaBuilder[word.BigEndian, hir.Constraint, hir.Term](extern.Components()...)
	t := translator{env, srcmap, builder, config}
	// Allocate all modules into schema
	t.translateModules(circuit)
	// Translate everything else
	if errs := t.translateDeclarations(circuit); len(errs) > 0 {
		return asm.MacroHirProgram{}, errs
	}
	// Build concrete modules from schema
	modules := ir.BuildSchema[hir.Module](t.schema)
	// Finally, construct the asm program
	return asm.NewMixedProgram[word.BigEndian](extern, modules...), nil
}

// Translator packages up information necessary for translating a circuit into
// the schema form required for the HIR level.
type translator struct {
	// Environment is needed for determining the identifiers for modules and
	// registers.
	env Environment
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
	// Represents the schema being constructed by this translator.
	schema SchemaBuilder
	// compilation configuration
	config Config
}

func (t *translator) translateModules(circuit *ast.Circuit) {
	// Add root module
	t.translateModule("")
	// Add nested modules
	for _, m := range circuit.Modules {
		t.translateModule(m.Name)
	}
}

// Translate the given Corset module into a family of one (or more) HIR modules.
// Normally, every Corset module corresponds to exactly one HIR module. More
// specifically, there will be one module for each distinct length multiplier.
// Thus, in the presence of interleavings, a Corset module will map to more than
// one HIR module.
func (t *translator) translateModule(name string) {
	// Always include module with base multiplier (even if empty).
	t.schema.NewModule(module.NewName(name, 1), true, true, false, false, false, 0)
	// Initialise the corresponding family of HIR modules.
	for _, regIndex := range t.env.RegistersOf(name) {
		var (
			// Identify register info
			regInfo = t.env.Register(regIndex)
			// Determine corresponding module name
			moduleName = regInfo.Context.ModuleName()
		)
		// Check whether module created this already (or not)
		if _, ok := t.schema.HasModule(moduleName); !ok {
			// No, therefore create new module.
			t.schema.NewModule(moduleName, true, true, false, false, false, 0)
		}
	}
	// Translate all corset registers in this module into HIR registers across
	// the corresponding *family* of modules.
	t.translateModuleRegisters(t.env.RegistersOf(name))
}

// Add all registers defined in the given Corset module into registers in one
// (or more) HIR modules.
func (t *translator) translateModuleRegisters(corsetRegisters []uint) {
	// Process each register in turn.
	for _, regIndex := range corsetRegisters {
		var (
			// Identify register info
			regInfo = t.env.Register(regIndex)
			// Identify enclosing HIR module
			module = t.schema.ModuleOf(regInfo.Context.ModuleName())
			//
			reg register.Register
		)
		// Declare corresponding register
		if regInfo.IsInput() {
			reg = register.NewInput(regInfo.Name(), regInfo.Bitwidth, regInfo.Padding)
		} else {
			reg = register.NewComputed(regInfo.Name(), regInfo.Bitwidth, regInfo.Padding)
		}
		// Add the register
		module.NewRegister(reg)
		// Add range constraints for underlying types (as necessary)
		t.translateTypeConstraints(*regInfo, module)
	}
}

// Translate any type constraints applicable for the given register.  Type
// constraints are determined by the source-level registers and, hence, there are
// several cases to consider:
//
// (1) none of the source-level registers allocated to this register was marked
// provable. Therefore, no need to do anything.
//
// (2) all source-level registers allocated to this register which are marked
// provable have the same type which, furthermore, is the largest type of any
// register allocated to this register.  In this case, we can use a single
// (global) constraint for the entire register.
//
// (3) source-level registers allocated to this register which are marked provable
// have the same type, but this is not the largest of any allocated to this
// register.  In fact, only binary@prove is supported here and we can assume
// each register is allocated to a different perspective.
//
// Any other cases are considered to be erroneous register allocations, and will
// lead to a panic.
func (t *translator) translateTypeConstraints(reg Register, mod ModuleBuilder) {
	var (
		regWidth = reg.Bitwidth
		required = t.config.EnforceTypes || (t.config.EnforceLimbTypes && regWidth > t.config.Field.RegisterWidth)
	)
	// Check for provability
	for _, col := range reg.Sources {
		if col.MustProve {
			required = true
			break
		}
	}
	// Apply provability (if it is required)
	if required {
		// For now, enforce all source registers have matching bitwidth.
		for _, col := range reg.Sources {
			// Determine bitwidth
			colWidth := col.Bitwidth
			// Sanity check (for now)
			if col.MustProve && colWidth != regWidth {
				// Currently, mixed-width proving types are not supported.
				panic("cannot (currently) prove type of mixed-width register")
			}
		}
		// Add appropriate type constraint
		constraint := hir.NewRangeConstraint(reg.Name(),
			mod.Id(),
			RegisterAccessOf(mod, reg.Name(), 0),
			reg.Bitwidth)
		//
		mod.AddConstraint(constraint)
	}
}

// Translate all assignment or constraint declarations in the circuit.
func (t *translator) translateDeclarations(circuit *ast.Circuit) []SyntaxError {
	rootPath := file.NewAbsolutePath()
	errors := t.translateDeclarationsInModule(rootPath, circuit.Declarations)
	// Translate each module
	for _, m := range circuit.Modules {
		modPath := rootPath.Extend(m.Name)
		errs := t.translateDeclarationsInModule(*modPath, m.Declarations)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// Translate all assignment or constraint declarations in a given module within
// the circuit.
func (t *translator) translateDeclarationsInModule(path file.Path, decls []ast.Declaration) []SyntaxError {
	var errors []SyntaxError
	//
	for _, d := range decls {
		errs := t.translateDeclaration(d, path)
		errors = append(errors, errs...)
	}
	// Done
	return errors
}

// Translate an assignment or constraint declaration which occurs within a given module.
func (t *translator) translateDeclaration(decl ast.Declaration, path file.Path) []SyntaxError {
	var errors []SyntaxError
	//
	switch d := decl.(type) {
	case *ast.DefAliases:
		// Not an assignment or a constraint, hence ignore.
	case *ast.DefCall:
		errors = t.translateDefCall(d)
	case *ast.DefComputed:
		return t.translateDefComputed(d, path)
	case *ast.DefColumns:
		// Not an assignment or a constraint, hence ignore.
	case *ast.DefConst:
		// For now, constants are always compiled out when going down to hir.
	case *ast.DefConstraint:
		errors = t.translateDefConstraint(d)
	case *ast.DefFun:
		// For now, functions are always compiled out when going down to hir.
		// In the future, this might change if we add support for macros to hir.
	case *ast.DefInRange:
		errors = t.translateDefInRange(d)
	case *ast.DefInterleaved:
		errors = t.translateDefInterleaved(d, path)
	case *ast.DefLookup:
		errors = t.translateDefLookup(d)
	case *ast.DefPermutation:
		t.translateDefPermutation(d, path)
	case *ast.DefPerspective:
		// As for defregisters, nothing generated here.
	case *ast.DefProperty:
		errors = t.translateDefProperty(d)
	case *ast.DefSorted:
		errors = t.translateDefSorted(d)
	case *ast.DefComputedColumn:
		errors = t.translateDefComputedColumn(d, path)
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// Translate a "deflookup" declaration.
func (t *translator) translateDefCall(decl *ast.DefCall) []SyntaxError {
	var (
		callerContext, _ = ast.ContextOfExpressions(decl.Arguments...)
		calleeContext    = ast.NewContext(decl.Function, 1)
		// Lookup callee module
		calleeModule = t.moduleOf(calleeContext)
		selector     = util.None[hir.LogicalTerm]()
	)
	// Translate target expressions whilst again checking for a conflicting
	// context.
	if callerContext.IsConflicted() {
		// This should be unreachable, as should already have been detected
		// during resolution.
		return t.srcmap.SyntaxErrors(decl, "conflicting argument context")
	} else if calleeModule == nil {
		return t.srcmap.SyntaxErrors(decl, fmt.Sprintf("unknown function \"%s\"", decl.Function))
	} else if !calleeModule.IsExtern() {
		return t.srcmap.SyntaxErrors(decl, "cannot call non-assembly module")
	}
	// Lookup caller module
	callerModule := t.moduleOf(callerContext)
	// Translate returns
	//nolint
	rets, errs1 := t.translateExpressions(callerModule, 0, decl.Returns...)
	// Translate arguments
	//nolint
	args, errs2 := t.translateExpressions(callerModule, 0, decl.Arguments...)
	// Check arguments / returns
	errs3 := t.checkArgsReturns(decl, rets, args, calleeModule)
	// Combine all errors
	errors := append(errs1, errs2...)
	errors = append(errors, errs3...)
	// Translate selector (if applicable)
	if decl.Selector.HasValue() {
		sel, errs := t.translateLogical(decl.Selector.Unwrap(), callerModule, 0)
		selector = util.Some(sel)

		errors = append(errors, errs...)
	}
	// Sanity check whether we can construct the constraint, or not.
	if len(errors) == 0 {
		handle := fmt.Sprintf("%s=>%s", callerModule.Name().Name, calleeModule.Name().Name)
		// FIXME: Sanity check argument / return subtying
		//
		callerModule.AddConstraint(hir.NewFunctionCall(
			handle, callerModule.Id(), calleeModule.Id(), rets, args, selector))
	}
	// Done
	return errors
}

func (t *translator) checkArgsReturns(decl *ast.DefCall, rets, args []hir.Term, callee ModuleBuilder) []SyntaxError {
	var (
		errors []SyntaxError
		nRets  = uint(len(rets))
		nArgs  = uint(len(args))
		n      = nRets + nArgs
	)
	//
	for i := range n {
		// Sanity check enough target registers
		if i >= callee.Width() {
			if i < nArgs {
				errors = append(errors, *t.srcmap.SyntaxError(decl.Arguments[i],
					fmt.Sprintf("too many arguments for function \"%s\"", decl.Function)))
			} else {
				errors = append(errors, *t.srcmap.SyntaxError(decl.Returns[i-nArgs],
					fmt.Sprintf("too many returns for function \"%s\"", decl.Function)))
			}
			// Cannot continue
			break
		}
		// Extract ith register
		var ith = callee.Register(register.NewId(i))
		// Santity arguments / returns align
		if i < nArgs && !ith.IsInput() {
			return append(errors, *t.srcmap.SyntaxError(decl.Arguments[i],
				fmt.Sprintf("too many arguments for function \"%s\"", decl.Function)))
		} else if i >= nArgs && ith.IsInput() {
			return append(errors, *t.srcmap.SyntaxError(decl.Returns[i-nArgs],
				fmt.Sprintf("insufficient arguments for function \"%s\"", decl.Function)))
		} else if i >= nArgs && !ith.IsOutput() {
			return append(errors, *t.srcmap.SyntaxError(decl.Returns[i-nArgs],
				fmt.Sprintf("too many arguments for function \"%s\"", decl.Function)))
		}
		// Sanity check bitwidth
		if i < nArgs {
			// subtype
			errors = append(errors, t.checkSubSuptype(true, args[i], ith.Width(), decl.Arguments[i])...)
		} else {
			// supertype
			errors = append(errors, t.checkSubSuptype(false, rets[i-nArgs], ith.Width(), decl.Returns[i-nArgs])...)
		}
	}
	//
	return errors
}

func (t *translator) checkSubSuptype(subtype bool, term hir.Term, bitwidth uint, node ast.Node) []SyntaxError {
	var (
		// Compute value range of term
		vals = term.ValueRange()
		// Convert into bitwidth
		termWidth, signed = vals.BitWidth()
	)
	// Sanity check signed lookup
	if signed {
		return t.srcmap.SyntaxErrors(node, "signed term encountered")
	} else if subtype && termWidth > bitwidth {
		return t.srcmap.SyntaxErrors(node, fmt.Sprintf("expected u%d, found u%d", bitwidth, termWidth))
	} else if !subtype && termWidth < bitwidth {
		return t.srcmap.SyntaxErrors(node, fmt.Sprintf("expected u%d, found u%d", termWidth, bitwidth))
	}
	//
	return nil
}

// Translate a "defcomputedcolumn" declaration.
func (t *translator) translateDefComputedColumn(d *ast.DefComputedColumn, path file.Path) []SyntaxError {
	var (
		// Determine enclosing module
		module = t.moduleOf(d.Computation.Context())
		// Determine direction of comptuation
		direction = d.Target.InnerBinding().Kind != ast.COMPUTED_BWD
		// Determine HIR identifier for target register
		targetPath            = path.Extend(d.Target.Name())
		targetId              = t.registerIndexOf(targetPath)
		targetAccess hir.Term = t.registerOf(targetPath, 0)
		// Translate computation
		computation, errors = t.translateExpression(d.Computation, module, 0)
	)
	// Sanity check any compilation errors
	if len(errors) != 0 {
		return errors
	}
	// Calculate padding value
	targetPadding := ir.PaddingFor(computation, module)
	// Calculate and update padding value
	module.Registers()[targetId.Unwrap()].SetPadding(&targetPadding)
	// Add assignment
	module.AddAssignment(assignment.NewComputedRegister[word.BigEndian](
		term.NewComputation[word.BigEndian, hir.LogicalTerm](computation), direction,
		module.Id(), targetId))
	// Add constraint (defconstraint target == computation)
	module.AddConstraint(hir.NewVanishingConstraint(
		d.Target.Name(), module.Id(),
		// no domain, since this is a global constraint (i.e. applies to all
		// rows).
		util.None[int](),
		//
		term.Equals[word.BigEndian, hir.LogicalTerm](targetAccess, computation),
	))
	// Done
	return nil
}

// Translate a "defcomputed" declaration.
func (t *translator) translateDefComputed(decl *ast.DefComputed, path file.Path) []SyntaxError {
	var context ast.Context = ast.VoidContext()
	//
	targets := make([]register.Refs, len(decl.Targets))
	sources := make([]register.Refs, len(decl.Sources))
	// Identify source registers
	for i := 0; i < len(decl.Sources); i++ {
		ith := decl.Sources[i].Binding().(*ast.ColumnBinding)
		source := t.env.Register(t.env.RegisterOf(&ith.Path))
		sources[i] = t.registerRefsOf(&ith.Path)
		// Join contexts
		context = context.Join(source.Context)
	}
	// Identify target registers
	for i := 0; i < len(decl.Targets); i++ {
		targetPath := path.Extend(decl.Targets[i].Name())
		target := t.env.Register(t.env.RegisterOf(targetPath))
		targets[i] = t.registerRefsOf(targetPath)
		// Join contexts
		context = context.Join(target.Context)
	}
	// Extract the binding
	binding := decl.Function.Binding().(*NativeDefinition)
	// Sanity check
	if context.IsConflicted() || context.IsVoid() {
		return t.srcmap.SyntaxErrors(decl, "conflicting (or void) constraint context")
	}
	// Determine enclosing module
	module := t.moduleOf(context)
	// Add the assignment and check the first identifier.
	module.AddAssignment(assignment.NewNativeComputation[word.BigEndian](binding.name, targets, sources))
	//
	return nil
}

// Translate a "defconstraint" declaration.
func (t *translator) translateDefConstraint(decl *ast.DefConstraint) []SyntaxError {
	var (
		module = t.moduleOf(decl.Constraint.Context())
		// Translate expr body
		expr, errors = t.translateLogical(decl.Constraint, module, 0)
	)
	// Apply guard
	if expr == nil {
		// NOTE: in this case, the constraint itself has been translated as nil.
		// This means there is no constraint (e.g. its a debug constraint, but
		// debug mode is not enabled).
		return errors
	}
	// Apply guard (if applicable)
	if decl.Guard != nil {
		// Translate (optional) guard
		gexpr, guardErrors := t.translateOptionalExpression(decl.Guard, module, 0)
		guard := term.Equals[word.BigEndian, hir.LogicalTerm](gexpr, term.Const64[word.BigEndian, hir.Term](0))
		expr = term.IfThenElse(guard, nil, expr)
		// Combine errors
		errors = append(errors, guardErrors...)
	}
	// Apply perspective selector (if applicable)
	if decl.Perspective != nil {
		// Translate (optional) perspective selector
		sexpr, selectorErrors := t.translateSelectorInModule(decl.Perspective, module)
		selector := term.Equals[word.BigEndian, hir.LogicalTerm](sexpr, term.Const64[word.BigEndian, hir.Term](0))
		expr = term.IfThenElse(selector, nil, expr)
		// Combine errors
		errors = append(errors, selectorErrors...)
	}
	// Sanity check
	if len(errors) == 0 {
		// Add translated constraint
		module.AddConstraint(hir.NewVanishingConstraint(decl.Handle, module.Id(), decl.Domain, expr))
	}
	// Done
	return errors
}

// Translate the selector for the perspective of a defconstraint.  Observe that
// a defconstraint may not be part of a perspective and, hence, would have no
// selector.
func (t *translator) translateSelectorInModule(perspective *ast.PerspectiveName,
	module ModuleBuilder) (hir.Term, []SyntaxError) {
	//
	if perspective != nil {
		return t.translateExpression(perspective.InnerBinding().Selector, module, 0)
	}
	//
	return nil, nil
}

// Translate a "deflookup" declaration.
func (t *translator) translateDefLookup(decl *ast.DefLookup) []SyntaxError {
	var (
		errors                 []SyntaxError
		srcContext, tgtContext ast.Context
		sources                []lookup.Vector[word.BigEndian, hir.Term]
		targets                []lookup.Vector[word.BigEndian, hir.Term]
	)
	// Translate sources
	for i, ith := range decl.Targets {
		ith_targets, ctx, errs := t.translateDefLookupSources(decl.TargetSelectors[i], ith)
		targets = append(targets, ith_targets)
		errors = append(errors, errs...)
		//
		if i == 0 {
			tgtContext = ctx
		}
	}
	// Translate targets
	for i, ith := range decl.Sources {
		ith_sources, ctx, errs := t.translateDefLookupSources(decl.SourceSelectors[i], ith)
		sources = append(sources, ith_sources)
		errors = append(errors, errs...)
		//
		if i == 0 {
			srcContext = ctx
		}
	}
	// Sanity check this is not an irregular lookup (since these are not
	// currently supported) and, if so, provide a useful error message.
	if len(errors) == 0 {
		errors = t.checkForIrregularLookup(targets, sources, decl.Targets, decl.Sources)
	}
	// Sanity check whether we can construct the constraint, or not.
	if len(errors) == 0 {
		// Default to adding constraint to source module
		var module = t.moduleOf(srcContext)
		// However, if external add to target module instead.
		if module.IsExtern() {
			module = t.moduleOf(tgtContext)
		}
		// Add translated constraint
		module.AddConstraint(hir.NewLookupConstraint(decl.Handle, targets, sources))
	}
	// Done
	return errors
}

func (t *translator) translateDefLookupSources(selector ast.Expr,
	sources []ast.Expr) (lookup.Vector[word.BigEndian, hir.Term], ast.Context, []SyntaxError) {
	// Determine context of ith set of targets
	var (
		context, j = ast.ContextOfExpressions(sources...)
		vector     lookup.Vector[word.BigEndian, hir.Term]
	)
	// Include selector (when present)
	if selector != nil {
		context = context.Join(selector.Context())
	}
	// Translate target expressions whilst again checking for a conflicting
	// context.
	if context.IsConflicted() {
		var source ast.Expr
		// Determine offending source expression
		if j >= uint(len(sources)) {
			source = selector
		} else {
			source = sources[j]
		}
		//
		return lookup.Vector[word.BigEndian, hir.Term]{}, context, t.srcmap.SyntaxErrors(source, "conflicting context")
	}
	// Determine enclosing module
	module := t.moduleOf(context)
	// Translate source expressions
	terms, errors := t.translateUnitExpressions(sources, module, 0)
	// handle selector
	if selector != nil {
		s, errs := t.translateExpression(selector, module, 0)
		errors = append(errors, errs...)

		vector = lookup.FilteredVector(module.Id(), s, terms...)
	} else {
		vector = lookup.UnfilteredVector(module.Id(), terms...)
	}
	// Sanity check vector
	if len(errors) == 0 {
		// NOTE: don't check vector if other errors, since we could have nil
		// entries in the vector, etc.
		errors = append(errors, t.checkLookupVector(module.IsExtern(), vector, selector, sources)...)
	}
	//
	return vector, context, errors
}

func (t *translator) checkLookupVector(extern bool, vector lookup.Vector[word.BigEndian, hir.Term], selector ast.Expr,
	terms []ast.Expr) []SyntaxError {
	//
	var (
		errors []SyntaxError
	)
	// Look for any negative terms
	for i, ith := range vector.Terms {
		if extern && !isConstantRegister(ith) {
			errors = append(errors, *t.srcmap.SyntaxError(terms[i],
				"arbitrary term not permitted here (i.e. only 0, 1, or register for external module)"))
		}
		// Determine value range of ith term
		valrange := ith.ValueRange()
		// Determine bitwidth for that range
		_, signed := valrange.BitWidth()
		// Sanity check signed lookups
		if signed {
			errors = append(errors, *t.srcmap.SyntaxError(terms[i], "signed term encountered"))
		}
	}
	// Check selector is binary
	if vector.HasSelector() {
		// Determine value range of ith term
		valrange := vector.Selector.Unwrap().ValueRange()
		// Determine bitwidth for that range
		bitwidth, signed := valrange.BitWidth()
		// Check for signed selector
		if signed {
			errors = append(errors, *t.srcmap.SyntaxError(selector, "signed selector encountered"))
		}
		// Check for non-binary selector
		if bitwidth > 1 {
			errors = append(errors, *t.srcmap.SyntaxError(selector, "non-binary selector encountered"))
		}
	}
	// Done
	return errors
}

func isConstantRegister(term hir.Term) bool {
	switch t := term.(type) {
	case *hir.Constant:
		val := t.Value.BigInt()
		// Check whether valid constant
		return val.IsUint64() && (val.Uint64() == 0 || val.Uint64() == 1)
	case *hir.RegisterAccess:
		return true
	}
	//
	return false
}

// An irregular lookup is an awkward scenario where a source/target pairing does
// not align properly.  This scenario is not currently supported and, hence, a
// suitable error message must be returned.  For example, support a pairing of
// u160 (source) into u256 (target) with a maximum register size of u160.  Then,
// the source will decompose into a single u160 limb, whilst the target will
// decompose into a two u128 limbs.
func (t *translator) checkForIrregularLookup(targets []lookup.Vector[word.BigEndian, hir.Term],
	sources []lookup.Vector[word.BigEndian, hir.Term], tgtTerms [][]ast.Expr, srcTerms [][]ast.Expr) []SyntaxError {
	var (
		n         = len(sources[0].Terms)
		srcWidths = t.determineLookupBitwidths(sources)
		tgtWidths = t.determineLookupBitwidths(targets)
		errors    []SyntaxError
	)
	//
	for i, ith := range srcWidths {
		for j, jth := range tgtWidths {
			for k := range n {
				// Check for error
				switch t.isIrregularLookup(ith[k], jth[k]) {
				case -1:
					// source failure
					errors = append(errors, *t.srcmap.SyntaxError(srcTerms[i][k], "irregular lookup detected"))
				case 1:
					// target failure
					errors = append(errors, *t.srcmap.SyntaxError(tgtTerms[j][k], "irregular lookup detected"))
				}
			}
		}
	}
	//
	return errors
}

func (t *translator) determineLookupBitwidths(terms []lookup.Vector[word.BigEndian, hir.Term]) [][]uint {
	var (
		bitwidths = make([][]uint, len(terms))
	)
	//
	for i := range terms {
		ith := make([]uint, len(terms[i].Terms))
		for j, jth := range terms[i].Terms {
			// Determine value range of ith term
			valrange := jth.ValueRange()
			// Determine bitwidth for that range
			ith[j], _ = valrange.BitWidth()
		}
		//
		bitwidths[i] = ith
	}
	//
	return bitwidths
}

func (t *translator) isIrregularLookup(srcWidth, tgtWidth uint) int {
	var (
		srcLimbWidths = register.LimbWidths(t.config.Field.RegisterWidth, srcWidth)
		tgtLimbWidths = register.LimbWidths(t.config.Field.RegisterWidth, tgtWidth)
		n             = min(len(srcLimbWidths), len(tgtLimbWidths))
	)
	//
	for i := range n {
		var (
			srcLast = i+1 == len(srcLimbWidths)
			tgtLast = i+1 == len(tgtLimbWidths)
		)
		// Check limbs
		if srcLimbWidths[i] > tgtLimbWidths[i] && !tgtLast {
			return -1
		} else if tgtLimbWidths[i] > srcLimbWidths[i] && !srcLast {
			return 1
		}
	}
	//
	return 0
}

// Translate a "definrange" declaration.
func (t *translator) translateDefInRange(decl *ast.DefInRange) []SyntaxError {
	module := t.moduleOf(decl.Expr.Context())
	// Translate constraint body
	expr, errors := t.translateExpression(decl.Expr, module, 0)
	//
	if len(errors) != 0 {
		return errors
	}
	//
	valrange := expr.ValueRange()
	// Sanity check sign of expression
	_, signed := valrange.BitWidth()
	// Sanity check signed lookups
	if signed {
		errors = append(errors, *t.srcmap.SyntaxError(decl.Expr, "signed term encountered"))
	} else {
		// Add translated constraint
		module.AddConstraint(hir.NewRangeConstraint("", module.Id(), expr, decl.Bitwidth))
	}
	// Done
	return errors
}

// Translate a "definterleaved" declaration.
// nolint
func (t *translator) translateDefInterleaved(decl *ast.DefInterleaved, path file.Path) []SyntaxError {
	//
	var (
		errors []SyntaxError
		//
		sources = make([]register.Refs, len(decl.Sources))
		targets = make([]register.Refs, 1)
		//
		sourceContext ast.Context
		sourceTerms   = make([]hir.Term, len(decl.Sources))
		// Lookup target register info
		targetPath = path.Extend(decl.Target.Name())
		targetId   = t.env.RegisterOf(targetPath)
		target     = t.env.Register(targetId)
	)
	// Determine source context
	for _, source := range decl.Sources {
		sourceBinding := source.Binding().(*ast.ColumnBinding)
		sourceContext = sourceContext.Join(sourceBinding.Context())
	}
	// Determine enclosing tgtModule
	tgtModule := t.moduleOf(target.Context)
	srcModule := t.moduleOf(sourceContext)
	// Determine source register refs
	for i, source := range decl.Sources {
		ith, errs := t.registerOfRegisterAccess(source, 0)
		//
		if len(errs) == 0 {
			sources[i] = register.NewRefs(srcModule.Id(), ith.Register())
			sourceTerms[i] = ith
		}
		//
		errors = append(errors, errs...)
	}
	// Determine target register refs
	targets[0] = register.NewRefs(tgtModule.Id(), t.registerIndexOf(targetPath))
	targetTerm := t.registerOf(targetPath, 0)
	// Register constraint
	tgtModule.AddConstraint(
		hir.NewInterleavingConstraint("", tgtModule.Id(), srcModule.Id(), targetTerm, sourceTerms),
	)
	// Register assignment
	tgtModule.AddAssignment(
		assignment.NewNativeComputation[word.BigEndian]("interleave", targets, sources))

	// Done
	return errors
}

// Translate a "defpermutation" declaration.
func (t *translator) translateDefPermutation(decl *ast.DefPermutation, path file.Path) []SyntaxError {
	//
	var (
		context     ast.Context = ast.VoidContext()
		targets                 = make([]register.Id, len(decl.Sources))
		targetTerms             = make([]hir.Term, len(decl.Sources))
		sources                 = make([]register.Id, len(decl.Sources))
		handle      strings.Builder
	)
	//
	for i := range decl.Sources {
		targetPath := path.Extend(decl.Targets[i].Name())
		targets[i] = t.registerIndexOf(targetPath)
		targetTerms[i] = t.registerOf(targetPath, 0)
		//
		target := t.env.Register(t.env.RegisterOf(targetPath))
		sourceBinding := decl.Sources[i].Binding().(*ast.ColumnBinding)
		sources[i] = t.registerIndexOf(&sourceBinding.Path)
		// Join contexts
		context = context.Join(target.Context)
		// Construct handle
		if i >= len(decl.Signs) {
			// No nothing
		} else if decl.Signs[i] {
			handle.WriteString("+")
		} else {
			handle.WriteString("-")
		}
		//
		handle.WriteString(target.Name())
	}

	if context.IsConflicted() || context.IsVoid() {
		return t.srcmap.SyntaxErrors(decl, "conflicting (or void) constraint context")
	}
	//
	module := t.moduleOf(context)
	// Clone the signs
	signs := slices.Clone(decl.Signs)
	bitwidth := determineMaxBitwidth(module, targetTerms[:len(signs)])
	// Add assignment for computing the sorted permutation
	module.AddAssignment(assignment.NewSortedPermutation[word.BigEndian](
		toRegisterRefs(module.Id(), targets), signs, toRegisterRefs(module.Id(), sources)))
	// Add Permutation Constraint
	module.AddConstraint(hir.NewPermutationConstraint(handle.String(), module.Id(), targets, sources))
	// Add Sorting Constraint
	module.AddConstraint(
		hir.NewSortedConstraint(handle.String(), module.Id(), bitwidth, util.None[hir.Term](), targetTerms, signs, false))
	//
	return nil
}

// Translate a "defproperty" declaration.
func (t *translator) translateDefProperty(decl *ast.DefProperty) []SyntaxError {
	module := t.moduleOf(decl.Assertion.Context())
	// Translate constraint body
	assertion, errors := t.translateLogical(decl.Assertion, module, 0)
	//
	if len(errors) == 0 {
		comp := term.NewLogicalComputation[word.BigEndian, hir.LogicalTerm, hir.Term](assertion)
		// Add translated constraint
		module.AddConstraint(hir.NewAssertion(decl.Handle, module.Id(), decl.Domain, comp))
	}
	// Done
	return errors
}

// Translate a "defsorted" declaration.
func (t *translator) translateDefSorted(decl *ast.DefSorted) []SyntaxError {
	var (
		selector util.Option[hir.Term]
		// Determine source context
		context, _ = ast.ContextOfExpressions(decl.Sources...)
		//
		module = t.moduleOf(context)
	)

	// Translate source expressions
	sources, errors := t.translateUnitExpressions(decl.Sources, module, 0)
	// Translate (optional) selector expression
	if decl.Selector.HasValue() {
		sel, errs := t.translateExpression(decl.Selector.Unwrap(), module, 0)
		selector = util.Some(sel)
		//
		errors = append(errors, errs...)
	}
	// Create construct (assuming no errors thus far)
	if len(errors) == 0 {
		// Clone the signs
		signs := slices.Clone(decl.Signs)
		bitwidth := determineMaxBitwidth(module, sources[:len(signs)])
		// Add translated constraint
		module.AddConstraint(
			hir.NewSortedConstraint(decl.Handle, module.Id(), bitwidth, selector, sources, signs, decl.Strict))
	}
	// Done
	return errors
}

// Translate an optional expression in a given context.  That is an expression
// which maybe nil (i.e. doesn't exist).  In such case, nil is returned (i.e.
// without any errors).
func (t *translator) translateUnitExpressions(exprs []ast.Expr, module ModuleBuilder,
	shift int) ([]hir.Term, []SyntaxError) {
	//
	errors := []SyntaxError{}
	hirExprs := make([]hir.Term, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		if e != nil {
			var errs []SyntaxError
			//
			expr, errs := t.translateExpression(e, module, shift)
			errors = append(errors, errs...)
			hirExprs[i] = expr
		}
	}
	// Done
	return hirExprs, errors
}

// Translate a sequence of zero or more expressions enclosed in a given module.
func (t *translator) translateExpressions(module ModuleBuilder, shift int,
	exprs ...ast.Expr) ([]hir.Term, []SyntaxError) {
	//
	errors := []SyntaxError{}
	nexprs := make([]hir.Term, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		if e != nil {
			var errs []SyntaxError
			//
			nexprs[i], errs = t.translateExpression(e, module, shift)
			errors = append(errors, errs...)
		} else {
			// Strictly speaking, this assignment is unnecessary.  However, the
			// purpose is just to make it clear what's going on.
			nexprs[i] = nil
		}
	}
	//
	return nexprs, errors
}

// Translate an optional expression in a given context.  That is an expression
// which maybe nil (i.e. doesn't exist).  In such case, nil is returned (i.e.
// without any errors).
func (t *translator) translateOptionalExpression(expr ast.Expr, module ModuleBuilder,
	shift int) (hir.Term, []SyntaxError) {
	//
	if expr != nil {
		return t.translateExpression(expr, module, shift)
	}

	return nil, nil
}

// Translate an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for register access, function
// invocations, etc).
func (t *translator) translateExpression(expr ast.Expr, module ModuleBuilder, shift int) (hir.Term, []SyntaxError) {
	switch e := expr.(type) {
	case *ast.ArrayAccess:
		// Lookup underlying register info
		return t.registerOfRegisterAccess(e, shift)
	case *ast.Add:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Sum(args...), errs
	case *ast.Cast:
		arg, errs := t.translateExpression(e.Arg, module, shift)
		//
		if !e.Unsafe {
			// safe casts are compiled out since they have already been checked
			// by the type checker.
			return arg, errs
		} else if intType, ok := e.Type.(*ast.IntType); ok {
			// unsafe casts cannot be checked by the type checker, but can be
			// exploited for the purposes of optimisation.
			return term.CastOf(arg, intType.BitWidth()), errs
		}
		// Should be unreachable.
		msg := fmt.Sprintf("cannot translate cast (%s)", e.Type.String())
		//
		return nil, t.srcmap.SyntaxErrors(expr, msg)
	case *ast.Constant:
		if e.Val.Sign() < 0 {
			// NOTE: this can be supported by including a sign within the
			// ir.Const datatype.  That is by far and away the best way to
			// manage this.  Do no, under any circumstance, allow negative big
			// integers.
			panic("signed constant encountered")
		}
		// Initialise field from bigint
		val := field.BigInt[word.BigEndian](e.Val)
		//
		return term.Const[word.BigEndian, hir.Term](val), nil
	case *ast.Exp:
		return t.translateExp(e, module, shift)
	case *ast.If:
		return t.translateIf(e, module, shift)
	case *ast.Mul:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Product(args...), errs
	case *ast.Normalise:
		arg, errs := t.translateExpression(e.Arg, module, shift)
		return term.Normalise(arg), errs
	case *ast.Sub:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Subtract(args...), errs
	case *ast.Shift:
		return t.translateShift(e, module, shift)
	case *ast.VariableAccess:
		return t.translateVariableAccess(e, shift)
	case *ast.Concat:
		return t.translateConcat(e, module, shift)
	default:
		typeStr := reflect.TypeOf(expr).String()
		msg := fmt.Sprintf("unknown arithmetic expression encountered during translation (%s)", typeStr)
		//
		return nil, t.srcmap.SyntaxErrors(expr, msg)
	}
}

func (t *translator) translateConcat(expr *ast.Concat, mod ModuleBuilder, shift int) (hir.Term, []SyntaxError) {
	var (
		limbs  []*hir.RegisterAccess = make([]*hir.RegisterAccess, len(expr.Args))
		errors []SyntaxError
	)
	//
	for i, v := range expr.Args {
		var (
			ith, errs = t.translateExpression(v, mod, shift)
		)
		// Sanity check it was a real register access
		if ra, ok := ith.(*hir.RegisterAccess); ok {
			limbs[i] = ra
		} else if len(errs) == 0 {
			errors = append(errors, *t.srcmap.SyntaxError(v, "invalid register access"))
		}
		//
		errors = append(errors, errs...)
	}
	//
	return term.NewVectorAccess(limbs), errors
}

func (t *translator) translateExp(expr *ast.Exp, module ModuleBuilder, shift int) (hir.Term, []SyntaxError) {
	arg, errs := t.translateExpression(expr.Arg, module, shift)
	pow := expr.Pow.AsConstant()
	// Identity constant for pow
	if pow == nil {
		errs = append(errs, *t.srcmap.SyntaxError(expr.Pow, "expected constant power"))
	} else if !pow.IsUint64() {
		errs = append(errs, *t.srcmap.SyntaxError(expr.Pow, "constant power too large"))
	}
	// Sanity check errors
	if len(errs) == 0 {
		return term.Exponent(arg, pow.Uint64()), errs
	}
	//
	return nil, errs
}

func (t *translator) translateIf(expr *ast.If, module ModuleBuilder, shift int) (hir.Term, []SyntaxError) {
	// Translate condition as a logical
	cond, condErrs := t.translateLogical(expr.Condition, module, shift)
	// Translate optional true / false branches
	args, argErrs := t.translateExpressions(module, shift, expr.TrueBranch, expr.FalseBranch)
	//
	errs := append(condErrs, argErrs...)
	//
	if len(errs) > 0 {
		return nil, errs
	}
	// Propagate emptiness (if applicable)
	if args[0] == nil && args[1] == nil {
		return nil, nil
	}
	// Construct appropriate if form
	return term.IfElse(cond, args[0], args[1]), nil
}

func (t *translator) translateShift(expr *ast.Shift, mod ModuleBuilder, shift int) (hir.Term, []SyntaxError) {
	constant := expr.Shift.AsConstant()
	// Determine the shift constant
	if constant == nil {
		return nil, t.srcmap.SyntaxErrors(expr.Shift, "expected constant shift")
	} else if !constant.IsInt64() {
		return nil, t.srcmap.SyntaxErrors(expr.Shift, "constant shift too large")
	}
	// Now translate target expression with updated shift.
	return t.translateExpression(expr.Arg, mod, shift+int(constant.Int64()))
}

func (t *translator) translateVariableAccess(expr *ast.VariableAccess, shift int) (hir.Term, []SyntaxError) {
	if _, ok := expr.Binding().(*ast.ColumnBinding); ok {
		return t.registerOfVariableAccess(expr, shift)
	} else if binding, ok := expr.Binding().(*ast.ConstantBinding); ok {
		// Initialise field from bigint
		constant := field.BigInt[word.BigEndian](*binding.Value.AsConstant())
		// Handle externalised constants slightly differently.
		if binding.Extern {
			//
			return term.LabelledConstant[word.BigEndian, hir.Term](binding.Path.String(), constant), nil
		}
		//
		return term.Const[word.BigEndian, hir.Term](constant), nil
	}
	// error
	return nil, t.srcmap.SyntaxErrors(expr, "unbound variable")
}

// Translate a sequence of zero or more logical expressions enclosed in a given module.
func (t *translator) translateLogicals(module ModuleBuilder, shift int,
	exprs ...ast.Expr) ([]hir.LogicalTerm, []SyntaxError) {
	//
	errors := []SyntaxError{}
	logicals := make([]hir.LogicalTerm, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		var errs []SyntaxError
		//
		logicals[i], errs = t.translateLogical(e, module, shift)
		errors = append(errors, errs...)
	}
	//
	return logicals, errors
}

// Translate an optional expression in a given context.  That is an expression
// which maybe nil (i.e. doesn't exist).  In such case, nil is returned (i.e.
// without any errors).
func (t *translator) translateOptionalLogical(expr ast.Expr, module ModuleBuilder,
	shift int) (hir.LogicalTerm, []SyntaxError) {
	//
	if expr != nil {
		return t.translateLogical(expr, module, shift)
	}

	return nil, nil
}

// Translate an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for register access, function
// invocations, etc).
func (t *translator) translateLogical(expr ast.Expr, mod ModuleBuilder, shift int) (hir.LogicalTerm, []SyntaxError) {
	switch e := expr.(type) {
	case *ast.Cast:
		if e.Type != ast.BOOL_TYPE {
			// This should be unreachable, since type checking should have
			// caught this already.  However, potentially, issues with the
			// preprocessor could result in some weird scenario.
			panic("malformed logical expression")
		}
		// Just ignore
		return t.translateLogical(e.Arg, mod, shift)
	case *ast.Connective:
		args, errs := t.translateLogicals(mod, shift, e.Args...)
		//
		if e.Sign {
			return term.Disjunction(args...), errs
		}
		//
		return term.Conjunction(args...), errs
	case *ast.Equation:
		lhs, errs1 := t.translateExpression(e.Lhs, mod, shift)
		rhs, errs2 := t.translateExpression(e.Rhs, mod, shift)
		errs := append(errs1, errs2...)
		//
		if len(errs) > 0 {
			return nil, errs
		}
		//
		switch e.Kind {
		case ast.EQUALS:
			return term.Equals[word.BigEndian, hir.LogicalTerm](lhs, rhs), nil
		case ast.NOT_EQUALS:
			return term.NotEquals[word.BigEndian, hir.LogicalTerm](lhs, rhs), nil
		default:
			panic("unreachable")
		}
	case *ast.If:
		return t.translateIte(e, mod, shift)
	case *ast.List:
		args, errs := t.translateLogicals(mod, shift, e.Args...)
		// Sanity check void
		if len(args) == 0 {
			return nil, errs
		}
		//
		return term.Conjunction(args...), errs
	case *ast.Not:
		arg, errs := t.translateLogical(e.Arg, mod, shift)
		return term.Negation(arg), errs
	case *ast.Shift:
		return t.translateLogicalShift(e, mod, shift)
	default:
		typeStr := reflect.TypeOf(expr).String()
		msg := fmt.Sprintf("unknown logical expression encountered during translation (%s)", typeStr)
		//
		return nil, t.srcmap.SyntaxErrors(expr, msg)
	}
}

func (t *translator) translateIte(expr *ast.If, module ModuleBuilder, shift int) (hir.LogicalTerm, []SyntaxError) {
	// Translate condition as a logical
	cond, errs := t.translateLogical(expr.Condition, module, shift)
	// Translate optional true / false branches
	truebranch, trueErrs := t.translateOptionalLogical(expr.TrueBranch, module, shift)
	// Translate optional true / false branches
	falsebranch, falseErrs := t.translateOptionalLogical(expr.FalseBranch, module, shift)
	//
	errs = append(errs, trueErrs...)
	errs = append(errs, falseErrs...)
	//
	if len(errs) > 0 {
		return nil, errs
	}
	// Propagate emptiness (if applicable)
	if truebranch == nil && falsebranch == nil {
		return nil, nil
	}
	// Construct appropriate if form
	return term.IfThenElse(cond, truebranch, falsebranch), nil
}

func (t *translator) translateLogicalShift(expr *ast.Shift, mod ModuleBuilder,
	shift int) (hir.LogicalTerm, []SyntaxError) {
	//
	constant := expr.Shift.AsConstant()
	// Determine the shift constant
	if constant == nil {
		return nil, t.srcmap.SyntaxErrors(expr.Shift, "expected constant shift")
	} else if !constant.IsInt64() {
		return nil, t.srcmap.SyntaxErrors(expr.Shift, "constant shift too large")
	}
	// Now translate target expression with updated shift.
	return t.translateLogical(expr.Arg, mod, shift+int(constant.Int64()))
}

// Determine the underlying register for a symbol which represents a register access.
func (t *translator) registerOfRegisterAccess(symbol ast.Symbol, shift int) (*hir.RegisterAccess, []SyntaxError) {
	switch e := symbol.(type) {
	case *ast.ArrayAccess:
		return t.registerOfArrayAccess(e, shift)
	case *ast.VariableAccess:
		return t.registerOfVariableAccess(e, shift)
	}
	//
	return nil, t.srcmap.SyntaxErrors(symbol, "invalid register access")
}

func (t *translator) registerOfVariableAccess(expr *ast.VariableAccess,
	shift int) (*hir.RegisterAccess, []SyntaxError) {
	//
	if binding, ok := expr.Binding().(*ast.ColumnBinding); ok {
		// Lookup register binding
		return t.registerOf(binding.AbsolutePath(), shift), nil
	}
	//
	return nil, t.srcmap.SyntaxErrors(expr, "invalid register access")
}

func (t *translator) registerOfArrayAccess(expr *ast.ArrayAccess, shift int) (*hir.RegisterAccess, []SyntaxError) {
	var (
		errors []SyntaxError
		min    uint = 0
		max    uint = math.MaxUint
	)
	// Lookup the register
	binding, ok := expr.Binding().(*ast.ColumnBinding)
	// Did we find it?
	if !ok {
		errors = append(errors, *t.srcmap.SyntaxError(expr.Arg, "invalid array index encountered during translation"))
	} else if arrType, ok := binding.DataType.(*ast.ArrayType); ok {
		min = arrType.MinIndex()
		max = arrType.MaxIndex()
	}
	// Array index should be statically known
	index := expr.Arg.AsConstant()
	//
	if index == nil {
		errors = append(errors, *t.srcmap.SyntaxError(expr.Arg, "expected constant array index"))
	} else if i := uint(index.Uint64()); i < min || i > max {
		errors = append(errors, *t.srcmap.SyntaxError(expr.Arg, "array index out-of-bounds"))
	}
	// Error check
	if len(errors) > 0 {
		return nil, errors
	}
	// Construct real register name
	path := &binding.Path
	name := fmt.Sprintf("%s_%d", path.Tail(), index.Uint64())
	path = path.Parent().Extend(name)
	//
	return t.registerOf(path, shift), errors
}

// Determine the appropriate name for a given module based on a module context.
func (t *translator) moduleOf(context ast.Context) ModuleBuilder {
	if context.IsVoid() {
		// NOTE: the intuition behind the choice to return nil here is allow for
		// situations where there is no context (e.g. constant expressions,
		// etc).  As such, return nil is safe as, for such expressions, the
		// module should never be accessed during their translation.
		return nil
	}
	//
	return t.schema.ModuleOf(context.ModuleName())
}

// Map columns to appropriate module register identifiers.
func (t *translator) registerOf(path *file.Path, shift int) *hir.RegisterAccess {
	// Determine register id
	rid := t.env.RegisterOf(path)
	//
	reg := t.env.Register(rid)
	// Lookup corresponding module builder
	module := t.moduleOf(reg.Context)
	//
	return RegisterAccessOf(module, reg.Name(), shift)
}

// Map columns to appropriate module register identifiers.
func (t *translator) registerIndexOf(path *file.Path) register.Id {
	// Determine register id
	rid := t.env.RegisterOf(path)
	//
	reg := t.env.Register(rid)
	// Lookup corresponding module builder
	module := t.moduleOf(reg.Context)
	//
	if rid, ok := module.HasRegister(reg.Name()); ok {
		return rid
	}
	//
	panic("unreachable")
}

func (t *translator) registerRefsOf(path *file.Path) register.Refs {
	// Determine register id
	rid := t.env.RegisterOf(path)
	//
	reg := t.env.Register(rid)
	// Lookup corresponding module builder
	module := t.moduleOf(reg.Context)
	//
	if rid, ok := module.HasRegister(reg.Name()); ok {
		return register.NewRefs(module.Id(), rid)
	}
	//
	panic("unreachable")
}

// RegisterAccessOf returns a register accessor for the register with the given name.
func RegisterAccessOf(module register.Map, name string, shift int) *hir.RegisterAccess {
	// Lookup register associated with this name
	var (
		rid, _ = module.HasRegister(name)
		reg    = module.Register(rid)
	)
	//
	return term.RawRegisterAccess[word.BigEndian, hir.Term](rid, reg.Width(), shift)
}

func toRegisterRefs(context schema.ModuleId, ids []register.Id) []register.Ref {
	var refs = make([]register.Ref, len(ids))
	//
	for i, id := range ids {
		refs[i] = register.NewRef(context, id)
	}
	//
	return refs
}

func determineMaxBitwidth(module ModuleBuilder, sources []hir.Term) uint {
	// Sanity check bitwidth
	bitwidth := uint(0)
	//
	for _, e := range sources {
		// Determine bitwidth of nth term
		switch e := e.(type) {
		case *term.RegisterAccess[word.BigEndian, hir.Term]:
			reg := module.Register(e.Register())
			//
			if reg.Width() > bitwidth {
				bitwidth = reg.Width()
			}
		default:
			// For now, we only supports simple column accesses.
			panic("bitwidth calculation only supported for column accesses")
		}
	}
	//
	return bitwidth
}
