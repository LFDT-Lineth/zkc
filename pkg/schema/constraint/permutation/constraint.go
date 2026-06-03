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
package permutation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint declares a constraint that one (or more) columns are a permutation
// of another.
type Constraint[F field.Element[F]] struct {
	Handle string
	// Evaluation Context for this constraint which must match that of the
	// source and target expressions.
	Context schema.ModuleId
	// Targets returns the indices of the columns composing the "left" table of the
	// permutation.
	Targets []register.Id
	// Sources returns the indices of the columns composing the "right" table of the
	// permutation.
	Sources []register.Id
}

// NewConstraint creates a new permutation
func NewConstraint[F field.Element[F]](handle string, context schema.ModuleId, targets []register.Id,
	sources []register.Id) Constraint[F] {
	if len(targets) != len(sources) {
		panic("differeng number of target / source permutation columns")
	}

	return Constraint[F]{handle, context, targets, sources}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p Constraint[F]) Consistent(schema schema.AnySchema[F]) []error {
	// TODO: check column access, and widths, etc.
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
	return []schema.ModuleId{p.Context}
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
func (p Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Accepts checks whether a permutation holds between the source and
// target columns.
func (p Constraint[F]) Accepts(tr trace.Trace[F], _ schema.AnySchema[F]) (bit.Set, schema.Failure) {
	var (
		// Coverage currently always empty for permutation constraints.
		coverage bit.Set
		// Determine enclosing module
		module = tr.Module(p.Context)
	)
	// Slice out data
	src := sliceColumns(p.Sources, module)
	dst := sliceColumns(p.Targets, module)
	// Sanity check whether column exists
	if arePermutationOf(dst, src) {
		// Success
		return coverage, nil
	}
	// Prepare suitable error message
	src_names := qualifiedColumnNamesToCommaSeparatedString(p.Sources, module)
	dst_names := qualifiedColumnNamesToCommaSeparatedString(p.Targets, module)
	//
	msg := fmt.Sprintf("Target columns (%s) not permutation of source columns (%s)",
		dst_names, src_names)
	// Done
	return coverage, &Failure{msg}
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (p Constraint[F]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
	var (
		module  = mapping.Module(p.Context)
		targets = sexp.EmptyList()
		sources = sexp.EmptyList()
	)

	for _, tid := range p.Targets {
		target := module.Register(tid)
		targets.Append(sexp.NewSymbol(target.QualifiedName(module)))
	}

	for _, sid := range p.Sources {
		source := module.Register(sid)
		sources.Append(sexp.NewSymbol(source.QualifiedName(module)))
	}

	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("permutation"),
		targets,
		sources,
	})
}

// Substitute any matchined labelled constants within this constraint
func (p Constraint[F]) Substitute(map[string]F) {
	// nothing to do here
}

func sliceColumns[F any](columns []register.Id, tr trace.Module[F]) []array.Array[F] {
	// Allocate return array
	cols := make([]array.Array[F], len(columns))
	// Slice out the data
	for i, n := range columns {
		nth := tr.Column(n.Unwrap())
		// Copy over
		cols[i] = nth.Data()
	}
	// Done
	return cols
}

// QualifiedColumnNamesToCommaSeparatedString produces a suitable string for use
// in error messages from a list of one or more column identifies.
func qualifiedColumnNamesToCommaSeparatedString[F any](columns []register.Id, module trace.Module[F]) string {
	var names strings.Builder

	for i, c := range columns {
		if i != 0 {
			names.WriteString(",")
		}

		names.WriteString(module.Column(c.Unwrap()).Name())
	}
	// Done
	return names.String()
}

// ArePermutationOf checks whether or not a set of given destination columns are
// a valid permutation of a given set of source columns.  The number of source
// and target columns must match.  Likewise, they are expected to have the same
// height. This function does not modify any columns (though it does allocate
// intermediate arrays).
//
// This function operators by cloning the arrays, sorting them and checking they
// are the same.
func arePermutationOf[F field.Element[F], T array.Array[F]](dst []T, src []T) bool {
	if len(dst) != len(src) {
		return false
	}
	//
	nrows := dst[0].Len()
	dstIndices := rangeOf(nrows)
	srcIndices := rangeOf(nrows)
	// Sort indexed arrays
	slices.SortFunc(dstIndices, indexPermutationFunc(dst))
	slices.SortFunc(srcIndices, indexPermutationFunc(src))
	// Check rotated arrays match
	return equalsPermutation(dstIndices, srcIndices, dst, src)
}

// Check whether two indexed arrays are equal.
func equalsPermutation[F field.Element[F], T array.Array[F]](lIndices []uint, rIndices []uint, lhs []T, rhs []T) bool {
	if len(lIndices) != len(rIndices) {
		return false
	} else if len(lhs) != len(rhs) {
		return false
	}
	//
	for i := range len(lhs) {
		var (
			lhs_i = lhs[i]
			rhs_i = rhs[i]
		)
		// Check lengths match
		if lhs_i.Len() != rhs_i.Len() {
			return false
		}
		// // Check elements match
		for j := uint(0); j < lhs_i.Len(); j++ {
			l := lhs_i.Get(lIndices[j])
			r := rhs_i.Get(rIndices[j])
			//
			if l.Cmp(r) != 0 {
				return false
			}
		}
	}
	//
	return true
}

func indexPermutationFunc[F field.Element[F], T array.Array[F]](elems []T) func(uint, uint) int {
	return func(lhs uint, rhs uint) int {
		//
		for i := range len(elems) {
			l := elems[i].Get(lhs)
			r := elems[i].Get(rhs)
			// Compare ith elements
			c := l.Cmp(r)
			// Check whether same
			if c != 0 {
				// Positive
				return c
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
