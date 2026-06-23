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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Equals constructs an Equal representing the equality of two expressions.
func Equals[F field.Element[F], S Logical[F, S], T Expr[F, T]](lhs T, rhs T) S {
	var term Logical[F, S] = &Equal[F, S, T]{
		Lhs: lhs,
		Rhs: rhs,
	}
	//
	return term.(S)
}

// ============================================================================

// Equal represents an Equal between two terms (e.g. "X==Y", or "X!=Y+1",
// etc).  Equals are either equalities (or negated equalities) or
// inequalities.
type Equal[F field.Element[F], S Logical[F, S], T Expr[F, T]] struct {
	Lhs Expr[F, T]
	Rhs Expr[F, T]
}

// ApplyShift implementation for LogicalTerm interface.
func (p *Equal[F, S, T]) ApplyShift(shift int) S {
	return Equals[F, S](p.Lhs.ApplyShift(shift), p.Rhs.ApplyShift(shift))
}

// ShiftRange implementation for LogicalTerm interface.
func (p *Equal[F, S, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Lhs.(T), p.Rhs.(T))
}

// Bounds implementation for Boundable interface.
func (p *Equal[F, S, T]) Bounds() util.Bounds {
	l := p.Lhs.Bounds()
	r := p.Rhs.Bounds()
	//
	l.Union(&r)
	//
	return l
}

// TestAt implementation for Testable interface.
func (p *Equal[F, S, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	lhs, err1 := p.Lhs.EvalAt(k, tr, sc)
	rhs, err2 := p.Rhs.EvalAt(k, tr, sc)
	// error check
	if err1 != nil {
		return false, 0, err1
	} else if err2 != nil {
		return false, 0, err2
	}
	// perform comparison
	c := lhs.Cmp(rhs)
	//
	return c == 0, 0, nil
}

// Lisp returns a lisp representation of this Equal, which is useful for
// debugging.
func (p *Equal[F, S, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	var (
		l = p.Lhs.Lisp(global, mapping)
		r = p.Rhs.Lisp(global, mapping)
	)
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("=="), l, r})
}

// Negate implementation for LogicalTerm interface
func (p *Equal[F, S, T]) Negate() S {
	var tmp Logical[F, S] = &NotEqual[F, S, T]{p.Lhs, p.Rhs}
	//
	return tmp.(S)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Equal[F, S, T]) RequiredRegisters() *set.SortedSet[uint] {
	set := p.Lhs.RequiredRegisters()
	set.InsertSorted(p.Rhs.RequiredRegisters())
	//
	return set
}

// RequiredCells implementation for Contextual interface
func (p *Equal[F, S, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	set := p.Lhs.RequiredCells(row, mid)
	set.InsertSorted(p.Rhs.RequiredCells(row, mid))
	//
	return set
}

// Simplify this term as much as reasonably possible.
// nolint
func (p *Equal[F, S, T]) Simplify(casts bool) S {
	var (
		lhs = p.Lhs.Simplify(casts)
		rhs = p.Rhs.Simplify(casts)
	)
	//
	lc, lok := IsConstant(lhs)
	rc, rok := IsConstant(rhs)
	//
	if lok && rok {
		// Can simplify
		if lc.Cmp(rc) == 0 {
			return True[F, S]()
		}
		//
		return False[F, S]()
	}
	// Cannot simplify
	var tmp Logical[F, S] = &Equal[F, S, T]{lhs, rhs}
	// Done
	return tmp.(S)
}

// Substitute implementation for Substitutable interface.
func (p *Equal[F, S, T]) Substitute(mapping map[string]F) {
	p.Lhs.Substitute(mapping)
	p.Rhs.Substitute(mapping)
}
