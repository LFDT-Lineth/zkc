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

	"github.com/consensys/go-corset/pkg/ir/air"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/source/sexp"
)

// PseudoIz represents a computation which produces the "is zero" indicator
// of a given expression: 1 when the expression evaluates to zero, 0
// otherwise.  It is the sibling of PseudoInverse and exists so that the
// MIR→AIR lowering can CSE the  1 - e*inv(e)  subtree as a single shared
// computed column referenced from every NotEqual constraint over the same
// e — see [pkg/ir/air/gadgets/normalisation.go].
type PseudoIz[F field.Element[F]] struct {
	// Target index for the computed column.
	Target register.Ref
	// Expression whose "is zero" indicator this column holds.
	Expr air.Term[F]
}

// NewPseudoIz constructs a new "is zero" indicator assignment for the given
// target register and expression.
func NewPseudoIz[F field.Element[F]](target register.Ref, expr air.Term[F]) *PseudoIz[F] {
	return &PseudoIz[F]{Target: target, Expr: expr}
}

// Bounds determines the well-definedness bounds for this assignment.  It is
// the same as that of the expression whose value is being checked.
func (e *PseudoIz[F]) Bounds(mid schema.ModuleId) util.Bounds {
	if mid == e.Target.Module() {
		return e.Expr.Bounds()
	}
	//
	return util.EMPTY_BOUND
}

// Compute fills the target column with 1 where the expression evaluates to
// zero and 0 elsewhere.
func (e *PseudoIz[F]) Compute(tr trace.Trace[F], schema schema.AnySchema[F]) ([]array.MutArray[F], error) {
	var (
		trModule = tr.Module(e.Target.Module())
		scModule = schema.Module(e.Target.Module())
		height   = trModule.Height()
		// 1-bit storage: each cell is 0 or 1.
		data = tr.Builder().NewArray(height, 1)
		one  F
	)
	//
	one = one.SetUint64(1)
	//
	for i := range height {
		val, err := e.Expr.EvalAt(int(i), trModule, scModule)
		if err != nil {
			return nil, err
		}
		//
		if val.IsZero() {
			data = data.Set(i, one)
		}
	}
	//
	return []array.MutArray[F]{data}, nil
}

// Consistent performs some simple checks that the given assignment is
// consistent with its enclosing schema.
func (e *PseudoIz[F]) Consistent(schema.AnySchema[F]) []error {
	return nil
}

// RegistersExpanded identifies registers expanded by this assignment.
func (e *PseudoIz[F]) RegistersExpanded() []register.Ref {
	return nil
}

// RegistersRead returns the set of columns that this assignment depends upon.
func (e *PseudoIz[F]) RegistersRead() []register.Ref {
	var (
		module = e.Target.Module()
		regs   = e.Expr.RequiredRegisters()
		rids   = make([]register.Ref, regs.Iter().Count())
	)
	//
	for i, iter := 0, regs.Iter(); iter.HasNext(); i++ {
		rid := register.NewId(iter.Next())
		rids[i] = register.NewRef(module, rid)
	}
	// Allow recursive definitions: never read the target itself.
	return array.RemoveMatching(rids, func(r register.Ref) bool {
		return r == e.Target
	})
}

// RegistersWritten identifies registers assigned by this assignment.
func (e *PseudoIz[F]) RegistersWritten() []register.Ref {
	return []register.Ref{e.Target}
}

// Lisp converts this assignment into an S-Expression.
//
//nolint:revive
func (e *PseudoIz[F]) Lisp(schema schema.AnySchema[F]) sexp.SExp {
	var (
		module   = schema.Module(e.Target.Module())
		target   = module.Register(e.Target.Register())
		datatype = "𝔽"
	)
	//
	if !target.IsNative() {
		datatype = fmt.Sprintf("u%d", target.Width())
	}
	//
	return sexp.NewList(
		[]sexp.SExp{sexp.NewSymbol("iz"),
			sexp.NewList([]sexp.SExp{
				sexp.NewSymbol(target.QualifiedName(module)),
				sexp.NewSymbol(datatype),
			}),
			e.Expr.Lisp(false, module),
		})
}

// RequiredRegisters returns the set of registers on which this term depends.
func (e *PseudoIz[F]) RequiredRegisters() *set.SortedSet[uint] {
	return e.Expr.RequiredRegisters()
}

// RequiredCells returns the set of trace cells on which this term depends.
func (e *PseudoIz[F]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	return e.Expr.RequiredCells(row, mid)
}

// Substitute implementation for Substitutable interface.
func (e *PseudoIz[F]) Substitute(map[string]F) {
	panic("unreachable")
}
