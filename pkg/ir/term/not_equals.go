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

// NotEqual represents an NotEqual between two terms (e.g. "X==Y", or "X!=Y+1",
// etc).  NotEquals are either NotEqualities (or negated NotEqualities) or
// inNotEqualities.
type NotEqual[F field.Element[F], S Logical[F, S], T Expr[F, T]] struct {
	Lhs Expr[F, T]
	Rhs Expr[F, T]
}

// NotEquals constructs an NotEqual representing the NotEquality of two expressions.
func NotEquals[F field.Element[F], S Logical[F, S], T Expr[F, T]](lhs T, rhs T) S {
	var (
		term Logical[F, S] = &NotEqual[F, S, T]{
			Lhs: lhs,
			Rhs: rhs,
		}
		res, ok = term.(S)
	)
	//
	if ok {
		return res
	}
	// Sanity check
	panic("invalid logical AIR term")
}

// ApplyShift implementation for LogicalTerm interface.
func (p *NotEqual[F, S, T]) ApplyShift(shift int) S {
	return NotEquals[F, S](p.Lhs.ApplyShift(shift), p.Rhs.ApplyShift(shift))
}

// ShiftRange implementation for LogicalTerm interface.
func (p *NotEqual[F, S, T]) ShiftRange() (int, int) {
	return shiftRangeOfTerms(p.Lhs.(T), p.Rhs.(T))
}

// Bounds implementation for Boundable interface.
func (p *NotEqual[F, S, T]) Bounds() util.Bounds {
	l := p.Lhs.Bounds()
	r := p.Rhs.Bounds()
	//
	l.Union(&r)
	//
	return l
}

// Negate implementation for LogicalTerm interface
func (p *NotEqual[F, S, T]) Negate() S {
	var tmp Logical[F, S] = &Equal[F, S, T]{p.Lhs, p.Rhs}
	//
	return tmp.(S)
}

// TestAt implementation for Testable interface.
func (p *NotEqual[F, S, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
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
	return c != 0, 0, nil
}

// Lisp returns a lisp representation of this NotEqual, which is useful for
// debugging.
func (p *NotEqual[F, S, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	var (
		l = p.Lhs.Lisp(global, mapping)
		r = p.Rhs.Lisp(global, mapping)
	)
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("!="), l, r})
}

// RequiredRegisters implementation for Contextual interface.
func (p *NotEqual[F, S, T]) RequiredRegisters() *set.SortedSet[uint] {
	set := p.Lhs.RequiredRegisters()
	set.InsertSorted(p.Rhs.RequiredRegisters())
	//
	return set
}

// RequiredCells implementation for Contextual interface
func (p *NotEqual[F, S, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	set := p.Lhs.RequiredCells(row, mid)
	set.InsertSorted(p.Rhs.RequiredCells(row, mid))
	//
	return set
}

// Simplify this term as much as reasonably possible.
//
// nolint
func (p *NotEqual[F, S, T]) Simplify(casts bool) S {
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
			return False[F, S]()
		}
		//
		return True[F, S]()
	}
	// Cannot simplify
	var tmp Logical[F, S] = &NotEqual[F, S, T]{lhs, rhs}
	// Done
	return tmp.(S)
}

// Substitute implementation for Substitutable interface.
func (p *NotEqual[F, S, T]) Substitute(mapping map[string]F) {
	p.Lhs.Substitute(mapping)
	p.Rhs.Substitute(mapping)
}
