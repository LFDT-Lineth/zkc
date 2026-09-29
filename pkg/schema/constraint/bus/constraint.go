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
package bus

import (
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Tally maps each message to its net count (sends minus receives).  A bus
// balances exactly when every net count is zero.  Net counts halve the
// memory of separate send / receive multisets; the per-side counts are
// reconstructed by rescanning, on failure only.
type Tally[F field.Element[F]] = hash.Map[hash.Array[F], int]

// Constraint (a "bus") requires that the multiset of messages sent on the bus
// equals the multiset received on it.
type Constraint[F field.Element[F]] struct {
	// Handle is the bus name
	Handle string
	// Sends are the ports through which messages are sent
	Sends []Port
	// Receives are the ports through which messages are received
	Receives []Port
}

// NewConstraint creates a bus constraint, requiring all ports share one width.
func NewConstraint[F field.Element[F]](handle string, sends []Port, receives []Port) *Constraint[F] {
	var width uint
	// Take the width from whichever side has ports, rather than from the sends
	// alone.  A bus missing one direction entirely is a user error reported by
	// Consistent, so it must not panic here.
	for i, ith := range slices.Concat(sends, receives) {
		if i == 0 {
			width = ith.Len()
		} else if ith.Len() != width {
			panic(fmt.Sprintf("inconsistent port widths on bus %q (%d vs %d)", handle, width, ith.Len()))
		}
	}

	return &Constraint[F]{Handle: handle,
		Sends:    sends,
		Receives: receives,
	}
}

// Consistent applies a number of internal consistency checks.
func (p *Constraint[F]) Consistent(sc schema.Schema[F]) []error {
	var (
		errors []error
		width  uint
	)
	//
	if len(p.Sends) == 0 {
		errors = append(errors, fmt.Errorf("bus %q has receives but no sends", p.Handle))
	}
	//
	if len(p.Receives) == 0 {
		errors = append(errors, fmt.Errorf("bus %q has sends but no receives", p.Handle))
	}
	//
	for i, port := range slices.Concat(p.Sends, p.Receives) {
		var mod = sc.Module(port.Module)
		//
		if port.Len() == 0 {
			errors = append(errors, fmt.Errorf("bus %q has an empty port", p.Handle))
		} else if i == 0 {
			width = port.Len()
		} else if port.Len() != width {
			errors = append(errors,
				fmt.Errorf("bus %q has ports of differing widths (%d vs %d)", p.Handle, width, port.Len()))
		}
		//
		if mod.Register(port.Selector).Width() != 1 {
			errors = append(errors, fmt.Errorf("bus %q has a non-binary selector", p.Handle))
		}
	}
	//
	return errors
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
