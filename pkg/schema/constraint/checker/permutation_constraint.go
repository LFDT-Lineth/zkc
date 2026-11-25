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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/permutation"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// PermutationFailure provides structural information about a failing Permutation constraint.
type PermutationFailure[F field.Element[F]] struct {
	rowFailure[F]
	// Constrained vector
	vector constraint.Vector
}

// Message provides a suitable error message
func (p *PermutationFailure[F]) Message() string {
	return fmt.Sprintf("permutation \"%s\" failed (row %d, shard %d)", p.handle, p.row, p.shardId)
}

func (p *PermutationFailure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *PermutationFailure[F]) RequiredCells() set.AnySortedSet[trace.CellRef] {
	var (
		res = set.NewAnySortedSet[trace.CellRef]()
	)
	//
	for _, reg := range p.vector.Registers {
		var ref = trace.NewColumnRef(p.vector.Module, reg)
		res.Insert(trace.NewCellRef(ref, int(p.row)))
	}
	//
	return *res
}

type vecRowId struct {
	// Register vector
	vector constraint.Vector
	// Row
	row uint
}

func processPermutationConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *permutation.Constraint[F],
) (state State[F], failures []Failure[F]) {
	var (
		left  = extractVectorsRows(c.Left, cp.shard)
		right = extractVectorsRows(c.Right, cp.shard)
	)
	// Sort row vectors
	sortVectorRows(left, cp.shard)
	sortVectorRows(right, cp.shard)
	// Check that vectors match exactly
	if r := equateVectorRows(left, right, cp.shard); r.HasValue() {
		var rowId = r.Unwrap()
		// Construct failure
		failures = append(failures, &PermutationFailure[F]{
			newRowFailure(c.Handle, rowId.row, cp), rowId.vector,
		})
	}
	// Check passed
	return state, failures
}

func extractVectorsRows[F field.Element[F]](vectors []constraint.Vector, shard trace.Shard[F]) []vecRowId {
	var rows []vecRowId
	//
	for _, vec := range vectors {
		rows = extractVectorRows(vec, shard, rows)
	}
	//
	return rows
}

func extractVectorRows[F field.Element[F]](vector constraint.Vector, shard trace.Shard[F], rows []vecRowId) []vecRowId {
	var (
		module = shard.Module(vector.Module)
	)
	//
	if vector.Selector.HasValue() {
		var (
			// Extract register ID for selector
			selectorId = vector.Selector.Unwrap()
			// Extract column data for selector
			selector = module.Column(selectorId.Unwrap())
		)
		// conditional
		for row := range module.Height() {
			if !selector.Get(row).IsZero() {
				rows = append(rows, vecRowId{vector, row})
			}
		}
	} else {
		// unconditional
		for row := range module.Height() {
			rows = append(rows, vecRowId{vector, row})
		}
	}
	//
	return rows
}

func sortVectorRows[F field.Element[F]](rows []vecRowId, shard trace.Shard[F]) {
	slices.SortFunc(rows, func(l, r vecRowId) int {
		return cmpVectorRow(l, r, shard)
	})
}

func equateVectorRows[F field.Element[F]](lhs, rhs []vecRowId, shard trace.Shard[F]) util.Option[vecRowId] {
	var (
		n = min(len(lhs), len(rhs))
	)
	// Check all rows accounted for
	for i := range n {
		if c := cmpVectorRow(lhs[i], rhs[i], shard); c < 0 {
			return util.Some(lhs[i])
		} else if c > 0 {
			return util.Some(rhs[i])
		}
	}
	// Check overhang
	if n < len(lhs) {
		return util.Some(lhs[n])
	} else if n < len(rhs) {
		return util.Some(rhs[n])
	}
	// Matched!
	return util.None[vecRowId]()
}

func cmpVectorRow[F field.Element[F]](lhs, rhs vecRowId, shard trace.Shard[F]) int {
	var (
		lhsRegs   = lhs.vector.Registers
		rhsRegs   = rhs.vector.Registers
		lhsModule = shard.Module(lhs.vector.Module)
		rhsModule = shard.Module(rhs.vector.Module)
		n         = uint(len(lhsRegs))
	)
	//
	util.Assert(len(lhsRegs) == len(rhsRegs), "incompatible register vectors")
	//
	for i := range n {
		var (
			lhsCol = lhsModule.Column(lhsRegs[i].Unwrap())
			rhsCol = rhsModule.Column(rhsRegs[i].Unwrap())
			lhsVal = lhsCol.Get(lhs.row)
			rhsVal = rhsCol.Get(rhs.row)
		)
		//
		if c := lhsVal.Cmp(rhsVal); c != 0 {
			return c
		}
	}
	//
	return 0
}
