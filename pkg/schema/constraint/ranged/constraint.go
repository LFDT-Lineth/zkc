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
package ranged

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint restricts all values for a given register to be within a range
// [0..n) for some bound n.  Any bound is supported, and the system will choose
// the best underlying implementation as needed.
//
// NOTE: as for a lookup, the constrained sources are registers (rather than
// arbitrary expressions).  Thus, anything else must be expanded into a register
// beforehand.
type Constraint[F field.Element[F]] struct {
	// A unique identifier for this constraint.  This is primarily useful for
	// debugging.
	Handle string
	// Evaluation Context for this constraint which must match that of the
	// constrained registers themselves.
	Context schema.ModuleId
	// The registers whose values are being constrained to be within the given
	// bound(s).
	Source register.Id
	// The number of bits permitted for all values of the corresponding register.
	// For example, with a bitwidth of 8, the maximum permitted value is 255.
	Bitwidth uint
}

// NewConstraint constructs a new Range constraint
func NewConstraint[F field.Element[F]](handle string, context schema.ModuleId,
	register register.Id, bitwidth uint) *Constraint[F] {
	return &Constraint[F]{handle, context, register, bitwidth}
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p *Constraint[F]) Consistent(schema schema.Schema[F]) []error {
	return nil
}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p *Constraint[F]) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p *Constraint[F]) Contexts() []schema.ModuleId {
	return []schema.ModuleId{p.Context}
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  Since a range constraint
// is made up of registers (rather than arbitrary expressions), it is always
// well defined on every row.
//
//nolint:revive
func (p *Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Lisp converts this schema element into a simple S-Expression, for example so
// it can be printed.
//
//nolint:revive
func (p *Constraint[F]) Lisp(mapping schema.Schema[F]) sexp.SExp {
	var (
		module = mapping.Module(p.Context)
	)
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("range"),
		sexp.NewSymbol(module.Register(p.Source).Name()),
		sexp.NewSymbol(fmt.Sprintf("u%d", p.Bitwidth)),
	})
}
