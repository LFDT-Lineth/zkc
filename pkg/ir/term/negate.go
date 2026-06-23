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

// Negation constructs a term representing the negation of a logical term.
func Negation[F field.Element[F], T Logical[F, T]](body T) T {
	var term Logical[F, T] = &Negate[F, T]{
		Arg: body,
	}
	//
	return term.(T)
}

// ============================================================================

// Negate represents an Negate between two terms (e.g. "X==Y", or "X!=Y+1",
// etc).  Negate are either Negateities (or negated Negateities) or
// inNegateities.
type Negate[F field.Element[F], T Logical[F, T]] struct {
	Arg T
}

// ApplyShift implementation for LogicalTerm interface.
func (p *Negate[F, T]) ApplyShift(shift int) T {
	return Negation(p.Arg.ApplyShift(shift))
}

// ShiftRange implementation for LogicalTerm interface.
func (p *Negate[F, T]) ShiftRange() (int, int) {
	return p.Arg.ShiftRange()
}

// Bounds implementation for Boundable interface.
func (p *Negate[F, T]) Bounds() util.Bounds {
	return p.Arg.Bounds()
}

// TestAt implementation for Testable interface.
func (p *Negate[F, T]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	val, branch, err := p.Arg.TestAt(k, tr, sc)
	//
	return !val, branch, err
}

// Lisp returns a lisp representation of this Negate, which is useful for
// debugging.
func (p *Negate[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	var l = p.Arg.Lisp(global, mapping)
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("¬"), l})
}

// RequiredRegisters implementation for Contextual interface.
func (p *Negate[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return p.Arg.RequiredRegisters()
}

// RequiredCells implementation for Contextual interface
func (p *Negate[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return p.Arg.RequiredCells(row, mid)
}

// Negate implementation for LogicalTerm interface
func (p *Negate[F, T]) Negate() T {
	return p.Arg
}

// Simplify this Negate as much as reasonably possible.
func (p *Negate[F, T]) Simplify(casts bool) T {
	return p.Arg.Negate().Simplify(casts)
}

// Substitute implementation for Substitutable interface.
func (p *Negate[F, T]) Substitute(mapping map[string]F) {
	p.Arg.Substitute(mapping)
}
