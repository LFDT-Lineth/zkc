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
package checker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
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
	// sends are the send ports of the failing constraint
	sends []bus.Port
	// receives are the receive ports of the failing constraint
	receives []bus.Port
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
				sends:      c.Sends,
				receives:   c.Receives,
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
	accumulate(cp.shard, c.Sends, tally, 1)
	accumulate(cp.shard, c.Receives, tally, -1)
	//
	return State[F]{tally}, nil
}

func accumulate[F field.Element[F]](tr trace.Shard[F], ports []bus.Port, tally *bus.Tally[F], sign int) {
	// add is the tally update applied to each selected row.
	var add = func(count int) int { return count + sign }
	//
	for _, port := range ports {
		var trModule = tr.Module(port.Module)
		// Allocate scratch space for this port.
		var buffer = make([]F, port.Len())
		//
		for row := range trModule.Height() {
			if isRowSelected(row, port.Selector, trModule) {
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

// isSelected determines whether or not the given row of the given vector is
// selected.  A row without a selector is always selected; otherwise, it is
// selected when its selector is non-zero.
func isRowSelected[F field.Element[F]](k uint, id register.Id, trModule trace.Module[F]) bool {
	// Otherwise, selected when selector non-zero.
	return !trModule.Column(id.Unwrap()).Get(k).IsZero()
}
