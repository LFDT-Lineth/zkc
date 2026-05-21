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
			varMapping := make([]VarMapping, len(fn.Variables))

			// Expand for variables and assignments
			expandedVars, expandedCode, hasArray := expandFixedArrays(fn, varMapping, env)
			// If no fixed-size array variables were found, skip the rewrite
			if !hasArray {
				continue
			}

			// Rewrite the expanded code to replace array accesses with scalar references
			rewrittenCode := rewriteFixedArrays(expandedCode, varMapping, program.Components(), env)

			// After rewriting, update fn's code, variables, input and output counts to reflect the expanded scalars
			fn.Code = rewrittenCode
			fn.Variables = expandedVars
			fn.NumInputs = countVarsOfKind(expandedVars, variable.PARAMETER)
			fn.NumOutputs = countVarsOfKind(expandedVars, variable.RETURN)
		}
	}
}

// VarMapping records how an old variable ID maps into the expanded variable
// list.  For scalar variables newBase is the single new ID.  For fixed arrays
// newBase..newBase+size-1 are the individual element variables and elemType
// is the canonical element type used by the lowering helpers.  elemType
// is nil for non-array entries.
type VarMapping struct {
	newBase  uint
	isArray  bool
	size     uint
	elemType data.Type[symbol.Resolved]
}

// PcMapping records that `shift` statements were inserted at the PC
// `pivotPC` during fixed-array expansion.  When remapping branch targets,
// any PC strictly greater than `pivotPC` is shifted forward by `shift`
// statements.
type PcMapping struct {
	pivotPC uint
	shift   uint
}

// expandFixedArrays builds the old→new id mapping and expands fixed-size array
// variables into scalars, expands whole-array assignment statements into element-wise
// array access assignments, and expands bare array arguments in ExternAccess calls
// into individual indexed accesses (e.g. sum(items) becomes
// sum(items[0], items[1], items[2])).  All expanded nodes use the original
// variable IDs so that the subsequent rewriting phase can remap them.
func expandFixedArrays(
	fn *decl.ResolvedFunction, varMapping []VarMapping, env ast.Environment,
) (expandedVars []variable.ResolvedDescriptor, expandedCode []stmt.Resolved, hasArray bool) {
	expandedVars, hasArray = expandFnVariables(fn, varMapping, env)

	if !hasArray {
		return
	}
	//
	expandedCode, pcMapping := expandFnCode(fn, varMapping, env)

	// remap the PC of the expanded code for
	if len(pcMapping) > 0 {
		for _, s := range expandedCode {
			switch s := s.(type) {
			case *stmt.IfGoto[symbol.Resolved]:
				s.Target = remapPC(s.Target, pcMapping)
			case *stmt.Goto[symbol.Resolved]:
				s.Target = remapPC(s.Target, pcMapping)
			}
		}
	}

	return
}

func expandFnVariables(
	fn *decl.ResolvedFunction, varMapping []VarMapping, env ast.Environment,
) (expandedVars []variable.ResolvedDescriptor, hasArray bool) {
	for oldID, v := range fn.Variables {
		base := uint(len(expandedVars))

		if vType := v.DataType.AsFixedArray(env); vType != nil {
			size := vType.Size.First()

			hasArray = true

			bitwidth, ok := data.BitWidthOf(vType, env)
			if !ok {
				panic("expected bitwidth to be resolved for the fixed array")
			}
			//
			elemType := data.NewUnsignedInt[symbol.Resolved](bitwidth, false)

			for j := range size {
				name := v.Name + "$" + strconv.FormatUint(uint64(j), 10)
				expandedVars = append(expandedVars, variable.New[symbol.Resolved](v.Kind, name, elemType))
			}

			varMapping[oldID] = VarMapping{newBase: base, isArray: true, size: size, elemType: elemType}
		} else {
			varMapping[oldID] = VarMapping{newBase: base}

			expandedVars = append(expandedVars, v)
		}
	}

	return
}

func expandFnCode(
	fn *decl.ResolvedFunction, varMapping []VarMapping, env ast.Environment,
) (expandedCode []stmt.Resolved, pcMapping []PcMapping) {
	var origPC uint
	//
	for _, s := range fn.Code {
		switch s := s.(type) {
		case *stmt.Assign[symbol.Resolved]:
			// Break whole-array assignments into per-element assignments if any
			if expanded := expandWholeArrayAssign(s, varMapping, env); expanded != nil {
				expandedCode = append(expandedCode, expanded...)

				if expLength := uint(len(expanded)); expLength > 1 {
					pcMapping = append(pcMapping, PcMapping{pivotPC: origPC, shift: expLength - 1})
				}

				origPC++

				continue
			}

			// Expand the targets
			for i, lv := range s.Targets {
				s.Targets[i] = expandLValArray(lv, varMapping, env)
			}

			// Expand the source
			s.Source = expandArrayExpression(s.Source, varMapping, env)
		case *stmt.IfGoto[symbol.Resolved]:
			if cmp, ok := s.Cond.(*expr.Cmp[symbol.Resolved]); ok {
				// Break whole-array equality/inequality: expand into per-element IfGotos.
				if expanded := expandWholeArrayCmp(s, cmp, varMapping, origPC); expanded != nil {
					expandedCode = append(expandedCode, expanded...)

					if expLength := uint(len(expanded)); expLength > 1 {
						pcMapping = append(pcMapping, PcMapping{pivotPC: origPC, shift: expLength - 1})
					}

					origPC++

					continue
				}

				cmp.Left = expandArrayExpression(cmp.Left, varMapping, env)
				cmp.Right = expandArrayExpression(cmp.Right, varMapping, env)
			}
		case *stmt.Printf[symbol.Resolved]:
			for i, arg := range s.Arguments {
				s.Arguments[i] = expandArrayExpression(arg, varMapping, env)
			}
		}

		expandedCode = append(expandedCode, s)
		origPC++
	}
	//
	return
}

func expandArrayExpression(e expr.Resolved, varMapping []VarMapping, env ast.Environment) expr.Resolved {
	switch e := e.(type) {
	case *expr.ExternAccess[symbol.Resolved]:
		e.Args = expandBareArray(e.Args, varMapping, env)
		return e
	case *expr.Add[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Sub[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Mul[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Div[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Rem[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Shl[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Shr[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.BitwiseAnd[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.BitwiseOr[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Xor[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.BitwiseNot[symbol.Resolved]:
		e.Expr = expandArrayExpression(e.Expr, varMapping, env)
		return e
	case *expr.LogicalAnd[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.LogicalOr[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.LogicalNot[symbol.Resolved]:
		e.Expr = expandArrayExpression(e.Expr, varMapping, env)
		return e
	case *expr.Cast[symbol.Resolved]:
		e.Expr = expandArrayExpression(e.Expr, varMapping, env)
		return e
	case *expr.Concat[symbol.Resolved]:
		expandArrayExpressions(e.Exprs, varMapping, env)
		return e
	case *expr.Cmp[symbol.Resolved]:
		e.Left = expandArrayExpression(e.Left, varMapping, env)
		e.Right = expandArrayExpression(e.Right, varMapping, env)

		return e
	case *expr.Ternary[symbol.Resolved]:
		// Break the condition if it is a whole-array comparison
		if cmp, ok := e.Cond.(*expr.Cmp[symbol.Resolved]); ok {
			if expanded := expandArrayCmpTernary(e, cmp, varMapping, env); expanded != nil {
				return expanded
			}
		}

		e.Cond = expandArrayExpression(e.Cond, varMapping, env)
		e.IfTrue = expandArrayExpression(e.IfTrue, varMapping, env)
		e.IfFalse = expandArrayExpression(e.IfFalse, varMapping, env)

		return e
	default:
		return e
	}
}

func expandArrayExpressions(exprs []expr.Resolved, varMapping []VarMapping, env ast.Environment) {
	for i, e := range exprs {
		exprs[i] = expandArrayExpression(e, varMapping, env)
	}
}

func expandLValArray(l lval.Resolved, varMapping []VarMapping, env ast.Environment) lval.Resolved {
	switch l := l.(type) {
	case *lval.MemAccess[symbol.Resolved]:
		expandArrayExpressions(l.Args, varMapping, env)
		return l
	default:
		return l
	}
}

// expandBareArray expands bare array variable arguments into individual
// ArrayAccess expressions
// e.g. sum(items) becomes sum(items[0], items[1], items[2])
func expandBareArray(args []expr.Resolved, varMapping []VarMapping, env ast.Environment) []expr.Resolved {
	var result []expr.Resolved

	for _, arg := range args {
		if la, ok := arg.(*expr.LocalAccess[symbol.Resolved]); ok {
			m := varMapping[la.Variable]
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

		result = append(result, expandArrayExpression(arg, varMapping, env))
	}

	return result
}

func rewriteFixedArrays(
	expandedCode []stmt.Resolved, varMapping []VarMapping,
	declarations []codegen.Declaration, env ast.Environment,
) (newCode []stmt.Resolved) {
	for _, s := range expandedCode {
		newCode = append(newCode, rewriteFixedArrayStmt(s, varMapping, declarations, env))
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

type arrayTarget struct {
	id   variable.Id
	size uint
}

// expandWholeArrayAssign expands whole-array to per-element assignments.
// Supported cases:
//   - a = b (one target, LocalAccess source)
//   - a = f(...) (one target, function returns one array)
//   - a, b, ... = f(...) (several targets, function returns a matching tuple of arrays)
func expandWholeArrayAssign(
	s *stmt.Assign[symbol.Resolved], varMapping []VarMapping, env ast.Environment,
) []stmt.Resolved {
	var targets []arrayTarget

	for _, t := range s.Targets {
		switch lv := t.(type) {
		case *lval.Variable[symbol.Resolved]:
			// Destructuring is checked in the typing phase, so len(lv.Ids) must be 1
			lhsID := lv.Ids[0]
			lm := varMapping[lhsID]
			// if lm is not an array, we don't need the expansion
			if !lm.isArray {
				return nil
			}

			targets = append(targets, arrayTarget{id: lhsID, size: lm.size})
		default:
			return nil
		}
	}

	if len(targets) == 0 {
		return nil
	}

	switch src := s.Source.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		// Case a = b
		// Does not support tuple assignments a, b = b, c
		if len(targets) != 1 {
			return nil
		}

		tgt := targets[0]
		elemType := varMapping[src.Variable].elemType

		splitAssignments := make([]stmt.Resolved, tgt.size)

		for i := range tgt.size {
			idx := *big.NewInt(int64(i))
			splitAssignments[i] = &stmt.Assign[symbol.Resolved]{
				Targets: []lval.LVal[symbol.Resolved]{
					lval.NewArray[symbol.Resolved](tgt.id, expr.NewConstant[symbol.Resolved](idx, 10)),
				},
				Source: &expr.ArrayAccess[symbol.Resolved]{
					Id:       src.Variable,
					Arg:      expr.NewConstant[symbol.Resolved](idx, 10),
					Datatype: elemType,
				},
			}
		}

		return splitAssignments
	case *expr.ExternAccess[symbol.Resolved]:
		// Case a, b, ... = f(...)
		var elemTargets []lval.LVal[symbol.Resolved]

		for _, tgt := range targets {
			for i := range tgt.size {
				idx := *big.NewInt(int64(i))
				elemTarget := lval.NewArray[symbol.Resolved](
					tgt.id, expr.NewConstant[symbol.Resolved](idx, 10),
				)
				elemTargets = append(elemTargets, elemTarget)
			}
		}

		src.Args = expandBareArray(src.Args, varMapping, env)

		return []stmt.Resolved{&stmt.Assign[symbol.Resolved]{
			Targets: elemTargets,
			Source:  src,
		}}
	}

	return nil
}

// expandWholeArrayCmp expands an IfGoto whose condition is a whole-array
// equality comparison (== or !=) on two bare-array LocalAccess operands into
// a sequence of element-wise IfGotos.
//
// Targets emitted by this helper are with the old PC.
func expandWholeArrayCmp(
	ifg *stmt.IfGoto[symbol.Resolved], cmp *expr.Cmp[symbol.Resolved],
	varMapping []VarMapping, origPC uint,
) []stmt.Resolved {
	// We only expand whole-array comparisons else we return nil
	l, lok := cmp.Left.(*expr.LocalAccess[symbol.Resolved])
	r, rok := cmp.Right.(*expr.LocalAccess[symbol.Resolved])
	//
	if !lok || !rok {
		return nil
	}
	//
	lm, rm := varMapping[l.Variable], varMapping[r.Variable]
	if !lm.isArray || !rm.isArray {
		return nil
	}
	//
	elemType := lm.elemType
	//
	var splitIfGotos []stmt.Resolved
	//
	for i := uint(0); i < lm.size; i++ {
		// EQ: any element differs -> SKIP (fall through, do not take branch).
		// NEQ: any element differs -> take branch.
		var elemTarget uint
		if cmp.Operator == expr.EQ {
			elemTarget = origPC + 1
		} else {
			elemTarget = ifg.Target
		}
		//
		splitIfGotos = append(splitIfGotos, &stmt.IfGoto[symbol.Resolved]{
			Cond:   newElementCmp(expr.NEQ, l.Variable, r.Variable, i, elemType),
			Target: elemTarget,
		})
	}
	//
	if cmp.Operator == expr.EQ {
		// All elements matched -> take the branch.
		splitIfGotos = append(splitIfGotos, &stmt.Goto[symbol.Resolved]{Target: ifg.Target})
	}
	//
	return splitIfGotos
}

// expandArrayCmpTernary rewrites a Ternary whose condition is a whole-array
// equality comparison (== or !=) on two bare-array LocalAccess operands into
// a right-nested chain of scalar ternaries.  Returns nil if the Ternary does
// not match this shape;
//
// EQ semantics: take T iff every element matches.  Build a right-nested chain
// in the IfTrue slot; IfFalse stays constant:
//
//	LHS[0] == RHS[0] ? (LHS[1] == RHS[1] ? ... ? T : F) : F
//
// NEQ semantics: take T iff any element differs.  Build the dual chain in the
// IfFalse slot; IfTrue stays constant:
//
//	LHS[0] != RHS[0] ? T : (LHS[1] != RHS[1] ? T : ... : F)
func expandArrayCmpTernary(
	tern *expr.Ternary[symbol.Resolved], cmp *expr.Cmp[symbol.Resolved],
	varMapping []VarMapping, env ast.Environment,
) expr.Resolved {
	// We only expand whole-array comparisons else we return nil
	l, lok := cmp.Left.(*expr.LocalAccess[symbol.Resolved])
	r, rok := cmp.Right.(*expr.LocalAccess[symbol.Resolved])
	//
	if !lok || !rok {
		return nil
	}
	//
	lm, rm := varMapping[l.Variable], varMapping[r.Variable]
	if !lm.isArray || !rm.isArray {
		return nil
	}
	// Recursively expand the two branches first; they may themselves contain
	// whole-array operations.
	ifTrue := expandArrayExpression(tern.IfTrue, varMapping, env)
	ifFalse := expandArrayExpression(tern.IfFalse, varMapping, env)
	//
	elemType := lm.elemType
	//
	// The "constant" branch is the one that occupies the same slot in every
	// level of the nested chain (IfFalse for EQ, IfTrue for NEQ).  Since the
	// subsequent rewrite phase mutates expression nodes in place (notably the
	// LocalAccess.Variable remap), we cannot let any node be referenced from
	// multiple positions in the resulting tree -- it would be remapped more
	// than once,
	//
	// We therefore use the original constBranch node on the deepest level and
	// a fresh deep copy of it for every shallower level.
	var inner expr.Resolved
	//
	if cmp.Operator == expr.EQ {
		inner = ifTrue
		constBranch := ifFalse
		//
		for i := int(lm.size) - 1; i >= 0; i-- {
			t := &expr.Ternary[symbol.Resolved]{
				Cond:    newElementCmp(cmp.Operator, l.Variable, r.Variable, uint(i), elemType),
				IfTrue:  inner,
				IfFalse: constBranch,
			}
			t.SetType(tern.Type())
			inner = t
			constBranch = cloneExpr(constBranch)
		}
	} else {
		inner = ifFalse
		constBranch := ifTrue
		//
		for i := int(lm.size) - 1; i >= 0; i-- {
			t := &expr.Ternary[symbol.Resolved]{
				Cond:    newElementCmp(cmp.Operator, l.Variable, r.Variable, uint(i), elemType),
				IfTrue:  constBranch,
				IfFalse: inner,
			}
			t.SetType(tern.Type())
			inner = t
			constBranch = cloneExpr(ifTrue)
		}
	}
	//
	return inner
}

// cloneExpr returns a deep copy of e.
func cloneExpr(e expr.Resolved) expr.Resolved {
	switch e := e.(type) {
	case *expr.Const[symbol.Resolved]:
		return e
	case *expr.LocalAccess[symbol.Resolved]:
		c := *e
		return &c
	case *expr.ArrayAccess[symbol.Resolved]:
		c := *e
		c.Arg = cloneExpr(e.Arg)

		return &c
	case *expr.Add[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Sub[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Mul[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Div[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Rem[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Shl[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Shr[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.BitwiseAnd[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.BitwiseOr[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Xor[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.BitwiseNot[symbol.Resolved]:
		c := *e
		c.Expr = cloneExpr(e.Expr)

		return &c
	case *expr.LogicalAnd[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.LogicalOr[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.LogicalNot[symbol.Resolved]:
		c := *e
		c.Expr = cloneExpr(e.Expr)

		return &c
	case *expr.Cast[symbol.Resolved]:
		c := *e
		c.Expr = cloneExpr(e.Expr)

		return &c
	case *expr.Concat[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	case *expr.Cmp[symbol.Resolved]:
		c := *e
		c.Left = cloneExpr(e.Left)
		c.Right = cloneExpr(e.Right)

		return &c
	case *expr.Ternary[symbol.Resolved]:
		c := *e
		c.Cond = cloneExpr(e.Cond)
		c.IfTrue = cloneExpr(e.IfTrue)
		c.IfFalse = cloneExpr(e.IfFalse)

		return &c
	case *expr.ExternAccess[symbol.Resolved]:
		c := *e
		c.Args = cloneExprs(e.Args)

		return &c
	case *expr.TupleInitialiser[symbol.Resolved]:
		c := *e
		c.Exprs = cloneExprs(e.Exprs)

		return &c
	default:
		panic(fmt.Sprintf("unhandled expression in cloneExpr: %T", e))
	}
}

// cloneExprs returns a fresh slice containing deep copies of every element
// of exprs.
func cloneExprs(exprs []expr.Resolved) []expr.Resolved {
	clonedExpressions := make([]expr.Resolved, len(exprs))
	for i, e := range exprs {
		clonedExpressions[i] = cloneExpr(e)
	}

	return clonedExpressions
}

// newElementCmp builds an element-wise comparison `lhsID[i] op rhsID[i]`
func newElementCmp(
	op expr.CmpOp, lhsID, rhsID variable.Id, i uint, elemType data.Type[symbol.Resolved],
) *expr.Cmp[symbol.Resolved] {
	idx := *big.NewInt(int64(i))
	lhs := &expr.ArrayAccess[symbol.Resolved]{
		Id: lhsID, Arg: expr.NewConstant[symbol.Resolved](idx, 10), Datatype: elemType,
	}
	rhs := &expr.ArrayAccess[symbol.Resolved]{
		Id: rhsID, Arg: expr.NewConstant[symbol.Resolved](idx, 10), Datatype: elemType,
	}

	return expr.NewCmp(op, lhs, rhs)
}

// remapPC rewrites the PC of a statement from old to new PC space based on the recorded shifts.
func remapPC(oldPC uint, pcMapping []PcMapping) uint {
	var acc uint
	//
	for _, s := range pcMapping {
		// if the PC is before the pivot, we don't apply the shift
		if oldPC <= s.pivotPC {
			break
		}

		acc += s.shift
	}
	//
	return oldPC + acc
}

func rewriteFixedArrayStmt(
	s stmt.Resolved, varMapping []VarMapping,
	declarations []codegen.Declaration, env ast.Environment,
) stmt.Resolved {
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		for i, lv := range s.Targets {
			s.Targets[i] = rewriteLValArray(lv, varMapping, declarations, env)
		}

		s.Source = rewriteArrayExpression(s.Source, varMapping, declarations, env)

		return s
	case *stmt.IfGoto[symbol.Resolved]:
		c, ok := s.Cond.(*expr.Cmp[symbol.Resolved])
		if !ok {
			panic(fmt.Sprintf("unknown condition encountered during fixed-array lowering: %T", s.Cond))
		}

		c.Left = rewriteArrayExpression(c.Left, varMapping, declarations, env)
		c.Right = rewriteArrayExpression(c.Right, varMapping, declarations, env)

		return s
	case *stmt.Printf[symbol.Resolved]:
		for i, arg := range s.Arguments {
			s.Arguments[i] = rewriteArrayExpression(arg, varMapping, declarations, env)
		}

		return s
	case *stmt.Return[symbol.Resolved], *stmt.Goto[symbol.Resolved], *stmt.Fail[symbol.Resolved]:
		return s
	default:
		panic(fmt.Sprintf("unknown statement encountered during fixed-array lowering: %T", s))
	}
}

func rewriteArrayExpression(
	e expr.Resolved, varMapping []VarMapping,
	declarations []codegen.Declaration, env ast.Environment,
) expr.Resolved {
	switch e := e.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		e.Variable = varMapping[e.Variable].newBase
		//
		return e
	case *expr.ArrayAccess[symbol.Resolved]:
		rewriteArrayExpression(e.Arg, varMapping, declarations, env)

		m := varMapping[e.Id]
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
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Sub[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Mul[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Div[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Rem[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Shl[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Shr[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.BitwiseAnd[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.BitwiseOr[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.Xor[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.BitwiseNot[symbol.Resolved]:
		e.Expr = rewriteArrayExpression(e.Expr, varMapping, declarations, env)
		return e
	case *expr.LogicalAnd[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.LogicalOr[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)
		return e
	case *expr.LogicalNot[symbol.Resolved]:
		e.Expr = rewriteArrayExpression(e.Expr, varMapping, declarations, env)
		return e
	case *expr.Cast[symbol.Resolved]:
		e.Expr = rewriteArrayExpression(e.Expr, varMapping, declarations, env)
		return e
	case *expr.Concat[symbol.Resolved]:
		rewriteArrayExpressions(e.Exprs, varMapping, declarations, env)

		return e
	case *expr.Cmp[symbol.Resolved]:
		e.Left = rewriteArrayExpression(e.Left, varMapping, declarations, env)
		e.Right = rewriteArrayExpression(e.Right, varMapping, declarations, env)

		return e
	case *expr.Ternary[symbol.Resolved]:
		e.Cond = rewriteArrayExpression(e.Cond, varMapping, declarations, env)
		e.IfTrue = rewriteArrayExpression(e.IfTrue, varMapping, declarations, env)
		e.IfFalse = rewriteArrayExpression(e.IfFalse, varMapping, declarations, env)

		return e
	case *expr.ExternAccess[symbol.Resolved]:
		rewriteArrayExpressions(e.Args, varMapping, declarations, env)
		return e
	case *expr.Const[symbol.Resolved]:
		return e
	default:
		panic(fmt.Sprintf("unknown expression encountered during fixed-array lowering: %T", e))
	}
}

func rewriteArrayExpressions(
	exprs []expr.Resolved, varMapping []VarMapping,
	declarations []codegen.Declaration, env ast.Environment,
) {
	for i, e := range exprs {
		exprs[i] = rewriteArrayExpression(e, varMapping, declarations, env)
	}
}

func rewriteLValArray(
	l lval.Resolved, varMapping []VarMapping,
	declarations []codegen.Declaration, env ast.Environment,
) lval.Resolved {
	switch l := l.(type) {
	case *lval.Variable[symbol.Resolved]:
		for i, id := range l.Ids {
			m := varMapping[id]
			l.Ids[i] = m.newBase
		}

		return l
	case *lval.Array[symbol.Resolved]:
		rewriteArrayExpression(l.Arg, varMapping, declarations, env)

		m := varMapping[l.Id]
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
		rewriteArrayExpressions(l.Args, varMapping, declarations, env)
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
