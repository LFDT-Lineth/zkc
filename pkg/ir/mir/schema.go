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
package mir

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Following types capture top-level abstractions at the MIR level.
type (
	// SchemaBuilder is used for building the MIR schemas
	SchemaBuilder[F field.Element[F]] = ir.SchemaBuilder[F, Constraint[F], Term[F]]
	// ModuleBuilder is used for building various MIR modules.
	ModuleBuilder[F field.Element[F]] = ir.ModuleBuilder[F, Constraint[F], Term[F]]
	// Module captures the essence of a module at the MIR level.  Specifically, it
	// is limited to only those constraint forms permitted at the MIR level.
	Module[F field.Element[F]] = *schema.Table[F, Constraint[F]]
	// Schema captures the notion of an MIR schema which is uniform and consists of
	// MIR modules only.
	Schema[F field.Element[F]] = schema.UniformSchema[F, Module[F]]
	// Term represents the fundamental for arithmetic expressions in the MIR
	// representation.
	Term[F any] interface {
		term.Expr[F, Term[F]]
	}
	// LogicalTerm represents the fundamental for logical expressions in the MIR
	// representation.
	LogicalTerm[F any] interface {
		term.Logical[F, LogicalTerm[F]]
	}
	// Computation captures the notion of computations used in a small number of places.
	Computation = term.Computation[word.BigEndian]
	// LogicalComputation captures the notion of computations used in a small number of places.
	LogicalComputation = term.LogicalComputation[word.BigEndian]
)

// Following types capture permitted constraint forms at the MIR level.
type (
	// LookupConstraint captures the essence of a lookup constraint at the MIR
	// level.
	LookupConstraint[F field.Element[F]] = lookup.Constraint[F]
	// LookupVector provides a convenient shorthand
	LookupVector = lookup.Vector
	// RangeConstraint captures the essence of a range constraints at the MIR level.
	RangeConstraint[F field.Element[F]] = ranged.Constraint[F]
	// VanishingConstraint captures the essence of a vanishing constraint at the MIR
	// level. A vanishing constraint is a row constraint which must evaluate to
	// zero.
	VanishingConstraint[F field.Element[F]] = vanishing.Constraint[F, LogicalTerm[F]]
)

// Following types capture permitted expression forms at the MIR level.
type (
	// Add represents the addition of zero or more expressions.
	Add[F field.Element[F]] = term.Add[F, Term[F]]
	// Constant represents a constant value within an expression.
	Constant[F field.Element[F]] = term.Constant[F, Term[F]]
	// RegisterAccess represents reading the value held at a given column in the
	// tabular context.  Furthermore, the current row maybe shifted up (or down) by
	// a given amount.
	RegisterAccess[F field.Element[F]] = term.RegisterAccess[F, Term[F]]
	// Mul represents the product over zero or more expressions.
	Mul[F field.Element[F]] = term.Mul[F, Term[F]]
	// Sub represents the subtraction over zero or more expressions.
	Sub[F field.Element[F]] = term.Sub[F, Term[F]]
	// VectorAccess represents a compound variable
	VectorAccess[F field.Element[F]] = term.VectorAccess[F, Term[F]]
)

// Following types capture permitted logical forms at the MIR level.
type (
	// Conjunct represents a logical conjunction at the MIR level.
	Conjunct[F field.Element[F]] = term.Conjunct[F, LogicalTerm[F]]
	// Disjunct represents a logical conjunction at the MIR level.
	Disjunct[F field.Element[F]] = term.Disjunct[F, LogicalTerm[F]]
	// Equal represents an equality comparator between two arithmetic terms
	// at the MIR level.
	Equal[F field.Element[F]] = term.Equal[F, LogicalTerm[F], Term[F]]
	// Ite represents an If-Then-Else expression where either branch is optional
	// (though we must have at least one).
	Ite[F field.Element[F]] = term.Ite[F, LogicalTerm[F]]
	// Negate represents a logical negation at the MIR level.
	Negate[F field.Element[F]] = term.Negate[F, LogicalTerm[F]]
	// NotEqual represents a non-equality comparator between two arithmetic terms
	// at the MIR level.
	NotEqual[F field.Element[F]] = term.NotEqual[F, LogicalTerm[F], Term[F]]
)
