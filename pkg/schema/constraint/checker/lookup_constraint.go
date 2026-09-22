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

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// LookupFailure provides structural information about a failing lookup constraint.
type LookupFailure[F field.Element[F]] struct {
	rowFailure[F]
	// sourceId gives the set identifier of the source
	sourceId constraint.SetId
}

// Message provides a suitable error message
func (p *LookupFailure[F]) Message() string {
	return fmt.Sprintf("lookup \"%s\" failed (row %d, shard %d)", p.handle, p.row, p.shardId)
}

func (p *LookupFailure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *LookupFailure[F]) RequiredCells() set.AnySortedSet[trace.CellRef] {
	res := set.NewAnySortedSet[trace.CellRef]()
	// Handle registers
	for i := range p.sourceId.Width() {
		var rid = p.sourceId.Ith(i)
		//
		ref := trace.NewColumnRef(p.sourceId.Module(), rid)
		res.Insert(trace.NewCellRef(ref, int(p.row)))
	}
	//
	return *res
}

func processLookupConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *lookup.Constraint[F],
) (State[F], []Failure[F]) {
	var (
		state State[F]
		// Load target sets
		targets = loadSets(cp.context, c.Targets...)
		// Initialise read buffer
		buffer = make([]F, c.Sources[0].Len())
		//
		failures []Failure[F]
	)
	// Subset check
	for _, source := range c.Sources {
		var (
			srcId = constraint.NewSetId(source.Module, source.Selector, source.Registers)
			// Determine first failing row (if any)
			witness = inclusionCheck(cp.shard.Module(source.Module), srcId, targets, buffer)
		)
		//
		if witness.HasValue() {
			failures = append(failures, &LookupFailure[F]{newRowFailure(c.Handle, witness.Unwrap(), cp), srcId})
		}
	}
	//
	return state, failures
}

// inclusionCheck checks whether all (selected) rows of the given source set are
// contained within any of the given target sets and, if not, returns a witness
// to this fact (i.e. a row not found in any of the target sets).
func inclusionCheck[F field.Element[F]](mod trace.Module[F], src SetId, sets []collection.Set[[]F],
	buffer []F) util.Option[uint] {
	if src.HasSelector() {
		var selector = src.Selector().Unwrap()
		//
		for row := range mod.Height() {
			if !mod.Column(selector).Get(row).IsZero() && !contains(row, src, mod, sets, buffer) {
				return util.Some(row)
			}
		}
	} else {
		// Optimised path when no selector
		for row := range mod.Height() {
			if !contains(row, src, mod, sets, buffer) {
				return util.Some(row)
			}
		}
	}
	//
	return util.None[uint]()
}

// check whether the given source row is contained within any of the given sets.
// A temporary buffer of sufficient width is provided to avoid memory
// allocation.
func contains[F field.Element[F]](row uint, src SetId, mod trace.Module[F], sets []collection.Set[[]F],
	buffer []F) bool {
	// Read registers into buffer
	for i := range src.Width() {
		var rid = src.Ith(i).Unwrap()
		// Read given row
		buffer[i] = mod.Column(rid).Get(row)
	}
	// Check for containment
	for _, set := range sets {
		if set.Contains(buffer) {
			return true
		}
	}
	//
	return false
}

// Load those sets from the context corresponding to the given vectors.
func loadSets[F field.Element[F]](ctx Context[F], vecs ...lookup.Vector) []collection.Set[[]F] {
	var (
		sets = make([]collection.Set[[]F], len(vecs))
	)
	// Load target sets
	for i, v := range vecs {
		var setId = constraint.NewSetId(v.Module, v.Selector, v.Registers)
		//
		sets[i] = ctx.Get(setId)
	}
	//
	return sets
}
