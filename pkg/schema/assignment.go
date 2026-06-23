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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Assignment represents an arbitrary computation which determines the values
// for one (or more) computed registers.  For any computed register, there
// should only ever be one assignment.  Likewise, every computed register should
// have an associated assignment.  A good example of an assignment is computed
// the multiplicative inverse of a column in order to implement a non-zero
// check.
type Assignment[F any] interface {
	// For the given module, determine any well-definedness bounds implied by
	// this assignment in  both the negative (left) or positive (right)
	// directions.  For example, consider an expression such as "(shift X -1)".
	// This is technically undefined for the first row of any trace and, by
	// association, any assignment evaluating this expression on that first row
	// is also undefined.
	Bounds(ModuleId) util.Bounds
	// ComputeColumns computes the values of columns defined by this assignment.
	// In order for this computation to makes sense, all columns on which this
	// assignment depends must exist (e.g. are either inputs or have been
	// computed already).  Computed columns do not exist in the original trace,
	// but are added during trace expansion to form the final trace.
	Compute(tr.Trace[F], AnySchema[F]) ([]array.MutArray[F], error)
	// Consistent applies a number of internal consistency checks.  Whilst not
	// strictly necessary, these can highlight otherwise hidden problems as an aid
	// to debugging.
	Consistent(AnySchema[F]) []error
	// Identifier registers which are expanded by this assignment.  A register
	// is expanded when its length maybe changed.  For example, when going from
	// a trace which contains only rows of input/output values to a trace where
	// each function instance can occupy more than one row.  Then the I/O
	// columns are said to be "expanded".
	RegistersExpanded() []register.Ref
	// Returns the set of columns that this assignment depends upon.  That can
	// include both input columns, as well as other computed columns.
	RegistersRead() []register.Ref
	// Identifier registers assigned by this assignment.
	RegistersWritten() []register.Ref
	// Substitute any matchined labelled constants within this assignment
	Substitute(map[string]F)
	// Lisp converts this schema element into a simple S-Expression, for example
	// so it can be printed.
	Lisp(AnySchema[F]) sexp.SExp
}
