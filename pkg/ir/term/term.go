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
// Testable) because logical terms do not implement Evaluable.
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
type Evaluable[F field.Element[F]] interface {
	util.Boundable
	Contextual
	// EvalAt evaluates this expression in a given tabular context.
	// Observe that if this expression is *undefined* within this
	// context then it returns "nil".  An expression can be
	// undefined for several reasons: firstly, if it accesses a
	// row which does not exist (e.g. at index -1); secondly, if
	// it accesses a register which does not exist.
	EvalAt(uint, trace.Module[F], register.Map) (F, error)
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(bool, register.Map) sexp.SExp
	// ValueRange returns the interval of values that this term can evaluate to.
	// For terms accessing registers, this is determined by the declared width of
	// the register.
	ValueRange() math.Interval
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

// Expr represents a component of an MIR/AIR expression.
type Expr[F field.Element[F], T any] interface {
	Contextual
	Shiftable[T]
	Evaluable[F]
	util.Boundable

	// Simplify constant expressions down to single values.  For example, "(+ 1
	// 2)" would be collapsed down to "3".  This is then progagated throughout
	// an expression, so that e.g. "(+ X (+ 1 2))" becomes "(+ X 3)"", etc.
	Simplify() T
}

// Costable represents a component which can self-determine an approximage cost measure.
type Costable interface {
	Complexity() uint
}

// Testable captures the notion of a constraint which can be tested on a given
// row of a given trace.  It is very similar to Evaluable, except that it only
// indicates success or failure.  The reason for using this interface over
// Evaluable is that, for historical reasons, logical constraints cannot be
// Evaluable (i.e. because they return multiple values, rather than a single
// value).  However, such constraints remain testable.
type Testable[F field.Element[F]] interface {
	util.Boundable
	Contextual
	// TestAt evaluates this expression in a given tabular context and checks it
	// against zero. Observe that if this expression is *undefined* within this
	// context then it returns "nil".  An expression can be undefined for
	// several reasons: firstly, if it accesses a row which does not exist (e.g.
	// at index -1); secondly, if it accesses a register which does not exist.
	TestAt(uint, trace.Module[F], register.Map) (bool, uint, error)
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(bool, register.Map) sexp.SExp
}

// Logical represents a term which can be tested for truth or falsehood.
// For example, an equality comparing two arithmetic terms is a logical term.
type Logical[F field.Element[F], T any] interface {
	Contextual
	Shiftable[T]
	Testable[F]

	// Simplify constant expressions down to single values.  For example, "(+ 1
	// 2)" would be collapsed down to "3".  This is then progagated throughout
	// an expression, so that e.g. "(+ X (+ 1 2))" becomes "(+ X 3)"", etc.
	Simplify() T

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
