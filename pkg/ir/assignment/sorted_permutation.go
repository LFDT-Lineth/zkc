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
package assignment

import (
	"encoding/gob"
	"fmt"
	"slices"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// SortedPermutation declares one or more columns as sorted permutations of
// existing columns.
type SortedPermutation[F field.Element[F]] struct {
	// Target columns declared by this sorted permutation (in the order
	// of declaration).
	Targets []register.Ref
	// Signs determines the sorting direction for each target column.
	Signs []bool
	// Source columns which define the new (sorted) columns.
	Sources []register.Ref
}

// NewSortedPermutation creates a new sorted permutation
func NewSortedPermutation[F field.Element[F]](targets []register.Ref, signs []bool,
	sources []register.Ref) *SortedPermutation[F] {
	//
	if len(targets) != len(sources) {
		panic("target and source column have differing lengths!")
	} else if len(signs) == 0 || len(signs) > len(targets) {
		panic("invalid sort directions")
	}
	//
	return &SortedPermutation[F]{targets, signs, sources}
}

// ============================================================================
// Assignment Interface
// ============================================================================

// Bounds determines the well-definedness bounds for this assignment for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
func (p *SortedPermutation[F]) Bounds(_ sc.ModuleId) util.Bounds {
	return util.EMPTY_BOUND
}

// Compute computes the values of columns defined by this assignment. This
// requires copying the data in the source columns, and sorting that data
// according to the permutation criteria.
func (p *SortedPermutation[F]) Compute(trace tr.Trace[F], schema sc.AnySchema[F]) ([]array.MutArray[F], error) {
	// Read inputs
	sources := ReadRegistersRef(trace, p.Sources...)
	// Apply native function
	data := sortedPermutationNativeFunction(sources, p.Signs, trace.Builder())
	//
	return data, nil
}

// Consistent performs some simple checks that the given schema is consistent.
// This provides a double check of certain key properties, such as that
// registers used for assignments are large enough, etc.
func (p *SortedPermutation[F]) Consistent(schema sc.AnySchema[F]) []error {
	var errors []error
	// // Sanity check source types
	for i := range p.Sources {
		source := schema.Register(p.Sources[i])
		target := schema.Register(p.Targets[i])
		// Sanit checkout
		if source.Width() != target.Width() {
			err := fmt.Errorf("sorted permutation has inconsistent type for column %s => %s (was u%d, expected u%d)",
				source.Name(), target.Name(), target.Width(), source.Width())
			errors = append(errors, err)
		}
	}
	//
	return errors
}

// RegistersExpanded identifies registers expanded by this assignment.
func (p *SortedPermutation[F]) RegistersExpanded() []register.Ref {
	return nil
}

// RegistersRead returns the set of columns that this assignment depends upon.
// That can include both input columns, as well as other computed columns.
func (p *SortedPermutation[F]) RegistersRead() []register.Ref {
	return p.Sources
}

// RegistersWritten identifies registers assigned by this assignment.
func (p *SortedPermutation[F]) RegistersWritten() []register.Ref {
	return p.Targets
}

// Substitute any matchined labelled constants within this assignment
func (p *SortedPermutation[F]) Substitute(mapping map[string]F) {
	// Nothing to do here.
}

// ============================================================================
// Lispify Interface
// ============================================================================

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (p *SortedPermutation[F]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	var (
		targets = sexp.EmptyList()
		sources = sexp.EmptyList()
	)

	for _, t := range p.Targets {
		ith := schema.Module(t.Module()).Register(t.Column())
		name := sexp.NewSymbol(ith.QualifiedName(schema.Module(t.Module())))
		datatype := sexp.NewSymbol(fmt.Sprintf("u%d", ith.Width()))
		def := sexp.NewList([]sexp.SExp{name, datatype})
		targets.Append(def)
	}

	for i, s := range p.Sources {
		ith := schema.Module(s.Module()).Register(s.Column())
		ith_name := ith.QualifiedName(schema.Module(s.Module()))
		//
		if i >= len(p.Signs) {

		} else if p.Signs[i] {
			ith_name = fmt.Sprintf("+%s", ith_name)
		} else {
			ith_name = fmt.Sprintf("-%s", ith_name)
		}
		//
		sources.Append(sexp.NewSymbol(ith_name))
	}

	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("sort"),
		targets,
		sources,
	})
}

// ============================================================================
// Native Function
// ============================================================================

// PermutationSort sorts an array of columns in row-wise fashion.  For
// example, suppose consider [ [0,4,3,3], [1,2,4,3] ].  We can imagine
// that this is first transformed into an array of rows (i.e.
// [[0,1],[4,2],[3,4],[3,3]]) and then sorted lexicographically (to
// give [[0,1],[3,3],[3,4],[4,2]]).  This is then projected back into
// the original column-wise formulation, to give: [[0,3,3,4],
// [1,3,4,2]].
//
// A further complication is that the direction of sorting for each
// columns is determined by its sign.
//
// NOTE: the current implementation is not intended to be particularly
// efficient.  In particular, would be better to do the sort directly
// on the columns array without projecting into the row-wise form.
func sortedPermutationNativeFunction[F field.Element[F]](sources []array.Array[F], signs []bool,
	builder array.Builder[F]) []array.MutArray[F] {
	//
	var (
		n = sources[0].Len()
		// TODO: can we avoid allocating this array?
		indices = rangeOf(n)
		targets = make([]array.MutArray[F], len(sources))
	)
	// Perform the permutation sort
	slices.SortFunc(indices, permutationSortFunc(sources, signs))
	//
	for i, source := range sources {
		target := builder.NewArray(n, source.BitWidth())
		//
		for j, index := range indices {
			target = target.Set(uint(j), source.Get(index))
		}
		//
		targets[i] = target
	}
	//
	return targets
}

func permutationSortFunc[F field.Element[F], T array.Array[F]](cols []T, signs []bool) func(uint, uint) int {
	return func(lhs, rhs uint) int {
		//
		for i := 0; i < len(signs); i++ {
			var (
				lval = cols[i].Get(lhs)
				rval = cols[i].Get(rhs)
			)
			// Compare ith elements
			c := lval.Cmp(rval)
			// Check whether same
			if c != 0 {
				if signs[i] {
					// Positive
					return c
				}
				// Negative
				return -c
			}
		}
		// Identical
		return 0
	}
}

// Constuct an array of contiguous integers from 0..n.
func rangeOf(n uint) []uint {
	items := make([]uint, n)
	//
	for i := range n {
		items[i] = i
	}
	//
	return items
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

func init() {
	gob.Register(sc.Assignment[word.BigEndian](&SortedPermutation[word.BigEndian]{}))
}
