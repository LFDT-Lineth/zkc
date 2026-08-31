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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// ComputedRegister describes a column whose values are computed on-demand, rather
// than being stored in a data array.  Typically computed columns read values
// from other columns in a trace in order to calculate their value.  There is an
// expectation that this computation is acyclic.  Furthermore, computed columns
// give rise to "trace expansion".  That is where the initial trace provided by
// the user is expanded by determining the value of all computed columns.
type ComputedRegister[F field.Element[F]] struct {
	// Module in which expression is evaluated
	Module sc.ModuleId
	// Target index for computed column
	Target register.Id
	// The computation which accepts a given trace and computes
	// the value of this column at a given row.
	Expr term.Computation[F]
}

// NewComputedRegister constructs a new set of computed column(s) with a given
// determining expression.  More specifically, that expression is used to
// compute the values for the columns during trace expansion.  For each, the
// resulting value is split across the target columns.
func NewComputedRegister[F field.Element[F]](target register.Id, expr term.Computation[F],
	module sc.ModuleId) *ComputedRegister[F] {
	//
	return &ComputedRegister[F]{module, target, expr}
}

// Bounds determines the well-definedness bounds for this assignment for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
func (p *ComputedRegister[F]) Bounds(mid sc.ModuleId) util.Bounds {
	if mid == p.Module {
		return p.Expr.Bounds()
	}
	// Not relevant
	return util.EMPTY_BOUND
}

// Compute the values of columns defined by this assignment. Specifically, this
// creates a new column which contains the result of evaluating a given
// expression on each row.
func (p *ComputedRegister[F]) Compute(tr trace.Shard[F], schema sc.AnySchema[F],
) ([]array.MutArray[F], error) {
	var (
		trModule = tr.Module(p.Module)
		scModule = schema.Module(p.Module)

		// Determine multiplied height
		height = trModule.Height()
		// FIXME: using a large bitwidth here ensures the underlying data is
		// represented using a full field element, rather than e.g. some smaller
		// number of bytes.  This is needed to handle reject tests which can produce
		// values outside the range of the computed register, but which we still
		// want to check are actually rejected (i.e. since they are simulating what
		// an attacker might do).
		data = array.Alloc[F](math.MaxUint, height)
		// Run computation
		err = fwdComputation(height, data, p.Expr, trModule, scModule, p.Module)
	)
	// Sanity check
	if err != nil {
		return nil, err
	}
	// Done
	return []array.MutArray[F]{data}, err
}

// Consistent performs some simple checks that the given assignment is
// consistent with its enclosing schema This provides a double check of certain
// key properties, such as that registers used for assignments are valid,
// etc.
func (p *ComputedRegister[F]) Consistent(schema sc.AnySchema[F]) []error {
	return nil
}

// RegistersExpanded identifies registers expanded by this assignment.
func (p *ComputedRegister[F]) RegistersExpanded() []register.Ref {
	return nil
}

// RegistersRead returns the set of columns that this assignment depends upon.
// That can include both input columns, as well as other computed columns.
func (p *ComputedRegister[F]) RegistersRead() []register.Ref {
	var (
		module = p.Module
		regs   = p.Expr.RequiredRegisters()
		rids   = make([]register.Ref, regs.Iter().Count())
	)
	//
	for i, iter := 0, regs.Iter(); iter.HasNext(); i++ {
		rid := register.NewId(iter.Next())
		rids[i] = register.NewRef(module, rid)
	}
	//
	return rids
}

// RegistersWritten identifies registers assigned by this assignment.
func (p *ComputedRegister[F]) RegistersWritten() []register.Ref {
	return []register.Ref{register.NewRef(p.Module, p.Target)}
}

// Lisp converts this constraint into an S-Expression.
//
//nolint:revive
func (p *ComputedRegister[F]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	var (
		module   = schema.Module(p.Module)
		target   sexp.SExp
		datatype = "𝔽"
		ith      = module.Register(p.Target)
	)
	//
	if !ith.IsNative() {
		datatype = fmt.Sprintf("u%d", ith.Width())
	}
	//
	target = sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(ith.QualifiedName(module)), sexp.NewSymbol(datatype),
	})
	//
	return sexp.NewList(
		[]sexp.SExp{sexp.NewSymbol("compute"),
			target,
			p.Expr.Lisp(false, module),
		})
}

func fwdComputation[F field.Element[F]](height uint, data array.MutArray[F], expr term.Evaluable[F],
	trMod trace.Module[F], scMod register.Map, ctx sc.ModuleId) error {
	// Forwards computation
	for i := range height {
		val, err := expr.EvalAt(i, trMod, scMod)
		// error check
		if err != nil {
			e := fmt.Sprintf("%s for %s", err.Error(), expr.Lisp(false, scMod).String(true))
			return constraint.NewInternalFailure[F](scMod.Name(), ctx, i, e)
		}
		// Write data
		data.Set(i, val)
	}
	//
	return nil
}
