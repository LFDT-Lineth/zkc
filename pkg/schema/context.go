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
package schema

import (
	"fmt"
	"iter"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Set provides a convenient alias
type Set[F field.Element[F]] = *hash.Set[hash.Array[F]]

// ChunkId identifies a particular set within a given shard.
type ChunkId struct {
	shard uint
	id    SetId
}

// SeqBuildContext constructs the context from a given schema and trace.
// Essentially, this means traversing the schema looking for lookups and
// constructing their sets.  NOTE: this is done sequentially
func SeqBuildContext[F field.Element[F]](tr trace.Trace[F], sc AnySchema[F]) Context[F] {
	var (
		stats    = util.NewPerfStats()
		contexts []map[string]*hash.Set[hash.Array[F]]
		sids     = determineSets(sc)
	)
	// Build context for each shard
	for i := range tr {
		var context = make(map[string]*hash.Set[hash.Array[F]])
		// sequential set construction (as a single chunk per set)
		for _, sid := range sids {
			// Construct data for this set
			context[sid.String()] = buildSetChunk(ChunkId{uint(i), sid}, tr, sc)
		}
		//
		contexts = append(contexts, context)
	}
	//
	stats.Log(fmt.Sprintf("Building context (%d sets in sequence)", len(sids)*len(tr)))
	//
	return contextImpl[F]{contexts}
}

// ParBuildContext constructs the context from a given schema and trace, using
// a map-reduce strategy.  Each set is first split into one or more row-range
// chunks (so a single large set does not bottleneck the whole process), which
// are then constructed in parallel via array.ParallelMap (the "map" phase).
// Chunks belonging to the same set are then merged back together (the
// "reduce" phase), which is likewise done in parallel across sets.
func ParBuildContext[F field.Element[F]](tr trace.Trace[F], sc AnySchema[F]) Context[F] {
	var (
		stats    = util.NewPerfStats()
		contexts = make([]map[string]*hash.Set[hash.Array[F]], len(tr))
		chunks   = determineChunks(tr, sc)
		// build every chunk in parallel
		sets = array.ParallelMap(chunks, func(_ uint, c ChunkId) Set[F] {
			return buildSetChunk(c, tr, sc)
		})
	)
	// Initialise context for each shard
	for i := range tr {
		contexts[i] = make(map[string]*hash.Set[hash.Array[F]])
	}
	// Flattern individual sets into their shards
	for i, chunk := range chunks {
		contexts[chunk.shard][chunk.id.String()] = sets[i]
	}
	//
	stats.Log(fmt.Sprintf("Building context (%d sets in parallel)", len(chunks)))
	//
	return contextImpl[F]{contexts}
}

// determineChunks splits every set identified by sids into one or more
// row-range chunks suitable for parallel construction.  The chunk size is
// determined dynamically from the total number of candidate rows across all
// sets (see determineChunkSize), rather than being fixed, so it scales with
// both the workload and the number of available processors.  Sets are always
// given at least one chunk (which may be empty), so every set ends up
// represented in the resulting context.  Alongside the flat chunk list, it
// returns the number of chunks generated for each set (aligned with sids), so
// callers can recover the chunks belonging to a given set without a map
// lookup.
func determineChunks[F field.Element[F]](tr trace.Trace[F], sc AnySchema[F]) []ChunkId {
	var (
		sids  = determineSets(sc)
		sets  = make([]ChunkId, len(sids)*len(tr))
		index = 0
	)
	//
	for i := range tr {
		for _, sid := range sids {
			sets[index] = ChunkId{uint(i), sid}
			index++
		}
	}
	//
	return sets
}

// DetermineSets extracts all unique set identifiers from lookup constraints.
func determineSets[F field.Element[F]](sc AnySchema[F]) []SetId {
	var sets set.AnySortedSet[SetId]
	//
	for iter := sc.Constraints(); iter.HasNext(); {
		var ith = iter.Next()
		//
		for _, sid := range ith.Sets() {
			sets.Insert(sid)
		}
	}
	//
	return sets.ToArray()
}

// buildSetChunk constructs the (partial) set of rows determined by a given
// chunk.
func buildSetChunk[F field.Element[F]](c ChunkId, tr trace.Trace[F], sc AnySchema[F]) Set[F] {
	var (
		scModule = sc.Module(c.id.Module())
		trModule = tr[c.shard].Module(c.id.Module())
	)
	//
	if scModule.IsStatic() {
		return buildStaticSetChunk(c.id, scModule)
	}
	//
	return buildDynamicSetChunk(c.id, trModule)
}

func buildStaticSetChunk[F field.Element[F]](id SetId, sm Module[F]) Set[F] {
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
	sets []map[string]*hash.Set[hash.Array[F]]
}

// Get implementation of Context interface.
func (p contextImpl[F]) Get(shard uint, id SetId) collection.Set[[]F] {
	if set, ok := p.sets[shard][id.String()]; ok {
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
