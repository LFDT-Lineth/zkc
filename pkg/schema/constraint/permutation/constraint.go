// Copyright Consensys Incorporated
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
package permutation

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint is a permutation constraint between two sets of vectors, where
// each vector is a tuple of registers in some module with an optional selector.
// A vector is active on those rows where its selector is non-zero or, when it
// has no selector, on all rows.  The constraint holds when the tuples read from
// the active rows of the left vectors form the same multiset as those read from
// the active rows of the right vectors.  Thus, unlike a lookup (which only
// requires inclusion), the order of the tuples does not matter but the number
// of occurrences of each tuple does.
//
// Every vector must have the same number of registers.  However, vectors can be
// in different modules and, hence, the number of rows on either side can differ
// provided the number of active rows does not.
//
// A permutation is checked within each shard of a trace independently.  That
// is, tuples in one shard are never matched against tuples in another.
type Constraint[F field.Element[F]] struct {
	// Handle returns the handle for this permutation which is simply an
	// identifier useful when debugging (i.e. to know which permutation failed,
	// etc).
	Handle string
	// Left holds the vectors forming the left-hand side of the permutation
	// check (i.e. its targets).
	Left []constraint.Vector
	// Right holds the vectors forming the right-hand side of the permutation
	// check (i.e. its sources).
	Right []constraint.Vector
}

// NewConstraint constructs a permutation between equally wide vector tuples.
func NewConstraint[F field.Element[F]](handle string, targets, sources []constraint.Vector) *Constraint[F] {
	var width uint
	// Check sources
	for i, ith := range sources {
		if i != 0 && ith.Len() != width {
			panic("inconsistent number of source permutation columns")
		}

		width = ith.Len()
	}
	// Check targets
	for _, ith := range targets {
		if ith.Len() != width {
			panic("inconsistent number of target permutation columns")
		}
	}
	// Done
	return &Constraint[F]{Handle: handle, Left: targets, Right: sources}
}

// Validate applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p *Constraint[F]) Validate(_ schema.Schema[F]) {

}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p *Constraint[F]) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. permutations) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p *Constraint[F]) Contexts() []schema.ModuleId {
	var contexts []schema.ModuleId
	// source contexts
	for _, source := range p.Right {
		contexts = append(contexts, source.Module)
	}
	// target contexts
	for _, target := range p.Left {
		contexts = append(contexts, target.Module)
	}
	//
	return contexts
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  Since a permutation is made
// up of registers (rather than arbitrary expressions), it is always well
// defined on every row.
//
//nolint:revive
func (p *Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Lisp prints the permutation using the same operand order as its source
// syntax (i.e. sources followed by targets).
func (p *Constraint[F]) Lisp(mapping schema.Schema[F]) sexp.SExp {
	var (
		sources = sexp.EmptyList()
		targets = sexp.EmptyList()
	)
	// Iterate source registers
	for _, ith := range p.Right {
		sources.Append(ith.Lisp[F](mapping.Module(ith.Module)))
	}
	// Iterate target registers
	for _, ith := range p.Left {
		targets.Append(ith.Lisp[F](mapping.Module(ith.Module)))
	}
	// Done
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("permutation"),
		sexp.NewSymbol(fmt.Sprintf("\"%s\"", p.Handle)),
		sources,
		targets,
	})
}
