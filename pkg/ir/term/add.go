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

// Add represents the addition of zero or more expressions.
type Add[F field.Element[F], T Expr[F, T]] struct{ Args []T }

// Sum zero or more expressions together.
func Sum[F field.Element[F], T Expr[F, T]](terms ...T) T {
	// Flatten any nested sums
	terms = array.Flatten(terms, flattenAdd[F, T])
	// Remove any zeros
	terms = array.RemoveMatching(terms, isZero)
	// Final simplifications
	switch len(terms) {
	case 0:
		return Const64[F, T](0)
	case 1:
		return terms[0]
	default:
		var term Expr[F, T] = &Add[F, T]{terms}
		//
		return term.(T)
	}
}

// Air indicates this term can be used at the AIR level.
func (p *Add[F, T]) Air() {}

// ApplyShift implementation for Term interface.
func (p *Add[F, T]) ApplyShift(shift int) T {
	var term Expr[F, T] = &Add[F, T]{applyShiftOfTerms(p.Args, shift)}
	return term.(T)
}

// Bounds implementation for Boundable interface.
func (p *Add[F, T]) Bounds() util.Bounds { return util.BoundsForArray(p.Args) }

// EvalAt implementation for Evaluable interface.
func (p *Add[F, T]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Evaluate first argument
	val, err := p.Args[0].EvalAt(k, tr, sc)
	// Continue evaluating the rest
	for i := 1; err == nil && i < len(p.Args); i++ {
		var ith F
		// Evaluate ith argument
		ith, err = p.Args[i].EvalAt(k, tr, sc)
		val = val.Add(ith)
	}
	// Done
	return val, err
}

// Lisp implementation for Lispifiable interface.
func (p *Add[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	return lispOfTerms(global, mapping, "+", p.Args)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Add[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return requiredRegistersOfTerms(p.Args)
}

// RequiredCells implementation for Contextual interface
func (p *Add[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return requiredCellsOfTerms(p.Args, row, mid)
}

// ShiftRange implementation for Term interface.
func (p *Add[F, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Args...)
}

// Simplify implementation for Term interface.
func (p *Add[F, T]) Simplify(casts bool) T {
	return simplifySum(p.Args, casts)
}

// Substitute implementation for Substitutable interface.
func (p *Add[F, T]) Substitute(mapping map[string]F) {
	substituteTerms(mapping, p.Args...)
}

// ValueRange implementation for Term interface.
func (p *Add[F, T]) ValueRange() math.Interval {
	var res math.Interval

	for i, arg := range p.Args {
		ith := arg.ValueRange()
		if i == 0 {
			res.Set(ith)
		} else {
			res.Add(ith)
		}
	}
	//
	return res
}

func simplifySum[F field.Element[F], T Expr[F, T]](args []T, casts bool) T {
	var (
		zero  F
		terms = simplifyTerms(args, addBinOp, zero, casts)
		tmp   Expr[F, T]
	)
	// Flatten any nested sums
	terms = array.Flatten(terms, flattenAdd[F, T])
	// Remove any zeros
	terms = array.RemoveMatching(terms, isZero)
	// Check anything left
	switch len(terms) {
	case 0:
		tmp = &Constant[F, T]{zero}
	case 1:
		return terms[0]
	default:
		tmp = &Add[F, T]{terms}
	}
	// Done
	return tmp.(T)
}

func flattenAdd[F field.Element[F], T Expr[F, T]](term T) []T {
	var e Expr[F, T] = term
	if t, ok := e.(*Add[F, T]); ok {
		return t.Args
	}
	//
	return nil
}
