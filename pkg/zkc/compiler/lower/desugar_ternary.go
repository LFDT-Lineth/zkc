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
)

// desugarCtx carries per-function state for the desugaring pass.
type desugarCtx struct {
	fn      *decl.ResolvedFunction
	env     ast.Environment
	decls   []decl.Resolved
	srcmaps source.Maps[any]
}

// desugarTernaries replaces every Ternary expression in fn's body with an
// explicit IfElse statement that writes to either the original target (when
// the ternary is the immediate Source of an Assign) or a fresh temp variable
// (otherwise).  After this pass, no Ternary nodes remain — the subsequent
// lowerStatements call flattens the new IfElse statements via lowerIfElse.
//
// This runs as the first stage of Flatten().  At this point expressions have
// not yet been typed (Typing runs after Flatten), so we infer the temp
// variable's type either from a propagated "expected" type (from enclosing
// context — e.g. the LHS target's declared type) or, when no context is
// available, from operand types via inferType.
func desugarTernaries(fn *decl.ResolvedFunction, env ast.Environment, decls []decl.Resolved,
	srcmaps source.Maps[any]) {
	ctx := &desugarCtx{fn: fn, env: env, decls: decls, srcmaps: srcmaps}
	fn.Code = ctx.desugarStmts(fn.Code)
}

func (c *desugarCtx) desugarStmts(stmts []stmt.Resolved) []stmt.Resolved {
	var out []stmt.Resolved
	//
	for _, s := range stmts {
		pre, ns := c.desugarStmt(s)
		out = append(out, pre...)
		out = append(out, ns)
	}
	//
	return out
}

// desugarStmt returns (pre, ns): pre statements (typically the IfElse blocks
// produced when hoisting ternaries out of sub-expressions) that must execute
// before ns, plus ns — the statement with all in-place ternaries replaced by
// LocalAccess to fresh temps.
func (c *desugarCtx) desugarStmt(s stmt.Resolved) ([]stmt.Resolved, stmt.Resolved) {
	switch t := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		// Fast path: when the entire Source is a Ternary we can avoid a temp
		// by cloning the LHS targets into each arm's Assign.
		if tern, ok := t.Source.(*expr.Ternary[symbol.Resolved]); ok {
			return c.expandTernaryAssign(tern, t.Targets, s)
		}
		// Otherwise hoist any nested ternaries via temp variables.  Use the
		// LHS target type as the expected type so hoisted temps inherit
		// width from context rather than from open Const arms alone.
		pre, ne := c.desugarExpr(t.Source, c.targetType(t.Targets))
		ns := &stmt.Assign[symbol.Resolved]{Targets: t.Targets, Source: ne}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	case *stmt.IfElse[symbol.Resolved]:
		pre, nc := c.desugarExpr(t.Cond, nil)
		ns := &stmt.IfElse[symbol.Resolved]{
			Cond:        nc,
			TrueBranch:  c.desugarStmts(t.TrueBranch),
			FalseBranch: c.desugarStmts(t.FalseBranch),
		}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	case *stmt.Switch[symbol.Resolved]:
		pre, nd := c.desugarExpr(t.Discriminant, nil)
		nbs := make([]stmt.SwitchBranch[symbol.Resolved], len(t.Branches))
		//
		for i, b := range t.Branches {
			nbs[i] = stmt.SwitchBranch[symbol.Resolved]{
				IsDefault: b.IsDefault,
				Labels:    b.Labels,
				Body:      c.desugarStmts(b.Body),
			}
		}
		//
		ns := &stmt.Switch[symbol.Resolved]{Discriminant: nd, Branches: nbs}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	case *stmt.While[symbol.Resolved]:
		// Hoisting a ternary out of the condition would put the temp-init
		// IfElse before the loop, but the cond is re-evaluated each
		// iteration.  Reject for now — none of the existing tests hit this.
		c.assertNoTernary(t.Cond, "while condition")
		//
		ns := &stmt.While[symbol.Resolved]{Cond: t.Cond, Body: c.desugarStmts(t.Body)}
		c.srcmaps.Copy(s, ns)
		//
		return nil, ns
	case *stmt.For[symbol.Resolved]:
		// Same reason as While — cond/post run per iteration.
		c.assertNoTernary(t.Cond, "for condition")
		_, initS := c.desugarStmt(t.Init)
		_, postS := c.desugarStmt(t.Post)
		ns := &stmt.For[symbol.Resolved]{
			Init: initS, Cond: t.Cond, Post: postS,
			Body: c.desugarStmts(t.Body),
		}
		c.srcmaps.Copy(s, ns)
		//
		return nil, ns
	case *stmt.VarDecl[symbol.Resolved]:
		if !t.Init.HasValue() {
			return nil, s
		}
		// Treat `var r:T = ternary` like the Assign fast path: clone the
		// VarDecl (no Init) and emit IfElse arms that assign to r.
		init := t.Init.Unwrap()
		//
		if tern, ok := init.(*expr.Ternary[symbol.Resolved]); ok && len(t.Variables) == 1 {
			targets := []lval.LVal[symbol.Resolved]{lval.NewVariable[symbol.Resolved](t.Variables[0])}
			declOnly := &stmt.VarDecl[symbol.Resolved]{
				Variables: t.Variables,
				Init:      util.None[expr.Expr[symbol.Resolved]](),
			}
			c.srcmaps.Copy(s, declOnly)
			pre, ifelse := c.expandTernaryAssign(tern, targets, s)
			//
			return append([]stmt.Resolved{declOnly}, pre...), ifelse
		}
		//
		var expected data.ResolvedType
		//
		if len(t.Variables) == 1 {
			expected = c.fn.Variables[t.Variables[0]].DataType
		}
		//
		pre, ne := c.desugarExpr(init, expected)
		ns := &stmt.VarDecl[symbol.Resolved]{
			Variables: t.Variables,
			Init:      util.Some(ne),
		}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	case *stmt.Printf[symbol.Resolved]:
		pre, nargs := c.desugarExprs(t.Arguments, nil)
		ns := &stmt.Printf[symbol.Resolved]{Chunks: t.Chunks, Arguments: nargs}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	case *stmt.Fail[symbol.Resolved]:
		pre, nargs := c.desugarExprs(t.Arguments, nil)
		ns := &stmt.Fail[symbol.Resolved]{Chunks: t.Chunks, Arguments: nargs}
		c.srcmaps.Copy(s, ns)
		//
		return pre, ns
	default:
		// Break, Continue, Return, IfGoto, Goto — no expressions carrying
		// ternaries at this point.
		return nil, s
	}
}

// expandTernaryAssign rewrites `targets = (cond ? a : b)` as
//
//	if cond { targets = a } else { targets = b }
//
// recursing into a and b so that nested ternaries unfold without introducing
// any temps for this chain.  Returns (pre, ifelse): pre statements arise only
// when the cond itself contains a hoisted ternary.
func (c *desugarCtx) expandTernaryAssign(tern *expr.Ternary[symbol.Resolved],
	targets []lval.LVal[symbol.Resolved], orig any,
) ([]stmt.Resolved, stmt.Resolved) {
	//
	condPre, ncond := c.desugarExpr(tern.Cond, nil)
	//
	trueAssign := &stmt.Assign[symbol.Resolved]{Targets: targets, Source: tern.IfTrue}
	c.srcmaps.Copy(orig, trueAssign)
	//
	falseAssign := &stmt.Assign[symbol.Resolved]{Targets: targets, Source: tern.IfFalse}
	c.srcmaps.Copy(orig, falseAssign)
	// Recursively desugar the new arm-assigns.  This handles both nested
	// ternaries-as-Source (which take the fast path again) and ternaries
	// embedded within larger expressions in the arms.
	truePre, trueS := c.desugarStmt(trueAssign)
	falsePre, falseS := c.desugarStmt(falseAssign)
	//
	ifelse := &stmt.IfElse[symbol.Resolved]{
		Cond:        ncond,
		TrueBranch:  append(truePre, trueS),
		FalseBranch: append(falsePre, falseS),
	}
	c.srcmaps.Copy(orig, ifelse)
	//
	return condPre, ifelse
}

// desugarExprs walks a slice of expressions with the given expected type,
// collecting pre statements from each in source order.
func (c *desugarCtx) desugarExprs(exprs []expr.Resolved,
	expected data.ResolvedType) ([]stmt.Resolved, []expr.Resolved) {
	//
	var pre []stmt.Resolved
	//
	out := make([]expr.Resolved, len(exprs))
	//
	for i, e := range exprs {
		ipre, ne := c.desugarExpr(e, expected)
		pre = append(pre, ipre...)
		out[i] = ne
	}
	//
	return pre, out
}

// desugarExpr returns (pre, e') — pre is the list of IfElse statements that
// must execute before evaluating e', and e' is the expression with every
// Ternary hoisted into a temp variable read via LocalAccess.  The expected
// argument is propagated from enclosing context (LHS target type, sibling
// operand type, …) and is used to size hoisted temps when their arms alone
// give an inadequate type (e.g. both arms are integer constants).
//
//nolint:gocyclo
func (c *desugarCtx) desugarExpr(e expr.Resolved,
	expected data.ResolvedType) ([]stmt.Resolved, expr.Resolved) {
	//
	switch t := e.(type) {
	case *expr.Ternary[symbol.Resolved]:
		return c.hoistTernary(t, expected)
	case *expr.Cmp[symbol.Resolved]:
		// In a comparison both operands share a type.  Take that type from
		// whichever side already has a concrete one and propagate it to the
		// other.  This is how `x > (cond ? 1 : 2)` learns the inner ternary
		// should be the same width as `x`.
		anchor := c.cmpAnchor(t.Left, t.Right)
		lpre, l := c.desugarExpr(t.Left, anchor)
		rpre, r := c.desugarExpr(t.Right, anchor)
		ne := expr.NewCmp[symbol.Resolved](t.Operator, l, r)
		c.srcmaps.Copy(e, ne)
		//
		return append(lpre, rpre...), ne
	case *expr.LogicalAnd[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewLogicalAnd[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.LogicalOr[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewLogicalOr[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.LogicalNot[symbol.Resolved]:
		pre, ns := c.desugarExpr(t.Expr, nil)
		ne := expr.NewLogicalNot[symbol.Resolved](ns)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Add[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewAdd[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Sub[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewSub[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Mul[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewMul[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Div[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewDiv[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Rem[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewRem[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.BitwiseAnd[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewBitwiseAnd[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.BitwiseOr[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewBitwiseOr[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Xor[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, expected)
		ne := expr.NewXor[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Shl[symbol.Resolved]:
		// `a << b << c …` is variadic with heterogeneous operand widths;
		// no clean expected-type to propagate.
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewShl[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Shr[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewShr[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.BitwiseNot[symbol.Resolved]:
		pre, ns := c.desugarExpr(t.Expr, expected)
		ne := expr.NewBitwiseNot[symbol.Resolved](ns)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Cast[symbol.Resolved]:
		// The cast normalises whatever the inner produces; don't propagate
		// the outer expected through it.
		pre, ns := c.desugarExpr(t.Expr, nil)
		ne := expr.NewCast[symbol.Resolved](ns, t.CastType)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.Concat[symbol.Resolved]:
		// Each fragment carries its own width.
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewConcat[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.ArrayAccess[symbol.Resolved]:
		pre, na := c.desugarExpr(t.Arg, nil)
		ne := &expr.ArrayAccess[symbol.Resolved]{Id: t.Id, Arg: na, Datatype: t.Datatype}
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.ExternAccess[symbol.Resolved]:
		pre, nargs := c.desugarExternArgs(t)
		ne := expr.NewExternAccess[symbol.Resolved](t.Name, nargs...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	case *expr.TupleInitialiser[symbol.Resolved]:
		pre, ns := c.desugarExprs(t.Exprs, nil)
		ne := expr.NewTupleInitialiser[symbol.Resolved](ns...)
		c.srcmaps.Copy(e, ne)
		//
		return pre, ne
	default:
		// LocalAccess, Const — no sub-expressions.
		return nil, e
	}
}

// desugarExternArgs walks the arguments of a function/memory access using each
// arg's declared parameter type as the expected type for that argument.
func (c *desugarCtx) desugarExternArgs(e *expr.ExternAccess[symbol.Resolved],
) ([]stmt.Resolved, []expr.Resolved) {
	//
	var pre []stmt.Resolved
	//
	out := make([]expr.Resolved, len(e.Args))
	//
	for i, arg := range e.Args {
		ipre, ne := c.desugarExpr(arg, c.externArgType(e, i))
		pre = append(pre, ipre...)
		out[i] = ne
	}
	//
	return pre, out
}

// externArgType returns the declared expected type for the i-th argument of an
// ExternAccess (function call, memory read, constant lookup, type alias).  Nil
// when the type cannot be statically determined from the declaration alone.
func (c *desugarCtx) externArgType(e *expr.ExternAccess[symbol.Resolved], i int) data.ResolvedType {
	if e.Name.IsUnknown() {
		return nil
	}
	//
	switch d := c.decls[e.Name.Index].(type) {
	case *decl.ResolvedFunction:
		if i < int(d.NumInputs) {
			return d.Variables[i].DataType
		}
	case *decl.ResolvedMemory:
		if i < len(d.Address) {
			return d.Address[i].DataType
		}
	}
	//
	return nil
}

// cmpAnchor returns a type to anchor a comparison's operands to.  If exactly
// one side already has a non-ternary type, that's the anchor; otherwise nil.
func (c *desugarCtx) cmpAnchor(left, right expr.Resolved) data.ResolvedType {
	switch {
	case !containsTernary(left):
		return c.inferType(left)
	case !containsTernary(right):
		return c.inferType(right)
	}
	//
	return nil
}

// targetType returns the declared type implied by the set of assignment
// targets, or nil when it cannot be resolved cheaply.  Used to feed the LHS
// type back to the RHS expression for ternary hoisting.
func (c *desugarCtx) targetType(targets []lval.LVal[symbol.Resolved]) data.ResolvedType {
	if len(targets) != 1 {
		return nil
	}
	//
	switch lv := targets[0].(type) {
	case *lval.Variable[symbol.Resolved]:
		if len(lv.Ids) == 1 {
			return c.fn.Variables[lv.Ids[0]].DataType
		}
	case *lval.MemAccess[symbol.Resolved]:
		if !lv.Name.IsUnknown() {
			if d, ok := c.decls[lv.Name.Index].(*decl.ResolvedMemory); ok {
				return variable.DescriptorsToType(d.Data...)
			}
		}
	}
	//
	return nil
}

// hoistTernary allocates a fresh temp, emits an IfElse that assigns the temp
// from either arm (after recursively hoisting nested ternaries within the
// arms), and returns a LocalAccess to the temp as the replacement expression.
// expected guides the temp's type; falls back to inferring from arm types
// when nil.
func (c *desugarCtx) hoistTernary(t *expr.Ternary[symbol.Resolved],
	expected data.ResolvedType) ([]stmt.Resolved, expr.Resolved) {
	//
	tempType := expected
	//
	if tempType == nil {
		tempType = c.inferType(t)
	}
	//
	if tempType == nil {
		panic(fmt.Sprintf("cannot infer type for ternary while desugaring: %s", t.String(c.fn)))
	}
	// Variables cannot hold "open" constant types.
	tempType = closeType(tempType)
	//
	tempID := variable.Id(len(c.fn.Variables))
	tempName := fmt.Sprintf("$tern_%d", tempID)
	c.fn.Variables = append(c.fn.Variables, variable.New[symbol.Resolved](variable.LOCAL, tempName, tempType))
	//
	targets := []lval.LVal[symbol.Resolved]{lval.NewVariable[symbol.Resolved](tempID)}
	pre, ifelse := c.expandTernaryAssign(t, targets, t)
	//
	access := expr.NewLocalAccess[symbol.Resolved](tempID)
	access.SetType(tempType)
	c.srcmaps.Copy(t, access)
	//
	return append(pre, ifelse), access
}

// inferType is a minimal recursive type-inferer used to size temp variables
// for hoisted ternaries when no context-supplied "expected" type is
// available.  It runs before the proper typing pass, so it only understands
// enough of the type system to give every reachable expression a sensible
// bitwidth.  Returns nil when it cannot determine a type.
//
//nolint:gocyclo
func (c *desugarCtx) inferType(e expr.Resolved) data.ResolvedType {
	switch t := e.(type) {
	case *expr.LocalAccess[symbol.Resolved]:
		return c.fn.Variables[t.Variable].DataType
	case *expr.Const[symbol.Resolved]:
		return data.NewUnsignedInt[symbol.Resolved](uint(t.Constant.BitLen()), true)
	case *expr.Cast[symbol.Resolved]:
		return t.CastType
	case *expr.Add[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Sub[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Mul[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Div[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Rem[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.BitwiseAnd[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.BitwiseOr[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Xor[symbol.Resolved]:
		return c.joinTypes(t.Exprs)
	case *expr.Shl[symbol.Resolved]:
		return c.inferType(t.Exprs[0])
	case *expr.Shr[symbol.Resolved]:
		return c.inferType(t.Exprs[0])
	case *expr.BitwiseNot[symbol.Resolved]:
		return c.inferType(t.Expr)
	case *expr.Concat[symbol.Resolved]:
		var bw uint
		//
		for _, ee := range t.Exprs {
			it := c.inferType(ee)
			//
			u, ok := it.(*data.UnsignedInt[symbol.Resolved])
			if !ok {
				return nil
			}
			//
			bw += u.BitWidth()
		}
		//
		return data.NewUnsignedInt[symbol.Resolved](bw, false)
	case *expr.Ternary[symbol.Resolved]:
		return joinType(c.inferType(t.IfTrue), c.inferType(t.IfFalse))
	case *expr.ArrayAccess[symbol.Resolved]:
		if v := c.fn.Variables[t.Id]; v.DataType != nil {
			if arr, ok := v.DataType.(*data.ResolvedFixedArray); ok {
				return arr.DataType
			}
		}
		//
		return nil
	case *expr.ExternAccess[symbol.Resolved]:
		return c.externAccessType(t)
	}
	//
	return nil
}

func (c *desugarCtx) externAccessType(e *expr.ExternAccess[symbol.Resolved]) data.ResolvedType {
	if e.Name.IsUnknown() {
		return nil
	}
	//
	switch d := c.decls[e.Name.Index].(type) {
	case *decl.ResolvedConstant:
		return d.DataType
	case *decl.ResolvedMemory:
		return variable.DescriptorsToType(d.Data...)
	case *decl.ResolvedFunction:
		return variable.DescriptorsToType(d.Outputs()...)
	}
	//
	return nil
}

func (c *desugarCtx) joinTypes(exprs []expr.Resolved) data.ResolvedType {
	if len(exprs) == 0 {
		return nil
	}
	//
	res := c.inferType(exprs[0])
	//
	for _, e := range exprs[1:] {
		res = joinType(res, c.inferType(e))
	}
	//
	return res
}

func joinType(a, b data.ResolvedType) data.ResolvedType {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	//
	ua, aok := a.(*data.UnsignedInt[symbol.Resolved])
	ub, bok := b.(*data.UnsignedInt[symbol.Resolved])
	//
	if aok && bok {
		return ua.Join(ub)
	}
	//
	return a
}

// closeType strips the "open" flag from an unsigned integer type so the value
// can be stored in a declared variable.  Other types pass through unchanged.
func closeType(t data.ResolvedType) data.ResolvedType {
	if u, ok := t.(*data.UnsignedInt[symbol.Resolved]); ok && u.IsOpen() {
		return data.NewUnsignedInt[symbol.Resolved](u.BitWidth(), false)
	}
	//
	return t
}

// assertNoTernary panics if the expression sub-tree contains any Ternary node.
// Used for contexts (loop conditions, post-iterators) where hoisting via a
// pre-statement would break re-evaluation semantics.
func (c *desugarCtx) assertNoTernary(e expr.Resolved, where string) {
	if containsTernary(e) {
		panic(fmt.Sprintf("ternary expressions are not yet supported inside %s", where))
	}
}

//nolint:gocyclo
func containsTernary(e expr.Resolved) bool {
	switch t := e.(type) {
	case *expr.Ternary[symbol.Resolved]:
		return true
	case *expr.Cmp[symbol.Resolved]:
		return containsTernary(t.Left) || containsTernary(t.Right)
	case *expr.LogicalAnd[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.LogicalOr[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.LogicalNot[symbol.Resolved]:
		return containsTernary(t.Expr)
	case *expr.Add[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Sub[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Mul[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Div[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Rem[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.BitwiseAnd[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.BitwiseOr[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Xor[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Shl[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.Shr[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.BitwiseNot[symbol.Resolved]:
		return containsTernary(t.Expr)
	case *expr.Cast[symbol.Resolved]:
		return containsTernary(t.Expr)
	case *expr.Concat[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	case *expr.ArrayAccess[symbol.Resolved]:
		return containsTernary(t.Arg)
	case *expr.ExternAccess[symbol.Resolved]:
		return anyContainsTernary(t.Args)
	case *expr.TupleInitialiser[symbol.Resolved]:
		return anyContainsTernary(t.Exprs)
	}
	//
	return false
}

func anyContainsTernary(exprs []expr.Resolved) bool {
	for _, e := range exprs {
		if containsTernary(e) {
			return true
		}
	}
	//
	return false
}
