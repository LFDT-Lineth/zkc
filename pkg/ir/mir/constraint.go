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
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/constraint"
	"github.com/consensys/go-corset/pkg/schema/constraint/interleaving"
	"github.com/consensys/go-corset/pkg/schema/constraint/lookup"
	"github.com/consensys/go-corset/pkg/schema/constraint/permutation"
	"github.com/consensys/go-corset/pkg/schema/constraint/ranged"
	"github.com/consensys/go-corset/pkg/schema/constraint/sorted"
	"github.com/consensys/go-corset/pkg/schema/constraint/vanishing"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/source/sexp"
)

// Constraint attempts to encapsulate the notion of a valid constraint at the MIR
// level.  Since this is the fundamental level, only certain constraint forms
// are permitted.  As such, we want to try and ensure that arbitrary constraints
// are not found at the Constraint[F] level.
type Constraint[F field.Element[F]] struct {
	constraint schema.Constraint[F]
}

// NewAssertion constructs a new assertion
func NewAssertion[F field.Element[F]](handle string, ctx schema.ModuleId, domain util.Option[int],
	term constraint.Property) Constraint[F] {
	//
	return Constraint[F]{constraint.NewAssertion[F](handle, ctx, domain, term)}
}

// NewVanishingConstraint constructs a new vanishing constraint
func NewVanishingConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, domain util.Option[int],
	term LogicalTerm[F]) Constraint[F] {
	//
	return Constraint[F]{vanishing.NewConstraint(handle, ctx, domain, term)}
}

// NewInterleavingConstraint creates a new interleaving constraint with a given handle.
func NewInterleavingConstraint[F field.Element[F]](handle string, targetContext schema.ModuleId,
	sourceContext schema.ModuleId, target *VectorAccess[F], sources []*VectorAccess[F]) Constraint[F] {
	return Constraint[F]{interleaving.NewConstraint(handle, targetContext, sourceContext, target, sources)}
}

// NewLookupConstraint creates a new lookup constraint with a given handle.
func NewLookupConstraint[F field.Element[F]](handle string, targets []LookupVector[F],
	sources []LookupVector[F]) Constraint[F] {
	//
	return Constraint[F]{lookup.NewConstraint(handle, targets, sources)}
}

// NewSendConstraint creates a new "send to logUpBus" constraint; it doesn't take into account
// the potential Id of the logup bus, which would for instance determine what shared randomness to use.
func NewSendConstraint[F field.Element[F]](handle string, sources []register.Id) Constraint[F] {
	// TODO
	return Constraint[F]{}
}

// NewReceiveConstraint creates a new "receive from logUpBus" constraint; it doesn't take into account
// the potential Id of the logup bus, which would for instance determine what shared randomness to use.
func NewReceiveConstraint[F field.Element[F]](handle string, sources []register.Id) Constraint[F] {
	// TODO
	return Constraint[F]{}
}

// NewPermutationConstraint creates a new permutation
func NewPermutationConstraint[F field.Element[F]](handle string, context schema.ModuleId, targets []register.Id,
	sources []register.Id) Constraint[F] {
	return Constraint[F]{permutation.NewConstraint[F](handle, context, targets, sources)}
}

// NewRangeConstraint constructs a new Range constraint
func NewRangeConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, registers []*RegisterAccess[F],
	bitwidths []uint) Constraint[F] {
	//
	return Constraint[F]{ranged.NewConstraint(handle, ctx, registers, bitwidths)}
}

// NewSortedConstraint creates a new Sorted
func NewSortedConstraint[F field.Element[F]](handle string, context schema.ModuleId, bitwidth uint,
	selector util.Option[*RegisterAccess[F]], sources []*RegisterAccess[F], signs []bool, strict bool) Constraint[F] {
	//
	return Constraint[F]{sorted.NewConstraint(handle, context, bitwidth, selector, sources, signs, strict)}
}

// Accepts determines whether a given constraint accepts a given trace or
// not.  If not, a failure is produced.  Otherwise, a bitset indicating
// branch coverage is returned.
func (p Constraint[F]) Accepts(trace trace.Trace[F],
	schema schema.AnySchema[F]) (bit.Set, schema.Failure) {
	//
	return p.constraint.Accepts(trace, schema)
}

// Bounds determines the well-definedness bounds for this constraint in both the
// negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass)
func (p Constraint[F]) Bounds(module uint) util.Bounds {
	return p.constraint.Bounds(module)
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Constraint[F]) Consistent(schema schema.AnySchema[F]) []error {
	return p.constraint.Consistent(schema)
}

// Complexity implementation for constraint interface
func (p Constraint[F]) Complexity() uint {
	return 0
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p Constraint[F]) Contexts() []schema.ModuleId {
	return p.constraint.Contexts()
}

// Name returns a unique name and case number for a given constraint.  This
// is useful purely for identifying constraints in reports, etc.
func (p Constraint[F]) Name() string {
	return p.constraint.Name()
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
//
//nolint:revive
func (p Constraint[F]) Lisp(schema schema.AnySchema[F]) sexp.SExp {
	return p.constraint.Lisp(schema)
}

// Substitute any matchined labelled constants within this constraint
func (p Constraint[F]) Substitute(mapping map[string]F) {
	p.constraint.Substitute(mapping)
}

// Unwrap provides access to the underlying constraint.
func (p Constraint[F]) Unwrap() schema.Constraint[F] {
	return p.constraint
}
