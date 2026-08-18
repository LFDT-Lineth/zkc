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
package lookup

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Set provides a convenient alias
type Set[F any] = collection.Set[F]

// SetId provides a convenient alias
type SetId = schema.SetId

// Constraint (sometimes also called an inclusion constraint) constrains
// two sets of columns (potentially in different modules). Specifically, every
// row in the source columns must match a row in the target columns (but not
// vice-versa).  As such, the number of source columns must be the same as the
// number of target columns.  Furthermore, every source column must be in the
// same module, and likewise for target modules.  However, the source columns
// can be in a different module from the target columns.
//
// Lookup constraints are typically used to "connect" modules together.  We can
// think of them (in some ways) as being a little like function calls.  In this
// analogy, the source module is making a "function call" into the target
// module.  That is, the target module contains the set of valid input/output
// pairs (and perhaps other constraints to ensure the required relationship) and
// the source module is just checking that a given set of input/output pairs
// makes sense.
type Constraint[F field.Element[F]] struct {
	// Handle returns the handle for this lookup constraint which is simply an
	// identifier useful when debugging (i.e. to know which lookup failed, etc).
	Handle string
	// Targets returns the target registers which are used to lookup into the
	// target registers.
	Targets []Vector
	// Sources returns the source registers which are used to lookup into the
	// target registers.
	Sources []Vector
}

// NewConstraint creates a new lookup constraint with a given handle.
func NewConstraint[F field.Element[F]](handle string, targets []Vector, sources []Vector) Constraint[F] {
	var width uint
	// Check sources
	for i, ith := range sources {
		if i != 0 && ith.Len() != width {
			panic("inconsistent number of source lookup columns")
		}

		width = ith.Len()
	}
	// Check targets
	for _, ith := range targets {
		if ith.Len() != width {
			panic("inconsistent number of target lookup columns")
		}
	}

	return Constraint[F]{Handle: handle,
		Targets: targets,
		Sources: sources,
	}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Constraint[F]) Consistent(_ schema.AnySchema[F]) []error {
	return nil
}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p Constraint[F]) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p Constraint[F]) Contexts() []schema.ModuleId {
	var contexts []schema.ModuleId
	// source contexts
	for _, source := range p.Sources {
		contexts = append(contexts, source.Module)
	}
	// target contexts
	for _, target := range p.Targets {
		contexts = append(contexts, target.Module)
	}
	//
	return contexts
}

// Sets implementation for schema.Constraint interface.
func (p Constraint[F]) Sets() (sets []SetId) {
	//
	for _, v := range p.Targets {
		sets = append(sets, v.SetId())
	}
	//
	return sets
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  Since a lookup is made
// up of registers (rather than arbitrary expressions), it is always well
// defined on every row.
//
//nolint:revive
func (p Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Accepts checks whether a lookup constraint into the target columns holds for
// all rows of the source columns.
//
//nolint:revive
func (p Constraint[F]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F], ctx schema.Context[F]) schema.Failure {
	var (
		// Load target sets
		targets = loadSets(ctx, p.Targets...)
		// Initialise read buffer
		buffer = make([]F, p.Sources[0].Len())
	)
	// Subset check
	for _, source := range p.Sources {
		var trModule = tr.Module(source.Module)
		// Check each row in the set determined by this vector.
		if err := p.checkSourceSet(source.SetId(), trModule, targets, buffer); err != nil {
			return err
		}
	}
	//
	return nil
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
//
//nolint:revive
func (p Constraint[F]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
	var (
		sources = sexp.EmptyList()
		targets = sexp.EmptyList()
	)
	// Iterate source registers
	for _, ith := range p.Sources {
		sources.Append(ith.Lisp(mapping.Module(ith.Module)))
	}
	// Iterate target registers
	for _, ith := range p.Targets {
		targets.Append(ith.Lisp(mapping.Module(ith.Module)))
	}
	// Done
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("lookup"),
		sexp.NewSymbol(fmt.Sprintf("\"%s\"", p.Handle)),
		targets,
		sources,
	})
}

// Substitute any matchined labelled constants within this constraint.  Since a
// lookup is made up of registers (rather than arbitrary expressions), there is
// nothing to substitute.
func (p Constraint[F]) Substitute(mapping map[string]F) {

}

// Check that all rows in a given source set are contained within at least one
// of the given target sets.
func (p Constraint[F]) checkSourceSet(src SetId, mod trace.Module[F], sets []Set[[]F], buffer []F) schema.Failure {
	if src.HasSelector() {
		var selector = src.Selector().Unwrap()
		//
		for row := range mod.Height() {
			if !mod.Column(selector).Get(row).IsZero() {
				if !contains(row, src, mod, sets, buffer) {
					return &Failure[F]{p.Handle, src, row}
				}
			}
		}
	} else {
		// Optimised path when no selector
		for row := range mod.Height() {
			if !contains(row, src, mod, sets, buffer) {
				return &Failure[F]{p.Handle, src, row}
			}
		}
	}
	//
	return nil
}

// check whether the given source row is contained within any of the given sets.
// A temporary buffer of sufficient width is provided to avoid memory
// allocation.
func contains[F field.Element[F]](row uint, src SetId, mod trace.Module[F], sets []Set[[]F], buffer []F) bool {
	// Read registers into buffer
	for i := range src.Width() {
		var rid = src.Ith(i).Unwrap()
		// Read given row
		buffer[i] = mod.Column(rid).Get(row)
	}
	// Check for containment
	for _, set := range sets {
		if set.Contains(buffer) {
			return true
		}
	}
	//
	return false
}

// Load those sets from the context corresponding to the given vectors.
func loadSets[F field.Element[F]](ctx schema.Context[F], vecs ...Vector) []Set[[]F] {
	var (
		sets = make([]Set[[]F], len(vecs))
	)
	// Load target sets
	for i, v := range vecs {
		sets[i] = ctx.Get(v.SetId())
	}
	//
	return sets
}
