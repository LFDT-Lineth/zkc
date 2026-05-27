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
	"math"
	"math/big"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/source/sexp"
)

var (
	biZERO big.Int = *big.NewInt(0)
	biONE  big.Int = *big.NewInt(1)
)

// IsConstant64 checks whether a given term is a 64bit constant (or not) and, if
// so, what constant it is.
func IsConstant64[F field.Element[F], T Expr[F, T]](term T) (constant uint64, ok bool) {
	var t Expr[F, T] = term
	//
	if t, ok := t.(*Constant[F, T]); ok && len(t.Value.Bytes()) <= 8 {
		return t.Value.Uint64(), true
	}
	//
	return 0, false
}

// Check whether a given term corresponds with the constant zero.
func isZero[F field.Element[F], T Expr[F, T]](term T) bool {
	var t Expr[F, T] = term
	//
	if t, ok := t.(*Constant[F, T]); ok {
		return t.Value.IsZero()
	}
	//
	return false
}

// Check whether a given term corresponds with the constant one.
func isOne[F field.Element[F], T Expr[F, T]](term T) bool {
	var t Expr[F, T] = term
	//
	if t, ok := t.(*Constant[F, T]); ok {
		return t.Value.IsOne()
	}
	//
	return false
}

func lispOfLogicalTerms[F any, T Logical[F, T]](global bool, mapping register.Map, op string,
	exprs []T) sexp.SExp {
	//
	arr := make([]sexp.SExp, 1+len(exprs))
	arr[0] = sexp.NewSymbol(op)
	// Translate arguments
	for i, e := range exprs {
		if any(e) != nil {
			arr[i+1] = e.Lisp(global, mapping)
		} else {
			arr[i+1] = sexp.NewSymbol("??")
		}
	}
	// Done
	return sexp.NewList(arr)
}

func lispOfTerms[F any, E any, T Expr[F, E]](global bool, mapping register.Map, op string, exprs []T) sexp.SExp {
	arr := make([]sexp.SExp, 1+len(exprs))
	arr[0] = sexp.NewSymbol(op)
	// Translate arguments
	for i, e := range exprs {
		arr[i+1] = e.Lisp(global, mapping)
	}
	// Done
	return sexp.NewList(arr)
}
func requiredRegistersOfTerms[T Contextual](args []T) *set.SortedSet[uint] {
	return set.UnionSortedSets(args, func(term T) *set.SortedSet[uint] {
		return term.RequiredRegisters()
	})
}

func requiredCellsOfTerms[T Contextual](args []T, row int, tr trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return set.UnionAnySortedSets(args, func(term T) *set.AnySortedSet[trace.CellRef] {
		return term.RequiredCells(row, tr)
	})
}

func substituteTerms[F any, T Substitutable[F]](mapping map[string]F, terms ...T) {
	//
	for _, term := range terms {
		term.Substitute(mapping)
	}
}

func shiftRangeOfTerms[E any, T Shiftable[E]](terms ...T) (int, int) {
	minShift := math.MaxInt
	maxShift := math.MinInt
	//
	for _, term := range terms {
		tMin, tMax := term.ShiftRange()
		minShift = min(minShift, tMin)
		maxShift = max(maxShift, tMax)
	}
	//
	return minShift, maxShift
}

func applyShiftOfTerms[T Shiftable[T]](terms []T, shift int) []T {
	nterms := make([]T, len(terms))
	//
	for i := range terms {
		nterms[i] = terms[i].ApplyShift(shift)
	}
	//
	return nterms
}

type binop[F any] func(F, F) F

// Simplify logical terms
func simplifyLogicalTerms[F field.Element[F], T Logical[F, T]](terms []T, casts bool) []T {
	var nterms = make([]T, len(terms))
	//
	for i, t := range terms {
		nterms[i] = t.Simplify(casts)
	}
	//
	return nterms
}

// General purpose constant propagation mechanism.  This reduces all terms to
// constants (where possible) and combines terms according to a given
// combinator.
func simplifyTerms[F field.Element[F], T Expr[F, T]](terms []T, fn binop[F], acc F, casts bool) []T {
	// Count how many terms reduced to constants.
	var (
		count  = 0
		nterms = make([]T, len(terms))
		ith    Expr[F, T]
	)
	// Propagate through all children
	for i, e := range terms {
		nterms[i] = e.Simplify(casts)
		ith = nterms[i]
		// Check for constant
		c, ok := ith.(*Constant[F, T])
		// Try to continue sum
		if ok {
			// Apply combinator
			acc = fn(acc, c.Value)
			// Increase count of constants
			count++
		}
	}
	// Merge all constants
	return mergeConstants(acc, nterms)
}

// Replace all constants within a given sequence of expressions with a single
// constant (whose value has been precomputed from those constants).  The new
// value replaces the first constant in the list.
func mergeConstants[F field.Element[F], T Expr[F, T]](constant F, terms []T) []T {
	var (
		j     = 0
		first = true
	)
	//
	for i := range terms {
		var tmp Expr[F, T] = terms[i]
		// Check for constant
		if _, ok := tmp.(*Constant[F, T]); ok && first {
			tmp = &Constant[F, T]{constant}
			terms[j] = tmp.(T)
			first = false
			j++
		} else if !ok {
			// Retain non-constant expression
			terms[j] = terms[i]
			j++
		}
	}
	// Return slice
	return terms[0:j]
}

func addBinOp[F field.Element[F]](lhs F, rhs F) F {
	return lhs.Add(rhs)
}

func mulBinOp[F field.Element[F]](lhs F, rhs F) F {
	return lhs.Mul(rhs)
}
