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
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/lval"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/stmt"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

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
	// copy source map information
	srcmaps.Copy(e, ne)
	//
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
