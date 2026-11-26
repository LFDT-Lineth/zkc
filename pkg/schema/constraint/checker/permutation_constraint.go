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

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/permutation"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// PermutationFailure provides structural information about a failing Permutation constraint.
type PermutationFailure[F field.Element[F]] struct {
	rowFailure[F]
	// Constrained vector
	vector VecId
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
	return hashPermutationCheck(cp, c)
}

// hashPermutationCheck checks a permutation by counting every tuple on the
// left-hand side and discounting every tuple on the right-hand side.  The
// permutation holds exactly when every count ends at zero.
func hashPermutationCheck[F field.Element[F]](cp ConstraintProcessor[F], c *permutation.Constraint[F],
) (state State[F], failures []Failure[F]) {
	var tally = hash.NewMap[hash.Array[F], int](32)
	// Apply left-hand side with positive tally
	accumulate(cp.shard, c.Left, tally, 1)
	// Apply right-hand side with negative tally
	accumulate(cp.shard, c.Right, tally, -1)
	// Only look for a witness when some tuple is unbalanced.
	if isBalanced(tally) {
		return state, nil
	}
	// Report the first row holding an unbalanced tuple.
	for _, vectors := range [][]VecId{c.Left, c.Right} {
		if r := unbalancedRow(vectors, cp.shard, tally); r.HasValue() {
			var rowId = r.Unwrap()
			//
			return state, []Failure[F]{&PermutationFailure[F]{
				newRowFailure(c.Handle, rowId.row, cp), rowId.vector,
			}}
		}
	}
	//
	panic("unreachable")
}

// isBalanced determines whether every tuple in the given tally has a count of
// zero.
func isBalanced[F field.Element[F]](tally *constraint.Tally[F]) bool {
	for iter := tally.KeyValues(); iter.HasNext(); {
		if iter.Next().Right != 0 {
			return false
		}
	}
	//
	return true
}

// unbalancedRow returns the first selected row of the given vectors whose tuple
// has a non-zero count in the given tally (if any).  Unlike iterating the tally
// itself, this gives the same witness on every run.
func unbalancedRow[F field.Element[F]](vectors []VecId, shard trace.Shard[F], tally *constraint.Tally[F],
) util.Option[vecRowId] {
	for _, vector := range vectors {
		var (
			module = shard.Module(vector.Module)
			buffer = make([]F, vector.Len())
		)
		//
		for row := range module.Height() {
			if isRowSelected(row, vector.Selector, module) {
				for i, rid := range vector.Registers {
					buffer[i] = module.Column(rid.Unwrap()).Get(row)
				}
				// A lookup never consumes the key, hence the buffer is reused.
				if count, _ := tally.Get(hash.NewArray(buffer)); count != 0 {
					return util.Some(vecRowId{vector, row})
				}
			}
		}
	}
	//
	return util.None[vecRowId]()
}
