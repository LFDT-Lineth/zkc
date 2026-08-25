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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Multiset maps each message to its number of occurrences.
type Multiset[F field.Element[F]] = hash.Map[hash.Array[F], uint]

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
	var (
		sends    = hash.NewMap[hash.Array[F], uint](32)
		receives = hash.NewMap[hash.Array[F], uint](32)
	)
	//
	for _, tr := range traces {
		p.accumulate(tr, p.Sends, sends)
		p.accumulate(tr, p.Receives, receives)
	}
	//
	if failure := p.compareMultisets(sends, receives); failure != nil {
		return failure
	}
	// Catch messages received but never sent.
	for iter := receives.KeyValues(); iter.HasNext(); {
		var pair = iter.Next()
		//
		if _, ok := sends.Get(pair.Left); !ok {
			return &Failure[F]{p.Handle, pair.Left.Elements(), 0, pair.Right, p.Sends, p.Receives}
		}
	}
	//
	return nil
}

// Multisets computes one trace's send and receive multisets, allowing a
// checking harness to combine shards itself.
func (p Constraint[F]) Multisets(tr trace.Trace[F]) (sends *Multiset[F], receives *Multiset[F]) {
	sends = hash.NewMap[hash.Array[F], uint](32)
	receives = hash.NewMap[hash.Array[F], uint](32)
	//
	p.accumulate(tr, p.Sends, sends)
	p.accumulate(tr, p.Receives, receives)
	//
	return sends, receives
}

// accumulate adds one message per selected row of each port.
func (p Constraint[F]) accumulate(tr trace.Trace[F], ports []Port, multiset *Multiset[F]) {
	for _, port := range ports {
		var trModule = tr.Module(port.Module)
		//
		for row := range trModule.Height() {
			if trModule.Column(port.Selector.Unwrap()).Get(row).IsZero() {
				continue
			}
			//
			var message = make([]F, port.Len())
			//
			for i, rid := range port.Registers {
				message[i] = trModule.Column(rid.Unwrap()).Get(row)
			}
			//
			var (
				key      = hash.NewArray(message)
				count, _ = multiset.Get(key)
			)
			//
			multiset.Insert(key, count+1)
		}
	}
}

// compareMultisets returns a failure for the first message whose send and
// receive counts differ.  Iteration order — hence which message is reported —
// is unspecified.
func (p Constraint[F]) compareMultisets(sends *Multiset[F], receives *Multiset[F]) schema.Failure {
	for iter := sends.KeyValues(); iter.HasNext(); {
		var (
			pair        = iter.Next()
			received, _ = receives.Get(pair.Left)
		)
		//
		if pair.Right != received {
			return &Failure[F]{p.Handle, pair.Left.Elements(), pair.Right, received, p.Sends, p.Receives}
		}
	}
	//
	return nil
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
