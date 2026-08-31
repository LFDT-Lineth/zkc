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
package schema

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint represents an element which can "accept" a trace, or either reject
// with an error (or eventually perhaps report a warning).
type Constraint[F any] interface {
	// Accepts determines whether a given (local) constraint accepts a given set
	// of traces or not.  If not, a failure is produced.  Observe that, for
	// global constraints, this is a no-op.
	Accepts(trace.Trace[F], AnySchema[F], Context[F]) []Failure[F]
	// Determine the well-definedness bounds for this constraint in both the
	// negative (left) or positive (right) directions.  For example, consider an
	// expression such as "(shift X -1)".  This is technically undefined for the
	// first row of any trace and, by association, any constraint evaluating
	// this expression on that first row is also undefined (and hence must pass)
	Bounds(module uint) util.Bounds
	// Consistent applies a number of internal consistency checks.  Whilst not
	// strictly necessary, these can highlight otherwise hidden problems as an aid
	// to debugging.
	Consistent(AnySchema[F]) []error
	// Contexts returns the evaluation contexts (i.e. enclosing module + length
	// multiplier) for this constraint.  Most constraints have only a single
	// evaluation context, though some (e.g. lookups) have more.  Note that all
	// constraints have at least one context (which we can call the "primary"
	// context).
	Contexts() []ModuleId
	// Sets returns the list of set identifiers needed for checking this
	// constraint (if any).
	Sets() []SetId
	// Name returns a unique name for a given constraint.  This is useful purely
	// for identifying constraints in reports, etc.
	Name() string
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(AnySchema[F]) sexp.SExp
}

// Context provides a single reference point for reusing contextual information
// whilst checking constraints.  For example, it provides cached access to data
// for lookups to prevent the need to recompute this for individual lookups.
type Context[F any] interface {
	// Get returns a given module viewed in a given shard as a set from the
	// perspective of a given set of columns, with an optional selector.
	Get(shard uint, id SetId) collection.Set[[]F]
}

// SetId provides a generic mechanism for referring to a particular set of data
// required for constraint checking.
type SetId struct {
	mid module.Id
	sel util.Option[register.Id]
	row []register.Id
}

// NewSetId constructs a new set id.
func NewSetId(mid module.Id, selector util.Option[register.Id], row []register.Id) SetId {
	return SetId{mid, selector, row}
}

// Cmp implementation for set.Comparable inteface.
func (p SetId) Cmp(o SetId) int {
	if c := cmp.Compare(p.mid, o.mid); c != 0 {
		return c
	} else if c := array.Compare(p.row, o.row); c != 0 {
		return c
	}
	//
	return compareSelectors(p.sel, o.sel)
}

// Module returns the module to which this identifier refers.
func (p SetId) Module() module.Id {
	return p.mid
}

// HasSelector determines whether or not this identifier has a selector line.
func (p SetId) HasSelector() bool {
	return p.sel.HasValue()
}

// Selector returns the selector line for this identifier, or panics if it has
// none.
func (p SetId) Selector() register.Id {
	return p.sel.Unwrap()
}

// Width returns the number of data lines for this identifier.
func (p SetId) Width() uint {
	return uint(len(p.row))
}

// Ith returns the ith data line for this identifier.
func (p SetId) Ith(index uint) register.Id {
	return p.row[index]
}

// String returns a (unique) string representation for this set.
func (p SetId) String() string {
	var builder strings.Builder
	//
	if p.HasSelector() {
		fmt.Fprintf(&builder, "%x:%x", p.mid, p.sel.Unwrap())
	} else {
		fmt.Fprintf(&builder, "%x", p.mid)
	}
	//
	for _, r := range p.row {
		fmt.Fprintf(&builder, ";%x", r.Unwrap())
	}
	//
	return builder.String()
}

func compareSelectors(l, r util.Option[register.Id]) int {
	switch {
	case l.IsEmpty() && r.IsEmpty():
		return 0
	case l.IsEmpty():
		return -1
	case r.IsEmpty():
		return 1
	default:
		return l.Unwrap().Cmp(r.Unwrap())
	}
}
