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
package checker

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// BusFailure records a message sent and received a differing number of times.
type BusFailure[F field.Element[F]] struct {
	// Handle (i.e. bus name) of the failing constraint
	handle string
	// unbalanced is the offending message
	unbalanced []F
	// tally is the overall send/receive tally
	tally int
}

// Handle implementation of schema.Failure interface
func (p *BusFailure[F]) Handle() string {
	return p.handle
}

// Message provides a suitable error message
func (p *BusFailure[F]) Message() string {
	var builder strings.Builder
	//
	for i, ith := range p.unbalanced {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(ith.String())
	}
	//
	return fmt.Sprintf("bus \"%s\" unbalanced: message (%s) send / receive tally %d",
		p.Handle(), builder.String(), p.tally)
}

func (p *BusFailure[F]) String() string {
	return p.Message()
}

// Trace implementation of schema.Failure interface.  A bus imbalance spans
// every shard of the trace, hence there is no single shard to return.
func (p *BusFailure[F]) Trace() trace.Shard[F] {
	return trace.Shard[F]{}
}

// RequiredCells implementation of schema.Failure interface.  A bus imbalance
// spans every shard of the trace, so there is no sensible set of cells to
// report and this is always empty.
func (p *BusFailure[F]) RequiredCells() set.AnySortedSet[trace.CellRef] {
	return nil
}

func finaliseBusConstraint[F field.Element[F]](c *bus.Constraint[F], state State[F]) (failures []Failure[F]) {
	var (
		tally = state.data
	)
	// The bus balances exactly when every net count is zero.  Iteration
	// order — hence which unbalanced message is reported — is unspecified.
	for iter := tally.KeyValues(); iter.HasNext(); {
		var pair = iter.Next()
		//
		if pair.Right != 0 {
			//
			failures = append(failures, &BusFailure[F]{
				handle:     c.Handle,
				unbalanced: pair.Left.Elements(),
				tally:      pair.Right,
			})
		}
	}
	//
	return failures
}

func processBusConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *bus.Constraint[F],
) (state State[F], failures []Failure[F]) {
	var (
		tally = hash.NewMap[hash.Array[F], int](32)
	)
	// Apply positive messages with positive tally
	accumulate(cp.shard, c.Sends, tally, 1)
	// Apply received messages with negative tally
	accumulate(cp.shard, c.Receives, tally, -1)
	//
	return State[F]{tally}, nil
}

func accumulate[F field.Element[F]](tr trace.Shard[F], vecs []constraint.Vector, tally *constraint.Tally[F], sign int) {
	// add is the tally update applied to each selected row.
	var add = func(count int) int { return count + sign }
	//
	for _, vector := range vecs {
		var (
			id       = constraint.NewSetId(vector.Module, vector.Selector, vector.Registers)
			trModule = tr.Module(vector.Module)
			buffer   []F
		)
		//
		for row := range trModule.Height() {
			if !isRowSelected(row, id.Selector(), trModule) {
				continue
			}
			// A new key takes ownership of the buffer; repeated keys reuse it.
			if buffer == nil {
				buffer = make([]F, id.Width())
			}

			readRegisters(row, id, trModule, buffer)

			if !tally.Update(hash.NewArray(buffer), add, sign) {
				buffer = nil
			}
		}
	}
}
