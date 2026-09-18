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
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

func processLookupConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *lookup.Constraint[F]) []Failure[F] {
	var (
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
			trModule = cp.shard.Module(source.Module)
			srcId    = constraint.NewSetId(source.Module, source.Selector, source.Registers)
		)
		// Check each row in the set determined by this vector.
		if err := checkSourceSet(c.Handle, srcId, cp.shardId, trModule, targets, buffer); err != nil {
			failures = append(failures, err)
		}
	}
	//
	return failures
}

// Check that all rows in a given source set are contained within at least one
// of the given target sets.
func checkSourceSet[F field.Element[F]](handle string, src SetId, shard uint, mod trace.Module[F],
	sets []collection.Set[[]F], buffer []F) Failure[F] {
	if src.HasSelector() {
		var selector = src.Selector().Unwrap()
		//
		for row := range mod.Height() {
			if !mod.Column(selector).Get(row).IsZero() {
				if !contains(row, src, mod, sets, buffer) {
					return &lookup.Failure[F]{
						LookupHandle: handle,
						SourceId:     src,
						Row:          row,
						Shard:        shard}
				}
			}
		}
	} else {
		// Optimised path when no selector
		for row := range mod.Height() {
			if !contains(row, src, mod, sets, buffer) {
				return &lookup.Failure[F]{
					LookupHandle: handle,
					SourceId:     src,
					Row:          row,
					Shard:        shard}
			}
		}
	}
	//
	return nil
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
