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

	"github.com/consensys/go-corset/pkg/util/field"
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
func FlattenFixedArrays(field field.Config, program ast.Program) {
	env := program.Environment()

	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok {
			var (
				varMapping  = make([]VarMapping, len(fn.Variables))
				rewriter = &Rewriter{field, varMapping, program.Components(), env}
			)
			// Expand for variables and assignments
			expandedVars, expandedCode, hasArray := expandFixedArrays(fn, varMapping, env)
			// If no fixed-size array variables were found, skip the rewrite
			if !hasArray {
				continue
			}
			// Rewrite the expanded code to replace array accesses with scalar references
			rewrittenCode := rewriter.rewriteFixedArrays(expandedCode)
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
					// construct array index
					index := expr.NewTypedConstant[symbol.Resolved](idx, 10, uint(idx.BitLen()))
					// construct array access
					access := &expr.ArrayAccess[symbol.Resolved]{
						Id:       la.Variable,
						Arg:      index,
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

// Rewriter encapsulates a statement / expression rewriter currently targeted
// towards array expressions.
type Rewriter struct {
	field        field.Config
	varMapping   []VarMapping
	declarations []codegen.Declaration
	env          ast.Environment
}

func (p *Rewriter) rewriteFixedArrays(expandedCode []stmt.Resolved) (newCode []stmt.Resolved) {
	for _, s := range expandedCode {
		newCode = append(newCode, p.rewriteFixedArrayStmt(s))
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
			index := expr.NewTypedConstant[symbol.Resolved](idx, 10, uint(idx.BitLen()))
			splitAssignments[i] = &stmt.Assign[symbol.Resolved]{
				Targets: []lval.LVal[symbol.Resolved]{
					lval.NewArray[symbol.Resolved](tgt.id, index),
				},
				Source: &expr.ArrayAccess[symbol.Resolved]{
					Id:       src.Variable,
					Arg:      index,
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
					tgt.id, expr.NewTypedConstant[symbol.Resolved](idx, 10, uint(idx.BitLen())),
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
	index := expr.NewTypedConstant[symbol.Resolved](idx, 10, uint(idx.BitLen()))
	lhs := &expr.ArrayAccess[symbol.Resolved]{
		Id: lhsID, Arg: index, Datatype: elemType,
	}
	rhs := &expr.ArrayAccess[symbol.Resolved]{
		Id: rhsID, Arg: index, Datatype: elemType,
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

func (p *Rewriter) rewriteFixedArrayStmt(s stmt.Resolved) stmt.Resolved {
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		for i, lv := range s.Targets {
			s.Targets[i] = p.rewriteLValArray(lv)
		}

		s.Source = p.rewriteArrayExpression(s.Source)

		return s
	case *stmt.IfGoto[symbol.Resolved]:
		c, ok := s.Cond.(*expr.Cmp[symbol.Resolved])
		if !ok {
			panic(fmt.Sprintf("unknown condition encountered during fixed-array lowering: %T", s.Cond))
		}

		c.Left = p.rewriteArrayExpression(c.Left)
		c.Right = p.rewriteArrayExpression(c.Right)

		return s
	case *stmt.Printf[symbol.Resolved]:
		for i, arg := range s.Arguments {
			s.Arguments[i] = p.rewriteArrayExpression(arg)
		}

		return s
	case *stmt.Return[symbol.Resolved], *stmt.Goto[symbol.Resolved], *stmt.Fail[symbol.Resolved]:
		return s
	default:
		panic(fmt.Sprintf("unknown statement encountered during fixed-array lowering: %T", s))
	}
}

func (p *Rewriter) rewriteArrayExpression(e expr.Resolved) expr.Resolved {
	var evaluator = codegen.NewConstantEvaluator(p.field, p.env, p.declarations...)
	switch e := e.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		e.Variable = p.varMapping[e.Variable].newBase
		//
		return e
	case *expr.ArrayAccess[symbol.Resolved]:
		p.rewriteArrayExpression(e.Arg)

		m := p.varMapping[e.Id]
		if !m.isArray {
			e.Id = m.newBase
			return e
		}

		val, err := evaluator.Eval(e.Arg, false)
		if err != "" {
			// This should have been checked in the typing phase already
			panic(err)
		}

		idx := uint(val.Uint64())

		result := &expr.LocalAccess[symbol.Resolved]{Variable: m.newBase + idx}
		result.SetType(e.Type())

		return result
	case *expr.Add[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Sub[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Mul[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Div[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Rem[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Shl[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Shr[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.BitwiseAnd[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.BitwiseOr[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.Xor[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.BitwiseNot[symbol.Resolved]:
		e.Expr = p.rewriteArrayExpression(e.Expr)
		return e
	case *expr.LogicalAnd[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.LogicalOr[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)
		return e
	case *expr.LogicalNot[symbol.Resolved]:
		e.Expr = p.rewriteArrayExpression(e.Expr)
		return e
	case *expr.Cast[symbol.Resolved]:
		e.Expr = p.rewriteArrayExpression(e.Expr)
		return e
	case *expr.Concat[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Exprs)

		return e
	case *expr.Cmp[symbol.Resolved]:
		e.Left = p.rewriteArrayExpression(e.Left)
		e.Right = p.rewriteArrayExpression(e.Right)

		return e
	case *expr.Ternary[symbol.Resolved]:
		e.Cond = p.rewriteArrayExpression(e.Cond)
		e.IfTrue = p.rewriteArrayExpression(e.IfTrue)
		e.IfFalse = p.rewriteArrayExpression(e.IfFalse)

		return e
	case *expr.ExternAccess[symbol.Resolved]:
		p.rewriteArrayExpressions(e.Args)
		return e
	case *expr.Const[symbol.Resolved]:
		return e
	default:
		panic(fmt.Sprintf("unknown expression encountered during fixed-array lowering: %T", e))
	}
}

func (p *Rewriter) rewriteArrayExpressions(exprs []expr.Resolved) {
	for i, e := range exprs {
		exprs[i] = p.rewriteArrayExpression(e)
	}
}

func (p *Rewriter) rewriteLValArray(l lval.Resolved) lval.Resolved {
	evaluator := codegen.NewConstantEvaluator(p.field, p.env, p.declarations...)
	switch l := l.(type) {
	case *lval.Variable[symbol.Resolved]:
		for i, id := range l.Ids {
			m := p.varMapping[id]
			l.Ids[i] = m.newBase
		}

		return l
	case *lval.Array[symbol.Resolved]:
		p.rewriteArrayExpression(l.Arg)

		m := p.varMapping[l.Id]
		if !m.isArray {
			l.Id = m.newBase
			return l
		}

		val, err := evaluator.Eval(l.Arg, false)
		if err != "" {
			// This should have already been checked in the typing phase
			panic("expected constant index for fixed array lval during lowering")
		}

		idx := uint(val.Uint64())

		return &lval.Variable[symbol.Resolved]{Ids: []variable.Id{m.newBase + idx}}
	case *lval.MemAccess[symbol.Resolved]:
		p.rewriteArrayExpressions(l.Args)
		return l
	default:
		panic(fmt.Sprintf("unknown lval encountered during fixed-array lowering: %T", l))
	}
}