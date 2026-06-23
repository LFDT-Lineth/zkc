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

// Ite represents an "If Then Else" expression which returns the (optional) true
// branch when the condition evaluates to zero, and the (optional false branch
// otherwise.
type Ite[F field.Element[F], T Logical[F, T]] struct {
	// Elements contained within this list.
	Condition T
	// True branch (optional).
	TrueBranch Logical[F, T]
	// False branch (optional).
	FalseBranch Logical[F, T]
}

// IfThenElse constructs a new conditional branch, where either the true branch
// or the false branch can (optionally) be nil (but both cannot).  Note, the
// true branch is taken when the condition evaluates to zero.
func IfThenElse[F field.Element[F], T Logical[F, T]](condition T, trueBranch T, falseBranch T) T {
	var term Logical[F, T] = &Ite[F, T]{condition, trueBranch, falseBranch}
	return term.(T)
}

// ApplyShift implementation for LogicalTerm interface.
func (p *Ite[F, T]) ApplyShift(shift int) T {
	var (
		c  = p.Condition.ApplyShift(shift)
		tb T
		fb T
	)
	//
	if p.TrueBranch != nil {
		tb = p.TrueBranch.ApplyShift(shift)
	}
	//
	if p.FalseBranch != nil {
		fb = p.FalseBranch.ApplyShift(shift)
	}
	//
	return IfThenElse(c, tb, fb)
}

// ShiftRange implementation for LogicalTerm interface.
func (p *Ite[F, T]) ShiftRange() (int, int) {
	switch {
	case p.TrueBranch == nil:
		return shiftRangeOfTerms(p.Condition, p.FalseBranch.(T))
	case p.FalseBranch == nil:
		return shiftRangeOfTerms(p.Condition, p.TrueBranch.(T))
	default:
		return shiftRangeOfTerms(p.Condition, p.TrueBranch.(T), p.FalseBranch.(T))
	}
}

// Bounds returns max shift in either the negative (left) or positive
// direction (right).
func (p *Ite[F, T]) Bounds() util.Bounds {
	c := p.Condition.Bounds()
	// Get bounds for true branch (if applicable)
	if p.TrueBranch != nil {
		tbounds := p.TrueBranch.Bounds()
		c.Union(&tbounds)
	}
	// Get bounds for false branch (if applicable)
	if p.FalseBranch != nil {
		fbounds := p.FalseBranch.Bounds()
		c.Union(&fbounds)
	}
	// Done
	return c
}

// Negate implementation for LogicalTerm interface
func (p *Ite[F, S]) Negate() S {
	var trueBranch, falseBranch S
	// Negate true branch
	if p.TrueBranch != nil {
		trueBranch = p.TrueBranch.Negate()
	} else {
		trueBranch = False[F, S]()
	}
	// Negate false branch
	if p.FalseBranch != nil {
		falseBranch = p.FalseBranch.Negate()
	} else {
		falseBranch = False[F, S]()
	}
	//
	return IfThenElse(p.Condition, trueBranch, falseBranch)
}

// TestAt implementation for Testable interface.
func (p *Ite[F, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	// Evaluate condition
	cond, branch, err := p.Condition.TestAt(k, tr, sc)
	//
	if err != nil {
		return cond, branch, err
	} else if cond && p.TrueBranch != nil {
		return p.TrueBranch.TestAt(k, tr, sc)
	} else if !cond && p.FalseBranch != nil {
		return p.FalseBranch.TestAt(k, tr, sc)
	}
	//
	return true, 0, nil
}

// Lisp implementation for Lispifiable interface.
func (p *Ite[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	// Translate Condition
	condition := p.Condition.Lisp(global, mapping)
	// Dispatch on type
	if p.FalseBranch == nil {
		return sexp.NewList([]sexp.SExp{
			sexp.NewSymbol("if"),
			condition,
			p.TrueBranch.Lisp(global, mapping),
		})
	} else if p.TrueBranch == nil {
		return sexp.NewList([]sexp.SExp{
			sexp.NewSymbol("ifnot"),
			condition,
			p.FalseBranch.Lisp(global, mapping),
		})
	}

	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("if"),
		condition,
		p.TrueBranch.Lisp(global, mapping),
		p.FalseBranch.Lisp(global, mapping),
	})
}

// RequiredRegisters implementation for Contextual interface.
func (p *Ite[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	set := p.Condition.RequiredRegisters()
	// Include true branch (if applicable)
	if p.TrueBranch != nil {
		set.InsertSorted(p.TrueBranch.RequiredRegisters())
	}
	// Include false branch (if applicable)
	if p.FalseBranch != nil {
		set.InsertSorted(p.FalseBranch.RequiredRegisters())
	}
	// Done
	return set
}

// RequiredCells implementation for Contextual interface
func (p *Ite[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	set := p.Condition.RequiredCells(row, mid)
	// Include true branch (if applicable)
	if p.TrueBranch != nil {
		set.InsertSorted(p.TrueBranch.RequiredCells(row, mid))
	}
	// Include false branch (if applicable)
	if p.FalseBranch != nil {
		set.InsertSorted(p.FalseBranch.RequiredCells(row, mid))
	}
	// Done
	return set
}

// Simplify this Negate as much as reasonably possible.  Overall, simplifying
// ite is surprisingly tricky.  However, its useful to retain ite rathe the
// compile it out completely as, in some cases, we can optimise things more
// effectively.
func (p *Ite[F, T]) Simplify(casts bool) T {
	var (
		cond        = p.Condition.Simplify(casts)
		trueBranch  Logical[F, T]
		falseBranch Logical[F, T]
	)
	// Handle reductive cases
	if IsTrue(cond) {
		if p.TrueBranch != nil {
			return p.TrueBranch.Simplify(casts)
		}
		//
		return True[F, T]()
	} else if IsFalse(cond) {
		if p.FalseBranch != nil {
			return p.FalseBranch.Simplify(casts)
		}
		//
		return True[F, T]()
	}
	// Simplify true branch (if applicable)
	if p.TrueBranch != nil {
		// If the branch logically true, then we can actually drop it
		// altogether (i.e. X || tt ==> tt)
		if tb := p.TrueBranch.Simplify(casts); !IsTrue(tb) {
			trueBranch = tb
		}
	}
	// Simplify false branch (if applicable)
	if p.FalseBranch != nil {
		// If the branch logically true, then we can actually drop it
		// altogether (i.e. !X || tt ==> tt)
		if fb := p.FalseBranch.Simplify(casts); !IsTrue(fb) {
			falseBranch = fb
		}
	}
	// More simplification opportunities
	if trueBranch == nil && falseBranch == nil {
		return True[F, T]()
	} else if trueBranch == nil && IsFalse(falseBranch.(T)) {
		return cond
	} else if falseBranch == nil && IsFalse(trueBranch.(T)) {
		return Negation(cond).Simplify(casts)
	}
	// Finally, done.
	var term Logical[F, T] = &Ite[F, T]{cond, trueBranch, falseBranch}
	//
	return term.(T)
}

// Substitute implementation for Substitutable interface.
func (p *Ite[F, T]) Substitute(mapping map[string]F) {
	p.Condition.Substitute(mapping)
	//
	if p.FalseBranch != nil {
		p.FalseBranch.Substitute(mapping)
	}
	//
	if p.TrueBranch != nil {
		p.TrueBranch.Substitute(mapping)
	}
}
