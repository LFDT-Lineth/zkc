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
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint attempts to encapsulate the notion of a valid constraint at the MIR
// level.  Since this is the fundamental level, only certain constraint forms
// are permitted.  As such, we want to try and ensure that arbitrary constraints
// are not found at the Constraint[F] level.
type Constraint[F field.Element[F]] struct {
	constraint schema.Constraint[F]
}

// NewVanishingConstraint constructs a new vanishing constraint
func NewVanishingConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, domain util.Option[int],
	term LogicalTerm[F]) Constraint[F] {
	//
	return Constraint[F]{vanishing.NewConstraint(handle, ctx, domain, term)}
}

// NewLookupConstraint creates a new lookup constraint with a given handle.
func NewLookupConstraint[F field.Element[F]](handle string, targets []LookupVector,
	sources []LookupVector) Constraint[F] {
	//
	return Constraint[F]{lookup.NewConstraint[F](handle, targets, sources)}
}

// NewRangeConstraint constructs a new Range constraint
func NewRangeConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, registers []register.Id,
	bitwidths []uint) Constraint[F] {
	//
	return Constraint[F]{ranged.NewConstraint[F](handle, ctx, registers, bitwidths)}
}

// Accepts determines whether a given constraint accepts a given trace or
// not.  If not, a failure is produced.  Otherwise, a bitset indicating
// branch coverage is returned.
func (p Constraint[F]) Accepts(trace rtrace.Trace[F], sc schema.AnySchema[F], ctx schema.Context[F]) schema.Failure {
	//
	return p.constraint.Accepts(trace, sc, ctx)
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

// Sets implementation for schema.Constraint interface.
func (p Constraint[F]) Sets() []schema.SetId {
	return p.constraint.Sets()
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

// Unwrap provides access to the underlying constraint.
func (p Constraint[F]) Unwrap() schema.Constraint[F] {
	return p.constraint
}
