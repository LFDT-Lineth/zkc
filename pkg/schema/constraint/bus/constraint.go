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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
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
func NewConstraint[F field.Element[F]](handle string, sends []Port, receives []Port) Constraint[F] {
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

// Accepts checks whether the bus balances across a group of traces
// judged together.
func (p Constraint[F]) Accepts(trace trace.Trace[F], sc schema.AnySchema[F], ctx schema.Context[F],
) (failures []schema.Failure[F]) {
	tally := hash.NewMap[hash.Array[F], int](32)
	//
	for _, tr := range trace {
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
				sent     = p.count(trace, p.Sends, message)
				received = p.count(trace, p.Receives, message)
			)
			//
			failures = append(failures, &Failure[F]{p.Handle, message, sent, received, p.Sends, p.Receives})
		}
	}
	//
	return failures
}

// accumulate adds the given sign to the tally for every selected row of each
// port.
func (p Constraint[F]) accumulate(tr trace.Shard[F], ports []Port, tally *Tally[F], sign int) {
	// add is the tally update applied to each selected row.
	var add = func(count int) int { return count + sign }
	//
	for _, port := range ports {
		var trModule = tr.Module(port.Module)
		// Allocate scratch space for this port.
		var buffer = make([]F, port.Len())
		//
		for row := range trModule.Height() {
			if isSelected(row, port.Selector, trModule) {
				//
				for i, rid := range port.Registers {
					buffer[i] = trModule.Column(rid.Unwrap()).Get(row)
				}
				//
				var key = hash.NewArray(buffer)
				// Insert item whilst checking whether the buffer was consumed or not
				if !tally.Update(key, add, sign) {
					// Yes, buffer consumed.  Therefore, construct fresh buffer to avoid
					// aliasing the value now stored in the hash set.
					buffer = slices.Clone(buffer)
				}
			}
		}
	}
}

// count returns how many times the given message is contributed by the given
// ports across all traces.  This rescans the traces, which is fine since it
// only ever runs when reporting a failure.
func (p Constraint[F]) count(traces trace.Trace[F], ports []Port, message []F) uint {
	var n uint
	//
	for _, tr := range traces {
		for _, port := range ports {
			var trModule = tr.Module(port.Module)
			//
			for row := range trModule.Height() {
				if isSelected(row, port.Selector, trModule) {
					var matches = true
					//
					for i, rid := range port.Registers {
						if !trModule.Column(rid.Unwrap()).Get(row).Equals(message[i]) {
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

// isSelected determines whether or not the given row of the given vector is
// selected.  A row without a selector is always selected; otherwise, it is
// selected when its selector is non-zero.
func isSelected[F field.Element[F]](k uint, id register.Id, trModule trace.Module[F]) bool {
	// Otherwise, selected when selector non-zero.
	return !trModule.Column(id.Unwrap()).Get(k).IsZero()
}
