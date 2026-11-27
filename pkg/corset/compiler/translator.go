// Copyright Consensys Incorporated
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
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/permutation"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// Config encapsulates various options which can affect compilation.
type Config struct {
	// Enforce all types by default
	EnforceTypes bool
}

// equateZero constructs the logical term which holds when the given term
// evaluates to zero.  This is used for guards and perspective selectors, both of
// which are "active" exactly when non-zero.  A nil term signals that translation
// failed and, hence, nil is returned (with the corresponding errors having been
// reported already).
func equateZero[F field.Element[F]](value mir.Term[F]) mir.LogicalTerm[F] {
	if value == nil {
		return nil
	}
	//
	zero := term.Const64[F, mir.Term[F]](0)
	//
	return term.Equals[F, mir.LogicalTerm[F]](value, zero)
}

// TranslateCircuit translates the components of a Corset circuit and add them
// to the schema.  By the time we get to this point, all malformed source files
// should have been rejected already and the translation should go through
// easily.  Thus, whilst syntax errors can be returned here, this should never
// happen.  The mechanism is supported, however, to simplify development of new
// features, etc.
func TranslateCircuit[F field.Element[F]](
	env Environment,
	srcmap *source.Maps[ast.Node],
	circuit *ast.Circuit,
	config Config) (mir.Schema[F], []SyntaxError) {
	//
	builder := ir.NewSchemaBuilder[F, mir.Term[F]]()
	t := translator[F]{env, srcmap, builder, config, make(map[string][]busPort[F]), nil}
	// Allocate all modules into schema
	t.translateModules(circuit)
	// Translate everything else
	if errs := t.translateDeclarations(circuit); len(errs) > 0 {
		return mir.Schema[F]{}, errs
	}
	// Merge send / receive ports into bus constraints, now all are known.
	if errs := t.translateBuses(); len(errs) > 0 {
		return mir.Schema[F]{}, errs
	}
	// Build concrete modules from schema
	modules := ir.BuildSchema[mir.Module[F]](t.schema)
	// Finally, construct the asm program
	return schema.NewUniformSchema(modules), nil
}

// Translator packages up information necessary for translating a circuit into
// the schema form required for the MIR level.
type translator[F field.Element[F]] struct {
	// Environment is needed for determining the identifiers for modules and
	// registers.
	env Environment
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
	// Represents the schema being constructed by this translator.
	schema mir.SchemaBuilder[F]
	// compilation configuration
	config Config
	// Ports declared by defsend / defrecv, keyed by bus name, pending the
	// per-bus merge (see translateBuses).
	busPorts map[string][]busPort[F]
	// Bus names in order of first declaration, for deterministic output.
	busOrder []string
}

// busPort is one translated defsend / defrecv declaration, awaiting the
// per-bus merge.
type busPort[F field.Element[F]] struct {
	// decl is the source declaration, for error reporting.
	decl *ast.DefSendReceive
	// module in which the port resides.
	module mir.ModuleBuilder[F]
	// port as it will appear in the bus constraint.
	port constraint.Vector
}

func (t *translator[F]) translateModules(circuit *ast.Circuit) {
	// Add root module
	t.translateModule("")
	// Add nested modules
	for _, m := range circuit.Modules {
		t.translateModule(m.Name)
	}
}

// Translate the given Corset module into its corresponding HIR module.
func (t *translator[F]) translateModule(name string) {
	var zero big.Int
	// Always include module (even if empty).
	t.schema.NewModule(name, true, false, false, false, false)
	// Process each register in turn.
	for _, regIndex := range t.env.RegistersOf(name) {
		var (
			// Identify register info
			regInfo = t.env.Register(regIndex)
			// Identify enclosing MIR module
			module = t.schema.ModuleOf(regInfo.Context.ModuleName())
			//
			reg register.Register[F]
			//
			padding F
		)
		// Initialise padding value
		padding = field.BigInt[F](regInfo.Padding.UnwrapOr(zero))
		// Declare corresponding register
		switch regInfo.Kind {
		case ast.INPUT_COLUMN, ast.UNSPECIFIED:
			reg = register.NewInput[F](regInfo.Name(), regInfo.Bitwidth, padding)
		case ast.OUTPUT_COLUMN:
			reg = register.NewOutput[F](regInfo.Name(), regInfo.Bitwidth, padding)
		case ast.COMPUTED_COLUMN:
			reg = register.NewComputed[F](regInfo.Name(), regInfo.Bitwidth, padding)
		case ast.CONSTANT_COLUMN:
			reg = register.NewConstant[F](regInfo.Name(), regInfo.Bitwidth, padding)
		default:
			panic("unknown register kind")
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
func (t *translator[F]) translateTypeConstraints(reg Register, mod mir.ModuleBuilder[F]) {
	var (
		required = (reg.MustProve || t.config.EnforceTypes) && reg.Kind != ast.CONSTANT_COLUMN
	)
	// Apply provability (if it is required)
	if required {
		// Determine register being constrained
		rid, _ := mod.HasRegister(reg.Name())
		// Add appropriate type constraint
		constraint := ranged.NewConstraint[F](reg.Name(), mod.Id(), rid, reg.Bitwidth)
		//
		mod.AddConstraint(constraint)
	}
}

// Translate all assignment or constraint declarations in the circuit.
func (t *translator[F]) translateDeclarations(circuit *ast.Circuit) []SyntaxError {
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
func (t *translator[F]) translateDeclarationsInModule(path file.Path, decls []ast.Declaration) []SyntaxError {
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
func (t *translator[F]) translateDeclaration(decl ast.Declaration, path file.Path) []SyntaxError {
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
	case *ast.DefSendReceive:
		errors = t.translateDefSendReceive(d)
	default:
		// Error handling
		panic("unknown declaration")
	}
	//
	return errors
}

// Translate a "defconstraint" declaration.
func (t *translator[F]) translateDefConstraint(decl *ast.DefConstraint) []SyntaxError {
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
		module.AddConstraint(vanishing.NewConstraint(decl.Handle, module.Id(), decl.Domain, expr))
	}
	// Done
	return errors
}

// Translate a lookup or permutation declaration.
func (t *translator[F]) translateDefLookup(decl *ast.DefLookup) []SyntaxError {
	var (
		errors               []SyntaxError
		srcModule, tgtModule mir.ModuleBuilder[F]
		sources, targets     []constraint.Vector
	)
	// Translate targets
	for i, ith := range decl.Targets {
		ith_targets, mod, errs := t.translateVector(decl.TargetSelectors[i], ith)
		targets = append(targets, ith_targets)
		errors = append(errors, errs...)
		//
		if i == 0 {
			tgtModule = mod
		}
	}
	// Translate sources
	for i, ith := range decl.Sources {
		ith_sources, mod, errs := t.translateVector(decl.SourceSelectors[i], ith)
		sources = append(sources, ith_sources)
		errors = append(errors, errs...)
		//
		if i == 0 {
			srcModule = mod
		}
	}
	// Sanity check whether we can construct the constraint, or not.
	if len(errors) == 0 {
		// Default to adding constraint to source module
		module := srcModule
		// However, if external add to target module instead.
		if module.IsExtern() {
			module = tgtModule
		}
		// Add translated constraint
		if decl.IsPermutation {
			module.AddConstraint(permutation.NewConstraint[F](decl.Handle, targets, sources))
		} else {
			module.AddConstraint(lookup.NewConstraint[F](decl.Handle, targets, sources))
		}
	}
	// Done
	return errors
}

// translateVector translates a register tuple and its optional selector for a
// lookup, permutation or bus, and returns its enclosing module.
func (t *translator[F]) translateVector(selector ast.TypedSymbol,
	sources []ast.TypedSymbol) (constraint.Vector, mir.ModuleBuilder[F], []SyntaxError) {
	var (
		context, j = ast.ContextOfSymbols(sources...)
		sel        *mir.RegisterAccess[F]
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
		return constraint.Vector{}, nil, t.srcmap.SyntaxErrors(source, "conflicting context")
	}
	// Determine enclosing module
	module := t.moduleOf(context)
	// Translate source columns
	registers, errors := t.translateVectorColumns(sources)
	// Translate selector (when present)
	if selector != nil {
		var errs []SyntaxError
		//
		sel, errs = t.registerOfRegisterAccess(selector, 0)
		errors = append(errors, errs...)
		// Sanity check selector.  NOTE: don't check if other errors, since the
		// selector may not have been translated.
		if len(errors) == 0 {
			errors = append(errors, t.checkVectorSelector(sel, selector)...)
		}
	}
	// Don't attempt to construct the vector if anything went wrong, since some
	// of its registers may not have been translated.
	if len(errors) > 0 {
		return constraint.Vector{}, nil, errors
	} else if selector != nil {
		return constraint.FilteredVector(module.Id(), sel.Register(), registers...), module, nil
	}
	//
	return constraint.UnfilteredVector(module.Id(), registers...), module, nil
}

// Translate a "defsend" or "defrecv" declaration into a pending bus port.
// The bus constraints themselves are only constructed once every port is
// known (see translateBuses).
func (t *translator[F]) translateDefSendReceive(decl *ast.DefSendReceive) []SyntaxError {
	port, module, errors := t.translateVector(decl.Selector, decl.Columns)
	if len(errors) > 0 {
		return errors
	}
	// Record port under its bus, tracking first-use order of bus names.
	if _, ok := t.busPorts[decl.Bus]; !ok {
		t.busOrder = append(t.busOrder, decl.Bus)
	}
	//
	t.busPorts[decl.Bus] = append(t.busPorts[decl.Bus], busPort[F]{decl, module, port})
	//
	return nil
}

// translateBuses merges the ports gathered for each bus into one bus
// constraint per bus.  This runs after all declarations have been translated
// (a bus's ports can be declared anywhere), and only when they translated
// cleanly.
func (t *translator[F]) translateBuses() []SyntaxError {
	var errors []SyntaxError
	//
	for _, busName := range t.busOrder {
		errors = append(errors, t.translateBus(busName, t.busPorts[busName])...)
	}
	//
	return errors
}

// translateBus checks that the ports of one bus agree (same column count, both
// directions present) and constructs the corresponding constraint.  The
// constraint is added to the first port's module; placement is cosmetic, but
// must be deterministic.
func (t *translator[F]) translateBus(busName string, ports []busPort[F]) []SyntaxError {
	var (
		errors          []SyntaxError
		sends, receives []constraint.Vector
		first           = ports[0]
	)
	//
	for _, ith := range ports {
		if ith.port.Len() != first.port.Len() {
			msg := fmt.Sprintf("bus port has %d columns, expected %d", ith.port.Len(), first.port.Len())
			errors = append(errors, *t.srcmap.SyntaxError(ith.decl, msg))
			//
			continue
		}
		//
		if ith.decl.IsSend {
			sends = append(sends, ith.port)
		} else {
			receives = append(receives, ith.port)
		}
	}
	// Check both directions are present.
	if len(errors) == 0 && len(sends) == 0 {
		errors = append(errors, *t.srcmap.SyntaxError(first.decl,
			fmt.Sprintf("bus %q has receives but no sends", busName)))
	}
	//
	if len(errors) == 0 && len(receives) == 0 {
		errors = append(errors, *t.srcmap.SyntaxError(first.decl,
			fmt.Sprintf("bus %q has sends but no receives", busName)))
	}
	//
	if len(errors) > 0 {
		return errors
	}
	//
	first.module.AddConstraint(bus.NewConstraint[F](busName, sends, receives))
	//
	return nil
}

// translateVectorColumns resolves a vector's columns to register IDs.
func (t *translator[F]) translateVectorColumns(columns []ast.TypedSymbol) ([]register.Id, []SyntaxError) {
	var (
		errors    []SyntaxError
		registers = make([]register.Id, len(columns))
	)
	//
	for i, column := range columns {
		reg, errs := t.registerOfRegisterAccess(column, 0)
		errors = append(errors, errs...)

		if reg != nil {
			registers[i] = reg.Register()
		}
	}
	//
	return registers, errors
}

// checkVectorSelector checks that a vector's selector is binary.
func (t *translator[F]) checkVectorSelector(sel *mir.RegisterAccess[F], selector ast.TypedSymbol) []SyntaxError {
	// Determine bitwidth for the value range of the selector
	valrange := sel.ValueRange()
	bitwidth, _ := valrange.BitWidth()
	// Check for non-binary selector
	if bitwidth > 1 {
		return t.srcmap.SyntaxErrors(selector, "non-binary selector encountered")
	}
	// Done
	return nil
}

// Translate a "definrange" declaration.
func (t *translator[F]) translateDefInRange(decl *ast.DefInRange) []SyntaxError {
	var module = t.moduleOf(ast.ContextOfSymbol(decl.Column))
	// Translate constrained column.  NOTE: no sanity check on the sign of the
	// resulting register is required, since registers are unsigned by
	// construction.
	access, errors := t.registerOfRegisterAccess(decl.Column, 0)
	//
	if len(errors) == 0 {
		// Add translated constraint
		module.AddConstraint(ranged.NewConstraint[F]("", module.Id(), access.Register(), decl.Bitwidth))
	}
	// Done
	return errors
}

// Translate a sequence of zero or more expressions enclosed in a given module.
func (t *translator[F]) translateExpressions(module mir.ModuleBuilder[F], shift int,
	exprs ...ast.Expr) ([]mir.Term[F], []SyntaxError) {
	//
	errors := []SyntaxError{}
	nexprs := make([]mir.Term[F], len(exprs))
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
func (t *translator[F]) translateExpression(expr ast.Expr, module mir.ModuleBuilder[F], shift int,
) (mir.Term[F], []SyntaxError) {
	switch e := expr.(type) {
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
		// Sanity check constant is representable in the field
		if e.Val.Cmp(field.Zero[F]().Modulus()) >= 0 {
			return nil, t.srcmap.SyntaxErrors(expr, "constant exceeds field modulus")
		}
		// Initialise field from bigint
		val := field.BigInt[F](e.Val)
		//
		return term.Const[F, mir.Term[F]](val), nil
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

func (t *translator[F]) translateShift(expr *ast.Shift, mod mir.ModuleBuilder[F], shift int,
) (mir.Term[F], []SyntaxError) {
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

func (t *translator[F]) translateVariableAccess(expr *ast.VariableAccess, shift int) (mir.Term[F], []SyntaxError) {
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
		var value = binding.Value.AsConstant()
		// Sanity check constant is representable in the field
		if value.Sign() < 0 {
			msg := fmt.Sprintf("constant %s is negative", expr.Name.Tail())
			return nil, t.srcmap.SyntaxErrors(expr, msg)
		} else if value.Cmp(field.Zero[F]().Modulus()) >= 0 {
			msg := fmt.Sprintf("constant %s exceeds field modulus", expr.Name.Tail())
			return nil, t.srcmap.SyntaxErrors(expr, msg)
		}
		// Initialise field from bigint
		constant := field.BigInt[F](*value)
		//
		return term.Const[F, mir.Term[F]](constant), nil
	}
	// error
	return nil, t.srcmap.SyntaxErrors(expr, "unbound variable")
}

// Translate a sequence of zero or more logical expressions enclosed in a given module.
func (t *translator[F]) translateLogicals(module mir.ModuleBuilder[F], shift int,
	exprs ...ast.Expr) ([]mir.LogicalTerm[F], []SyntaxError) {
	//
	errors := []SyntaxError{}
	logicals := make([]mir.LogicalTerm[F], len(exprs))
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
func (t *translator[F]) translateOptionalLogical(expr ast.Expr, module mir.ModuleBuilder[F],
	shift int) (mir.LogicalTerm[F], []SyntaxError) {
	//
	if expr != nil {
		return t.translateLogical(expr, module, shift)
	}

	return nil, nil
}

// Translate an expression situated in a given context.  The context is
// necessary to resolve unqualified names (e.g. for register access, function
// invocations, etc).
func (t *translator[F]) translateLogical(expr ast.Expr, mod mir.ModuleBuilder[F], shift int,
) (mir.LogicalTerm[F], []SyntaxError) {
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
			return term.Equals[F, mir.LogicalTerm[F]](lhs, rhs), nil
		case ast.NOT_EQUALS:
			return term.NotEquals[F, mir.LogicalTerm[F]](lhs, rhs), nil
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

func (t *translator[F]) translateIte(expr *ast.If, module mir.ModuleBuilder[F], shift int,
) (mir.LogicalTerm[F], []SyntaxError) {
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

func (t *translator[F]) translateLogicalShift(expr *ast.Shift, mod mir.ModuleBuilder[F],
	shift int) (mir.LogicalTerm[F], []SyntaxError) {
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
func (t *translator[F]) registerOfRegisterAccess(symbol ast.Symbol, shift int) (*mir.RegisterAccess[F], []SyntaxError) {
	if e, ok := symbol.(*ast.VariableAccess); ok {
		return t.registerOfVariableAccess(e, shift)
	}
	//
	return nil, t.srcmap.SyntaxErrors(symbol, "invalid register access")
}

func (t *translator[F]) registerOfVariableAccess(expr *ast.VariableAccess,
	shift int) (*mir.RegisterAccess[F], []SyntaxError) {
	//
	if binding, ok := expr.Binding().(*ast.ColumnBinding); ok {
		// Lookup register binding
		return t.registerOf(binding.AbsolutePath(), shift), nil
	}
	//
	return nil, t.srcmap.SyntaxErrors(expr, "invalid register access")
}

// Determine the appropriate name for a given module based on a module context.
func (t *translator[F]) moduleOf(context ast.Context) mir.ModuleBuilder[F] {
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
func (t *translator[F]) registerOf(path *file.Path, shift int) *mir.RegisterAccess[F] {
	// Determine register id
	rid := t.env.RegisterOf(path)
	//
	reg := t.env.Register(rid)
	// Lookup corresponding module builder
	module := t.moduleOf(reg.Context)
	//
	return RegisterAccessOf[F](module, reg.Name(), shift)
}

// RegisterAccessOf returns a register accessor for the register with the given name.
func RegisterAccessOf[F field.Element[F]](module register.Map[F], name string, shift int) *mir.RegisterAccess[F] {
	// Lookup register associated with this name
	var (
		rid, _ = module.HasRegister(name)
		reg    = module.Register(rid)
	)
	//
	return term.RawRegisterAccess[F, mir.Term[F]](rid, reg.Width(), shift)
}
