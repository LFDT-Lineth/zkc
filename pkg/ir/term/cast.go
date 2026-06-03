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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Cast attempts to narrow the width a given expression.
type Cast[F field.Element[F], T Expr[F, T]] struct {
	Arg      T
	BitWidth uint
	Bound    F
}

// CastOf constructs a new expression which has been annotated by the user to be
// within a given range.
func CastOf[F field.Element[F], T Expr[F, T]](arg T, bitwidth uint) T {
	var (
		// Compute 2^bitwidth
		bound F = field.TwoPowN[F](bitwidth)
		// Construct term
		term Expr[F, T] = &Cast[F, T]{Arg: arg, BitWidth: bitwidth, Bound: bound}
	)
	// Done
	return term.(T)
}

// ApplyShift implementation for Term interface.
func (p *Cast[F, T]) ApplyShift(shift int) T {
	return CastOf(p.Arg.ApplyShift(shift), p.BitWidth)
}

// Bounds implementation for Boundable interface.
func (p *Cast[F, T]) Bounds() util.Bounds {
	return p.Arg.Bounds()
}

// EvalAt implementation for Evaluable interface.
func (p *Cast[F, T]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Check whether argument evaluates to zero or not.
	val, err := p.Arg.EvalAt(k, tr, sc)
	// Dynamic cast check
	if err == nil && val.Cmp(p.Bound) >= 0 {
		// Construct error
		err = fmt.Errorf("cast failure (value %s not a u%d)", val.String(), p.BitWidth)
	}
	// All good
	return val, err
}

// Lisp implementation for Lispifiable interface.
func (p *Cast[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	arg := p.Arg.Lisp(global, mapping)
	name := sexp.NewSymbol(fmt.Sprintf(":u%d", p.BitWidth))

	return sexp.NewList([]sexp.SExp{name, arg})
}

// RequiredRegisters implementation for Contextual interface.
func (p *Cast[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return p.Arg.RequiredRegisters()
}

// RequiredCells implementation for Contextual interface
func (p *Cast[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return p.Arg.RequiredCells(row, mid)
}

// Range returns the range of values which this cast represents.
func (p *Cast[F, T]) Range() math.Interval {
	var bound = big.NewInt(2)
	// Determine bound for static type check
	bound.Exp(bound, big.NewInt(int64(p.BitWidth)), nil)
	// Subtract 1 because interval is inclusive.
	bound.Sub(bound, &biONE)
	// Determine casted interval
	return math.NewInterval(biZERO, *bound)
}

// ShiftRange implementation for Term interface.
func (p *Cast[F, T]) ShiftRange() (int, int) {
	return p.Arg.ShiftRange()
}

// Simplify implementation for Term interface.
func (p *Cast[F, T]) Simplify(casts bool) T {
	var (
		arg  T          = p.Arg.Simplify(casts)
		targ Expr[F, T] = arg
	)
	//
	if c, ok := targ.(*Constant[F, T]); ok && c.Value.Cmp(p.Bound) < 0 {
		// Done
		return arg
	} else if ok {
		// Type failure
		panic(fmt.Sprintf("type cast failure (have %s with expected bitwidth %d)", c.Value.String(), p.BitWidth))
	} else if casts {
		targ = CastOf(arg, p.BitWidth)
		arg = targ.(T)
	}
	// elide cast
	return arg
}

// Substitute implementation for Substitutable interface.
func (p *Cast[F, T]) Substitute(mapping map[string]F) {
	p.Arg.Substitute(mapping)
}

// ValueRange implementation for Term interface.
func (p *Cast[F, T]) ValueRange() math.Interval {
	cast := p.Range()
	// Compute actual interval
	res := p.Arg.ValueRange()
	// Check whether is within (or not)
	if res.Within(cast) {
		return res
	}
	//
	return cast
}
