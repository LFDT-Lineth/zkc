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
package lower

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/consensys/go-corset/pkg/util"
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
)

// FlattenFixedArrays expands
// fixed-size array variables into individual scalar variables.  A variable
// arrayName of type [uM;n] is replaced by n scalars arrayName$0 .. arrayName$(n-1),
// each of type uM.  Corresponding expr.ArrayAccess and lval.Array nodes are
// rewritten to plain LocalAccess / lval.Variable references.
func FlattenFixedArrays(program ast.Program) {
	env := program.Environment()

	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok {
			mapping := make([]varMapping, len(fn.Variables))

			// Expand for variables and assignments
			expandedVars, expandedCode, hasArray := expandFixedArrays(fn, mapping, env)
			// If no fixed-size array variables were found, skip the rewrite
			if !hasArray {
				continue
			}

			// Rewrite the expanded code to replace array accesses with scalar references
			rewrittenCode := rewriteFixedArrays(expandedCode, mapping, program.Components(), env)

			// After rewriting, update fn's code, variables, input and output counts to reflect the expanded scalars
			fn.Code = rewrittenCode
			fn.Variables = expandedVars
			fn.NumInputs = countVarsOfKind(expandedVars, variable.PARAMETER)
			fn.NumOutputs = countVarsOfKind(expandedVars, variable.RETURN)
		}
	}
}

// varMapping records how an old variable ID maps into the expanded variable
// list.  For scalar variables newBase is the single new ID.  For fixed arrays
// newBase..newBase+size-1 are the individual element variables.
type varMapping struct {
	newBase uint
	isArray bool
	size    uint
}

// expandFixedArrays builds the old→new id mapping and expands fixed-size array 
// variables into scalars, expands whole-array assignment statements into element-wise 
// array access assignments, and expands bare array arguments in ExternAccess calls
// into individual indexed accesses (e.g. sum(items) becomes
// sum(items[0], items[1], items[2])).  All expanded nodes use the original
// variable IDs so that the subsequent rewriting phase can remap them.
func expandFixedArrays(
	fn *decl.ResolvedFunction, mapping []varMapping, env ast.Environment,
) (expandedVars []variable.ResolvedDescriptor, expandedCode []stmt.Resolved, hasArray bool) {
	for oldID, v := range fn.Variables {
		base := uint(len(expandedVars))
		if vType := v.DataType.AsFixedArray(env); vType != nil {
			size := vType.Size.First()

			hasArray = true

			for j := range size {
				name := v.Name + "$" + strconv.FormatUint(uint64(j), 10)
				bitwidth, _ := data.BitWidthOf(vType, env)
				elemType := data.NewUnsignedInt[symbol.Resolved](bitwidth, false)
				expandedVars = append(expandedVars, variable.New[symbol.Resolved](v.Kind, name, elemType))
			}

			mapping[oldID] = varMapping{newBase: base, isArray: true, size: size}
		} else {
			mapping[oldID] = varMapping{newBase: base}
			expandedVars = append(expandedVars, v)
		}
	}

	if !hasArray {
		return
	}

	for _, s := range fn.Code {
		switch s := s.(type) {
		case *stmt.Assign[symbol.Resolved]:
			if expanded := expandArrayAssign(s, mapping, env); expanded != nil {
				expandedCode = append(expandedCode, expanded...)
				continue
			}

			for i, lv := range s.Targets {
				s.Targets[i] = expandLValArrayArgs(lv, mapping, env)
			}

			s.Source = expandExprArrayArgs(s.Source, mapping, env)
		case *stmt.IfGoto[symbol.Resolved]:
			expandCondArrayArgs(s.Cond, mapping, env)
		case *stmt.Printf[symbol.Resolved]:
			for i, arg := range s.Arguments {
				s.Arguments[i] = expandExprArrayArgs(arg, mapping, env)
			}
		}

		expandedCode = append(expandedCode, s)
	}

	return
}

func expandExprArrayArgs(e expr.Resolved, mapping []varMapping, env ast.Environment) expr.Resolved {
	switch e := e.(type) {
	case *expr.ExternAccess[symbol.Resolved]:
		e.Args = expandArrayArgs(e.Args, mapping, env)
		return e
	case *expr.Add[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Sub[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Mul[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Div[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Rem[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Shl[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Shr[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.BitwiseAnd[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.BitwiseOr[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Xor[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.BitwiseNot[symbol.Resolved]:
		e.Expr = expandExprArrayArgs(e.Expr, mapping, env)
		return e
	case *expr.LogicalAnd[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.LogicalOr[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.LogicalNot[symbol.Resolved]:
		e.Expr = expandExprArrayArgs(e.Expr, mapping, env)
		return e
	case *expr.Cast[symbol.Resolved]:
		e.Expr = expandExprArrayArgs(e.Expr, mapping, env)
		return e
	case *expr.Concat[symbol.Resolved]:
		expandExprSliceArrayArgs(e.Exprs, mapping, env)
		return e
	case *expr.Cmp[symbol.Resolved]:
		e.Left = expandExprArrayArgs(e.Left, mapping, env)
		e.Right = expandExprArrayArgs(e.Right, mapping, env)

		return e
	case *expr.Ternary[symbol.Resolved]:
		e.Cond = expandExprArrayArgs(e.Cond, mapping, env)
		e.IfTrue = expandExprArrayArgs(e.IfTrue, mapping, env)
		e.IfFalse = expandExprArrayArgs(e.IfFalse, mapping, env)

		return e
	default:
		return e
	}
}

func expandExprSliceArrayArgs(exprs []expr.Resolved, mapping []varMapping, env ast.Environment) {
	for i, e := range exprs {
		exprs[i] = expandExprArrayArgs(e, mapping, env)
	}
}

func expandCondArrayArgs(c expr.ResolvedCondition, mapping []varMapping, env ast.Environment) {
	if cmp, ok := c.(*expr.Cmp[symbol.Resolved]); ok {
		cmp.Left = expandExprArrayArgs(cmp.Left, mapping, env)
		cmp.Right = expandExprArrayArgs(cmp.Right, mapping, env)
	}
}

func expandLValArrayArgs(l lval.Resolved, mapping []varMapping, env ast.Environment) lval.Resolved {
	switch l := l.(type) {
	case *lval.MemAccess[symbol.Resolved]:
		expandExprSliceArrayArgs(l.Args, mapping, env)
		return l
	default:
		return l
	}
}

// expandArrayArgs expands bare array variable arguments into individual
// ArrayAccess expressions using the original variable IDs.  env is used to
// resolve through type aliases (e.g. `type ARR = [u8;4]`) so that a bare
// alias-typed array variable is splatted the same way as a directly-typed one.
func expandArrayArgs(args []expr.Resolved, mapping []varMapping, env ast.Environment) []expr.Resolved {
	var result []expr.Resolved

	for _, arg := range args {
		if la, ok := arg.(*expr.LocalAccess[symbol.Resolved]); ok {
			m := mapping[la.Variable]
			if m.isArray {
				arrayType := la.Type().AsFixedArray(env)

				for i := range m.size {
					idx := *big.NewInt(int64(i))
					access := &expr.ArrayAccess[symbol.Resolved]{
						Id:       la.Variable,
						Arg:      expr.NewConstant[symbol.Resolved](idx, 10),
						Datatype: arrayType.DataType,
					}
					result = append(result, access)
				}

				continue
			}
		}

		result = append(result, expandExprArrayArgs(arg, mapping, env))
	}

	return result
}

func rewriteFixedArrays(
	expandedCode []stmt.Resolved, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) (newCode []stmt.Resolved) {
	for _, s := range expandedCode {
		newCode = append(newCode, rewriteFixedArrayStmt(s, mapping, declarations, env))
	}

	return
}

func countVarsOfKind(vars []variable.ResolvedDescriptor, kind variable.Kind) uint {
	var n uint

	for _, v := range vars {
		if v.Kind == kind {
			n++
		}
	}

	return n
}

// expandArrayAssign rewrites an assignment whose left-hand side is a bare
// fixed-size array variable into a form the rewriting phase can lower to
// scalars.  Two regimes are handled, dispatching on the source kind:
//
//  1. Bare array variable read (a = b).  The source is side-effect-free, so
//     the statement is split into n element-wise statements
//
//	a[0] = b[0]; ...; a[n-1] = b[n-1]
//
//     each of which the rewrite phase trivially remaps to a scalar copy.
//
//  2. Function call returning an array (a = f(...)).  The call must be
//     evaluated exactly once, so the statement is kept as a single assignment
//     with n Array lvals as targets and the call as a single source
//
//	(a[0], ..., a[n-1]) = f(...)
//
//     The argument list is splatted in place so that any bare array arguments
//     are turned into per-element accesses; the rewrite phase then remaps the
//     n Array targets to n scalar Variable targets and the args to scalar
//     reads, yielding the destructuring form codegen already supports for
//     multi-return calls (see mapLVals in codegen/statement.go).
//
// Returns nil when no rewrite applies (lhs is not a bare array variable, or
// the source shape isn't supported).  All returned statements still reference
// the original variable IDs; the rewriting phase remaps them.
func expandArrayAssign(
	s *stmt.Assign[symbol.Resolved], mapping []varMapping, env ast.Environment,
) []stmt.Resolved {
	if len(s.Targets) != 1 {
		return nil
	}

	lv, ok := s.Targets[0].(*lval.Variable[symbol.Resolved])
	if !ok || len(lv.Ids) != 1 {
		return nil
	}

	lhsID := lv.Ids[0]
	lm := mapping[lhsID]

	if !lm.isArray {
		return nil
	}
	// Resolve through any type aliases (the rhs of e.g. `actual = rev(before)`
	// where `rev` returns a named alias for `[u8;4]`).
	srcArr := s.Source.Type().AsFixedArray(env)
	if srcArr == nil || !srcArr.Size.HasFirst() || srcArr.Size.First() != lm.size {
		return nil
	}

	elemTargets := make([]lval.LVal[symbol.Resolved], lm.size)
	for i := range lm.size {
		idx := *big.NewInt(int64(i))
		elemTargets[i] = lval.NewArray[symbol.Resolved](
			lhsID, expr.NewConstant[symbol.Resolved](idx, 10),
		)
	}

	switch src := s.Source.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		result := make([]stmt.Resolved, lm.size)

		for i := range lm.size {
			idx := *big.NewInt(int64(i))
			result[i] = &stmt.Assign[symbol.Resolved]{
				Targets: []lval.LVal[symbol.Resolved]{elemTargets[i]},
				Source: &expr.ArrayAccess[symbol.Resolved]{
					Id:       src.Variable,
					Arg:      expr.NewConstant[symbol.Resolved](idx, 10),
					Datatype: srcArr.DataType,
				},
			}
		}

		return result
	case *expr.ExternAccess[symbol.Resolved]:
		src.Args = expandArrayArgs(src.Args, mapping, env)

		return []stmt.Resolved{&stmt.Assign[symbol.Resolved]{
			Targets: elemTargets,
			Source:  src,
		}}
	}

	return nil
}

func rewriteFixedArrayStmt(
	s stmt.Resolved, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) stmt.Resolved {
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		for i, lv := range s.Targets {
			s.Targets[i] = rewriteFixedArrayLVal(lv, mapping, declarations, env)
		}

		s.Source = rewriteFixedArrayExpr(s.Source, mapping, declarations, env)

		return s
	case *stmt.IfGoto[symbol.Resolved]:
		s.Cond = rewriteFixedArrayCondition(s.Cond, mapping, declarations, env)

		return s
	case *stmt.Printf[symbol.Resolved]:
		for i, arg := range s.Arguments {
			s.Arguments[i] = rewriteFixedArrayExpr(arg, mapping, declarations, env)
		}

		return s
	case *stmt.Return[symbol.Resolved], *stmt.Goto[symbol.Resolved], *stmt.Fail[symbol.Resolved]:
		return s
	default:
		panic(fmt.Sprintf("unknown statement encountered during fixed-array lowering: %T", s))
	}
}

func rewriteFixedArrayExpr(
	e expr.Resolved, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) expr.Resolved {
	switch e := e.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		e.Variable = mapping[e.Variable].newBase
		//
		return e
	case *expr.ArrayAccess[symbol.Resolved]:
		rewriteFixedArrayExpr(e.Arg, mapping, declarations, env)

		m := mapping[e.Id]
		if !m.isArray {
			e.Id = m.newBase
			return e
		}

		val, ko := codegen.EvalConstant(e.Arg, false, declarations, env)
		if ko != "" {
			// This should have been checked in the typing phase already
			panic("expected constant index for fixed array access during lowering")
		}

		idx := uint(val.Uint64())

		result := &expr.LocalAccess[symbol.Resolved]{Variable: m.newBase + idx}
		result.SetType(e.Type())

		return result
	case *expr.Add[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Sub[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Mul[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Div[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Rem[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Shl[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Shr[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.BitwiseAnd[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.BitwiseOr[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.Xor[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.BitwiseNot[symbol.Resolved]:
		e.Expr = rewriteFixedArrayExpr(e.Expr, mapping, declarations, env)
		return e
	case *expr.LogicalAnd[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.LogicalOr[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)
		return e
	case *expr.LogicalNot[symbol.Resolved]:
		e.Expr = rewriteFixedArrayExpr(e.Expr, mapping, declarations, env)
		return e
	case *expr.Cast[symbol.Resolved]:
		e.Expr = rewriteFixedArrayExpr(e.Expr, mapping, declarations, env)
		return e
	case *expr.Concat[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Exprs, mapping, declarations, env)

		return e
	case *expr.Cmp[symbol.Resolved]:
		e.Left = rewriteFixedArrayExpr(e.Left, mapping, declarations, env)
		e.Right = rewriteFixedArrayExpr(e.Right, mapping, declarations, env)

		return e
	case *expr.Ternary[symbol.Resolved]:
		e.Cond = rewriteFixedArrayExpr(e.Cond, mapping, declarations, env)
		e.IfTrue = rewriteFixedArrayExpr(e.IfTrue, mapping, declarations, env)
		e.IfFalse = rewriteFixedArrayExpr(e.IfFalse, mapping, declarations, env)

		return e
	case *expr.ExternAccess[symbol.Resolved]:
		rewriteFixedArrayExprs(e.Args, mapping, declarations, env)
		return e
	case *expr.Const[symbol.Resolved]:
		return e
	default:
		panic(fmt.Sprintf("unknown expression encountered during fixed-array lowering: %T", e))
	}
}

func rewriteFixedArrayExprs(
	exprs []expr.Resolved, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) {
	for i, e := range exprs {
		exprs[i] = rewriteFixedArrayExpr(e, mapping, declarations, env)
	}
}

func rewriteFixedArrayCondition(
	c expr.ResolvedCondition, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) expr.ResolvedCondition {
	switch c := c.(type) {
	case *expr.Cmp[symbol.Resolved]:
		c.Left = rewriteFixedArrayExpr(c.Left, mapping, declarations, env)
		c.Right = rewriteFixedArrayExpr(c.Right, mapping, declarations, env)

		return c
	default:
		panic(fmt.Sprintf("unknown condition encountered during fixed-array lowering: %T", c))
	}
}

func rewriteFixedArrayLVal(
	l lval.Resolved, mapping []varMapping,
	declarations []codegen.Declaration, env ast.Environment,
) lval.Resolved {
	switch l := l.(type) {
	case *lval.Variable[symbol.Resolved]:
		for i, id := range l.Ids {
			m := mapping[id]
			l.Ids[i] = m.newBase
		}

		return l
	case *lval.Array[symbol.Resolved]:
		rewriteFixedArrayExpr(l.Arg, mapping, declarations, env)

		m := mapping[l.Id]
		if !m.isArray {
			l.Id = m.newBase
			return l
		}

		val, ko := codegen.EvalConstant(l.Arg, false, declarations, env)
		if ko != "" {
			// This should have already been checked in the typing phase
			panic("expected constant index for fixed array lval during lowering")
		}

		idx := uint(val.Uint64())

		return &lval.Variable[symbol.Resolved]{Ids: []variable.Id{m.newBase + idx}}
	case *lval.MemAccess[symbol.Resolved]:
		rewriteFixedArrayExprs(l.Args, mapping, declarations, env)
		return l
	default:
		panic(fmt.Sprintf("unknown lval encountered during fixed-array lowering: %T", l))
	}
}

// Flatten flattens all block-level statements (IfElse, Switch, While, For,
// Break, Continue) in each function of the program into the flat if-goto form
// expected by subsequent validation and code generation passes.  Source map
// entries for generated nodes are inherited from the original block node.
func Flatten(program ast.Program, srcmaps source.Maps[any]) {
	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok {
			fn.Code = lowerStatements(0, fn.Code, newLowerEnv(), srcmaps)
		}
	}
}

// lowerEnv tracks state needed during the lowering pass.
type lowerEnv struct {
	// nextLabel is a counter for generating unique placeholder labels.
	// Labels count downward from math.MaxUint to avoid colliding with real PCs.
	nextLabel uint
	// breakLabel is the placeholder label for break statements, or None if not in a loop.
	breakLabel util.Option[uint]
	// contLabel is the placeholder label for continue statements, or None if not in a loop.
	contLabel util.Option[uint]
}

func newLowerEnv() *lowerEnv {
	return &lowerEnv{nextLabel: ^uint(0)} // starts at MaxUint
}

func (e *lowerEnv) freshLabel() uint {
	lab := e.nextLabel
	e.nextLabel--

	return lab
}

// lowerStatements flattens a slice of statements starting at the given PC.
func lowerStatements(pc uint, stmts []stmt.Resolved, env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	var result []stmt.Resolved

	for _, s := range stmts {
		lowered := lowerStatement(pc, s, env, srcmaps)
		result = append(result, lowered...)
		pc += uint(len(lowered))
	}

	return result
}

// lowerStatement lowers a single statement into a flat sequence.
func lowerStatement(pc uint, s stmt.Resolved, env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	switch t := s.(type) {
	case *stmt.IfElse[symbol.Resolved]:
		return lowerIfElse(pc, t, env, srcmaps)
	case *stmt.Switch[symbol.Resolved]:
		return lowerSwitch(pc, t, env, srcmaps)
	case *stmt.While[symbol.Resolved]:
		return lowerWhile(pc, t, env, srcmaps)
	case *stmt.For[symbol.Resolved]:
		return lowerFor(pc, t, env, srcmaps)
	case *stmt.Break[symbol.Resolved]:
		return lowerBreak(t, env, srcmaps)
	case *stmt.Continue[symbol.Resolved]:
		return lowerContinue(t, env, srcmaps)
	case *stmt.VarDecl[symbol.Resolved]:
		return lowerVarDecl(t, srcmaps)
	default:
		return []stmt.Resolved{lowerStatementExprs(s, srcmaps)}
	}
}

// lowerStatementExprs lowers ternary conditions within the expressions of a
// flat (leaf) statement so that every Ternary.Cond is a single Cmp node.
func lowerStatementExprs(s stmt.Resolved, srcmaps source.Maps[any]) stmt.Resolved {
	switch t := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		ns := &stmt.Assign[symbol.Resolved]{Targets: t.Targets, Source: lowerExpr(t.Source, srcmaps)}
		srcmaps.Copy(s, ns)

		return ns
	case *stmt.Printf[symbol.Resolved]:
		ns := &stmt.Printf[symbol.Resolved]{Chunks: t.Chunks, Arguments: lowerExprs(t.Arguments, srcmaps)}
		srcmaps.Copy(s, ns)

		return ns
	default:
		return s
	}
}

// lowerExprs lowers a slice of expressions.
func lowerExprs(exprs []expr.Resolved, srcmaps source.Maps[any]) []expr.Resolved {
	result := make([]expr.Resolved, len(exprs))

	for i, e := range exprs {
		result[i] = lowerExpr(e, srcmaps)
	}

	return result
}

// lowerExpr recursively lowers ternary conditions within an expression so that
// every Ternary.Cond is a single Cmp node after lowering.  A new node is always
// created for composite expressions and its source map entry is copied from the
// original so that subsequent passes (e.g. type checking) can report errors with
// correct source locations.
func lowerExpr(e expr.Resolved, srcmaps source.Maps[any]) expr.Resolved {
	var ne expr.Resolved

	switch t := e.(type) {
	case *expr.Ternary[symbol.Resolved]:
		cond := lowerExpr(t.Cond, srcmaps)
		ifTrue := lowerExpr(t.IfTrue, srcmaps)
		ifFalse := lowerExpr(t.IfFalse, srcmaps)

		return lowerTernaryCondition(cond, ifTrue, ifFalse, srcmaps, e)
	case *expr.Cmp[symbol.Resolved]:
		ne = expr.NewCmp[symbol.Resolved](t.Operator, lowerExpr(t.Left, srcmaps), lowerExpr(t.Right, srcmaps))
	case *expr.LogicalAnd[symbol.Resolved]:
		ne = expr.NewLogicalAnd[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.LogicalOr[symbol.Resolved]:
		ne = expr.NewLogicalOr[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.LogicalNot[symbol.Resolved]:
		ne = expr.NewLogicalNot[symbol.Resolved](lowerExpr(t.Expr, srcmaps))
	case *expr.Add[symbol.Resolved]:
		ne = expr.NewAdd[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Sub[symbol.Resolved]:
		ne = expr.NewSub[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Mul[symbol.Resolved]:
		ne = expr.NewMul[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Div[symbol.Resolved]:
		ne = expr.NewDiv[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Rem[symbol.Resolved]:
		ne = expr.NewRem[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.BitwiseAnd[symbol.Resolved]:
		ne = expr.NewBitwiseAnd[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.BitwiseOr[symbol.Resolved]:
		ne = expr.NewBitwiseOr[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Xor[symbol.Resolved]:
		ne = expr.NewXor[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Shl[symbol.Resolved]:
		ne = expr.NewShl[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Shr[symbol.Resolved]:
		ne = expr.NewShr[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Concat[symbol.Resolved]:
		ne = expr.NewConcat[symbol.Resolved](lowerExprs(t.Exprs, srcmaps)...)
	case *expr.Cast[symbol.Resolved]:
		ne = expr.NewCast[symbol.Resolved](lowerExpr(t.Expr, srcmaps), t.CastType)
	case *expr.BitwiseNot[symbol.Resolved]:
		ne = expr.NewBitwiseNot[symbol.Resolved](lowerExpr(t.Expr, srcmaps))
	case *expr.ExternAccess[symbol.Resolved]:
		ne = expr.NewExternAccess[symbol.Resolved](t.Name, lowerExprs(t.Args, srcmaps)...)
	default:
		// Const, LocalAccess — leaf nodes with no sub-expressions to lower.
		return e
	}

	srcmaps.Copy(e, ne)

	return ne
}

// lowerTernaryCondition converts a ternary with a complex condition into nested
// ternaries each having a simple Cmp condition:
//   - (a && b) ? v1 : v2  →  a ? (b ? v1 : v2) : v2
//   - (a || b) ? v1 : v2  →  a ? v1 : (b ? v1 : v2)
//   - (!a)     ? v1 : v2  →  a ? v2 : v1
func lowerTernaryCondition(
	cond, ifTrue, ifFalse expr.Resolved, srcmaps source.Maps[any], orig expr.Resolved,
) expr.Resolved {
	switch c := cond.(type) {
	case *expr.Cmp[symbol.Resolved]:
		t := &expr.Ternary[symbol.Resolved]{Cond: cond, IfTrue: ifTrue, IfFalse: ifFalse}

		srcmaps.Copy(orig, t)

		return t
	case *expr.LogicalNot[symbol.Resolved]:
		return lowerTernaryCondition(c.Expr, ifFalse, ifTrue, srcmaps, orig)
	case *expr.LogicalAnd[symbol.Resolved]:
		first := c.Exprs[0]

		var rest expr.Resolved

		if len(c.Exprs) == 2 {
			rest = c.Exprs[1]
		} else {
			r := expr.NewLogicalAnd[symbol.Resolved](c.Exprs[1:]...)
			srcmaps.Copy(orig, r)
			rest = r
		}

		inner := lowerTernaryCondition(rest, ifTrue, ifFalse, srcmaps, orig)

		return lowerTernaryCondition(first, inner, ifFalse, srcmaps, orig)
	case *expr.LogicalOr[symbol.Resolved]:
		first := c.Exprs[0]

		var rest expr.Resolved

		if len(c.Exprs) == 2 {
			rest = c.Exprs[1]
		} else {
			r := expr.NewLogicalOr[symbol.Resolved](c.Exprs[1:]...)
			srcmaps.Copy(orig, r)
			rest = r
		}

		inner := lowerTernaryCondition(rest, ifTrue, ifFalse, srcmaps, orig)

		return lowerTernaryCondition(first, ifTrue, inner, srcmaps, orig)
	default:
		panic("unexpected condition type in ternary lowering")
	}
}

// lowerSwitch converts a switch statement to a nested if-(else-if)-else statement, e.g.
//
//	switch discr {
//		case A, B: { stmts_AB }
//		case C: { stmts_C }
//		default: { stmts_default }	// 'misplaced' default
//		case D, E, G { stmts_DEF }
//	}
//
// should convert to
//
//	if (discr == A || discr == B) {
//		stmts_AB
//	} else if (discr == C) {
//		stmts_C
//	} else if (discr == D || discr == E || discr == F) {
//		stmts_DEF
//	} else {
//		stmts_default
//	}
//
// and applies the lowering to the resulting if-then-else statement.
//
// Note: the default statement, if present, is moved to the deepest nesting level.
func lowerSwitch(pc uint, s *stmt.Switch[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	// special case: empty switch statement
	if len(s.Branches) == 0 {
		return []stmt.Resolved{}
	}

	var (
		defaultCaseCount uint
		containsDefault  bool
	)

	// pathological case with more than one default cases
	if defaultCaseCount = s.DefaultCaseCount(); defaultCaseCount > 1 {
		return nil
	}

	containsDefault = defaultCaseCount == 1

	// special case: the default case is the only case in the switch statement
	if len(s.Branches) == 1 && containsDefault {
		return s.Branches[0].Body
	}

	// beyond this point a proper (non default) case is present
	var (
		defaultStatement     *[]stmt.Stmt[symbol.Resolved]
		equivalentIfThenElse *stmt.IfElse[symbol.Resolved]
		mostNestedIfThenElse *stmt.IfElse[symbol.Resolved]
		falseBranch          *stmt.IfElse[symbol.Resolved]
	)

	// this loop builds a nested if-then-else statement
	for _, branch := range s.Branches {
		// if we come across the default statement we store it
		// and continue to the next branch
		if branch.IsDefault {
			defaultStatement = &branch.Body
			continue
		}

		logicalOrOfCases := branch.LogicalOrOfCases(s.Discriminant)

		// we initialize the equivalent if-then-else statement and point the
		// 'most nested if-then-else statement' to it
		if equivalentIfThenElse == nil {
			equivalentIfThenElse = &stmt.IfElse[symbol.Resolved]{
				Cond:        &logicalOrOfCases,
				TrueBranch:  branch.Body,
				FalseBranch: []stmt.Stmt[symbol.Resolved]{},
			}
			srcmaps.Copy(s, equivalentIfThenElse)
			mostNestedIfThenElse = equivalentIfThenElse
		} else {
			falseBranch = &stmt.IfElse[symbol.Resolved]{
				Cond:        &logicalOrOfCases,
				TrueBranch:  branch.Body,
				FalseBranch: []stmt.Stmt[symbol.Resolved]{},
			}
			srcmaps.Copy(s, falseBranch)
			mostNestedIfThenElse.FalseBranch = append(mostNestedIfThenElse.FalseBranch, falseBranch)
			mostNestedIfThenElse = falseBranch
		}
	}

	// the default statement, if present, becomes the final "else"
	// of the equivalent nested if-then-else statement
	if containsDefault {
		mostNestedIfThenElse.FalseBranch = *defaultStatement
	}

	return lowerIfElse(pc, equivalentIfThenElse, env, srcmaps)
}

func lowerIfElse(pc uint, s *stmt.IfElse[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	falseLabel := env.freshLabel()
	// Flatten condition: generates IfGoto/Assign sequence that jumps to falseLabel if condition is false
	condInsns := flatternCondition(s.Cond, pc, false, falseLabel, env, s, srcmaps)
	n := uint(len(condInsns))
	// Lower true branch
	trueBranch := lowerStatements(pc+n, s.TrueBranch, env, srcmaps)
	falseTarget := pc + n + uint(len(trueBranch))
	// Build up sequence starting with condition
	insns := append(condInsns, trueBranch...)

	if len(s.FalseBranch) > 0 {
		// Check if true branch ends with an unconditional terminator (return/fail/goto)
		trueTerminates := branchTerminates(s.TrueBranch)
		if !trueTerminates {
			falseTarget++ // account for the skip-goto we're about to add
		}
		// Lower false branch
		falseBranch := lowerStatements(falseTarget, s.FalseBranch, env, srcmaps)
		// Add bypass goto if the true branch may fall through
		if !trueTerminates {
			endTarget := falseTarget + uint(len(falseBranch))
			bypass := &stmt.Goto[symbol.Resolved]{Target: endTarget}
			srcmaps.Copy(s, bypass)
			insns = append(insns, bypass)
		}

		insns = append(insns, falseBranch...)
	}

	// Patch the false label to its actual PC
	patchBranches(falseLabel, insns, falseTarget)

	return insns
}

func lowerWhile(pc uint, s *stmt.While[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	breakLabel := env.freshLabel()
	contLabel := env.freshLabel()
	condLabel := env.freshLabel()
	// Flatten condition
	condInsns := flatternCondition(s.Cond, pc, false, condLabel, env, s, srcmaps)
	n := uint(len(condInsns))
	// Lower body with loop context
	innerEnv := *env
	innerEnv.breakLabel = util.Some(breakLabel)
	innerEnv.contLabel = util.Some(contLabel)
	body := lowerStatements(pc+n, s.Body, &innerEnv, srcmaps)
	env.nextLabel = innerEnv.nextLabel // sync label counter
	// Build sequence: cond + body + back-goto
	insns := append(condInsns, body...)
	backGoto := &stmt.Goto[symbol.Resolved]{Target: pc}
	srcmaps.Copy(s, backGoto)
	insns = append(insns, backGoto)
	exitTarget := pc + uint(len(insns))
	// Patch labels
	patchBranches(condLabel, insns, exitTarget)
	patchBranches(breakLabel, insns, exitTarget)
	patchBranches(contLabel, insns, pc)

	return insns
}

func lowerFor(pc uint, s *stmt.For[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	breakLabel := env.freshLabel()
	contLabel := env.freshLabel()
	condLabel := env.freshLabel()
	// Lower init statement
	initInsns := lowerStatement(pc, s.Init, env, srcmaps)
	condPC := pc + uint(len(initInsns))
	// Flatten condition
	condInsns := flatternCondition(s.Cond, condPC, false, condLabel, env, s, srcmaps)
	bodyPC := condPC + uint(len(condInsns))
	// Lower body with loop context
	innerEnv := *env
	innerEnv.breakLabel = util.Some(breakLabel)
	innerEnv.contLabel = util.Some(contLabel)
	body := lowerStatements(bodyPC, s.Body, &innerEnv, srcmaps)
	env.nextLabel = innerEnv.nextLabel
	postPC := bodyPC + uint(len(body))
	// Lower post statement
	postInsns := lowerStatement(postPC, s.Post, env, srcmaps)
	// Build instruction sequence
	insns := append(initInsns, condInsns...)
	insns = append(insns, body...)
	insns = append(insns, postInsns...)
	// Add back-goto to re-evaluate condition
	backGoto := &stmt.Goto[symbol.Resolved]{Target: condPC}
	srcmaps.Copy(s, backGoto)
	insns = append(insns, backGoto)
	exitTarget := pc + uint(len(insns))
	// Patch labels
	patchBranches(condLabel, insns, exitTarget)
	patchBranches(breakLabel, insns, exitTarget)
	patchBranches(contLabel, insns, postPC)

	return insns
}

func lowerBreak(s *stmt.Break[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	if !env.breakLabel.HasValue() {
		panic("break outside loop (should have been caught by parser)")
	}

	g := &stmt.Goto[symbol.Resolved]{Target: env.breakLabel.Unwrap()}
	srcmaps.Copy(s, g)

	return []stmt.Resolved{g}
}

func lowerContinue(s *stmt.Continue[symbol.Resolved], env *lowerEnv, srcmaps source.Maps[any]) []stmt.Resolved {
	if !env.contLabel.HasValue() {
		panic("continue outside loop (should have been caught by parser)")
	}

	g := &stmt.Goto[symbol.Resolved]{Target: env.contLabel.Unwrap()}
	srcmaps.Copy(s, g)

	return []stmt.Resolved{g}
}

func lowerVarDecl(s *stmt.VarDecl[symbol.Resolved], srcmaps source.Maps[any]) []stmt.Resolved {
	if s.Init.IsEmpty() {
		return nil
	}

	assign := &stmt.Assign[symbol.Resolved]{
		Targets: []lval.LVal[symbol.Resolved]{lval.NewVariable[symbol.Resolved](s.Variables[0])},
		Source:  lowerExpr(s.Init.Unwrap(), srcmaps),
	}
	srcmaps.Copy(s, assign)

	return []stmt.Resolved{assign}
}

// flatternCondition converts a condition expression into a flat sequence of
// IfGoto/Goto statements.  sign=false means "jump to target if condition is false"
// (the normal use for if/while/for).
func flatternCondition(cond expr.Expr[symbol.Resolved], pc uint, sign bool, target uint,
	env *lowerEnv, orig stmt.Resolved, srcmaps source.Maps[any]) []stmt.Resolved {
	switch c := cond.(type) {
	case *expr.Cmp[symbol.Resolved]:
		return flatternComparison(c, sign, target, orig, srcmaps)
	case *expr.LogicalAnd[symbol.Resolved]:
		if sign {
			return flatternLogicalAnd(c.Exprs, pc, true, target, env, orig, srcmaps)
		}

		return flatternLogicalOr(c.Exprs, pc, false, target, env, orig, srcmaps)
	case *expr.LogicalOr[symbol.Resolved]:
		if sign {
			return flatternLogicalOr(c.Exprs, pc, true, target, env, orig, srcmaps)
		}

		return flatternLogicalAnd(c.Exprs, pc, false, target, env, orig, srcmaps)
	case *expr.LogicalNot[symbol.Resolved]:
		return flatternCondition(c.Expr, pc, !sign, target, env, orig, srcmaps)
	default:
		panic("invalid condition type (should have been caught by parser)")
	}
}

func flatternLogicalAnd(args []expr.Expr[symbol.Resolved], pc uint, sign bool, target uint,
	env *lowerEnv, orig stmt.Resolved, srcmaps source.Maps[any]) []stmt.Resolved {
	label := env.freshLabel()

	var stmts []stmt.Resolved

	for _, arg := range args {
		ss := flatternCondition(arg, pc+uint(len(stmts)), !sign, label, env, orig, srcmaps)
		stmts = append(stmts, ss...)
	}
	// Success path: jump to the target
	g := &stmt.Goto[symbol.Resolved]{Target: target}
	srcmaps.Copy(orig, g)
	stmts = append(stmts, g)
	// Patch the short-circuit label to point past the success goto
	patchBranches(label, stmts, pc+uint(len(stmts)))

	return stmts
}

func flatternLogicalOr(args []expr.Expr[symbol.Resolved], pc uint, sign bool, target uint,
	env *lowerEnv, orig stmt.Resolved, srcmaps source.Maps[any]) []stmt.Resolved {
	var stmts []stmt.Resolved

	for _, arg := range args {
		ss := flatternCondition(arg, pc+uint(len(stmts)), sign, target, env, orig, srcmaps)
		stmts = append(stmts, ss...)
	}

	return stmts
}

func flatternComparison(cond *expr.Cmp[symbol.Resolved], sign bool, target uint,
	orig stmt.Resolved, srcmaps source.Maps[any]) []stmt.Resolved {
	var ifg *stmt.IfGoto[symbol.Resolved]

	if sign {
		ifg = &stmt.IfGoto[symbol.Resolved]{Cond: cond, Target: target}
	} else {
		ifg = &stmt.IfGoto[symbol.Resolved]{Cond: cond.Negate(), Target: target}
	}

	srcmaps.Copy(orig, ifg)

	return []stmt.Resolved{ifg}
}

// patchBranches replaces all occurrences of the given label in branch targets
// (Goto and IfGoto) with the given target PC.
func patchBranches(label uint, insns []stmt.Resolved, target uint) {
	for _, insn := range insns {
		if g, ok := insn.(*stmt.Goto[symbol.Resolved]); ok && g.Target == label {
			g.Target = target
		} else if g, ok := insn.(*stmt.IfGoto[symbol.Resolved]); ok && g.Target == label {
			g.Target = target
		}
	}
}

// branchTerminates returns true if the last statement in a sequence is an
// unconditional terminator (Return, Fail, Goto, or another IfElse that
// terminates both branches).
func branchTerminates(stmts []stmt.Resolved) bool {
	if len(stmts) == 0 {
		return false
	}

	switch t := stmts[len(stmts)-1].(type) {
	case *stmt.Break[symbol.Resolved]:
		return true
	case *stmt.Continue[symbol.Resolved]:
		return true
	case *stmt.Return[symbol.Resolved]:
		return true
	case *stmt.Fail[symbol.Resolved]:
		return true
	case *stmt.Goto[symbol.Resolved]:
		return true
	case *stmt.IfElse[symbol.Resolved]:
		return len(t.FalseBranch) > 0 && branchTerminates(t.TrueBranch) && branchTerminates(t.FalseBranch)
	default:
		return false
	}
}
