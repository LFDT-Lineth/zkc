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
package gadgets

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Normalise constructs an expression representing the normalised value of e.
// That is, an expression which is 0 when e is 0, and 1 when e is non-zero.
// This is done by introducing a computed column to hold the (pseudo)
// multiplicative inverse of e.
func Normalise[F field.Element[F]](e air.Term[F], module air.ModuleBuilder[F]) air.Term[F] {
	// Construct pseudo multiplicative inverse of e.
	ie := applyPseudoInverseGadget(e, module)
	// Return e * e⁻¹.
	return term.Product(e, ie)
}

// applyPseudoInverseGadget constructs an expression representing the
// (pseudo) multiplicative inverse of another expression.  Since this cannot be computed
// directly using arithmetic constraints, it is done by adding a new computed
// column which holds the multiplicative inverse.  Constraints are also added to
// ensure it really holds the inverted value.
func applyPseudoInverseGadget[F field.Element[F]](e air.Term[F], module air.ModuleBuilder[F]) air.Term[F] {
	var (
		// Construct inverse computation
		ie = &pseudoInverse[F]{Expr: e}
		// Determine computed column name
		name = ie.Lisp(true, module).String(false)
		// Look up column
		index, ok = module.HasRegister(name)
		// Default padding (for now)
		padding = ir.PaddingFor[F](ie, module)
		// Indicate column has "field element width".
		bitwidth uint = math.MaxUint
	)
	// Add new column (if it does not already exist)
	if !ok {
		// Add computed register.
		index = module.NewRegister(register.NewComputed(name, bitwidth, padding))
		target := register.NewRef(module.Id(), index)
		// Add inverse assignment
		module.AddAssignment(assignment.NewPseudoInverse(target, e))
		// Construct proof of 1/e
		inv_e := term.FieldAccess[F, air.Term[F]](index, 0)
		// Construct e/e
		e_inv_e := term.Product[F, air.Term[F]](e, inv_e)
		// Construct 1 == e/e
		one_e_e := term.Subtract(term.Const64[F, air.Term[F]](1), e_inv_e)
		// Construct (e != 0) ==> (1 == e/e)
		e_implies_one_e_e := term.Product(e, one_e_e)
		l_name := fmt.Sprintf("%s <=", name)
		module.AddConstraint(air.NewVanishingConstraint(l_name, module.Id(), util.None[int](), e_implies_one_e_e))
	}
	// Done
	return term.FieldAccess[F, air.Term[F]](index, 0)
}

// IsZeroIndicator returns an AIR term that holds the "is zero" indicator of
// e — 1 when e evaluates to zero, 0 otherwise.  This is the value
// 1 - e*inv(e), realised as a shared computed column so that every caller
// that asks for the same e gets a single FieldAccess back instead of
// reconstructing a fresh 4-node subtree.
//
// The column is constrained by two vanishings:
//
//	e * (1 - e*inv) == 0       (the existing inverse constraint, emitted by
//	                            applyPseudoInverseGadget)
//	iz + e*inv - 1 == 0        (defining vanishing: iz = 1 - e*inv)
//
// Together with iz's 1-bit declared width (range constraint to {0,1}) these
// force iz to be the correct indicator value: 1 iff e == 0.
func IsZeroIndicator[F field.Element[F]](e air.Term[F], module air.ModuleBuilder[F]) air.Term[F] {
	// Ensure the inv column exists (creates it + its defining vanishing on
	// first call for this e).
	inv_e := applyPseudoInverseGadget(e, module)
	//
	return applyIsZeroGadget(e, inv_e, module)
}

// applyIsZeroGadget creates (or reuses) the iz column for e and returns a
// FieldAccess to it.  Sharing-by-name follows the same pattern as
// applyPseudoInverseGadget.
func applyIsZeroGadget[F field.Element[F]](
	e, inv_e air.Term[F], module air.ModuleBuilder[F],
) air.Term[F] {
	var (
		// Construct iz indicator term (used for name + padding only).
		iz = &pseudoIz[F]{Expr: e}
		// Determine computed column name.
		name = iz.Lisp(true, module).String(false)
		// Look up existing column.
		index, ok = module.HasRegister(name)
		// Padding value (0 or 1 depending on whether e is zero in padding).
		padding = ir.PaddingFor[F](iz, module)
	)
	// Add new column (if it does not already exist).
	if !ok {
		// iz ∈ {0,1}: width 1 doubles as a range constraint.
		index = module.NewRegister(register.NewComputed(name, 1, padding))
		target := register.NewRef(module.Id(), index)
		// Trace-filling assignment.
		module.AddAssignment(assignment.NewPseudoIz(target, e))
		// Defining vanishing:  iz + e*inv - 1 == 0
		var iz_access air.Term[F] = term.FieldAccess[F, air.Term[F]](index, 0)
		e_inv := term.Product[F, air.Term[F]](e, inv_e)
		defn := term.Subtract(
			term.Sum[F, air.Term[F]](iz_access, e_inv),
			term.Const64[F, air.Term[F]](1),
		)
		l_name := fmt.Sprintf("%s <=", name)
		module.AddConstraint(air.NewVanishingConstraint(l_name, module.Id(), util.None[int](), defn))
	}
	//
	return term.FieldAccess[F, air.Term[F]](index, 0)
}

// pseudoIz mirrors pseudoInverse but represents the "is zero" indicator
// (1 if Expr is zero, 0 otherwise).  Used for the column name and for the
// padding-value computation; the row-by-row trace fill lives in
// assignment.PseudoIz.
type pseudoIz[F field.Element[F]] struct {
	Expr air.Term[F]
}

// EvalAt returns 1 when Expr evaluates to zero, 0 otherwise.
func (e *pseudoIz[F]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	val, err := e.Expr.EvalAt(k, tr, sc)
	if err != nil {
		return val, err
	}
	//
	var one F
	if val.IsZero() {
		return one.SetUint64(1), nil
	}
	//
	var zero F
	return zero, nil
}

// Bounds delegates to the underlying expression.
func (e *pseudoIz[F]) Bounds() util.Bounds { return e.Expr.Bounds() }

// RequiredRegisters delegates to the underlying expression.
func (e *pseudoIz[F]) RequiredRegisters() *set.SortedSet[uint] {
	return e.Expr.RequiredRegisters()
}

// RequiredCells delegates to the underlying expression.
func (e *pseudoIz[F]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return e.Expr.RequiredCells(row, mid)
}

// Lisp encodes the iz indicator as (iz <expr>) for naming purposes.
func (e *pseudoIz[F]) Lisp(global bool, mapping register.Map) sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("iz"),
		e.Expr.Lisp(global, mapping),
	})
}

// Substitute implementation for Substitutable interface.
func (e *pseudoIz[F]) Substitute(mapping map[string]F) {
	panic("unreachable")
}

// ValueRange implementation for Term interface.  iz is always in {0,1}.
func (e *pseudoIz[F]) ValueRange() util_math.Interval {
	return util_math.NewInterval64(0, 1)
}

// pseudoInverse represents a computation which computes the multiplicative
// inverse of a given expression.  This is only needed now for the padding
// computation.
type pseudoInverse[F field.Element[F]] struct {
	Expr air.Term[F]
}

// EvalAt computes the multiplicative inverse of a given expression at a given
// row in the table.
func (e *pseudoInverse[F]) EvalAt(k int, tr trace.Module[F], sc register.Map) (F, error) {
	// Convert expression into something which can be evaluated, then evaluate
	// it.
	val, err := e.Expr.EvalAt(k, tr, sc)
	// Go syntax huh?
	inv := val.Inverse()
	// Done
	return inv, err
}

// Bounds returns max shift in either the negative (left) or positive
// direction (right).
func (e *pseudoInverse[F]) Bounds() util.Bounds { return e.Expr.Bounds() }

// RequiredRegisters returns the set of registers on which this term depends.
// That is, registers whose values may be accessed when evaluating this term on
// a given trace.
func (e *pseudoInverse[F]) RequiredRegisters() *set.SortedSet[uint] {
	return e.Expr.RequiredRegisters()
}

// RequiredCells returns the set of trace cells on which this term depends.
// In this case, that is the empty set.
func (e *pseudoInverse[F]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return e.Expr.RequiredCells(row, mid)
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (e *pseudoInverse[F]) Lisp(global bool, mapping register.Map) sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("inv"),
		e.Expr.Lisp(global, mapping),
	})
}

// Substitute implementation for Substitutable interface.
func (e *pseudoInverse[F]) Substitute(mapping map[string]F) {
	panic("unreachable")
}

// ValueRange implementation for Term interface.
func (e *pseudoInverse[F]) ValueRange() util_math.Interval {
	// This could be managed by having a mechanism for representing infinity
	// (e.g. nil). For now, this is never actually used, so we can just ignore
	// it.
	panic("unreachable")
}
