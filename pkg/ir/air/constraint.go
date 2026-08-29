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
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// ============================================================================
// Helpers
// ============================================================================

// ConstraintBound limits the permitted set of underlying constraints.  This
// should never change, unless the underlying prover changes in some way to
// offer different or more fundamental primitives (as it did for the bus
// constraint).
type ConstraintBound[F field.Element[F]] interface {
	schema.Constraint[F]

	bus.Constraint[F] |
		lookup.Constraint[F] |
		ranged.Constraint[F] |
		vanishing.Constraint[F, LogicalTerm[F]]
}

// Air attempts to encapsulate the notion of a valid constraint at the AIR
// level.  Since this is the fundamental level, only certain constraint forms
// are permitted.  As such, we want to try and ensure that arbitrary constraints
// are not found at the Air level.
type Air[F field.Element[F], C ConstraintBound[F]] struct {
	constraint C
}

// newAir is a helper method for the various constraint constructors, basically
// to avoid lots of generic types.
func newAir[F field.Element[F], C ConstraintBound[F]](constraint C) Air[F, C] {
	return Air[F, C]{constraint}
}

// NewBusConstraint constructs a new AIR bus constraint
func NewBusConstraint[F field.Element[F]](handle string, sends []bus.Port,
	receives []bus.Port) BusConstraint[F] {
	//
	return newAir(bus.NewConstraint[F](handle, sends, receives))
}

// NewLookupConstraint constructs a new AIR lookup constraint
func NewLookupConstraint[F field.Element[F]](handle string, targets []lookup.Vector,
	sources []lookup.Vector) LookupConstraint[F] {
	//
	return newAir(lookup.NewConstraint[F](handle, targets, sources))
}

// NewRangeConstraint constructs a new AIR range constraint
func NewRangeConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, registers []register.Id,
	bitwidths []uint) RangeConstraint[F] {
	//
	return newAir(ranged.NewConstraint[F](handle, ctx, registers, bitwidths))
}

// NewVanishingConstraint constructs a new AIR vanishing constraint
func NewVanishingConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, domain util.Option[int],
	term Term[F]) VanishingConstraint[F] {
	//
	return newAir(vanishing.NewConstraint(handle, ctx, domain, LogicalTerm[F]{term}))
}

// Air marks the constraint as being valid for the AIR representation.
func (p Air[F, C]) Air() {
	// nothing as just a marker.
}

// Accepts determines whether a given constraint accepts a given trace or
// not.  If not, a failure is produced.  Otherwise, a bitset indicating
// branch coverage is returned.
func (p Air[F, C]) Accepts(trace trace.Trace[F], schema schema.AnySchema[F], ctx schema.Context[F],
) []schema.Failure[F] {
	return p.constraint.Accepts(trace, schema, ctx)
}

// Bounds determines the well-definedness bounds for this constraint in both the
// negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass)
func (p Air[F, C]) Bounds(module uint) util.Bounds {
	return p.constraint.Bounds(module)
}

// Complexity implementation for constraint interface
func (p Air[F, C]) Complexity() uint {
	var bound schema.Constraint[F] = p.constraint
	//
	if c, ok := bound.(vanishing.Constraint[F, LogicalTerm[F]]); ok {
		var t = c.Constraint.Term
		return term.ComplexityOfTerm[F](t)
	} else if c, ok := bound.(*vanishing.Constraint[F, LogicalTerm[F]]); ok {
		var t = c.Constraint.Term
		return term.ComplexityOfTerm[F](t)
	}

	//
	return 0
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Air[F, C]) Consistent(schema schema.AnySchema[F]) []error {
	return p.constraint.Consistent(schema)
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p Air[F, C]) Contexts() []schema.ModuleId {
	return p.constraint.Contexts()
}

// Sets implementation for schema.Constraint interface.
func (p Air[F, C]) Sets() []schema.SetId {
	return p.constraint.Sets()
}

// Name returns a unique name and case number for a given constraint.  This
// is useful purely for identifying constraints in reports, etc.
func (p Air[F, C]) Name() string {
	return p.constraint.Name()
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
//
//nolint:revive
func (p Air[F, C]) Lisp(schema schema.AnySchema[F]) sexp.SExp {
	return p.constraint.Lisp(schema)
}

// Unwrap provides access to the underlying constraint.
func (p Air[F, C]) Unwrap() C {
	return p.constraint
}
