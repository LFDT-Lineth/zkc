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
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/hir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
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
	config Config) (hir.Schema, []SyntaxError) {
	//
	builder := ir.NewSchemaBuilder[word.BigEndian, hir.Constraint, hir.Term]()
	t := translator{env, srcmap, builder, config}
	// Allocate all modules into schema
	t.translateModules(circuit)
	// Translate everything else
	if errs := t.translateDeclarations(circuit); len(errs) > 0 {
		return hir.Schema{}, errs
	}
	// Build concrete modules from schema
	modules := ir.BuildSchema[hir.Module](t.schema)
	// Finally, construct the asm program
	return schema.NewUniformSchema(modules), nil
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
	t.schema.NewModule(module.NewName(name, 1), true, true, false, false, false, false, 0)
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
			t.schema.NewModule(moduleName, true, true, false, false, false, false, 0)
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
	case *ast.DefLookup:
		errors = t.translateDefLookup(d)
	case *ast.DefPerspective:
		// As for defregisters, nothing generated here.
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// Translate a "defconstraint" declaration.
func (t *translator) translateDefConstraint(decl *ast.DefConstraint) []SyntaxError {
	// NOTE: a nil constraint arises when the constraint expression consists
	// entirely of debug constraints, and debug mode is not enabled.  In such
	// case, there is no constraint to translate.
	if decl.Constraint == nil {
		return nil
	}
	//
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
		sources                []lookup.Vector
		targets                []lookup.Vector
		// Bitwidths of the registers making up each source / target vector.
		srcWidths [][]uint
		tgtWidths [][]uint
	)
	// Translate sources
	for i, ith := range decl.Targets {
		ith_targets, widths, ctx, errs := t.translateDefLookupSources(decl.TargetSelectors[i], ith)
		targets = append(targets, ith_targets)
		tgtWidths = append(tgtWidths, widths)
		errors = append(errors, errs...)
		//
		if i == 0 {
			tgtContext = ctx
		}
	}
	// Translate targets
	for i, ith := range decl.Sources {
		ith_sources, widths, ctx, errs := t.translateDefLookupSources(decl.SourceSelectors[i], ith)
		sources = append(sources, ith_sources)
		srcWidths = append(srcWidths, widths)
		errors = append(errors, errs...)
		//
		if i == 0 {
			srcContext = ctx
		}
	}
	// Sanity check this is not an irregular lookup (since these are not
	// currently supported) and, if so, provide a useful error message.
	if len(errors) == 0 {
		errors = t.checkForIrregularLookup(tgtWidths, srcWidths, decl.Targets, decl.Sources)
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

// translateDefLookupSources translates one side of a lookup (i.e. either its
// sources or its targets) into a corresponding lookup vector.  Since a lookup
// vector is made up of registers, this also returns the bitwidth of each
// register in the vector (as needed for detecting irregular lookups).
func (t *translator) translateDefLookupSources(selector ast.TypedSymbol,
	sources []ast.TypedSymbol) (lookup.Vector, []uint, ast.Context, []SyntaxError) {
	// Determine context of ith set of targets
	var (
		context, j = ast.ContextOfSymbols(sources...)
		sel        *hir.RegisterAccess
	)
	// Include selector (when present)
	if selector != nil {
		context = context.Join(ast.ContextOfSymbol(selector))
	}
	// Translate target columns whilst again checking for a conflicting context.
	if context.IsConflicted() {
		var source ast.TypedSymbol
		// Determine offending source column
		if j >= uint(len(sources)) {
			source = selector
		} else {
			source = sources[j]
		}
		//
		return lookup.Vector{}, nil, context, t.srcmap.SyntaxErrors(source, "conflicting context")
	}
	// Determine enclosing module
	module := t.moduleOf(context)
	// Translate source columns
	accesses, errors := t.translateLookupColumns(sources)
	// handle selector
	if selector != nil {
		s, errs := t.registerOfRegisterAccess(selector, 0)
		errors = append(errors, errs...)
		sel = s
	}
	// Sanity check vector
	if len(errors) == 0 {
		// NOTE: don't check vector if other errors, since we could have nil
		// entries in the vector, etc.
		errors = append(errors, t.checkLookupVector(sel, selector)...)
	}
	// Don't attempt to construct the vector if anything went wrong, since some
	// of its registers may not have been translated.
	if len(errors) > 0 {
		return lookup.Vector{}, nil, context, errors
	}
	//
	registers, widths := registersOfAccesses(accesses)
	// Done
	if sel != nil {
		return lookup.FilteredVector(module.Id(), sel.Register(), registers...), widths, context, errors
	}
	//
	return lookup.UnfilteredVector(module.Id(), registers...), widths, context, errors
}

// translateLookupColumns translates the column accesses making up one side of a
// lookup into their corresponding register accesses.
func (t *translator) translateLookupColumns(columns []ast.TypedSymbol) ([]*hir.RegisterAccess, []SyntaxError) {
	var (
		errors   []SyntaxError
		accesses = make([]*hir.RegisterAccess, len(columns))
	)
	//
	for i, column := range columns {
		if column == nil {
			continue
		}
		//
		reg, errs := t.registerOfRegisterAccess(column, 0)
		errors = append(errors, errs...)
		accesses[i] = reg
	}
	//
	return accesses, errors
}

// registersOfAccesses determines the registers underlying a given set of
// register accesses, along with the bitwidth of each.
func registersOfAccesses(accesses []*hir.RegisterAccess) ([]register.Id, []uint) {
	var (
		registers = make([]register.Id, len(accesses))
		widths    = make([]uint, len(accesses))
	)
	//
	for i, access := range accesses {
		valrange := access.ValueRange()
		registers[i] = access.Register()
		widths[i], _ = valrange.BitWidth()
	}
	//
	return registers, widths
}

// checkLookupVector sanity checks one side of a lookup.  Since the registers
// making up a lookup vector are unsigned by construction, all that remains is
// to check the selector (when present) is binary.
func (t *translator) checkLookupVector(sel *hir.RegisterAccess, selector ast.TypedSymbol) []SyntaxError {
	//
	var errors []SyntaxError
	// Check selector is binary
	if sel != nil {
		// Determine value range of the selector
		valrange := sel.ValueRange()
		// Determine bitwidth for that range
		bitwidth, _ := valrange.BitWidth()
		// Check for non-binary selector
		if bitwidth > 1 {
			errors = append(errors, *t.srcmap.SyntaxError(selector, "non-binary selector encountered"))
		}
	}
	// Done
	return errors
}

// An irregular lookup is an awkward scenario where a source/target pairing does
// not align properly.  This scenario is not currently supported and, hence, a
// suitable error message must be returned.  For example, support a pairing of
// u160 (source) into u256 (target) with a maximum register size of u160.  Then,
// the source will decompose into a single u160 limb, whilst the target will
// decompose into a two u128 limbs.
func (t *translator) checkForIrregularLookup(tgtWidths [][]uint, srcWidths [][]uint,
	tgtTerms [][]ast.TypedSymbol, srcTerms [][]ast.TypedSymbol) []SyntaxError {
	//
	var (
		n      = len(srcWidths[0])
		errors []SyntaxError
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
