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
package term

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Contextual captures something which requires an evaluation context (i.e. a
// single enclosing module) in order to make sense.  For example, expressions
// require a single context.  This interface is separated from Evaluable (and
// Testable) because HIR expressions do not implement Evaluable.
type Contextual interface {
	// RequiredRegisters returns the set of registers on which this term depends.
	// That is, registers whose values may be accessed when evaluating this term
	// on a given trace.
	RequiredRegisters() *set.SortedSet[uint]
	// RequiredCells returns the set of trace cells on which evaluation of this
	// constraint element depends.
	RequiredCells(int, trace.ModuleId) *set.AnySortedSet[trace.CellRef]
}

// Evaluable captures something which can be evaluated on a given table row to
// produce an evaluation point.  For example, expressions in the
// Mid-Level or Arithmetic-Level IR can all be evaluated at rows of a
// table.
type Evaluable[F any] interface {
	util.Boundable
	Contextual
	Substitutable[F]
	// EvalAt evaluates this expression in a given tabular context.
	// Observe that if this expression is *undefined* within this
	// context then it returns "nil".  An expression can be
	// undefined for several reasons: firstly, if it accesses a
	// row which does not exist (e.g. at index -1); secondly, if
	// it accesses a register which does not exist.
	EvalAt(int, trace.Module[F], register.Map) (F, error)
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(bool, register.Map) sexp.SExp
	// ValueRange returns the interval of values that this term can evaluate to.
	// For terms accessing registers, this is determined by the declared width of
	// the register.
	ValueRange() math.Interval
}

// Substitutable captures the notion of a term which may contain labelled
// constants that can be substituted.
type Substitutable[F any] interface {
	// Substitute any matchined labelled constants within this constraint
	Substitute(map[string]F)
}

// Shiftable captures something which can contain row shifted accesses, and
// where we want information or to manipulate those accesses.
type Shiftable[T any] interface {
	// ApplyShift applies a given shift to all variable accesses in a given term
	// by a given amount. This can be used to normalise shifting in certain
	// circumstances.
	ApplyShift(int) T

	// ShiftRange returns the minimum and maximum shift value used anywhere in
	// the given term.
	ShiftRange() (int, int)
}

// Expr represents a component of an HIR/MIR/AIR expression.
type Expr[F any, T any] interface {
	Contextual
	Shiftable[T]
	Evaluable[F]
	util.Boundable
	Substitutable[F]

	// Simplify constant expressions down to single values.  For example, "(+ 1
	// 2)" would be collapsed down to "3".  This is then progagated throughout
	// an expression, so that e.g. "(+ X (+ 1 2))" becomes "(+ X 3)"", etc.
	// There is also an option to retain casts, or not.
	Simplify(casts bool) T
}

// Costable represents a component which can self-determine an approximage cost measure.
type Costable interface {
	Complexity() uint
}

// Testable captures the notion of a constraint which can be tested on a given
// row of a given trace.  It is very similar to Evaluable, except that it only
// indicates success or failure.  The reason for using this interface over
// Evaluable is that, for historical reasons, constraints at the HIR cannot be
// Evaluable (i.e. because they return multiple values, rather than a single
// value).  However, constraints at the HIR level remain testable.
type Testable[F any] interface {
	util.Boundable
	Contextual
	Substitutable[F]
	// TestAt evaluates this expression in a given tabular context and checks it
	// against zero. Observe that if this expression is *undefined* within this
	// context then it returns "nil".  An expression can be undefined for
	// several reasons: firstly, if it accesses a row which does not exist (e.g.
	// at index -1); secondly, if it accesses a register which does not exist.
	TestAt(int, trace.Module[F], register.Map) (bool, uint, error)
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(bool, register.Map) sexp.SExp
}

// Logical represents a term which can be tested for truth or falsehood.
// For example, an equality comparing two arithmetic terms is a logical term.
type Logical[F any, T any] interface {
	Contextual
	Shiftable[T]
	Testable[F]

	// Simplify constant expressions down to single values.  For example, "(+ 1
	// 2)" would be collapsed down to "3".  This is then progagated throughout
	// an expression, so that e.g. "(+ X (+ 1 2))" becomes "(+ X 3)"", etc.
	// There is also an option to retain casts, or not.
	Simplify(casts bool) T

	// Negate this logical term
	Negate() T
}

// ============================================================================
// Subdivision
// ============================================================================

// ComplexityOfTerm attempts to provide a cost estimate for the given expression.
func ComplexityOfTerm[F field.Element[F], T Expr[F, T]](c T) uint {
	var f Expr[F, T] = any(c).(Expr[F, T])
	//
	switch t := f.(type) {
	case *Add[F, T]:
		var r = uint(0)
		//
		for _, arg := range t.Args {
			r = max(r, ComplexityOfTerm[F](arg))
		}
		//
		return r
	case *Constant[F, T]:
		return 0
	case *Mul[F, T]:
		var r = uint(0)
		//
		for _, arg := range t.Args {
			r += ComplexityOfTerm[F](arg)
		}
		//
		return r
	case *RegisterAccess[F, T]:
		return 1
	case *Sub[F, T]:
		var r = uint(0)
		//
		for _, arg := range t.Args {
			r = max(r, ComplexityOfTerm[F](arg))
		}
		//
		return r
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", c.Lisp(false, nil).String(false)))
	}
}

// ============================================================================
// Subdivision
// ============================================================================

// SubdivideExpr subdivides a computation by splitting all register
// accesses into vector accesses over their limbs.
func SubdivideExpr[F field.Element[F], S Logical[F, S], T Expr[F, T]](c T, mapping register.LimbsMap) T {
	var f Expr[F, T] = any(c).(Expr[F, T])
	//
	switch t := f.(type) {
	case *Add[F, T]:
		args := SubdivideExprs[F, S](t.Args, mapping)
		return Sum(args...)
	case *Cast[F, T]:
		arg := SubdivideExpr[F, S](t.Arg, mapping)
		return CastOf(arg, t.BitWidth)
	case *Constant[F, T]:
		var val F
		return Const[F, T](val.SetBytes(t.Value.Bytes()))
	case *Exp[F, T]:
		arg := SubdivideExpr[F, S](t.Arg, mapping)
		return Exponent(arg, t.Pow)
	case *IfZero[F, S, T]:
		condition := SubdivideLogical[F, S, T](t.Condition, mapping)
		trueBranch := SubdivideExpr[F, S](t.TrueBranch, mapping)
		falseBranch := SubdivideExpr[F, S](t.FalseBranch, mapping)
		// Done
		return IfElse(condition, trueBranch, falseBranch)
	case *LabelledConst[F, T]:
		var val F
		return LabelledConstant[F, T](t.Label, val.SetBytes(t.Value.Bytes()))
	case *Mul[F, T]:
		args := SubdivideExprs[F, S](t.Args, mapping)
		return Product(args...)
	case *Norm[F, T]:
		arg := SubdivideExpr[F, S](t.Arg, mapping)
		return Normalise(arg)
	case *RegisterAccess[F, T]:
		return subdivideRegAccesses[F, S](mapping, t)
	case *Sub[F, T]:
		args := SubdivideExprs[F, S](t.Args, mapping)
		return Subtract(args...)
	case *VectorAccess[F, T]:
		return subdivideRegAccesses[F, S](mapping, t.Vars...)
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", c.Lisp(false, nil).String(false)))
	}
}

// SubdivideExprs subdivides an array of zero or more logical computations.
func SubdivideExprs[F field.Element[F], S Logical[F, S], T Expr[F, T]](cs []T, mapping register.LimbsMap) []T {
	var computations = make([]T, len(cs))
	//
	for i, t := range cs {
		computations[i] = SubdivideExpr[F, S](t, mapping)
	}
	//
	return computations
}

func subdivideRegAccesses[F field.Element[F], S Logical[F, S], T Expr[F, T]](mapping register.LimbsMap,
	regs ...*RegisterAccess[F, T]) T {
	var nterms []*RegisterAccess[F, T]
	//
	for _, v := range regs {
		var bitwidth = v.MaskWidth()
		//
		for _, limbId := range mapping.LimbIds(v.Register()) {
			var (
				limb = mapping.Limb(limbId)
				mask = min(limb.Width(), bitwidth)
			)
			//
			if mask > 0 {
				// Construct access for given limb
				ith := RawRegisterAccess[F, T](limbId, limb.Width(), v.RelativeShift())
				// Mask access to eliminate any unused bits
				nterms = append(nterms, ith.Mask(mask))
			}
			//
			bitwidth -= mask
		}
	}
	// Simplify (when possible)
	if len(nterms) == 1 {
		return any(nterms[0]).(T)
	}
	//
	return NewVectorAccess(nterms)
}

// SubdivideLogical subdivides a logical computation by splitting all
// register accesses into vector accesses over their limbs.
func SubdivideLogical[F field.Element[F], S Logical[F, S], T Expr[F, T]](c S, mapping register.LimbsMap) S {
	var f Logical[F, S] = any(c).(Logical[F, S])
	//
	switch t := f.(type) {
	case *Conjunct[F, S]:
		args := SubdivideLogicals[F, S, T](t.Args, mapping)
		return Conjunction(args...)
	case *Disjunct[F, S]:
		args := SubdivideLogicals[F, S, T](t.Args, mapping)
		return Disjunction(args...)
	case *Equal[F, S, T]:
		lhs := SubdivideExpr[F, S, T](t.Lhs.(T), mapping)
		rhs := SubdivideExpr[F, S, T](t.Rhs.(T), mapping)

		return Equals[F, S, T](lhs, rhs)
	case *Ite[F, S]:
		var trueBranch, falseBranch S

		condition := SubdivideLogical[F, S, T](t.Condition, mapping)
		//
		if t.TrueBranch != nil {
			trueBranch = SubdivideLogical[F, S, T](t.TrueBranch.(S), mapping)
		}
		//
		if t.FalseBranch != nil {
			falseBranch = SubdivideLogical[F, S, T](t.FalseBranch.(S), mapping)
		}
		//
		return IfThenElse(condition, trueBranch, falseBranch)
	case *Negate[F, S]:
		arg := SubdivideLogical[F, S, T](t.Arg, mapping)
		return Negation(arg)
	case *NotEqual[F, S, T]:
		lhs := SubdivideExpr[F, S, T](t.Lhs.(T), mapping)
		rhs := SubdivideExpr[F, S, T](t.Rhs.(T), mapping)

		return NotEquals[F, S](lhs, rhs)
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", c.Lisp(false, nil).String(false)))
	}
}

// SubdivideLogicals Subdivides an array of zero or more logical computations.
func SubdivideLogicals[F field.Element[F], S Logical[F, S], T Expr[F, T]](cs []S, mapping register.LimbsMap,
) []S {
	//
	var computations = make([]S, len(cs))
	//
	for i, t := range cs {
		computations[i] = SubdivideLogical[F, S, T](t, mapping)
	}
	//
	return computations
}

// ============================================================================
// IsUnsafe
// ============================================================================

// IsUnsafeExpr determines whether or not a given expression contains an unsafe
// operation (i.e. a runtime cast).  Specifically, something which could fail at
// runtime.
func IsUnsafeExpr[F field.Element[F], S Logical[F, S], T Expr[F, T]](c T) bool {
	var f Expr[F, T] = any(c).(Expr[F, T])
	//
	switch t := f.(type) {
	case *Add[F, T]:
		return isUnsafeExprs[F, S](t.Args)
	case *Cast[F, T]:
		return true
	case *Constant[F, T]:
		return false
	case *Exp[F, T]:
		return IsUnsafeExpr[F, S](t.Arg)
	case *IfZero[F, S, T]:
		condition := IsUnsafeLogical[F, S, T](t.Condition)
		trueBranch := IsUnsafeExpr[F, S](t.TrueBranch)
		falseBranch := IsUnsafeExpr[F, S](t.FalseBranch)
		// Done
		return condition || trueBranch || falseBranch
	case *LabelledConst[F, T]:
		return false
	case *Mul[F, T]:
		return isUnsafeExprs[F, S](t.Args)
	case *Norm[F, T]:
		return IsUnsafeExpr[F, S](t.Arg)
	case *RegisterAccess[F, T]:
		return t.bitwidth != t.maskwidth
	case *Sub[F, T]:
		return isUnsafeExprs[F, S](t.Args)
	case *VectorAccess[F, T]:
		for _, v := range t.Vars {
			if v.bitwidth != v.maskwidth {
				return true
			}
		}
		//
		return false
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", c.Lisp(false, nil).String(false)))
	}
}

func isUnsafeExprs[F field.Element[F], S Logical[F, S], T Expr[F, T]](exprs []T) bool {
	for _, v := range exprs {
		if IsUnsafeExpr[F, S, T](v) {
			return true
		}
	}
	//
	return false
}

// IsUnsafeLogical determines whether or not a given logical expression contains
// an unsafe operation (i.e. a runtime case).  Specifically, something which
// could fail at runtime.
func IsUnsafeLogical[F field.Element[F], S Logical[F, S], T Expr[F, T]](c S) bool {
	var f Logical[F, S] = any(c).(Logical[F, S])
	//
	switch t := f.(type) {
	case *Conjunct[F, S]:
		return isUnsafeLogicals[F, S, T](t.Args)
	case *Disjunct[F, S]:
		return isUnsafeLogicals[F, S, T](t.Args)
	case *Equal[F, S, T]:
		lhs := IsUnsafeExpr[F, S, T](t.Lhs.(T))
		rhs := IsUnsafeExpr[F, S, T](t.Rhs.(T))

		return lhs || rhs
	case *Ite[F, S]:
		var condition, trueBranch, falseBranch bool

		condition = IsUnsafeLogical[F, S, T](t.Condition)
		//
		if t.TrueBranch != nil {
			trueBranch = IsUnsafeLogical[F, S, T](t.TrueBranch.(S))
		}
		//
		if t.FalseBranch != nil {
			falseBranch = IsUnsafeLogical[F, S, T](t.FalseBranch.(S))
		}
		//
		return condition || trueBranch || falseBranch
	case *Negate[F, S]:
		return IsUnsafeLogical[F, S, T](t.Arg)
	case *NotEqual[F, S, T]:
		lhs := IsUnsafeExpr[F, S, T](t.Lhs.(T))
		rhs := IsUnsafeExpr[F, S, T](t.Rhs.(T))

		return lhs || rhs
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", c.Lisp(false, nil).String(false)))
	}
}

func isUnsafeLogicals[F field.Element[F], S Logical[F, S], T Expr[F, T]](exprs []S) bool {
	for _, v := range exprs {
		if IsUnsafeLogical[F, S, T](v) {
			return true
		}
	}
	//
	return false
}
