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

	"github.com/LFDT-Lineth/zkc/pkg/schema"
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
func (p Constraint[F]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var (
		coverage bit.Set
		// Insert all active target vectors
		st = p.insertTargetVectors(tr, sc)
	)
	// Check against all active source vectors
	if err := st.checkSourceVectors(p.Sources, tr); err != nil {
		return coverage, err
	}
	//
	return coverage, nil
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

func (p *Constraint[F]) insertTargetVectors(tr trace.Trace[F], sc schema.AnySchema[F]) State[F] {
	var (
		st State[F]
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
		if scModule.IsStatic() {
			st.insertStaticTarget(scModule)
		} else if target.HasSelector() {
			// filtered
			for i := range int(height) {
				st.insertFilteredVector(i, target, trModule)
			}
		} else {
			// unfiltered
			for i := range int(height) {
				st.insertTargetVector(i, target, trModule)
			}
		}
	}
	//
	return st
}

// State is just bringing somethings together to make life simpler
type State[F field.Element[F]] struct {
	handle string
	// Set of target rows
	rows *hash.Set[hash.Array[F]]
	// Temporary buffer to avoid lots of reallocations.
	buffer []F
}

func (p *State[F]) checkSourceVectors(sources []Vector, tr trace.Trace[F]) schema.Failure {
	// Choose optimised loop
	for _, source := range sources {
		var (
			trModule = tr.Module(source.Module)
			height   = trModule.Height()
		)
		//
		if source.HasSelector() {
			// filtered
			for i := range int(height) {
				if err := p.checkFilteredSourceVector(i, source, trModule); err != nil {
					return err
				}
			}
		} else {
			// unfiltered
			for i := range int(height) {
				if err := p.checkSourceVector(i, source, trModule); err != nil {
					return err
				}
			}
		}
	}
	// success
	return nil
}

func (p *State[F]) insertFilteredVector(k int, vec Vector, trMod trace.Module[F]) {
	// If row selected, then insert contents!
	if isSelected(k, vec, trMod) {
		p.insertTargetVector(k, vec, trMod)
	}
}

func (p *State[F]) insertTargetVector(k int, vec Vector, trModule trace.Module[F]) {
	// Read each register of this vector
	p.readRegisters(k, vec, trModule)
	// Insert item whilst checking whether the buffer was consumed or not
	if !p.rows.Insert(hash.NewArray(p.buffer)) {
		// Yes, buffer consumed.  Therefore, construct fresh buffer to avoid
		// aliasing the value now stored in the hash set.
		p.buffer = slices.Clone(p.buffer)
	}
}

func (p *State[F]) insertStaticTarget(scModule schema.Module[F]) {
	var (
		contents = scModule.StaticContents()
	)
	// Insert all rows
	for _, row := range contents {
		p.rows.Insert(hash.NewArray(row))
	}
}

func (p *State[F]) checkFilteredSourceVector(k int, vec Vector, trModule trace.Module[F]) schema.Failure {
	// If row selected, then check contents!
	if isSelected(k, vec, trModule) {
		return p.checkSourceVector(k, vec, trModule)
	}
	//
	return nil
}

func (p *State[F]) checkSourceVector(k int, vec Vector, trModule trace.Module[F]) schema.Failure {
	// Read each register of this vector
	p.readRegisters(k, vec, trModule)
	// Check whether contained.
	if !p.rows.Contains(hash.NewArray(p.buffer)) {
		// Construct failure
		return &Failure[F]{p.handle, vec.Module, slices.Clone(vec.Registers), uint(k)}
	}
	// success
	return nil
}

// readRegisters reads the value held in each register of the given vector on
// the given row into the temporary buffer.
func (p *State[F]) readRegisters(k int, vec Vector, trModule trace.Module[F]) {
	for i, rid := range vec.Registers {
		p.buffer[i] = trModule.Column(rid.Unwrap()).Get(k)
	}
}

// isSelected determines whether or not the given row of the given vector is
// selected.  A row without a selector is always selected; otherwise, it is
// selected when its selector is non-zero.
func isSelected[F field.Element[F]](k int, vec Vector, trModule trace.Module[F]) bool {
	// If no selector, then always selected
	if !vec.HasSelector() {
		return true
	}
	// Otherwise, selected when selector non-zero.
	return !trModule.Column(vec.Selector.Unwrap().Unwrap()).Get(k).IsZero()
}
