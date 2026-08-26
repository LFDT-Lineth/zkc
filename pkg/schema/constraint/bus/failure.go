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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Failure records a message sent and received a differing number of times.
type Failure[F field.Element[F]] struct {
	// Handle (i.e. bus name) of the failing constraint
	Handle string
	// Unbalanced is the offending message
	Unbalanced []F
	// Sent is the number of times the message was sent
	Sent uint
	// Received is the number of times the message was received
	Received uint
	// Sends are the send ports of the failing constraint
	Sends []Port
	// Receives are the receive ports of the failing constraint
	Receives []Port
}

// Message provides a suitable error message
func (p *Failure[F]) Message() string {
	var builder strings.Builder
	//
	for i, ith := range p.Unbalanced {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(ith.String())
	}
	//
	return fmt.Sprintf("bus \"%s\" unbalanced: message (%s) sent %d time(s), received %d time(s)",
		p.Handle, builder.String(), p.Sent, p.Received)
}

func (p *Failure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells contributing the offending message
// (selectors included).  Rescanning the trace is fine here, as this only ever
// runs on a failure.
func (p *Failure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	res := set.NewAnySortedSet[trace.CellRef]()
	//
	for _, port := range p.Sends {
		p.requiredCellsOfPort(tr, port, res)
	}
	//
	for _, port := range p.Receives {
		p.requiredCellsOfPort(tr, port, res)
	}
	//
	return res
}

// requiredCellsOfPort adds the cells of the port's selected rows holding the
// offending message.
func (p *Failure[F]) requiredCellsOfPort(tr trace.Trace[F], port Port, res *set.AnySortedSet[trace.CellRef]) {
	var trModule = tr.Module(port.Module)
	//
	for row := range trModule.Height() {
		if trModule.Column(port.Selector.Unwrap()).Get(row).IsZero() {
			continue
		}
		//
		var matches = true
		//
		for i, rid := range port.Registers {
			if !trModule.Column(rid.Unwrap()).Get(row).Equals(p.Unbalanced[i]) {
				matches = false
				break
			}
		}
		//
		if matches {
			selRef := trace.NewColumnRef(port.Module, port.Selector)
			res.Insert(trace.NewCellRef(selRef, int(row)))
			//
			for _, rid := range port.Registers {
				ref := trace.NewColumnRef(port.Module, rid)
				res.Insert(trace.NewCellRef(ref, int(row)))
			}
		}
	}
}
