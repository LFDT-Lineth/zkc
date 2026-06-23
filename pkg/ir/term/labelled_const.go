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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// LabelledConst represents a constant value which is labelled with a given
// name.  The purpose of this is to allow labelled constants to be substituted
// for different values when desired.
type LabelledConst[F field.Element[F], T Expr[F, T]] struct {
	Label string
	Value F
}

// LabelledConstant construct an expression representing a constant with a given
// label.
func LabelledConstant[F field.Element[F], T Expr[F, T]](label string, value F) T {
	var term Expr[F, T] = &LabelledConst[F, T]{Label: label, Value: value}
	return term.(T)
}

// ApplyShift implementation for Term interface.
func (p *LabelledConst[F, T]) ApplyShift(int) T {
	var term Expr[F, T] = p
	return term.(T)
}

// Bounds implementation for Boundable interface.
func (p *LabelledConst[F, T]) Bounds() util.Bounds {
	return util.EMPTY_BOUND
}

// EvalAt implementation for Evaluable interface.
func (p *LabelledConst[F, T]) EvalAt(k int, _ trace.Module[F], _ register.Map) (F, error) {
	return p.Value, nil
}

// Lisp implementation for Lispifiable interface.
func (p *LabelledConst[F, T]) Lisp(_ bool, _ register.Map) sexp.SExp {
	return sexp.NewSymbol(p.Value.String())
}

// RequiredRegisters implementation for Contextual interface.
func (p *LabelledConst[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return set.NewSortedSet[uint]()
}

// RequiredCells implementation for Contextual interface
func (p *LabelledConst[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return set.NewAnySortedSet[trace.CellRef]()
}

// ShiftRange implementation for Term interface.
func (p *LabelledConst[F, T]) ShiftRange() (int, int) {
	return math.MaxInt, math.MinInt
}

// Simplify implementation for Term interface.
func (p *LabelledConst[F, T]) Simplify(casts bool) T {
	var tmp Expr[F, T] = p
	return tmp.(T)
}

// Substitute implementation for Substitutable interface.
func (p *LabelledConst[F, T]) Substitute(mapping map[string]F) {
	// Attempt to apply substitution
	if nval, ok := mapping[p.Label]; ok {
		p.Value = nval
	}
}

// ValueRange implementation for Term interface.
func (p *LabelledConst[F, T]) ValueRange() util_math.Interval {
	var c big.Int
	// Extract big integer from field element
	c.SetBytes(p.Value.Bytes())
	// Return as interval
	return util_math.NewInterval(c, c)
}
