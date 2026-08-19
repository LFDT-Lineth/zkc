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
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Config encapsulates various options which can affect compilation.
type Config struct {
	// Enforce all types by default
	EnforceTypes bool
	// Enforce types for all limbs arising from splitting registers
	EnforceLimbTypes bool
	// Target field configuration.  This is only used to assist in reporting
	// errors which are specific to the given field configuration.
	Field field.Config
}

// SchemaBuilder is used within this translator for building the final MIR
// schema.
type SchemaBuilder = mir.SchemaBuilder[word.BigEndian]

// ModuleBuilder is used within this translator for building the various modules
// which are contained within the MIR schema.
type ModuleBuilder = mir.ModuleBuilder[word.BigEndian]

// mirTerm provides a convenient shorthand for the arithmetic terms produced by
// this translator.
type mirTerm = mir.Term[word.BigEndian]

// mirLogicalTerm provides a convenient shorthand for the logical terms produced
// by this translator.
type mirLogicalTerm = mir.LogicalTerm[word.BigEndian]

// mirRegisterAccess provides a convenient shorthand for a register access, as
// required (for example) by lookup and range constraints.
type mirRegisterAccess = mir.RegisterAccess[word.BigEndian]

// equateZero constructs the logical term which holds when the given term
// evaluates to zero.  This is used for guards and perspective selectors, both of
// which are "active" exactly when non-zero.  A nil term signals that translation
// failed and, hence, nil is returned (with the corresponding errors having been
// reported already).
func equateZero(value mirTerm) mirLogicalTerm {
	if value == nil {
		return nil
	}
	//
	zero := term.Const64[word.BigEndian, mirTerm](0)
	//
	return term.Equals[word.BigEndian, mirLogicalTerm](value, zero)
}

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
	config Config) (mir.Schema[word.BigEndian], []SyntaxError) {
	//
	builder := ir.NewSchemaBuilder[word.BigEndian, mir.Constraint[word.BigEndian], mirTerm]()
	t := translator{env, srcmap, builder, config}
	// Allocate all modules into schema
	t.translateModules(circuit)
	// Translate everything else
	if errs := t.translateDeclarations(circuit); len(errs) > 0 {
		return mir.Schema[word.BigEndian]{}, errs
	}
	// Build concrete modules from schema
	modules := ir.BuildSchema[mir.Module[word.BigEndian]](t.schema)
	// Finally, construct the asm program
	return schema.NewUniformSchema(modules), nil
}

// Translator packages up information necessary for translating a circuit into
// the schema form required for the MIR level.
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

// Translate the given Corset module into its corresponding HIR module.
func (t *translator) translateModule(name string) {
	// Always include module (even if empty).
	t.schema.NewModule(name, true, true, false, false, false, false)
	// Process each register in turn.
	for _, regIndex := range t.env.RegistersOf(name) {
		var (
			// Identify register info
			regInfo = t.env.Register(regIndex)
			// Identify enclosing MIR module
			module = t.schema.ModuleOf(regInfo.Context.ModuleName())
			//
			reg register.Register
		)
		// Declare corresponding register
		if regInfo.IsInput() {
			reg = register.NewInput(regInfo.Name(), regInfo.Bitwidth)
		} else {
			reg = register.NewComputed(regInfo.Name(), regInfo.Bitwidth)
		}
		// Add the register
		module.NewRegister(reg)
		// Add range constraints for underlying types (as necessary)
		t.translateTypeConstraints(*regInfo, module)
	}
}

// Translate the type constraint applicable for the given register (if any).
// This is required when the register's source column is marked provable, or
// when the compiler is configured to enforce types more broadly.
func (t *translator) translateTypeConstraints(reg Register, mod ModuleBuilder) {
	var (
		regWidth = reg.Bitwidth
		required = reg.MustProve || t.config.EnforceTypes ||
			(t.config.EnforceLimbTypes && regWidth > t.config.Field.RegisterWidth)
	)
	// Apply provability (if it is required)
	if required {
		// Determine register being constrained
		rid, _ := mod.HasRegister(reg.Name())
		// Add appropriate type constraint
		constraint := mir.NewRangeConstraint[word.BigEndian](reg.Name(), mod.Id(),
			[]register.Id{rid}, []uint{reg.Bitwidth})
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
	case *ast.DefColumns:
		// Not an assignment or a constraint, hence ignore.
	case *ast.DefConst:
		// For now, constants are always compiled out when going down to MIR.
	case *ast.DefConstraint:
		errors = t.translateDefConstraint(d)
	case *ast.DefInRange:
		errors = t.translateDefInRange(d)
	case *ast.DefLookup:
		errors = t.translateDefLookup(d)
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// Translate a "defconstraint" declaration.
func (t *translator) translateDefConstraint(decl *ast.DefConstraint) []SyntaxError {
	var (
		module = t.moduleOf(decl.Constraint.Context())
		// Translate expr body
		expr, errors = t.translateLogical(decl.Constraint, module, 0)
	)
	// NOTE: a nil expression indicates translation failed, and the
	// corresponding errors have already been reported.
	if expr == nil {
		return errors
	}
	// Apply guard (if applicable)
	if decl.Guard != nil {
		// Translate (optional) guard
		gexpr, guardErrors := t.translateExpression(decl.Guard, module, 0)
		guard := equateZero(gexpr)
		expr = term.IfThenElse(guard, nil, expr)
		// Combine errors
		errors = append(errors, guardErrors...)
	}
	// Sanity check
	if len(errors) == 0 {
		// Add translated constraint
		module.AddConstraint(mir.NewVanishingConstraint(decl.Handle, module.Id(), decl.Domain, expr))
	}
	// Done
	return errors
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
		module.AddConstraint(mir.NewLookupConstraint[word.BigEndian](decl.Handle, targets, sources))
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
		sel        *mirRegisterAccess
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
func (t *translator) translateLookupColumns(columns []ast.TypedSymbol) ([]*mirRegisterAccess, []SyntaxError) {
	var (
		errors   []SyntaxError
		accesses = make([]*mirRegisterAccess, len(columns))
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
func registersOfAccesses(accesses []*mirRegisterAccess) ([]register.Id, []uint) {
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
func (t *translator) checkLookupVector(sel *mirRegisterAccess, selector ast.TypedSymbol) []SyntaxError {
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
	var module = t.moduleOf(ast.ContextOfSymbol(decl.Column))
	// Translate constrained column.  NOTE: no sanity check on the sign of the
	// resulting register is required, since registers are unsigned by
	// construction.
	access, errors := t.registerOfRegisterAccess(decl.Column, 0)
	//
	if len(errors) == 0 {
		// Add translated constraint
		module.AddConstraint(mir.NewRangeConstraint[word.BigEndian]("", module.Id(),
			[]register.Id{access.Register()}, []uint{decl.Bitwidth}))
	}
	// Done
	return errors
}

// Translate a sequence of zero or more expressions enclosed in a given module.
func (t *translator) translateExpressions(module ModuleBuilder, shift int,
	exprs ...ast.Expr) ([]mirTerm, []SyntaxError) {
	//
	errors := []SyntaxError{}
	nexprs := make([]mirTerm, len(exprs))
	// Iterate each expression in turn
	for i, e := range exprs {
		var errs []SyntaxError
		//
		nexprs[i], errs = t.translateExpression(e, module, shift)
		errors = append(errors, errs...)
	}
	//
	return nexprs, errors
}

// Translate an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for register access, function
// invocations, etc).
func (t *translator) translateExpression(expr ast.Expr, module ModuleBuilder, shift int) (mirTerm, []SyntaxError) {
	switch e := expr.(type) {
	case *ast.ArrayAccess:
		// Lookup underlying register info.  NOTE: a nil access signals that
		// translation failed and, hence, the errors have been reported already.
		access, errs := t.registerOfRegisterAccess(e, shift)
		if access == nil {
			return nil, errs
		}
		//
		return access, errs
	case *ast.Add:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Sum(args...), errs
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
		return term.Const[word.BigEndian, mirTerm](val), nil
	case *ast.If:
		// NOTE: conditionals are only permitted at the logical level, since MIR
		// arithmetic terms cannot contain them.
		return nil, t.srcmap.SyntaxErrors(expr, "conditional not permitted in arithmetic expression")
	case *ast.Mul:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Product(args...), errs
	case *ast.Sub:
		args, errs := t.translateExpressions(module, shift, e.Args...)
		return term.Subtract(args...), errs
	case *ast.Shift:
		return t.translateShift(e, module, shift)
	case *ast.VariableAccess:
		return t.translateVariableAccess(e, shift)
	default:
		typeStr := reflect.TypeOf(expr).String()
		msg := fmt.Sprintf("unknown arithmetic expression encountered during translation (%s)", typeStr)
		//
		return nil, t.srcmap.SyntaxErrors(expr, msg)
	}
}

func (t *translator) translateShift(expr *ast.Shift, mod ModuleBuilder, shift int) (mirTerm, []SyntaxError) {
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

func (t *translator) translateVariableAccess(expr *ast.VariableAccess, shift int) (mirTerm, []SyntaxError) {
	if _, ok := expr.Binding().(*ast.ColumnBinding); ok {
		// NOTE: a nil access signals that translation failed and, hence, the
		// errors have been reported already.
		access, errs := t.registerOfVariableAccess(expr, shift)
		if access == nil {
			return nil, errs
		}
		//
		return access, errs
	} else if binding, ok := expr.Binding().(*ast.ConstantBinding); ok {
		// Initialise field from bigint
		constant := field.BigInt[word.BigEndian](*binding.Value.AsConstant())
		//
		return term.Const[word.BigEndian, mirTerm](constant), nil
	}
	// error
	return nil, t.srcmap.SyntaxErrors(expr, "unbound variable")
}

// Translate a sequence of zero or more logical expressions enclosed in a given module.
func (t *translator) translateLogicals(module ModuleBuilder, shift int,
	exprs ...ast.Expr) ([]mirLogicalTerm, []SyntaxError) {
	//
	errors := []SyntaxError{}
	logicals := make([]mirLogicalTerm, len(exprs))
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
	shift int) (mirLogicalTerm, []SyntaxError) {
	//
	if expr != nil {
		return t.translateLogical(expr, module, shift)
	}

	return nil, nil
}

// Translate an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for register access, function
// invocations, etc).
func (t *translator) translateLogical(expr ast.Expr, mod ModuleBuilder, shift int) (mirLogicalTerm, []SyntaxError) {
	switch e := expr.(type) {
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
			return term.Equals[word.BigEndian, mirLogicalTerm](lhs, rhs), nil
		case ast.NOT_EQUALS:
			return term.NotEquals[word.BigEndian, mirLogicalTerm](lhs, rhs), nil
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

func (t *translator) translateIte(expr *ast.If, module ModuleBuilder, shift int) (mirLogicalTerm, []SyntaxError) {
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
	shift int) (mirLogicalTerm, []SyntaxError) {
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
func (t *translator) registerOfRegisterAccess(symbol ast.Symbol, shift int) (*mirRegisterAccess, []SyntaxError) {
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
	shift int) (*mirRegisterAccess, []SyntaxError) {
	//
	if binding, ok := expr.Binding().(*ast.ColumnBinding); ok {
		// Lookup register binding
		return t.registerOf(binding.AbsolutePath(), shift), nil
	}
	//
	return nil, t.srcmap.SyntaxErrors(expr, "invalid register access")
}

func (t *translator) registerOfArrayAccess(expr *ast.ArrayAccess, shift int) (*mirRegisterAccess, []SyntaxError) {
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
func (t *translator) registerOf(path *file.Path, shift int) *mirRegisterAccess {
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
func RegisterAccessOf(module register.Map, name string, shift int) *mirRegisterAccess {
	// Lookup register associated with this name
	var (
		rid, _ = module.HasRegister(name)
		reg    = module.Register(rid)
	)
	//
	return term.RawRegisterAccess[word.BigEndian, mirTerm](rid, reg.Width(), shift)
}
