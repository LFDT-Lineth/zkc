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
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint represents an element which can "accept" a trace, or either reject
// with an error (or eventually perhaps report a warning).
type Constraint[F any] interface {
	// Accepts determines whether a given constraint accepts a given trace or
	// not.  If not, a failure is produced.  Otherwise, a bitset indicating
	// branch coverage is returned.
	Accepts(trace.Trace[F], AnySchema[F]) (bit.Set, Failure)
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
	// Name returns a unique name for a given constraint.  This is useful purely
	// for identifying constraints in reports, etc.
	Name() string
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(AnySchema[F]) sexp.SExp
	// Substitute any matchined labelled constants within this constraint
	Substitute(map[string]F)
}
