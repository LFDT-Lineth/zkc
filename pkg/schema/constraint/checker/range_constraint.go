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

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// RangeFailure provides structural information about a failing type constraint.
type RangeFailure[F field.Element[F]] struct {
	rowFailure[F]
	// Enclosing context
	context schema.ModuleId
	// Constrained register
	source register.Id
	// Range restriction
	bitwidth uint
}

// Message provides a suitable error message
func (p *RangeFailure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("range \"%s\" is u%d does not hold (row %d, shard %d)", p.handle, p.bitwidth, p.row, p.shardId)
}

func (p *RangeFailure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *RangeFailure[F]) RequiredCells() set.AnySortedSet[trace.CellRef] {
	var (
		res = set.NewAnySortedSet[trace.CellRef]()
		ref = trace.NewColumnRef(p.context, p.source)
	)
	//
	res.Insert(trace.NewCellRef(ref, int(p.row)))
	//
	return *res
}

func processRangeConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *ranged.Constraint[F],
) (State[F], []Failure[F]) {
	var (
		state    State[F]
		trModule = cp.shard.Module(c.Context)
		handle   = constraint.DetermineHandle(c.Handle, c.Context, cp.shard)
		column   = trModule.Column(c.Source.Unwrap())
		// Compute 2^n
		bound    = field.TwoPowN[F](c.Bitwidth)
		failures []Failure[F]
	)
	// Iterate every row
	for k := range trModule.Height() {
		// Perform the range check
		if column.Get(k).Cmp(bound) >= 0 {
			// Evaluation failure
			failures = append(failures, &RangeFailure[F]{newRowFailure(handle, k, cp), c.Context, c.Source, c.Bitwidth})
		}
	}
	// All good
	return state, failures
}
