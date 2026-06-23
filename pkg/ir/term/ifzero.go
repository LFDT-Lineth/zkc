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
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// IfZero returns the true branch when the condition evaluates to zero, and the
// false branch otherwise.
type IfZero[F field.Element[F], S Logical[F, S], T Expr[F, T]] struct {
	// Elements contained within this list.
	Condition S
	// True branch
	TrueBranch T
	// False branch
	FalseBranch T
}

// IfElse constructs a new conditional with true and false branches.  Note, the
// true branch is taken when the condition evaluates to zero.
func IfElse[F field.Element[F], S Logical[F, S], T Expr[F, T]](condition S, trueBranch T, falseBranch T) T {
	var term Expr[F, T] = &IfZero[F, S, T]{condition, trueBranch, falseBranch}
	return term.(T)
}

// ApplyShift implementation for Term interface.
func (p *IfZero[F, S, T]) ApplyShift(shift int) T {
	var (
		c  = p.Condition.ApplyShift(shift)
		tb = p.TrueBranch.ApplyShift(shift)
		fb = p.FalseBranch.ApplyShift(shift)
	)
	//
	return IfElse(c, tb, fb)
}

// Bounds returns max shift in either the negative (left) or positive
// direction (right).
func (p *IfZero[F, S, T]) Bounds() util.Bounds {
	c := p.Condition.Bounds()
	// Get bounds for true branch
	tbounds := p.TrueBranch.Bounds()
	c.Union(&tbounds)
	// Get bounds for false branch
	fbounds := p.FalseBranch.Bounds()
	c.Union(&fbounds)
	// Done
	return c
}

// EvalAt implementation for Evaluable interface.
func (p *IfZero[F, S, T]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Evaluate condition
	cond, _, err := p.Condition.TestAt(k, tr, sc)
	//
	if err != nil {
		var dummy F
		return dummy, err
	} else if cond {
		return p.TrueBranch.EvalAt(k, tr, sc)
	}
	//
	return p.FalseBranch.EvalAt(k, tr, sc)
}

// Lisp implementation for Lispifiable interface.
func (p *IfZero[F, S, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	// Translate Condition
	condition := p.Condition.Lisp(global, mapping)
	// Dispatch on type
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("if"),
		condition,
		p.TrueBranch.Lisp(global, mapping),
		p.FalseBranch.Lisp(global, mapping),
	})
}

// RequiredRegisters implementation for Contextual interface.
func (p *IfZero[F, S, T]) RequiredRegisters() *set.SortedSet[uint] {
	set := p.Condition.RequiredRegisters()
	// Include true branch
	set.InsertSorted(p.TrueBranch.RequiredRegisters())
	// Include false branch
	set.InsertSorted(p.FalseBranch.RequiredRegisters())
	// Done
	return set
}

// RequiredCells implementation for Contextual interface
func (p *IfZero[F, S, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	set := p.Condition.RequiredCells(row, mid)
	// Include true branch
	set.InsertSorted(p.TrueBranch.RequiredCells(row, mid))
	// Include false branch
	set.InsertSorted(p.FalseBranch.RequiredCells(row, mid))
	// Done
	return set
}

// ShiftRange implementation for Term interface.
func (p *IfZero[F, S, T]) ShiftRange() (int, int) {
	cMin, cMax := p.Condition.ShiftRange()
	tMin, tMax := p.TrueBranch.ShiftRange()
	fMin, fMax := p.FalseBranch.ShiftRange()
	//
	return min(cMin, tMin, fMin), max(cMax, tMax, fMax)
}

// ValueRange implementation for Term interface.
func (p *IfZero[F, S, T]) ValueRange() math.Interval {
	var (
		trueBounds  = p.TrueBranch.ValueRange()
		falseBounds = p.FalseBranch.ValueRange()
	)
	//
	return trueBounds.Union(falseBounds)
}

// Substitute implementation for Substitutable interface.
func (p *IfZero[F, S, T]) Substitute(mapping map[string]F) {
	p.Condition.Substitute(mapping)
	p.FalseBranch.Substitute(mapping)
	p.TrueBranch.Substitute(mapping)
}

// Simplify implementation for Term interface.
//
// nolint
func (p *IfZero[F, S, T]) Simplify(casts bool) T {
	var (
		cond        = p.Condition.Simplify(casts)
		trueBranch  = p.TrueBranch.Simplify(casts)
		falseBranch = p.FalseBranch.Simplify(casts)
	)
	// Handle reductive cases
	if IsTrue(cond) {
		return trueBranch
	} else if IsFalse(cond) {
		return falseBranch
	}
	// Done
	var term Expr[F, T] = &IfZero[F, S, T]{cond, trueBranch, falseBranch}
	//
	return term.(T)
}
