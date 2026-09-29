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
	"iter"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Set provides a convenient alias
type Set[F field.Element[F]] = *hash.Set[hash.Array[F]]

// SetId provides a convenient alias
type SetId = constraint.SetId

// Schema provides a convenient alias
type Schema[F field.Element[F]] = schema.Schema[F]

// Context provides a single reference point for reusing contextual information
// whilst checking constraints.  For example, it provides cached access to data
// for lookups to prevent the need to recompute this for individual lookups.
type Context[F any] interface {
	// Get returns a given module viewed in a given shard as a set from the
	// perspective of a given set of columns, with an optional selector.
	Get(id SetId) collection.Set[[]F]
}

// seqBuildContext constructs the context from a given schema and trace.
// Essentially, this means traversing the schema looking for lookups and
// constructing their sets.  NOTE: this is done sequentially
func seqBuildContext[F field.Element[F]](tr trace.Shard[F], sc Schema[F]) Context[F] {
	var (
		context = make(map[string]*hash.Set[hash.Array[F]])
		sids    = determineSets(sc)
	)
	// sequential set construction (as a single chunk per set)
	for _, sid := range sids {
		// Construct data for this set
		context[sid.String()] = buildSetChunk(sid, tr, sc)
	}
	//
	return contextImpl[F]{context}
}

// parBuildContext constructs the context from a given schema and trace, using
// a parallel map.
func parBuildContext[F field.Element[F]](tr trace.Shard[F], sc Schema[F]) Context[F] {
	var (
		context = make(map[string]*hash.Set[hash.Array[F]])
		// Determine sets to build
		sids = determineSets(sc)
		// build every chunk in parallel
		sets = array.ParallelMap(sids, func(_ uint, c SetId) Set[F] {
			return buildSetChunk(c, tr, sc)
		})
	)
	// Flattern individual sets
	for i, id := range sids {
		context[id.String()] = sets[i]
	}
	//
	return contextImpl[F]{context}
}

// DetermineSets extracts all unique set identifiers from lookup constraints.
func determineSets[F field.Element[F]](sc Schema[F]) []SetId {
	var sets set.AnySortedSet[SetId]
	//
	for iter := sc.Constraints(); iter.HasNext(); {
		var ith = iter.Next()
		//
		for _, sid := range targetSetsOf(ith) {
			sets.Insert(sid)
		}
	}
	//
	return sets.ToArray()
}

// Determine whether given constraint encodes any sets
func targetSetsOf[F field.Element[F]](c Constraint[F]) (sets []SetId) {
	if c, ok := c.(*lookup.Constraint[F]); ok {
		for _, v := range c.Targets {
			sets = append(sets, constraint.NewSetId(v.Module, v.Selector, v.Registers))
		}
	}
	//
	return sets
}

// buildSetChunk constructs the (partial) set of rows determined by a given
// chunk.
func buildSetChunk[F field.Element[F]](id SetId, tr trace.Shard[F], sc Schema[F]) Set[F] {
	var (
		scModule = sc.Module(id.Module())
		trModule = tr.Module(id.Module())
	)
	//
	if scModule.IsStatic() {
		return buildStaticSetChunk(id, scModule)
	}
	//
	return buildDynamicSetChunk(id, trModule)
}

func buildStaticSetChunk[F field.Element[F]](id SetId, sm schema.Module[F]) Set[F] {
	var (
		buffer   = make([]F, id.Width())
		contents = sm.StaticContents()
		data     = hash.NewSet[hash.Array[F]](uint(len(contents)))
	)
	// Insert all selected rows within this chunk
	for _, row := range contents {
		if isStaticSelected(id, row) {
			// Read each register of this vector
			readStaticRegisters(id, row, buffer)
			// Insert item whilst checking whether the buffer was consumed or not
			if !data.Insert(hash.NewArray(buffer)) {
				// Yes, buffer consumed.  Therefore, construct fresh buffer to avoid
				// aliasing the value now stored in the hash set.
				buffer = slices.Clone(buffer)
			}
		}
	}
	//
	return data
}

func buildDynamicSetChunk[F field.Element[F]](id SetId, trModule trace.Module[F]) Set[F] {
	var (
		buffer = make([]F, id.Width())
		data   = hash.NewSet[hash.Array[F]](trModule.Height() >> 4)
	)
	//
	for i := range trModule.Height() {
		if isSelected(i, id, trModule) {
			// Read each register of this vector
			readRegisters(i, id, trModule, buffer)
			// Insert item whilst checking whether the buffer was consumed or not
			if !data.Insert(hash.NewArray(buffer)) {
				// Yes, buffer consumed.  Therefore, construct fresh buffer to avoid
				// aliasing the value now stored in the hash set.
				buffer = slices.Clone(buffer)
			}
		}
	}
	//
	return data
}

// readRegisters reads the value held in each register of the given vector on
// the given row into the temporary buffer.
func readRegisters[F field.Element[F]](k uint, id SetId, trModule trace.Module[F], buffer []F) {
	for i := range id.Width() {
		rid := id.Ith(i)
		buffer[i] = trModule.Column(rid.Unwrap()).Get(k)
	}
}

// readStaticRegisters reads the value held in each register of the given vector
// on the given row into the temporary buffer.
func readStaticRegisters[F field.Element[F]](id SetId, row []F, buffer []F) {
	for i := range id.Width() {
		rid := id.Ith(i)
		buffer[i] = row[rid.Unwrap()]
	}
}

// isSelected determines whether or not the given row of the given vector is
// selected.  A row without a selector is always selected; otherwise, it is
// selected when its selector is non-zero.
func isSelected[F field.Element[F]](k uint, id SetId, trModule trace.Module[F]) bool {
	// If no selector, then always selected
	if !id.HasSelector() {
		return true
	}
	// Otherwise, selected when selector non-zero.
	return !trModule.Column(id.Selector().Unwrap()).Get(k).IsZero()
}

// isStaticSelected determines whether or not the given row of the given vector
// is selected.  A row without a selector is always selected; otherwise, it is
// selected when its selector is non-zero.
func isStaticSelected[F field.Element[F]](id SetId, row []F) bool {
	// If no selector, then always selected
	if !id.HasSelector() {
		return true
	}
	// Otherwise, selected when selector non-zero.
	return !row[id.Selector().Unwrap()].IsZero()
}

// ============================================================================
// Context Implementation
// ============================================================================

// Context provides suitable constrant context
type contextImpl[F field.Element[F]] struct {
	sets map[string]*hash.Set[hash.Array[F]]
}

// Get implementation of Context interface.
func (p contextImpl[F]) Get(id SetId) collection.Set[[]F] {
	if set, ok := p.sets[id.String()]; ok {
		return contextSet[F]{set}
	}
	//
	panic(fmt.Sprintf("unknown set %s in context", id.String()))
}

// An adaptor to make a Set[hash.Array[F]] look like a Set[[]F]
type contextSet[F field.Element[F]] struct {
	data *hash.Set[hash.Array[F]]
}

// Contains implementation for collection.Set interface
func (p contextSet[F]) Contains(row []F) bool {
	return p.data.Contains(hash.NewArray(row))
}

// Iter implementation of Set interface.
func (p contextSet[F]) Iter() iter.Seq[[]F] {
	return func(yield func([]F) bool) {
		for v := range p.data.Iter() {
			if !yield(v.Elements()) {
				return
			}
		}
	}
}
