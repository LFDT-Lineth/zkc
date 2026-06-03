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

// Disjunct erpresents the logical OR of zero or more terms.  Observe that if
// there are no terms, then this is equivalent to logical falsehood.
type Disjunct[F field.Element[F], T Logical[F, T]] struct {
	Args []T
}

// False constructs a logical falsehood
func False[F field.Element[F], T Logical[F, T]]() T {
	return Disjunction[F, T]()
}

// IsFalse Check whether a given term corresponds to logical falsehood which, in
// this system, corresponds to an empty disjunct.
func IsFalse[F field.Element[F], T Logical[F, T]](term T) bool {
	var t Logical[F, T] = term
	//
	if t, ok := t.(*Disjunct[F, T]); ok {
		return len(t.Args) == 0
	}
	//
	return false
}

// Disjunction creates a constraint representing the disjunction of a given set of
// constraints.
func Disjunction[F field.Element[F], T Logical[F, T]](terms ...T) T {
	var term Logical[F, T] = &Disjunct[F, T]{terms}
	//
	if len(terms) == 1 {
		return terms[0]
	}
	//
	return term.(T)
}

// ApplyShift implementation for LogicalTerm interface.
func (p *Disjunct[F, T]) ApplyShift(shift int) T {
	return Disjunction(applyShiftOfTerms(p.Args, shift)...)
}

// ShiftRange implementation for LogicalTerm interface.
func (p *Disjunct[F, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Args...)
}

// Bounds implementation for Boundable interface.
func (p *Disjunct[F, T]) Bounds() util.Bounds {
	return util.BoundsForArray(p.Args)
}

// Negate implementation for LogicalTerm interface
func (p *Disjunct[F, S]) Negate() S {
	var nargs = make([]S, len(p.Args))
	//
	for i, t := range p.Args {
		nargs[i] = t.Negate()
	}
	//
	return Conjunction(nargs...)
}

// TestAt implementation for Testable interface.
func (p *Disjunct[F, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	//
	for _, disjunct := range p.Args {
		val, _, err := disjunct.TestAt(k, tr, sc)
		//
		if err != nil {
			return val, 0, err
		} else if val {
			// Success
			return val, 0, nil
		}
	}
	// Failure
	return false, 0, nil
}

// Lisp returns a lisp representation of this equation, which is useful for
// debugging.
func (p *Disjunct[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	if len(p.Args) == 0 {
		return sexp.NewSymbol("⊥")
	}

	return lispOfLogicalTerms(global, mapping, "∨", p.Args)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Disjunct[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return requiredRegistersOfTerms(p.Args)
}

// RequiredCells implementation for Contextual interface
func (p *Disjunct[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return requiredCellsOfTerms(p.Args, row, mid)
}

// Simplify this term as much as reasonably possible.
func (p *Disjunct[F, T]) Simplify(casts bool) T {
	// Simplify terms
	terms := simplifyLogicalTerms(p.Args, casts)
	// Flatten any nested disjuncts
	terms = array.Flatten(terms, flatternDisjunct[F, T])
	// True if contains True
	if array.ContainsMatching(terms, IsTrue[F, T]) {
		return True[F, T]()
	}
	// Remove false values
	terms = array.RemoveMatching(terms, IsFalse[F, T])
	// Final checks
	switch len(terms) {
	case 0:
		return False[F, T]()
	case 1:
		return terms[0]
	default:
		return Disjunction(terms...)
	}
}

// Substitute implementation for Substitutable interface.
func (p *Disjunct[F, T]) Substitute(mapping map[string]F) {
	substituteTerms(mapping, p.Args...)
}

func flatternDisjunct[F field.Element[F], T Logical[F, T]](term T) []T {
	var e Logical[F, T] = term
	if t, ok := e.(*Disjunct[F, T]); ok {
		return t.Args
	}
	//
	return nil
}
