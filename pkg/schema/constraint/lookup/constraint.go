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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

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
type Constraint[F field.Element[F], E term.Evaluable[F]] struct {
	// Handle returns the handle for this lookup constraint which is simply an
	// identifier useful when debugging (i.e. to know which lookup failed, etc).
	Handle string
	// Targets returns the target expressions which are used to lookup into the
	// target expressions.  NOTE: the first element here is *always* the target
	// selector.
	Targets []Vector[F, E]
	// Sources returns the source expressions which are used to lookup into the
	// target expressions.  NOTE: the first element here is *always* the source
	// selector.
	Sources []Vector[F, E]
}

// NewConstraint creates a new lookup constraint with a given handle.
func NewConstraint[F field.Element[F], E term.Evaluable[F]](handle string, targets []Vector[F, E],
	sources []Vector[F, E]) Constraint[F, E] {
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

	return Constraint[F, E]{Handle: handle,
		Targets: targets,
		Sources: sources,
	}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Constraint[F, E]) Consistent(_ schema.AnySchema[F]) []error {
	return nil
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

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
//
//nolint:revive
func (p Constraint[F, E]) Bounds(module uint) util.Bounds {
	var bound util.Bounds
	// sources
	for _, ith := range p.Sources {
		eth := ith.Bounds(module)
		bound.Union(&eth)
	}
	// targets
	for _, ith := range p.Targets {
		eth := ith.Bounds(module)
		bound.Union(&eth)
	}
	//
	return bound
}

// Accepts checks whether a lookup constraint into the target columns holds for
// all rows of the source columns.
//
//nolint:revive
func (p Constraint[F, E]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var (
		coverage bit.Set
		st       State[F, E]
		err      schema.Failure
	)
	// Insert all active target vectors
	if st, err = p.insertTargetVectors(tr, sc); err != nil {
		return coverage, err
	}
	// Check against all active source vectors
	if err = st.checkSourceVectors(p.Sources, tr, sc); err != nil {
		return coverage, err
	}
	//
	return coverage, nil
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
//
//nolint:revive
func (p Constraint[F, E]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
	var (
		sources = sexp.EmptyList()
		targets = sexp.EmptyList()
	)
	// Iterate source expressions
	for _, ith := range p.Sources {
		sources.Append(ith.Lisp(mapping))
	}
	// Iterate target expressions
	for _, ith := range p.Targets {
		targets.Append(ith.Lisp(mapping))
	}
	// Done
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("lookup"),
		sexp.NewSymbol(fmt.Sprintf("\"%s\"", p.Handle)),
		targets,
		sources,
	})
}

// Substitute any matchined labelled constants within this constraint
func (p Constraint[F, E]) Substitute(mapping map[string]F) {
	// Sources
	for _, ith := range p.Sources {
		ith.Substitute(mapping)
	}
	// Targets
	for _, ith := range p.Targets {
		ith.Substitute(mapping)
	}
}

func (p *Constraint[F, E]) insertTargetVectors(tr trace.Trace[F], sc schema.AnySchema[F]) (
	State[F, E], schema.Failure) {
	//
	var (
		st State[F, E]
		// Determine width (in columns) of this lookup
		width uint = p.Sources[0].Len()
	)
	// Initialise target state
	st.handle = p.Handle
	st.rows = hash.NewSet[hash.Array[F]](tr.Module(p.Targets[0].Module).Height())
	st.buffer = make([]F, width)
	// Choose optimised loop
	for _, target := range p.Targets {
		var (
			trModule = tr.Module(target.Module)
			scModule = sc.Module(target.Module)
			height   = trModule.Height()
		)
		//
		if target.HasSelector() {
			// unfiltered
			for i := range int(height) {
				if err := st.insertFilteredVector(i, target, trModule, scModule); err != nil {
					return st, err
				}
			}
		} else {
			// unfiltered
			for i := range int(height) {
				if err := st.insertTargetVector(i, target, trModule, scModule); err != nil {
					return st, err
				}
			}
		}
	}
	//
	return st, nil
}

// State is just bringing somethings together to make life simpler
type State[F field.Element[F], E term.Evaluable[F]] struct {
	handle string
	// Set of target rows
	rows *hash.Set[hash.Array[F]]
	// Temporary buffer to avoid lots of reallocations.
	buffer []F
}

func (p *State[F, E]) checkSourceVectors(sources []Vector[F, E], tr trace.Trace[F], sc schema.AnySchema[F],
) schema.Failure {
	// Choose optimised loop
	for _, source := range sources {
		var (
			trModule = tr.Module(source.Module)
			scModule = sc.Module(source.Module)
			height   = trModule.Height()
		)
		//
		if source.HasSelector() {
			// filtered
			for i := range int(height) {
				if err := p.checkFilteredSourceVector(i, source, trModule, scModule); err != nil {
					return err
				}
			}
		} else {
			// unfiltered
			for i := range int(height) {
				if err := p.checkSourceVector(i, source, trModule, scModule); err != nil {
					return err
				}
			}
		}
	}
	// success
	return nil
}

func (p *State[F, E]) insertFilteredVector(k int, vec Vector[F, E], trMod trace.Module[F], scMod schema.Module[F],
) schema.Failure {
	// If no selector, then always selected
	var selected bool = !vec.HasSelector()
	//
	if vec.HasSelector() {
		// Otherwise, check whether selector enabled (or not).
		var (
			selector = vec.Selector.Unwrap()
			ith, err = selector.EvalAt(k, trMod, scMod)
		)
		//
		if err != nil {
			return constraint.NewInternalFailure[F](p.handle, vec.Module, uint(k), vec.Selector.Unwrap(), err.Error())
		}
		// Selected when non-zero
		selected = !ith.IsZero()
	}
	// If row selected, then insert contents!
	if selected {
		return p.insertTargetVector(k, vec, trMod, scMod)
	}
	//
	return nil
}

func (p *State[F, E]) insertTargetVector(k int, vec Vector[F, E], trModule trace.Module[F],
	scModule schema.Module[F]) schema.Failure {
	// Check each source is included
	if err := p.evalExprsArray(k, vec, trModule, scModule); err != nil {
		return err
	}
	// Insert item whilst checking whether the buffer was consumed or not
	if !p.rows.Insert(hash.NewArray(p.buffer)) {
		// Yes, buffer consumed.  Therefore, construct fresh buffer to avoid
		// aliasing the value now stored in the hash set.
		p.buffer = slices.Clone(p.buffer)
	}
	//
	return nil
}

func (p *State[F, E]) checkFilteredSourceVector(k int, vec Vector[F, E], trModule trace.Module[F],
	scModule schema.Module[F]) schema.Failure {
	// If no selector, then always selected
	var selected bool = !vec.HasSelector()
	//
	if vec.HasSelector() {
		// Otherwise, check whether selector enabled (or not).
		var (
			selector = vec.Selector.Unwrap()
			ith, err = selector.EvalAt(k, trModule, scModule)
		)
		//
		if err != nil {
			return constraint.NewInternalFailure[F](p.handle, vec.Module, uint(k), vec.Selector.Unwrap(), err.Error())
		}
		// Selected when non-zero
		selected = !ith.IsZero()
	}
	// If row selected, then check contents!
	if selected {
		return p.checkSourceVector(k, vec, trModule, scModule)
	}
	//
	return nil
}

func (p *State[F, E]) checkSourceVector(k int, vec Vector[F, E], trModule trace.Module[F], scModule schema.Module[F],
) schema.Failure {
	// Check each source is included
	if err := p.evalExprsArray(k, vec, trModule, scModule); err != nil {
		return err
	}
	// Check whether contained.
	if !p.rows.Contains(hash.NewArray(p.buffer)) {
		sources := make([]term.Evaluable[F], vec.Len())
		for i, e := range vec.Terms {
			sources[i] = e
		}
		// Construct failures
		return &Failure[F]{p.handle, vec.Module, sources, uint(k)}
	}
	// success
	return nil
}

func (p *State[F, E]) evalExprsArray(k int, vec Vector[F, E], trModule trace.Module[F], scModule schema.Module[F],
) schema.Failure {
	var err error
	// Evaluate each expression in turn (remembering that the first element is
	// the selector)
	for i := uint(0); i < vec.Len(); i++ {
		p.buffer[i], err = vec.Ith(i).EvalAt(k, trModule, scModule)
		// error check
		if err != nil {
			return constraint.NewInternalFailure[F](p.handle, vec.Module, uint(k), vec.Ith(i), err.Error())
		}
	}
	// Done
	return nil
}
