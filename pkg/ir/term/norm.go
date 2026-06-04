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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Norm reduces the value of an expression to either zero (if it was zero)
// or one (otherwise).
type Norm[F field.Element[F], T Expr[F, T]] struct{ Arg T }

// Normalise normalises the result of evaluating a given expression to be
// either 0 (if its value was 0) or 1 (otherwise).
func Normalise[F field.Element[F], T Expr[F, T]](arg T) T {
	var term Expr[F, T] = &Norm[F, T]{arg}
	return term.(T)
}

// ApplyShift implementation for Term interface.
func (p *Norm[F, T]) ApplyShift(shift int) T {
	return Normalise(p.Arg.ApplyShift(shift))
}

// Bounds implementation for Boundable interface.
func (p *Norm[F, T]) Bounds() util.Bounds {
	return p.Arg.Bounds()
}

// EvalAt implementation for Evaluable interface.
func (p *Norm[F, T]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Check whether argument evaluates to zero or not.
	val, err := p.Arg.EvalAt(k, tr, sc)
	// Normalise value (if necessary)
	if !val.IsZero() {
		val = field.One[F]()
	}
	// Done
	return val, err
}

// IsDefined implementation for Evaluable interface.
func (p *Norm[F, T]) IsDefined() bool {
	// NOTE: this is technically safe given the limited way that IsDefined is
	// used for lookup selectors.
	return true
}

// Lisp implementation for Lispifiable interface.
func (p *Norm[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	arg := p.Arg.Lisp(global, mapping)
	return sexp.NewList([]sexp.SExp{sexp.NewSymbol("~"), arg})
}

// RequiredRegisters implementation for Contextual interface.
func (p *Norm[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return p.Arg.RequiredRegisters()
}

// RequiredCells implementation for Contextual interface
func (p *Norm[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return p.Arg.RequiredCells(row, mid)
}

// ShiftRange implementation for Term interface.
func (p *Norm[F, T]) ShiftRange() (int, int) {
	return p.Arg.ShiftRange()
}

// Simplify implementation for Term interface.
func (p *Norm[F, T]) Simplify(casts bool) T {
	var (
		arg  T          = p.Arg.Simplify(casts)
		targ Expr[F, T] = arg
	)
	//
	if c, ok := targ.(*Constant[F, T]); ok {
		val := c.Value
		// Normalise (in place)
		if !val.IsZero() {
			val = field.One[F]()
		}
		// Done
		targ = &Constant[F, T]{val}
	} else {
		targ = &Norm[F, T]{arg}
	}
	//
	return targ.(T)
}

// Substitute implementation for Substitutable interface.
func (p *Norm[F, T]) Substitute(mapping map[string]F) {
	p.Arg.Substitute(mapping)
}

// ValueRange implementation for Term interface.
func (p *Norm[F, T]) ValueRange() math.Interval {
	return math.NewInterval(*big.NewInt(0), *big.NewInt(1))
}
