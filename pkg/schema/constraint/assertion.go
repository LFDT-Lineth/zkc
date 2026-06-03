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
package constraint

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Property defines the type of logical properties which can be asserted.  This
// is intentionally left wide, and could include many things which cannot
// directly be represented at the AIR level.
type Property = term.LogicalComputation[word.BigEndian]

// AssertionFailure provides structural information about a failing vanishing constraint.
type AssertionFailure[F any] struct {
	// Handle of the failing constraint
	Handle string
	//
	Context schema.ModuleId
	// Constraint expression
	Constraint Property
	// Row on which the constraint failed
	Row uint
}

// Message provides a suitable error message
func (p *AssertionFailure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("assertion \"%s\" does not hold (row %d)", p.Handle, p.Row)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *AssertionFailure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	return p.Constraint.RequiredCells(int(p.Row), p.Context)
}

func (p *AssertionFailure[F]) String() string {
	return p.Message()
}

// Assertion is similar to a vanishing constraint but is used only for
// debugging / testing / verification.  Unlike vanishing constraints, property
// assertions do not represent something that the prover can enforce.  Rather,
// they represent properties which are expected to hold for every valid trace.
// That is, they should be implied by the actual constraints.  Thus, whilst the
// prover cannot enforce such properties, external tools (such as for formal
// verification) can attempt to ensure they do indeed always hold.
type Assertion[F field.Element[F]] struct {
	// A unique identifier for this constraint.  This is primarily
	// useful for debugging.
	Handle string
	// Enclosing module for this assertion.  This restricts the asserted
	// property to access only columns from within this module.
	Context schema.ModuleId
	// Indicates (when empty) a property that applies to all rows. Otherwise,
	// indicates a property which applies to the specific row given.
	Domain util.Option[int]
	// The actual assertion itself, namely an expression which
	// should hold (i.e. vanish) for every row of a trace.
	// Observe that this can be any function which is computable
	// on a given trace --- we are not restricted to expressions
	// which can be arithmetised.
	Property term.LogicalComputation[word.BigEndian]
}

// NewAssertion constructs a new property assertion!
func NewAssertion[F field.Element[F]](handle string, ctx schema.ModuleId, domain util.Option[int],
	property Property) Assertion[F] {
	//
	return Assertion[F]{handle, ctx, domain, property}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Assertion[F]) Consistent(schema schema.AnySchema[F]) []error {
	return CheckConsistent(p.Context, schema, p.Property)
}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p Assertion[F]) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p Assertion[F]) Contexts() []schema.ModuleId {
	return []schema.ModuleId{p.Context}
}

// Bounds is not required for a property assertion since these are not real
// constraints.
func (p Assertion[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Accepts checks whether a vanishing constraint evaluates to zero on every row
// of a table. If so, return nil otherwise return an error.
//
//nolint:revive
func (p Assertion[F]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var (
		coverage bit.Set
		// Determine height of enclosing module
		height = tr.Module(p.Context).Height()
		// Determine well-definedness bounds for this constraint
		bounds = p.Property.Bounds()
	)
	// Sanity check enough rows
	if p.Domain.HasValue() {
		var row int = p.Domain.Unwrap()
		//
		if row < 0 {
			row += int(height)
		}
		//
		return p.acceptRange(uint(row), uint(row)+1, tr, sc)
	} else if bounds.End < height {
		return p.acceptRange(bounds.Start, height-bounds.End, tr, sc)
	}
	// All good
	return coverage, nil
}

// Lisp converts this constraint into an S-Expression.
//
//nolint:revive
func (p Assertion[F]) Lisp(schema schema.AnySchema[F]) sexp.SExp {
	var (
		module           = schema.Module(p.Context)
		assertion string = "assert"
	)
	// Handle attributes
	if p.Domain.HasValue() {
		switch p.Domain.Unwrap() {
		case 0:
			assertion = fmt.Sprintf("%s:first", assertion)
		case -1:
			assertion = fmt.Sprintf("%s:last", assertion)
		default:
			domain := p.Domain.Unwrap()
			panic(fmt.Sprintf("domain value %d not supported for local constraint", domain))
		}
	}
	// Construct the list
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(assertion),
		sexp.NewSymbol(p.Handle),
		p.Property.Lisp(false, module),
	})
}

// Substitute any matchined labelled constants within this constraint
func (p Assertion[F]) Substitute(mapping map[string]F) {
	// Sanity check we have what we expect
	if m, ok := any(mapping).(map[string]word.BigEndian); ok {
		p.Property.Substitute(m)
		return
	}
	// Fail (should be unreachable)
	panic("cannot substitute arbitrary field elements")
}

func (p Assertion[F]) acceptRange(start, end uint, tr trace.Trace[F], sc schema.AnySchema[F],
) (bit.Set, schema.Failure) {
	var (
		coverage bit.Set
		trModule = trace.ModuleAdapter[F, word.BigEndian](tr.Module(p.Context))
		scModule = sc.Module(p.Context)
	)
	// Check all in-bounds values
	for k := start; k < end; k++ {
		// Check whether property holds (or was undefined)
		if ok, id, err := p.Property.TestAt(int(k), trModule, scModule); err != nil {
			// Evaluation failure
			return coverage, NewInternalFailure[F](p.Handle, p.Context, k, nil, err.Error())
		} else if !ok {
			return coverage, &AssertionFailure[F]{p.Handle, p.Context, p.Property, k}
		} else {
			// Update coverage
			coverage.Insert(id)
		}
	}
	//
	return coverage, nil
}
