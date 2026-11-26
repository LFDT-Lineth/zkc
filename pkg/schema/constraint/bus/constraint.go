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
package bus

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint (a "bus") requires that the multiset of messages sent on the bus
// equals the multiset received on it across all shards. Each vector contributes
// one message per selected row, or per row (including padding) if unfiltered.
type Constraint[F field.Element[F]] struct {
	// Handle is the bus name
	Handle string
	// Sends are the ports through which messages are sent
	Sends []constraint.Vector
	// Receives are the ports through which messages are received
	Receives []constraint.Vector
}

// NewConstraint creates a bus constraint, requiring all ports share one width.
func NewConstraint[F field.Element[F]](handle string, sends, receives []constraint.Vector) *Constraint[F] {
	c := &Constraint[F]{Handle: handle, Sends: sends, Receives: receives}
	c.checkWidths()

	return c
}

// Validate applies a number of internal consistency checks.
func (p *Constraint[F]) Validate(sc schema.Schema[F]) {
	p.checkWidths()

	for _, side := range [2][]constraint.Vector{p.Sends, p.Receives} {
		for _, port := range side {
			if port.HasSelector() {
				sel := sc.Module(port.Module).Register(port.Selector.Unwrap())
				util.Assert(sel.Width() == 1, "bus %q has a non-binary selector", p.Handle)
			}
		}
	}
}

func (p *Constraint[F]) checkWidths() {
	width := -1

	for _, side := range [2][]constraint.Vector{p.Sends, p.Receives} {
		for _, port := range side {
			n := len(port.Registers)
			if width >= 0 && width != n {
				panic(fmt.Sprintf("inconsistent port widths on bus %q (%d vs %d)", p.Handle, width, n))
			}

			width = n
		}
	}
}

// Name returns a unique name for this constraint.
func (p *Constraint[F]) Name() string {
	return p.Handle
}

// Contexts returns the modules of all ports.
func (p *Constraint[F]) Contexts() []schema.ModuleId {
	var contexts []schema.ModuleId
	//
	for _, send := range p.Sends {
		contexts = append(contexts, send.Module)
	}
	//
	for _, receive := range p.Receives {
		contexts = append(contexts, receive.Module)
	}
	//
	return contexts
}

// Bounds implementation for schema.Constraint interface.  Ports are made of
// registers, hence well defined on every row.
//
//nolint:revive
func (p *Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Lisp converts this constraint into an S-Expression.
//
//nolint:revive
func (p *Constraint[F]) Lisp(mapping schema.Schema[F]) sexp.SExp {
	var (
		sends    = sexp.EmptyList()
		receives = sexp.EmptyList()
	)
	//
	for _, ith := range p.Sends {
		sends.Append(ith.Lisp(mapping.Module(ith.Module)))
	}
	//
	for _, ith := range p.Receives {
		receives.Append(ith.Lisp(mapping.Module(ith.Module)))
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("bus"),
		sexp.NewSymbol(fmt.Sprintf("\"%s\"", p.Handle)),
		sends,
		receives,
	})
}
