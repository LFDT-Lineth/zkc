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

// Constant represents a constant value within an expression.
type Constant[F field.Element[F], T Expr[F, T]] struct{ Value F }

// Const construct an AIR expression representing a given constant.
func Const[F field.Element[F], T Expr[F, T]](val F) T {
	var term Expr[F, T] = &Constant[F, T]{Value: val}
	return term.(T)
}

// Const64 construct an AIR expression representing a given constant from a
// uint64.
func Const64[F field.Element[F], T Expr[F, T]](val uint64) T {
	var term Expr[F, T] = &Constant[F, T]{
		Value: field.Uint64[F](val),
	}
	//
	return term.(T)
}

// IsConstant checks whether an artibrary term corresponds to a constant or not.
func IsConstant[F field.Element[F], T Expr[F, T]](term T) (F, bool) {
	var (
		tmp   Expr[F, T] = term
		dummy F
	)
	//
	if c, ok := tmp.(*Constant[F, T]); ok {
		return c.Value, true
	}
	//
	return dummy, false
}

// Air indicates this term can be used at the AIR level.
func (p *Constant[F, T]) Air() {}

// ApplyShift implementation for Term interface.
func (p *Constant[F, T]) ApplyShift(int) T {
	var term Expr[F, T] = p
	return term.(T)
}

// Bounds implementation for Boundable interface.
func (p *Constant[F, T]) Bounds() util.Bounds {
	return util.EMPTY_BOUND
}

// EvalAt implementation for Evaluable interface.
func (p *Constant[F, T]) EvalAt(k int, _ trace.Module[F], _ register.Map) (F, error) {
	return p.Value, nil
}

// Lisp implementation for Lispifiable interface.
func (p *Constant[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	var val big.Int
	//
	val.SetBytes(p.Value.Bytes())
	// // Check if power of 2
	// if n, ok := IsPowerOf2(val); ok && n > 8 {
	// 	// Not power of 2
	// 	return sexp.NewSymbol(fmt.Sprintf("2^%d", n))
	// }
	//
	return sexp.NewNumber(val)
}

// RequiredRegisters implementation for Contextual interface.
func (p *Constant[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	return set.NewSortedSet[uint]()
}

// RequiredCells implementation for Contextual interface
func (p *Constant[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return set.NewAnySortedSet[trace.CellRef]()
}

// ShiftRange implementation for Term interface.
func (p *Constant[F, T]) ShiftRange() (int, int) {
	return math.MaxInt, math.MinInt
}

// Substitute implementation for Substitutable interface.
func (p *Constant[F, T]) Substitute(mapping map[string]F) {

}

// Simplify implementation for Term interface.
func (p *Constant[F, T]) Simplify(casts bool) T {
	var tmp Expr[F, T] = p
	return tmp.(T)
}

// ValueRange implementation for Term interface.
func (p *Constant[F, T]) ValueRange() util_math.Interval {
	var c big.Int
	// Extract big integer from field element
	c.SetBytes(p.Value.Bytes())
	// Return as interval
	return util_math.NewInterval(c, c)
}

// IsPowerOf2 checks whether a given big integer matches 2^n for some n and, if
// so, n is returned.
func IsPowerOf2(val big.Int) (n uint, ok bool) {
	w := val.BitLen()
	//
	if w > 0 {
		m := big.NewInt(2)
		// compute 2^n-1
		m.Exp(m, big.NewInt(int64(w-1)), nil)
		// check for match
		return uint(w - 1), val.Cmp(m) == 0
	}
	//
	return 0, false
}
