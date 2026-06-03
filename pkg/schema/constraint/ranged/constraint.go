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
package ranged

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint restricts all values for a given expression to be within a
// range [0..n) for some bound n.  Any bound is supported, and the system will
// choose the best underlying implementation as needed.
type Constraint[F field.Element[F], E term.Evaluable[F]] struct {
	// A unique identifier for this constraint.  This is primarily useful for
	// debugging.
	Handle string
	// Evaluation Context for this constraint which must match that of the
	// constrained expression itself.
	Context schema.ModuleId
	// The expressions whose values are being constrained to be within the given
	// bound(s).
	Sources []E
	// The number of bits permitted for all values of the corresponding expression.
	// For example, with a bitwidth of 8, the maximum permitted value is 255.
	Bitwidths []uint
}

// NewConstraint constructs a new Range constraint!
func NewConstraint[F field.Element[F], E term.Evaluable[F]](handle string, context schema.ModuleId,
	exprs []E, bitwidths []uint) Constraint[F, E] {
	return Constraint[F, E]{handle, context, exprs, bitwidths}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Constraint[F, E]) Consistent(schema schema.AnySchema[F]) []error {
	var errors []error
	//
	if len(p.Bitwidths) != len(p.Sources) {
		errors = append(errors,
			fmt.Errorf("inconsistent number of expressions (%d) and bitwdiths (%d)", len(p.Sources), len(p.Bitwidths)))
	}
	//
	for _, e := range p.Sources {
		errs := constraint.CheckConsistent(p.Context, schema, e)
		errors = append(errors, errs...)
	}
	//
	return errors
}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p Constraint[F, E]) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p Constraint[F, E]) Contexts() []schema.ModuleId {
	return []schema.ModuleId{p.Context}
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
//
//nolint:revive
func (p Constraint[F, E]) Bounds(module uint) util.Bounds {
	var bound util.Bounds
	//
	if module == p.Context {
		for _, e := range p.Sources {
			eth := e.Bounds()
			bound.Union(&eth)
		}
	}
	//
	return bound
}

// Accepts checks whether a range constraint holds on every row of a table. If so, return
// nil otherwise return an error.
//
//nolint:revive
func (p Constraint[F, E]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var coverage bit.Set
	//
	for i := range p.Sources {
		_, err := p.accepts(i, tr, sc)
		//
		if err != nil {
			return coverage, err
		}
	}
	// All good
	return coverage, nil
}

// Lisp converts this schema element into a simple S-Expression, for example so
// it can be printed.
//
//nolint:revive
func (p Constraint[F, E]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
	var (
		module = mapping.Module(p.Context)
		pairs  = make([]sexp.SExp, len(p.Sources))
	)
	//
	for i, e := range p.Sources {
		pairs[i] = sexp.NewList([]sexp.SExp{
			e.Lisp(false, module),
			sexp.NewSymbol(fmt.Sprintf("u%d", p.Bitwidths[i])),
		})
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("range"),
		sexp.NewList(pairs),
	})
}

// Substitute any matchined labelled constants within this constraint
func (p Constraint[F, E]) Substitute(mapping map[string]F) {
	for _, s := range p.Sources {
		s.Substitute(mapping)
	}
}

func (p Constraint[F, E]) accepts(i int, tr trace.Trace[F], sc schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var (
		coverage bit.Set
		trModule = tr.Module(p.Context)
		scModule = sc.Module(p.Context)
		handle   = constraint.DetermineHandle(p.Handle, p.Context, tr)
		bound    F
		expr     = p.Sources[i]
		bitwidth = p.Bitwidths[i]
	)
	// Compute 2^n
	bound = field.TwoPowN[F](bitwidth)
	// Determine height of enclosing module
	height := tr.Module(p.Context).Height()
	// Iterate every row
	for k := 0; k < int(height); k++ {
		// Get the value on the kth row
		kth, err := expr.EvalAt(k, trModule, scModule)
		// Perform the range check
		if err != nil {
			return coverage, constraint.NewInternalFailure[F](p.Handle, p.Context, uint(k), expr, err.Error())
		} else if kth.Cmp(bound) >= 0 {
			// Evaluation failure
			return coverage, &Failure[F]{handle, p.Context, expr, bitwidth, uint(k)}
		}
	}
	// All good
	return coverage, nil
}
