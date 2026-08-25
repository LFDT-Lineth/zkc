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
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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
func NewConstraint[F field.Element[F]](handle string, sends []Port, receives []Port) Constraint[F] {
	var width uint
	//
	for i, ith := range sends {
		if i != 0 && ith.Len() != width {
			panic("inconsistent number of send registers on bus")
		}

		width = ith.Len()
	}
	//
	for _, ith := range receives {
		if ith.Len() != width {
			panic("inconsistent number of receive registers on bus")
		}
	}

	return Constraint[F]{Handle: handle,
		Sends:    sends,
		Receives: receives,
	}
}

// Consistent applies a number of internal consistency checks.
func (p Constraint[F]) Consistent(sc schema.AnySchema[F]) []error {
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
func (p Constraint[F]) Name() string {
	return p.Handle
}

// Contexts returns the modules of all ports.
func (p Constraint[F]) Contexts() []schema.ModuleId {
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

// Sets implementation for schema.Constraint interface.  A bus builds its own
// multisets; the shared context only holds de-duplicating sets.
func (p Constraint[F]) Sets() []schema.SetId {
	return nil
}

// Bounds implementation for schema.Constraint interface.  Ports are made of
// registers, hence well defined on every row.
//
//nolint:revive
func (p Constraint[F]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Accepts checks whether the bus balances within a single trace.
//
//nolint:revive
func (p Constraint[F]) Accepts(tr trace.Trace[F], sc schema.AnySchema[F], ctx schema.Context[F]) schema.Failure {
	return p.AcceptsGroup(tr)
}

// AcceptsGroup checks whether the bus balances across a group of traces
// judged together.
func (p Constraint[F]) AcceptsGroup(traces ...trace.Trace[F]) schema.Failure {
	tally := hash.NewMap[hash.Array[F], int](32)
	//
	for _, tr := range traces {
		p.accumulate(tr, p.Sends, tally, 1)
		p.accumulate(tr, p.Receives, tally, -1)
	}
	// The bus balances exactly when every net count is zero.  Iteration
	// order — hence which unbalanced message is reported — is unspecified.
	for iter := tally.KeyValues(); iter.HasNext(); {
		var pair = iter.Next()
		//
		if pair.Right != 0 {
			var (
				message  = pair.Left.Elements()
				sent     = p.count(traces, p.Sends, message)
				received = p.count(traces, p.Receives, message)
			)
			//
			return &Failure[F]{p.Handle, message, sent, received, p.Sends, p.Receives}
		}
	}
	//
	return nil
}

// portColumns looks up a port's selector column and its message columns.  Doing
// this once per port keeps the lookups out of the row loops below, where they
// would otherwise be repeated on every single row.
func portColumns[F field.Element[F]](trModule trace.Module[F], port Port) (
	selectorCol array.Array[F], messageCols []array.Array[F]) {
	//
	selectorCol = trModule.Column(port.Selector.Unwrap())
	messageCols = make([]array.Array[F], port.Len())
	//
	for i, rid := range port.Registers {
		messageCols[i] = trModule.Column(rid.Unwrap())
	}
	//
	return selectorCol, messageCols
}

// accumulate adds the given sign to the tally for every selected row of each
// port.
func (p Constraint[F]) accumulate(tr trace.Trace[F], ports []Port, tally *Tally[F], sign int) {
	for _, port := range ports {
		var (
			trModule                 = tr.Module(port.Module)
			selectorCol, messageCols = portColumns(trModule, port)
		)
		//
		for row := range trModule.Height() {
			if selectorCol.Get(row).IsZero() {
				continue
			}
			//
			var message = make([]F, len(messageCols))
			//
			for i, col := range messageCols {
				message[i] = col.Get(row)
			}
			//
			var (
				key      = hash.NewArray(message)
				count, _ = tally.Get(key)
			)
			//
			tally.Insert(key, count+sign)
		}
	}
}

// count returns how many times the given message is contributed by the given
// ports across all traces.  This rescans the traces, which is fine since it
// only ever runs when reporting a failure.
func (p Constraint[F]) count(traces []trace.Trace[F], ports []Port, message []F) uint {
	var n uint
	//
	for _, tr := range traces {
		for _, port := range ports {
			var (
				trModule                 = tr.Module(port.Module)
				selectorCol, messageCols = portColumns(trModule, port)
			)
			//
			for row := range trModule.Height() {
				if selectorCol.Get(row).IsZero() {
					continue
				}
				//
				var matches = true
				//
				for i, col := range messageCols {
					if !col.Get(row).Equals(message[i]) {
						matches = false
						break
					}
				}
				//
				if matches {
					n++
				}
			}
		}
	}
	//
	return n
}

// Lisp converts this constraint into an S-Expression.
//
//nolint:revive
func (p Constraint[F]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
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
