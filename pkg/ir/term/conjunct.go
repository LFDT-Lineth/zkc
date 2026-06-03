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
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Conjunct erpresents the logical AND of zero or more terms.  Observe that if
// there are no terms, then this is equivalent to logical truth.
type Conjunct[F field.Element[F], T Logical[F, T]] struct {
	// Terms here are disjuncted to formulate the final logical result.
	Args []T
}

// True constructs a logical truth
func True[F field.Element[F], T Logical[F, T]]() T {
	return Conjunction[F, T]()
}

// IsTrue checks whether a given term corresponds to logical truth which, in
// this system, corresponds to an empty conjunct.
func IsTrue[F field.Element[F], T Logical[F, T]](term T) bool {
	var t Logical[F, T] = term
	//
	if t, ok := t.(*Conjunct[F, T]); ok {
		return len(t.Args) == 0
	}
	//
	return false
}

// Conjunction builds the logical conjunction (i.e. and) for a given set of constraints.
func Conjunction[F field.Element[F], T Logical[F, T]](terms ...T) T {
	var term Logical[F, T] = &Conjunct[F, T]{terms}
	//
	if len(terms) == 1 {
		return terms[0]
	}
	//
	return term.(T)
}

// ApplyShift implementation for LogicalTerm interface.
func (p *Conjunct[F, T]) ApplyShift(shift int) T {
	return Conjunction(applyShiftOfTerms(p.Args, shift)...)
}

// Bounds implementation for Boundable interface.
func (p *Conjunct[F, T]) Bounds() util.Bounds {
	return util.BoundsForArray(p.Args)
}

// ShiftRange implementation for LogicalTerm interface.
func (p *Conjunct[F, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Args...)
}

// Negate implementation for LogicalTerm interface
func (p *Conjunct[F, S]) Negate() S {
	var nargs = make([]S, len(p.Args))
	//
	for i, t := range p.Args {
		nargs[i] = t.Negate()
	}
	//
	return Disjunction(nargs...)
}

// TestAt implementation for Testable interface.
func (p *Conjunct[F, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	//
	for _, disjunct := range p.Args {
		val, _, err := disjunct.TestAt(k, tr, sc)
		//
		if err != nil {
			return val, 0, err
		} else if !val {
			// Failure
			return val, 0, nil
		}
	}
	// Success
	return true, 0, nil
}

// Lisp returns a lisp representation of this equation, which is useful for
// debugging.
func (p *Conjunct[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	if len(p.Args) == 0 {
		return sexp.NewSymbol("⊤")
	}

	return lispOfLogicalTerms(global, mapping, "∧", p.Args)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Conjunct[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return requiredRegistersOfTerms(p.Args)
}

// RequiredCells implementation for Contextual interface
func (p *Conjunct[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return requiredCellsOfTerms(p.Args, row, mid)
}

// Simplify this term as much as reasonably possible.
func (p *Conjunct[F, T]) Simplify(casts bool) T {
	// Simplify terms
	terms := simplifyLogicalTerms(p.Args, casts)
	// Flatten any nested conjuncts
	terms = array.Flatten(terms, flattenConjunct[F, T])
	// False if contains false
	if array.ContainsMatching(terms, IsFalse) {
		return False[F, T]()
	}
	// Remove true values
	terms = array.RemoveMatching(terms, IsTrue[F, T])
	// Final checks
	switch len(terms) {
	case 0:
		return True[F, T]()
	case 1:
		return terms[0]
	default:
		return Conjunction(terms...)
	}
}

// Substitute implementation for Substitutable interface.
func (p *Conjunct[F, T]) Substitute(mapping map[string]F) {
	substituteTerms(mapping, p.Args...)
}

func flattenConjunct[F field.Element[F], T Logical[F, T]](term T) []T {
	var e Logical[F, T] = term
	if t, ok := e.(*Conjunct[F, T]); ok {
		return t.Args
	}
	//
	return nil
}
