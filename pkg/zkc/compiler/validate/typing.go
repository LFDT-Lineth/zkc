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
package validate

import (
	"fmt"
	"math"
	"reflect"

	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/lval"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/stmt"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen"
	zkc_util "github.com/consensys/go-corset/pkg/zkc/util"
)

// Stmt is a convenient alias.
type Stmt = stmt.Stmt[symbol.Resolved]

// LVal is a convenient alias.
type LVal = lval.LVal[symbol.Resolved]

// Type is a convenient alias.
type Type = data.Type[symbol.Resolved]

// VariableMap is a convenient alias.
type VariableMap = variable.Map[symbol.Resolved]

// Typing validates that each declaration in a program is "correctly typed". For
// example, the following constant declaration is ill-typed:
//
// constant u8 MAX_U8 = 256
//
// The reason being that the constant's value does not fit in a u8.  Likewise,
// this set of declarations are ill-typed:
//
//	 public input u8[u8] ROM
//
//		function f(val u8) -> (r u10) {
//		  r = ROM + 1
//		  return
//		}
//
// The problem above is that ROM is an input memory and, hence, ROM+1 does not
// make sense.
func Typing(program ast.Program, field field.Config, srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		typer  = TypeChecker{program, field, program.Environment(), srcmaps}
	)
	// NOTE: finalising constants must be done first to ensure their expressions
	// are given types.  Otherwise, finalising components could fail.
	for _, d := range program.Components() {
		//nolint
		switch d := d.(type) {
		case *decl.ResolvedConstant:
			errors = append(errors, typer.typeConstant(*d)...)
		}
	}
	// finalise declared types
	for _, d := range program.Components() {
		switch d := d.(type) {
		case *decl.ResolvedFunction:
			errors = append(errors, typer.finaliseVariableDescriptors(d.Variables)...)
		case *decl.ResolvedMemory:
			errors = append(errors, typer.finaliseVariableDescriptors(d.Address)...)
			errors = append(errors, typer.finaliseVariableDescriptors(d.Data)...)
		case *decl.ResolvedTypeAlias:
			errors = append(errors, typer.finaliseDeclaredType(d.DataType)...)
		}
	}
	// type all components
	for _, d := range program.Components() {
		switch d := d.(type) {
		case *decl.ResolvedConstant:
			// skip: already done above
		case *decl.ResolvedFunction:
			errors = append(errors, typer.typeFunction(*d)...)
		case *decl.ResolvedInclude:
			// ignore
		case *decl.ResolvedMemory:
			errors = append(errors, typer.typeMemory(*d)...)
		case *decl.ResolvedTypeAlias:
		default:
			panic(fmt.Sprintf("unknown component: %s", reflect.TypeOf(d).String()))
		}
	}
	//
	return errors
}

// TypeChecker embodies information needed for type checking a given program.
type TypeChecker struct {
	program ast.Program
	field   field.Config
	env     ast.Environment
	srcmaps source.Maps[any]
}

func (p *TypeChecker) lookup(id symbol.Resolved) decl.Resolved {
	return p.program.Component(id.Index)
}

func (p *TypeChecker) finaliseVariableDescriptors(vars []variable.Descriptor[symbol.Resolved]) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, v := range vars {
		errors = append(errors, p.finaliseDeclaredType(v.DataType)...)
	}
	//
	return errors
}

func (p *TypeChecker) finaliseDeclaredType(datatype data.ResolvedType) (errors []source.SyntaxError) {
	switch t := datatype.(type) {
	case nil:
		// signals an upstream error occurred, so do nothing.
		return nil
	case *data.UnsignedInt[symbol.Resolved]:
		return nil
	case *data.ResolvedAlias:
		return p.finaliseDeclaredType(p.env.TypeOf(t.Name))
	case *data.ResolvedFixedArray:
		return p.finaliseDeclaredArrayType(t)
	case *data.ResolvedFieldElement:
		return nil
	case *data.Tuple[symbol.Resolved]:
		for i := range t.Width() {
			errors = append(errors, p.finaliseDeclaredType(t.Ith(i))...)
		}
	}
	//
	panic(fmt.Sprintf("unknown type encountered (%s)", datatype.String(p.env)))
}

func (p *TypeChecker) finaliseDeclaredArrayType(datatype *data.ResolvedFixedArray) (errors []source.SyntaxError) {
	var evaluator = codegen.NewConstantEvaluator(p.field, p.env, p.program.Components()...)
	// Finalise datatype itself
	errors = p.finaliseDeclaredType(datatype.DataType)
	// Check whether we have a symbolic bound (or not)
	if datatype.Size.HasSecond() {
		var (
			// resolve symbolic bound
			expr = p.env.ConstOf(datatype.Size.Second())
			// evaluate symbolic bound
			valSize, errMsg = evaluator.Eval(expr, false)
		)
		// check for errors arising
		if errMsg != "" {
			errors = append(errors, p.srcmaps.SyntaxErrors(datatype, errMsg)...)
		} else {
			datatype.Size = util.Union1[uint, symbol.Resolved](uint(valSize.Uint64()))
		}
	}
	//
	return errors
}

func (p *TypeChecker) typeConstant(c decl.ResolvedConstant) []source.SyntaxError {
	var (
		effects     bit.Set
		rhs, errors = p.typeExpression(c.DataType, c.ConstExpr, variable.ArrayMap[symbol.Resolved](), effects)
	)
	// Sanity check
	if len(errors) != 0 {
		return errors
	}
	// Subtype check
	return p.checkEquiTypes(rhs, c.DataType, c.ConstExpr)
}

func (p *TypeChecker) typeFunction(fn decl.ResolvedFunction) []source.SyntaxError {
	var (
		errors  []source.SyntaxError
		effects bit.Set
	)
	// initialise effects
	for _, s := range fn.Effects {
		effects.Insert(s.Index)
	}
	// type instructions
	for _, s := range fn.Code {
		switch s := s.(type) {
		case *stmt.Assign[symbol.Resolved]:
			errors = append(errors, p.typeAssignment(s, &fn, effects)...)
		case *stmt.Fail[symbol.Resolved]:
			errors = append(errors, p.typeFormatArgs(s.Chunks, s.Arguments, &fn, effects)...)
		case *stmt.IfGoto[symbol.Resolved]:
			errors = append(errors, p.typeIfGoto(s, &fn, effects)...)
		case *stmt.Printf[symbol.Resolved]:
			errors = append(errors, p.typeFormatArgs(s.Chunks, s.Arguments, &fn, effects)...)
		}
	}
	//
	return errors
}

func (p *TypeChecker) typeMemory(c decl.ResolvedMemory) []source.SyntaxError {
	var (
		errors       []source.SyntaxError
		dataType     = variable.DescriptorsToType(c.Data...)
		emptyEffects bit.Set
	)
	// Check static initialiser (if applicable)
	if c.IsStatic() {
		for _, v := range c.Contents {
			valType, errs := p.typeExpression(dataType, v, variable.ArrayMap[symbol.Resolved](), emptyEffects)
			if len(errs) != 0 {
				errors = append(errors, errs...)
			} else {
				errors = append(errors, p.checkEquiTypes(valType, dataType, v)...)
			}
		}
	}
	//
	return errors
}

func (p *TypeChecker) typeAssignment(s *stmt.Assign[symbol.Resolved], env VariableMap, effects bit.Set,
) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		types  = make([]Type, len(s.Targets))
	)
	// Sanity check assignment arity
	if len(s.Targets) == 0 {
		// Special case for empty targets.  This can only arise for a function
		// call which does not assign any return values.  This ensures that, in
		// such case, the source expression is typed.
		_, errors = p.typeExpression(nil, s.Source, env, effects)
		//
		return errors
	}
	// Type check left-hand side
	for i, lval := range s.Targets {
		var lhsErrs []source.SyntaxError
		// type lval
		types[i], lhsErrs = p.typeLval(lval, env, effects)
		// Accumulate lhs errors
		errors = append(errors, lhsErrs...)
	}
	// Type check right-hand side
	var (
		// Construct lhs type (potentially as a tuple)
		lhs_t          = data.FromTypes(types...)
		rhs_t, rhsErrs = p.typeExpression(lhs_t, s.Source, env, effects)
	)
	// Account for rhs errrors
	errors = append(errors, rhsErrs...)
	// accumulate errors
	if p.env.WellFormed(lhs_t) && p.env.WellFormed(rhs_t) {
		// resolve fixed-array size in whole assignments
		errors = append(errors, p.checkEquiTypes(rhs_t, lhs_t, s.Source)...)
	}
	//
	return append(errors, checkTargets(s, env, p.srcmaps)...)
}

func (p *TypeChecker) typeLval(target LVal, env VariableMap, effects bit.Set) (Type, []source.SyntaxError) {
	// determine lhs width
	switch t := target.(type) {
	case *lval.Variable[symbol.Resolved]:
		// Special case single variables
		if len(t.Ids) == 1 {
			return env.Variable(t.Ids[0]).DataType, nil
		}
		// Consider destructurings
		return p.typeDestructuringLval(t, env)
	case *lval.Array[symbol.Resolved]:
		return p.typeArrayAccess(t.Id, t.Arg, env, effects)
	case *lval.MemAccess[symbol.Resolved]:
		// Unknown external access cannot be typed.  This case handles some kind of
		// linking error earlier in the compilation pipeline.
		if t.Name.IsUnknown() {
			return nil, nil
		}
		// Lookup the symbol
		var extern = p.lookup(t.Name)
		//
		switch e := extern.(type) {
		case *decl.ResolvedConstant:
			return nil, p.srcmaps.SyntaxErrors(target, "cannot assign constant")
		case *decl.ResolvedMemory:
			return p.typeMemoryLVal(e, t, env, effects)
		case *decl.ResolvedFunction:
			return nil, p.srcmaps.SyntaxErrors(target, "cannot assign function")
		case *decl.ResolvedTypeAlias:
			return nil, p.srcmaps.SyntaxErrors(target, "cannot assign type alias")
		}
	}
	//
	return nil, p.srcmaps.SyntaxErrors(target, "unknown lval")
}

func (p *TypeChecker) typeDestructuringLval(target *lval.Variable[symbol.Resolved], env VariableMap,
) (Type, []source.SyntaxError) {
	//
	var (
		bitwidth uint
		ok       bool = true
		errors   []source.SyntaxError
	)
	// Consider destructurings
	for _, id := range target.Ids {
		var (
			// Extract type of this variable
			ith_t = env.Variable(id).DataType
			// Check well-formedness of this variable
			ith_ok = p.env.WellFormed(ith_t)
		)
		//
		if !ith_ok {
			// Component not well-formed, hence cannot determine result type.
			ok = false
		} else if t := ith_t.AsUint(p.env); t != nil {
			bitwidth += t.BitWidth()
		} else {
			// Component not well-formed, hence cannot determine result type.
			ok = false
			// Include syntax error
			errors = append(errors, *p.srcmaps.SyntaxError(target, "expected integer type"))
		}
	}
	//
	if !ok {
		return nil, errors
	}
	//
	return data.NewUnsignedInt[symbol.Resolved](bitwidth, false), nil
}

func (p *TypeChecker) typeMemoryLVal(c *decl.ResolvedMemory, e *lval.MemAccess[symbol.Resolved],
	env VariableMap, effects bit.Set) (Type, []source.SyntaxError) {
	var errors []source.SyntaxError
	// Initial sanity checks
	if len(e.Args) != len(c.Address) {
		return nil, p.srcmaps.SyntaxErrors(e,
			fmt.Sprintf("mismatched arguments (expected %d, found %d)", len(c.Address), len(e.Args)))
	} else if !effects.Contains(e.Name.Index) && c.IsReadable() && c.IsWriteable() {
		return nil, p.srcmaps.SyntaxErrors(e, "read/write memory not visible here")
	}
	// check argument types
	for i, e := range e.Args {
		var (
			ith         = c.Address[i].DataType
			arg_t, errs = p.typeExpression(c.Address[i].DataType, e, env, effects)
		)
		//
		errors = append(errors, errs...)
		// Subtype check (if no other errors)
		if len(errs) == 0 {
			errors = append(errors, p.checkEquiTypes(arg_t, ith, e)...)
		}
	}
	// Done
	return variable.DescriptorsToType(c.Data...), errors
}

// CheckTargetRegisters performs some simple checks on a set of target registers
// being written.  Firstly, they cannot be input registers (as this are always
// constant).  Secondly, we cannot write to the same register more than once
// (i.e. a conflicting write).
func checkTargets(s *stmt.Assign[symbol.Resolved], env VariableMap, srcmaps source.Maps[any]) []source.SyntaxError {
	var targets []variable.Id
	//
	for _, id := range s.Targets {
		targets = append(targets, lval.Definitions(id)...)
	}
	//
	for i, id := range targets {
		ith := env.Variable(id)
		//
		if ith.IsParameter() {
			return srcmaps.SyntaxErrors(s, fmt.Sprintf("cannot write parameter %s", ith.Name))
		}
		//
		for j := i + 1; j < len(targets); j++ {
			if targets[i] == targets[j] {
				return srcmaps.SyntaxErrors(s, fmt.Sprintf("conflicting write to %s", ith.Name))
			}
		}
	}
	//
	return nil
}

func (p *TypeChecker) typeIfGoto(s *stmt.IfGoto[symbol.Resolved], env VariableMap, effects bit.Set,
) []source.SyntaxError {
	return p.typeCondition(s.Cond, env, effects)
}

func (p *TypeChecker) typeFormatArgs(chunks []stmt.FormattedChunk, args []expr.Resolved, env VariableMap,
	effects bit.Set,
) []source.SyntaxError {
	var (
		errs    []source.SyntaxError
		formats = chunkFormats(chunks)
	)
	//
	for i, e := range args {
		ith, ierrs := p.typeExpression(nil, e, env, effects)
		//
		switch {
		case len(ierrs) > 0:
			errs = append(errs, ierrs...)
		case !p.env.WellFormed(ith):
			// silently skip - error already reported earlier in pipeline
		case i < len(formats) && formats[i].Code == zkc_util.FORMAT_CHR:
			// %c is the one specifier that constrains the argument
			// type: it must be a concrete u8
			if t := ith.AsUint(p.env); t == nil || t.IsOpen() || t.BitWidth() != 8 {
				errs = append(errs, *p.srcmaps.SyntaxError(e, "%c expects u8"))
			}
		default:
			if ith_t := ith.AsUint(p.env); ith_t == nil && ith.AsField(p.env) == nil {
				errs = append(errs, *p.srcmaps.SyntaxError(e, "expected uint (or 𝔽)"))
			} else if ith_t != nil && ith.AsUint(p.env).IsOpen() {
				errs = append(errs, *p.srcmaps.SyntaxError(e, "concrete type required"))
			}
		}
	}
	//
	return errs
}

// chunkFormats returns, in argument order, the format of every chunk that
// actually consumes an argument (i.e. chunks whose Format is not EMPTY).
func chunkFormats(chunks []stmt.FormattedChunk) []zkc_util.Format {
	var out []zkc_util.Format
	//
	for _, c := range chunks {
		if c.Format.HasFormat() {
			out = append(out, c.Format)
		}
	}
	//
	return out
}

func (p *TypeChecker) typeCondition(e expr.ResolvedCondition, env VariableMap, effects bit.Set) []source.SyntaxError {
	switch e := e.(type) {
	case *expr.Cmp[symbol.Resolved]:
		return p.typeComparison(e, env, effects)
	default:
		return p.srcmaps.SyntaxErrors(e, "unknown condition")
	}
}

// typeTernaryCondition type-checks the condition of a ternary expression.
// After lowering, the condition is always a single Cmp node.
func (p *TypeChecker) typeTernaryCondition(e expr.Resolved, env VariableMap, effects bit.Set) []source.SyntaxError {
	if cmp, ok := e.(*expr.Cmp[symbol.Resolved]); ok {
		return p.typeComparison(cmp, env, effects)
	}

	return p.srcmaps.SyntaxErrors(e, "invalid ternary condition")
}

func (p *TypeChecker) typeComparison(e *expr.Cmp[symbol.Resolved], env VariableMap,
	effects bit.Set) []source.SyntaxError {
	//
	var (
		lhs, lerrs = p.typeExpression(nil, e.Left, env, effects)
		rhs, rerrs = p.typeExpression(lhs, e.Right, env, effects)
		anchor, _  = p.determineTypeAnchor(lhs, rhs)
	)
	// disambiguate field comparison from other
	if p.env.WellFormed(anchor) && anchor.AsField(p.env) != nil && e.Operator.Fieldable() {
		return p.typeFieldComparison(e, env, effects)
	}
	// whole-array equality (==, !=) on matching fixed arrays
	if p.env.WellFormed(anchor) && anchor.AsFixedArray(p.env) != nil {
		return p.typeArrayComparison(e, lhs, rhs, lerrs, rerrs)
	}
	// Check left-hand side
	if len(lerrs) == 0 && p.env.WellFormed(lhs) && lhs.AsUint(p.env) == nil {
		lerrs = p.srcmaps.SyntaxErrors(e.Left, "expected uint")
	}
	// Check right-hand side
	if len(rerrs) == 0 && p.env.WellFormed(rhs) && rhs.AsUint(p.env) == nil {
		rerrs = p.srcmaps.SyntaxErrors(e.Right, "expected uint")
	}
	// Check matching types
	if len(lerrs)+len(rerrs) == 0 {
		// Equivalence check
		return p.checkEquiTypes(rhs, lhs, e.Right)
	}
	//
	return append(lerrs, rerrs...)
}

// typeArrayComparison type-checks a comparison whose operands are fixed
// arrays.  Only `==` and `!=` are supported; both sides must be fixed arrays
// of equal size and element type.
func (p *TypeChecker) typeArrayComparison(e *expr.Cmp[symbol.Resolved],
	lhs, rhs Type, lerrs, rerrs []source.SyntaxError,
) []source.SyntaxError {
	// Only == and != make sense element-wise on whole arrays.
	if !e.Operator.Fieldable() {
		return append(append(lerrs, rerrs...),
			*p.srcmaps.SyntaxError(e, "only == and != are supported on fixed arrays"))
	}
	// Both sides must be fixed arrays.
	if len(lerrs) == 0 && p.env.WellFormed(lhs) && lhs.AsFixedArray(p.env) == nil {
		lerrs = p.srcmaps.SyntaxErrors(e.Left, "expected fixed array")
	}
	//
	if len(rerrs) == 0 && p.env.WellFormed(rhs) && rhs.AsFixedArray(p.env) == nil {
		rerrs = p.srcmaps.SyntaxErrors(e.Right, "expected fixed array")
	}
	// Same size + same element type (recursive via EquiTypes).
	if len(lerrs)+len(rerrs) == 0 {
		return p.checkEquiTypes(rhs, lhs, e.Right)
	}
	//
	return append(lerrs, rerrs...)
}

func (p *TypeChecker) typeFieldComparison(e *expr.Cmp[symbol.Resolved], env VariableMap,
	effects bit.Set) []source.SyntaxError {
	//
	var (
		field_t    = data.NewFieldElement[symbol.Resolved]()
		lhs, lerrs = p.typeExpression(field_t, e.Left, env, effects)
		rhs, rerrs = p.typeExpression(field_t, e.Right, env, effects)
	)
	// Check left-hand side
	if len(lerrs) == 0 && p.env.WellFormed(lhs) && lhs.AsField(p.env) == nil {
		lerrs = p.srcmaps.SyntaxErrors(e.Left, "expected field")
	}
	// Check right-hand side
	if len(rerrs) == 0 && p.env.WellFormed(rhs) && rhs.AsField(p.env) == nil {
		rerrs = p.srcmaps.SyntaxErrors(e.Right, "expected field")
	}
	// Check matching types
	if len(lerrs)+len(rerrs) == 0 {
		// Equivalence check
		return p.checkEquiTypes(rhs, lhs, e.Right)
	}
	//
	return append(lerrs, rerrs...)
}

func (p *TypeChecker) typeExpression(expected Type, e expr.Resolved, env VariableMap, effects bit.Set,
) (actual Type, errs []source.SyntaxError) {
	// Sanity check for expressions which failed earlier in the compiler
	// pipeline.  This is necessary because we want to produce as many errors as
	// possible (i.e. producing errors even when other parts fail for some
	// reaosn).
	if e == nil {
		return nil, nil
	}
	// Decide what kind of expression we have.
	switch e := e.(type) {
	case *expr.Cast[symbol.Resolved]:
		actual, errs = p.typeCastExpression(expected, e, env, effects)
	case *expr.Concat[symbol.Resolved]:
		actual, errs = p.typeConcatExpression(expected, e, env, effects)
	case *expr.Add[symbol.Resolved]:
		actual, errs = p.typeUintOrFieldExpression(expected, e.Exprs, env, effects)
	case *expr.BitwiseAnd[symbol.Resolved]:
		actual, errs = p.typeUintExpressions(expected, e.Exprs, env, effects)
	case *expr.Const[symbol.Resolved]:
		actual, errs = p.typeConst(expected, e, env)
	case *expr.LocalAccess[symbol.Resolved]:
		actual, errs = p.typeLocalAccess(e, env)
	case *expr.ArrayAccess[symbol.Resolved]:
		actual, errs = p.typeArrayAccess(e.Id, e.Arg, env, effects)
	case *expr.Mul[symbol.Resolved]:
		actual, errs = p.typeUintOrFieldExpression(expected, e.Exprs, env, effects)
	case *expr.BitwiseNot[symbol.Resolved]:
		actual, errs = p.typeBitwiseNot(expected, e, env, effects)
	case *expr.BitwiseOr[symbol.Resolved]:
		actual, errs = p.typeUintExpressions(expected, e.Exprs, env, effects)
	case *expr.ExternAccess[symbol.Resolved]:
		actual, errs = p.typeExternAccess(e, env, effects)
	case *expr.Shl[symbol.Resolved]:
		actual, errs = p.typeShiftExpression(expected, e.Exprs, env, effects)
	case *expr.Shr[symbol.Resolved]:
		actual, errs = p.typeShiftExpression(expected, e.Exprs, env, effects)
	case *expr.Div[symbol.Resolved]:
		actual, errs = p.typeUintExpressions(expected, e.Exprs, env, effects)
	case *expr.Rem[symbol.Resolved]:
		actual, errs = p.typeUintExpressions(expected, e.Exprs, env, effects)
	case *expr.Sub[symbol.Resolved]:
		actual, errs = p.typeUintOrFieldExpression(expected, e.Exprs, env, effects)
	case *expr.Xor[symbol.Resolved]:
		actual, errs = p.typeUintExpressions(expected, e.Exprs, env, effects)
	case *expr.Ternary[symbol.Resolved]:
		actual, errs = p.typeTernaryExpr(expected, e, env, effects)
	case *expr.TupleInitialiser[symbol.Resolved]:
		actual, errs = p.typeTupleExpr(expected, e, env, effects)
	default:
		return nil, p.srcmaps.SyntaxErrors(e, "invalid expression")
	}
	// Associate type
	e.SetType(actual)
	//
	return actual, errs
}

func (p *TypeChecker) typeUintExpressions(t Type, exprs []expr.Resolved, env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var (
		errors []source.SyntaxError
		res    *data.UnsignedInt[symbol.Resolved]
	)
	//
	for i, e := range exprs {
		ith_t, errs := p.typeExpression(t, e, env, effects)
		//
		if len(errs) > 0 || !p.env.WellFormed(ith_t) {
			errors = append(errors, errs...)
		} else if i == 0 && ith_t.AsUint(p.env) == nil {
			return nil, append(errors, *p.srcmaps.SyntaxError(exprs[i], "expected uint"))
		} else if i == 0 {
			res = ith_t.AsUint(p.env)
		} else if res == nil {
			// skip type checking
		} else if errs := p.checkEquiTypes(ith_t, res, exprs[i]); len(errs) > 0 {
			errors = append(errors, errs...)
		} else {
			res = res.Join(exprs[i].Type().AsUint(p.env))
		}
	}
	// Return true nil
	if res == nil {
		return nil, errors
	}
	//
	return res, errors
}

func (p *TypeChecker) typeUintOrFieldExpression(t Type, exprs []expr.Resolved, env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var (
		// Type arguments
		types, errors = p.typeExpressions(t, exprs, env, effects)
		// Attempt to determine anchor
		anchor, index = p.determineTypeAnchor(types...)
	)
	// Attempt to distinguish cases
	if anchor != nil && anchor.AsUint(p.env) != nil {
		// We have an integer term here, therefore,
		return p.typeUintExpressions(anchor.AsUint(p.env), exprs, env, effects)
	} else if anchor != nil {
		// Retype expressions.  This is necessary to ensure that constants are
		// given field element type.
		types, errors = p.typeExpressions(data.NewFieldElement[symbol.Resolved](), exprs, env, effects)
		// Check arguments compatible with anchor
		for i, ith_t := range types {
			if i != index && p.env.WellFormed(ith_t) {
				if errs := p.checkEquiTypes(ith_t, anchor, exprs[i]); len(errs) > 0 {
					errors = append(errors, errs...)
				}
			}
		}
	}
	//
	return anchor, errors
}

func (p *TypeChecker) determineTypeAnchor(types ...Type) (Type, int) {
	//
	for i, t := range types {
		if p.env.WellFormed(t) {
			// Constants cannot be anchors.
			if ut := t.AsUint(p.env); ut == nil || !ut.IsOpen() {
				return t, i
			}
		}
	}
	//
	return nil, math.MaxInt
}

func (p *TypeChecker) typeExpressions(t Type, exprs []expr.Resolved, env VariableMap, effects bit.Set,
) ([]Type, []source.SyntaxError) {
	var (
		errors, errs []source.SyntaxError
		types        = make([]Type, len(exprs))
	)
	for i, e := range exprs {
		types[i], errs = p.typeExpression(t, e, env, effects)
		errors = append(errors, errs...)
	}
	//
	return types, errors
}

func (p *TypeChecker) typeConcatExpression(t Type, e *expr.Concat[symbol.Resolved], env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var (
		errors     []source.SyntaxError
		bitwidth   uint
		untypeable bool
	)
	//
	for _, e := range e.Exprs {
		ith_t, errs := p.typeExpression(t, e, env, effects)
		//
		if len(errs) > 0 {
			errors = append(errors, errs...)
		} else if !p.env.WellFormed(ith_t) {
			// This expression is untypeable because it contains a component
			// which is untypeable due to some earlier error in the pipeline.
			untypeable = true
		} else if ith_t := ith_t.AsUint(p.env); ith_t == nil {
			return nil, append(errors, *p.srcmaps.SyntaxError(e, "expected uint"))
		} else if ith_t := ith_t.AsUint(p.env); ith_t.IsOpen() {
			return nil, append(errors, *p.srcmaps.SyntaxError(e, "expected fixed-width uint"))
		} else {
			bitwidth += ith_t.BitWidth()
		}
	}
	//
	if untypeable {
		return nil, errors
	}
	//
	return data.NewUnsignedInt[symbol.Resolved](bitwidth, false), errors
}

func (p *TypeChecker) typeCastExpression(_ Type, e *expr.Cast[symbol.Resolved], env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var (
		srcType, errors = p.typeExpression(nil, e.Expr, env, effects)
	)
	// Sanity check whether the cast makes sense (or not).
	if len(errors) == 0 {
		errors = p.checkCastType(e.CastType, srcType, e.Expr)
	}
	//
	return e.CastType, errors
}

// typeShiftExpression types a shift expression (Shl or Shr). The result type
// is that of the first (value) operand. All operands must be uint and the
// shift amount must have a compatible type with the value being shifted.
func (p *TypeChecker) typeShiftExpression(t Type, exprs []expr.Resolved, env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var (
		arg_t, errors = p.typeExpression(t, exprs[0], env, effects)
		_, errs2      = p.typeUintExpressions(nil, exprs[1:], env, effects)
		res           *data.UnsignedInt[symbol.Resolved]
	)
	// Sanity check argument
	if len(errors) > 0 {
		// don't type check as other problems
	} else if !p.env.WellFormed(arg_t) {
		// silently skip - error already reported earlier in pipeline
	} else if res = arg_t.AsUint(p.env); res == nil {
		return nil, append(errors, *p.srcmaps.SyntaxError(exprs[0], "expected uint"))
	}
	// Return true nil
	if res == nil {
		return nil, append(errors, errs2...)
	}
	//
	return res, append(errors, errs2...)
}

func (p *TypeChecker) typeBitwiseNot(t Type, e *expr.BitwiseNot[symbol.Resolved], env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	t, errs := p.typeExpression(t, e.Expr, env, effects)
	//
	if len(errs) > 0 {
		return nil, errs
	} else if !p.env.WellFormed(t) {
		// silently skip - error already reported earlier in pipeline
		return nil, nil
	} else if t.AsUint(p.env) == nil {
		return nil, p.srcmaps.SyntaxErrors(e.Expr, "expected uint")
	}

	return t, nil
}

func (p *TypeChecker) typeConst(t Type, e *expr.Const[symbol.Resolved], env VariableMap) (Type, []source.SyntaxError) {
	var (
		bitwidth = uint(e.Constant().BitLen())
		actual   = data.NewUnsignedInt[symbol.Resolved](bitwidth, true)
	)
	//
	// Sanity check for field type
	if p.env.WellFormed(t) && t.AsField(p.env) != nil {
		// Sanity check for overflow
		if mod := p.field.Modulus(); e.Constant().Cmp(mod) >= 0 {
			return nil, p.srcmaps.SyntaxErrors(e, "field overflow")
		}
		//
		return t, nil
	} else if t == nil {
		return actual, nil
	}
	//
	return t, p.checkEquiTypes(actual, t, e)
}

func (p *TypeChecker) typeLocalAccess(e *expr.LocalAccess[symbol.Resolved], env VariableMap,
) (Type, []source.SyntaxError) {
	//
	return env.Variable(e.Variable).DataType, nil
}

func (p *TypeChecker) typeArrayAccess(id variable.Id, arg expr.Resolved, env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	var errors []source.SyntaxError

	varType := env.Variable(id).DataType
	// Check well-formedness of this variable
	varType_ok := p.env.WellFormed(varType)

	// Check argument is unsigned integers and well-formed
	arg_t, errs := p.typeExpression(nil, arg, env, effects)
	arg_t_ok := p.env.WellFormed(arg_t)

	errors = append(errors, errs...)
	//
	if !varType_ok {
		return nil, errors
	} else if arg_t_ok && arg_t.AsUint(p.env) == nil {
		errors = append(errors, *p.srcmaps.SyntaxError(arg, "expected uint"))
	} else if varType.AsFixedArray(p.env) == nil {
		errors = append(errors, *p.srcmaps.SyntaxError(arg, "variable is not a fixed-size array"))
	} else if arg_t_ok {
		fixedArr := varType.AsFixedArray(p.env)
		errors = append(errors, p.checkArrayBounds(arg, fixedArr)...)

		return fixedArr.DataType, errors
	}

	return nil, errors
}

func (p *TypeChecker) typeExternAccess(e *expr.ExternAccess[symbol.Resolved], env VariableMap, effects bit.Set,
) (Type, []source.SyntaxError) {
	// Unknown external access cannot be typed.  This case handles some kind of
	// linking error earlier in the compilation pipeline.
	if e.Name.IsUnknown() {
		return nil, nil
	}
	// Known external access, therefore lookup symbol
	var extern = p.lookup(e.Name)
	// Decide what kind of symbol it is
	switch t := extern.(type) {
	case *decl.ResolvedConstant:
		return p.typeConstantAccess(t)
	case *decl.ResolvedMemory:
		return p.typeMemoryAccess(t, e, env, effects)
	case *decl.ResolvedFunction:
		return p.typeFunctionCall(t, e, env, effects)
	case *decl.ResolvedTypeAlias:
		return p.typeAlias(e)
	default:
		return nil, p.srcmaps.SyntaxErrors(e, "unknown symbol type")
	}
}
func (p *TypeChecker) typeConstantAccess(c *decl.ResolvedConstant) (Type, []source.SyntaxError) {
	return c.DataType, nil
}

func (p *TypeChecker) typeMemoryAccess(c *decl.ResolvedMemory, e *expr.ExternAccess[symbol.Resolved],
	env VariableMap, effects bit.Set) (Type, []source.SyntaxError) {
	var errors []source.SyntaxError
	//
	if len(e.Args) != len(c.Address) {
		return nil, p.srcmaps.SyntaxErrors(e,
			fmt.Sprintf("mismatched arguments (expected %d, found %d)", len(c.Address), len(e.Args)))
	} else if c.IsReadable() && c.IsWriteable() && !effects.Contains(e.Name.Index) {
		return nil, p.srcmaps.SyntaxErrors(e, "read/write memory not visible here")
	}
	// check argument types
	for i, arg := range e.Args {
		var (
			ith         = c.Address[i].DataType
			ith_t, errs = p.typeExpression(ith, arg, env, effects)
		)
		//
		errors = append(errors, errs...)
		// Perofmr subtype check (if no other errors)
		if len(errs) == 0 {
			// Subtype check
			errors = append(errors, p.checkEquiTypes(ith_t, ith, e.Args[i])...)
		}
	}
	// Done
	return variable.DescriptorsToType(c.Data...), errors
}

func (p *TypeChecker) typeAlias(e *expr.ExternAccess[symbol.Resolved]) (Type, []source.SyntaxError) {
	return nil, p.srcmaps.SyntaxErrors(e, "not an expression")
}

func (p *TypeChecker) typeFunctionCall(c *decl.ResolvedFunction, e *expr.ExternAccess[symbol.Resolved],
	env VariableMap, effects bit.Set) (Type, []source.SyntaxError) {
	var (
		errors []source.SyntaxError
		n      = uint(len(e.Args))
	)
	//
	if n != c.NumInputs {
		return nil, p.srcmaps.SyntaxErrors(e,
			fmt.Sprintf("mismatched arguments (expected %d, found %d)", c.NumInputs, n))
	}
	// check argument types
	for i, arg := range e.Args {
		var (
			ith         = c.Variables[i].DataType
			ith_t, errs = p.typeExpression(ith, arg, env, effects)
		)
		//
		errors = append(errors, errs...)
		// Subtype check (if no other errors)
		if len(errs) == 0 {
			// resolve fixed-array size in function arguments
			errors = append(errors, p.checkEquiTypes(ith_t, ith, e.Args[i])...)
		}
	}
	// Sanity check callee effects visible at call site
	for _, effect := range c.Effects {
		if !effects.Contains(effect.Index) {
			errors = append(errors,
				*p.srcmaps.SyntaxError(e, fmt.Sprintf("read/write memory \"%s\" not visible here", effect.Name)))
		}
	}
	// Done
	return variable.DescriptorsToType(c.Outputs()...), errors
}

func (p *TypeChecker) typeTernaryExpr(expected Type, e *expr.Ternary[symbol.Resolved], env VariableMap,
	effects bit.Set) (Type, []source.SyntaxError) {
	//
	var (
		errs      = p.typeTernaryCondition(e.Cond, env, effects)
		tt, terrs = p.typeExpression(expected, e.IfTrue, env, effects)
		ft, ferrs = p.typeExpression(expected, e.IfFalse, env, effects)
	)
	// Combine all errors
	errs = append(append(errs, terrs...), ferrs...)
	//
	if p.env.WellFormed(ft) && p.env.WellFormed(tt) {
		errs = append(errs, p.checkEquiTypes(ft, tt, e.IfFalse)...)
		if len(errs) == 0 {
			ttu := tt.AsUint(p.env)
			ftu := ft.AsUint(p.env)

			if ttu != nil && ftu != nil {
				return ttu.Join(ftu), nil
			}
		}

		return tt, errs
	}
	//
	return nil, errs
}

func (p *TypeChecker) typeTupleExpr(expected Type, e *expr.TupleInitialiser[symbol.Resolved], env VariableMap,
	effects bit.Set) (Type, []source.SyntaxError) {
	//
	var (
		errors, errs []source.SyntaxError
		tupTypes     = p.destructTupleType(expected)
		types        = make([]Type, len(e.Exprs))
	)
	//
	for i, e := range e.Exprs {
		var ith_t Type
		//
		if i < len(tupTypes) {
			ith_t = tupTypes[i]
		}
		//
		types[i], errs = p.typeExpression(ith_t, e, env, effects)
		errors = append(errors, errs...)
	}
	//
	return data.FromTypes(types...), errors
}

// DestructTupleTypes attempts to break the given type into a tuple.  If it
// cannot do this, then it simple returns an empty array.
func (p *TypeChecker) destructTupleType(t Type) []Type {
	if p.env.WellFormed(t) {
		if tt := t.AsTuple(p.env); tt != nil {
			return tt.Types()
		}
	}
	//
	return nil
}

// For an expression "(T1) e" where "e : T2" under the given environment, check
// that either "T1 <: T2" or "T1 :> T2".  In order to undertake this check
// safely, it must be the case that both T1 and T2 are "well-formed".  The
// assumption is that, if either type is not well-formed, some error was already
// reported upstream for this.
func (p *TypeChecker) checkCastType(to, from Type, node any) []source.SyntaxError {
	//
	if !p.env.WellFormed(to) || !p.env.WellFormed(from) {
		return nil
	} else if data.SubtypeOf(to, from, p.env) || data.SubtypeOf(from, to, p.env) {
		return nil
	}
	//
	return p.srcmaps.SyntaxErrors(node, fmt.Sprintf("expected type %s", to.String(p.env)))
}

func (p *TypeChecker) checkArrayBounds(arg expr.Resolved, fixedArray *data.ResolvedFixedArray,
) []source.SyntaxError {
	var evaluator = codegen.NewConstantEvaluator(p.field, p.env, p.program.Components()...)
	// Evaluate argument as a comptime expression.
	val, errMsg := evaluator.Eval(arg, false)
	// Check for any errors (e.g. arithmetic overflow)
	if errMsg != "" {
		return p.srcmaps.SyntaxErrors(arg, errMsg)
	}
	// Check within bounds (note fixed array size finalised already in
	// finaliseDeclaredType())
	if val.Uint64() >= uint64(fixedArray.Size.First()) {
		return p.srcmaps.SyntaxErrors(arg,
			fmt.Sprintf("index %s out of bounds for array of size %s", val.Text(10), fixedArray.SizeString()))
	}
	//
	return nil
}

// For two expressions "e1", "e2" where "e1 : T1" and "e2 : T2" under the given
// environment, check that "T1 ~ T2.  In order to undertake this check safely,
// it must be the case that both T1 and T2 are "well-formed".  The assumption is
// that, if either type is not well-formed, some error was already reported
// upstream for this.
func (p *TypeChecker) checkEquiTypes(lhs, rhs Type, node any) []source.SyntaxError {
	if p.env.WellFormed(lhs) && p.env.WellFormed(rhs) && !data.EquiTypes(lhs, rhs, p.env) {
		return p.srcmaps.SyntaxErrors(node, fmt.Sprintf("expected type %s", rhs.String(p.env)))
	}
	//
	return nil
}
