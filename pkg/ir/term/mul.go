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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Mul represents the product over zero or more expressions.
type Mul[F field.Element[F], T Expr[F, T]] struct{ Args []T }

// Product returns the product of zero or more multiplications.
func Product[F field.Element[F], T Expr[F, T]](terms ...T) T {
	// flatten any nested products
	terms = array.Flatten(terms, flattenMul[F])
	// Remove all multiplications by one
	terms = array.RemoveMatching(terms, isOne)
	// Check for zero
	if array.ContainsMatching(terms, isZero) {
		return Const64[F, T](0)
	}
	// Final optimisation
	switch len(terms) {
	case 0:
		return Const64[F, T](1)
	case 1:
		return terms[0]
	default:
		var term Expr[F, T] = &Mul[F, T]{terms}
		//
		return term.(T)
	}
}

// Air indicates this term can be used at the AIR level.
func (p *Mul[F, T]) Air() {}

// ApplyShift implementation for Term interface.
func (p *Mul[F, T]) ApplyShift(shift int) T {
	var term Expr[F, T] = &Mul[F, T]{applyShiftOfTerms(p.Args, shift)}
	return term.(T)
}

// Bounds implementation for Boundable interface.
func (p *Mul[F, T]) Bounds() util.Bounds { return util.BoundsForArray(p.Args) }

// EvalAt implementation for Evaluable interface.
func (p *Mul[F, T]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Evaluate first argument
	val, err := p.Args[0].EvalAt(k, tr, sc)
	// Continue evaluating the rest
	for i := 1; err == nil && i < len(p.Args); i++ {
		var ith F
		// Can short-circuit evaluation?
		if val.IsZero() {
			return val, nil
		}
		// No
		ith, err = p.Args[i].EvalAt(k, tr, sc)
		val = val.Mul(ith)
	}
	// Done
	return val, err
}

// IsDefined implementation for Evaluable interface.
func (p *Mul[F, T]) IsDefined() bool {
	// NOTE: this is technically safe given the limited way that IsDefined is
	// used for lookup selectors.
	return true
}

// Lisp implementation for Lispifiable interface.
func (p *Mul[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	return lispOfTerms(global, mapping, "*", p.Args)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Mul[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return requiredRegistersOfTerms(p.Args)
}

// RequiredCells implementation for Contextual interface
func (p *Mul[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return requiredCellsOfTerms(p.Args, row, mid)
}

// ShiftRange implementation for Term interface.
func (p *Mul[F, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Args...)
}

// Substitute implementation for Substitutable interface.
func (p *Mul[F, T]) Substitute(mapping map[string]F) {
	substituteTerms(mapping, p.Args...)
}

// Simplify implementation for Term interface.
func (p *Mul[F, T]) Simplify(casts bool) T {
	var (
		zero F = field.Zero[F]()
		one  F = field.One[F]()
		targ Expr[F, T]
	)
	//
	terms := simplifyTerms(p.Args, mulBinOp, one, casts)
	// Flatten any nested products
	terms = array.Flatten(terms, flattenMul[F])
	// Check for zero
	if array.ContainsMatching(terms, isZero) {
		// Yes, is zero
		targ = &Constant[F, T]{zero}
	} else {
		// Remove any ones
		terms = array.RemoveMatching(terms, isOne)
		// Check whats left
		switch len(terms) {
		case 0:
			targ = &Constant[F, T]{one}
		case 1:
			return terms[0]
		default:
			// Done
			targ = &Mul[F, T]{terms}
		}
	}
	//
	return targ.(T)
}

// ValueRange implementation for Term interface.
func (p *Mul[F, T]) ValueRange() math.Interval {
	var res math.Interval

	for i, arg := range p.Args {
		ith := arg.ValueRange()
		if i == 0 {
			res.Set(ith)
		} else {
			res.Mul(ith)
		}
	}
	//
	return res
}

func flattenMul[F field.Element[F], T Expr[F, T]](term T) []T {
	var e Expr[F, T] = term
	if t, ok := e.(*Mul[F, T]); ok {
		return t.Args
	}
	//
	return nil
}
