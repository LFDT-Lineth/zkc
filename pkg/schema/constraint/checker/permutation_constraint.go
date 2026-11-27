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
	"iter"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
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

func processPermutationConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *permutation.Constraint[F],
) (state State[F], failures []Failure[F]) {
	return hashPermutationCheck(cp, c)
}

// hashPermutationCheck checks a permutation by counting every tuple on the
// left-hand side and discounting every tuple on the right-hand side.  The
// permutation holds exactly when every count ends at zero.
func hashPermutationCheck[F field.Element[F]](cp ConstraintProcessor[F], c *permutation.Constraint[F],
) (state State[F], failures []Failure[F]) {
	var (
		res   vecRow
		tally = hash.NewMap[hash.Array[F], int](32)
	)
	// Apply left-hand side with positive tally
	accumulate(cp.shard, cp.parent.schema, c.Left, tally, 1)
	// Apply right-hand side with negative tally
	accumulate(cp.shard, cp.parent.schema, c.Right, tally, -1)
	// Only look for a witness when some tuple is unbalanced.
	if isBalanced(tally) {
		return state, nil
	}
	// Report the first row holding an unbalanced tuple.
	if row := findUnbalancedRow(c.Left, cp.shard, cp.parent.schema, tally, true); row.HasValue() {
		res = row.Unwrap()
	} else if row := findUnbalancedRow(c.Right, cp.shard, cp.parent.schema, tally, false); row.HasValue() {
		res = row.Unwrap()
	} else {
		panic("unreachable")
	}
	//
	return state, []Failure[F]{&PermutationFailure[F]{
		newRowFailure(c.Handle, res.row, cp), res.id,
	}}
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

type vecRow struct {
	id  VecId
	row uint
}

type dataRow[F field.Element[F]] struct {
	row  uint
	data []F
}

// findUnbalancedRow returns the first selected row of the given vectors whose tuple
// has a non-zero count in the given tally (if any).  Unlike iterating the tally
// itself, this gives the same witness on every run.
func findUnbalancedRow[F field.Element[F]](vectors []VecId, shard trace.Shard[F], schema schema.Schema[F],
	tally *constraint.Tally[F], sign bool) util.Option[vecRow] {
	//
	for _, vector := range vectors {
		var module = schema.Module(vector.Module)
		//
		if module.IsStatic() {
			for r := range iterateStaticRows(vector, schema) {
				if isUnbalancedRow(r.data, tally, sign) {
					return util.Some(vecRow{vector, r.row})
				}
			}
		} else {
			for r := range iterateTraceRows(vector, shard) {
				if isUnbalancedRow(r.data, tally, sign) {
					return util.Some(vecRow{vector, r.row})
				}
			}
		}
	}
	//
	return util.None[vecRow]()
}

func isUnbalancedRow[F field.Element[F]](data []F, tally *constraint.Tally[F], sign bool) bool {
	var count int
	//
	if count, _ = tally.Get(hash.NewArray(data)); sign {
		return count > 0
	}
	//
	return count < 0
}

func iterateTraceRows[F field.Element[F]](vector VecId, shard trace.Shard[F]) iter.Seq[dataRow[F]] {
	var (
		module = shard.Module(vector.Module)
		buffer = make([]F, vector.Len())
	)
	//
	return func(yield func(dataRow[F]) bool) {
		for row := range module.Height() {
			if isTraceRowSelected(row, vector.Selector, module) {
				// Read registers from trace
				readTraceRegisters(row, vector, module, buffer)
				// Yield row data
				if !yield(dataRow[F]{row, buffer}) {
					return
				}
			}
		}
	}
}

func iterateStaticRows[F field.Element[F]](vector VecId, schema schema.Schema[F]) iter.Seq[dataRow[F]] {
	var (
		module   = schema.Module(vector.Module)
		contents = module.StaticContents()
		buffer   = make([]F, vector.Len())
	)
	//
	return func(yield func(dataRow[F]) bool) {
		for row := range uint(len(contents)) {
			if isStaticRowSelected(vector.Selector, contents[row]) {
				// Read registers from schema
				readStaticRegisters(vector, contents[row], buffer)
				// Yield row data
				if !yield(dataRow[F]{row, buffer}) {
					return
				}
			}
		}
	}
}
