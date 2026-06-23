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
package air

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/interleaving"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/permutation"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Following types capture top-level abstractions at the AIR level.
type (
	// Schema captures the essence of an arithmetisation at the AIR level.
	// Specifically, it is limited to only those constraint forms permitted at the
	// AIR level.
	Schema[F field.Element[F]] = schema.UniformSchema[F, Module[F]]
	// Module captures the essence of a module at the AIR level.  Specifically, it
	// is limited to only those constraint forms permitted at the AIR level.
	Module[F field.Element[F]] = *schema.Table[F, Constraint[F]]
	// Constraint captures the essence of a constraint at the AIR level.
	Constraint[F field.Element[F]] interface {
		schema.Constraint[F]
		// Air marks the constraints as been valid for the AIR representation.
		Air()
	}
	// Term represents the fundamental for arithmetic expressions in the AIR
	// representation.  This should only support addition, subtraction and
	// multiplication of constants and column accesses.  No other terms are
	// permitted at this, the lowest, layer of the stack.
	Term[F field.Element[F]] interface {
		term.Expr[F, Term[F]]
		// Air marks terms which are valid for the AIR representation.
		Air()
	}
)

type (
	// SchemaBuilder is used for building the AIR schemas
	SchemaBuilder[F field.Element[F]] = ir.SchemaBuilder[F, Constraint[F], Term[F]]
	// ModuleBuilder is used for building various AIR modules.
	ModuleBuilder[F field.Element[F]] = ir.ModuleBuilder[F, Constraint[F], Term[F]]
)

// Following types capture permitted constraint forms at the AIR level.
type (
	// Assertion captures the notion of an arbitrary property which should hold for
	// all acceptable traces.  However, such a property is not enforced by the
	// prover.
	Assertion[F field.Element[F]] = Air[F, constraint.Assertion[F]]
	// InterleavingConstraint captures the essence of an interleaving constraint
	// at the MIR level.
	InterleavingConstraint[F field.Element[F]] = Air[F, interleaving.Constraint[F, *ColumnAccess[F]]]
	// LookupConstraint captures the essence of a lookup constraint at the AIR
	// level.  At the AIR level, lookup constraints are only permitted between
	// columns (i.e. not arbitrary expressions).
	LookupConstraint[F field.Element[F]] = Air[F, lookup.Constraint[F, *ColumnAccess[F]]]
	// PermutationConstraint captures the essence of a permutation constraint at the
	// AIR level. Specifically, it represents a constraint that one (or more)
	// columns are a permutation of another.
	PermutationConstraint[F field.Element[F]] = Air[F, permutation.Constraint[F]]
	// RangeConstraint captures the essence of a range constraints at the AIR level.
	RangeConstraint[F field.Element[F]] = Air[F, ranged.Constraint[F, *ColumnAccess[F]]]
	// VanishingConstraint captures the essence of a vanishing constraint at the AIR level.
	VanishingConstraint[F field.Element[F]] = Air[F, vanishing.Constraint[F, LogicalTerm[F]]]
)

// Following types capture permitted expression forms at the AIR level.
type (
	// Add represents the addition of zero or more AIR expressio
	Add[F field.Element[F]] = term.Add[F, Term[F]]
	// Constant represents a constant value within AIR an expression.
	Constant[F field.Element[F]] = term.Constant[F, Term[F]]
	// ColumnAccess represents reading the value held at a given column in the
	// tabular context.  Furthermore, the current row maybe shifted up (or down) by
	// a given amount.
	ColumnAccess[F field.Element[F]] = term.RegisterAccess[F, Term[F]]
	// Mul represents the product over zero or more expressions.
	Mul[F field.Element[F]] = term.Mul[F, Term[F]]
	// Sub represents the subtraction over zero or more expressions.
	Sub[F field.Element[F]] = term.Sub[F, Term[F]]
)

// LogicalTerm provides a wrapper around a given term allowing to be "testable".
// That is, it provides a default TestAt implementation.
type LogicalTerm[F field.Element[F]] struct {
	Term Term[F]
}

// Bounds implementation for Boundable interface.
func (p LogicalTerm[F]) Bounds() util.Bounds {
	return p.Term.Bounds()
}

// TestAt implementation for Testable interface.
func (p LogicalTerm[F]) TestAt(k int, tr trace.Module[F], sc register.Map) (bool, uint, error) {
	var (
		val, err = p.Term.EvalAt(k, tr, sc)
		zero     F
	)
	//
	if err != nil {
		return false, 0, err
	}
	//
	return val.Cmp(zero) == 0, 0, nil
}

// Lisp returns a lisp representation of this NotEqual, which is useful for
// debugging.
func (p LogicalTerm[F]) Lisp(global bool, mapping register.Map) sexp.SExp {
	return p.Term.Lisp(global, mapping)
}

// RequiredRegisters implementation for Contextual interface.
func (p LogicalTerm[F]) RequiredRegisters() *set.SortedSet[uint] {
	return p.Term.RequiredRegisters()
}

// RequiredCells implementation for Contextual interface
func (p LogicalTerm[F]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return p.Term.RequiredCells(row, mid)
}

// Substitute implementation for Substitutable interface.
func (p LogicalTerm[F]) Substitute(mapping map[string]F) {
	p.Term.Substitute(mapping)
}
