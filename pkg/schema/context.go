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
	"runtime"
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

// chunksPerWorker determines how many chunks, on average, each available
// processor should receive when building sets in parallel.  Using more
// chunks than workers allows the workload to be rebalanced dynamically: a
// worker which finishes its chunks early can pick up further chunks, rather
// than sitting idle whilst a single large chunk is still being built
// elsewhere.
const chunksPerWorker = 10

// determineChunkSize computes a suitable chunk size for parallel set
// construction, based on the total number of candidate rows across all sets
// and the number of available processors.  Aiming for chunksPerWorker chunks
// per processor balances parallelism against the overhead of the reduce
// phase (more, smaller chunks means more merging work later).
func determineChunkSize(totalRows uint) uint {
	return max(1, totalRows/(uint(runtime.NumCPU())*chunksPerWorker))
}

// setChunk identifies a sub-range of rows within the set determined by a
// given SetId.
type setChunk struct {
	id         SetId
	start, end uint
}

// SeqBuildContext constructs the context from a given schema and trace.
// Essentially, this means traversing the schema looking for lookups and
// constructing their sets.  NOTE: this is done sequentially
func SeqBuildContext[F field.Element[F]](tr trace.Trace[F], sc AnySchema[F]) Context[F] {
	var (
		stats   = util.NewPerfStats()
		context = make(map[string]*hash.Set[hash.Array[F]])
		sids    = determineSets(sc)
	)
	// sequential set construction (as a single chunk per set)
	for _, sid := range sids {
		height := setHeight(sid, tr, sc)
		// Construct data for this set
		context[sid.String()] = buildSetChunk(setChunk{sid, 0, height}, tr, sc)
	}
	//
	stats.Log(fmt.Sprintf("Building context (%d sets; sequential)", len(context)))
	//
	return contextImpl[F]{context}
}

// ParBuildContext constructs the context from a given schema and trace, using
// a map-reduce strategy.  Each set is first split into one or more row-range
// chunks (so a single large set does not bottleneck the whole process), which
// are then constructed in parallel via array.ParallelMap (the "map" phase).
// Chunks belonging to the same set are then merged back together (the
// "reduce" phase), which is likewise done in parallel across sets.
func ParBuildContext[F field.Element[F]](tr trace.Trace[F], sc AnySchema[F]) Context[F] {
	var (
		stats          = util.NewPerfStats()
		context        = make(map[string]*hash.Set[hash.Array[F]])
		sids           = determineSets(sc)
		chunks, counts = determineChunks(sids, tr, sc)
		// map phase: build every chunk in parallel
		partials = array.ParallelMap(chunks, func(_ uint, c setChunk) Set[F] {
			return buildSetChunk(c, tr, sc)
		})
		// group chunk results by their originating set.  Since chunks are
		// generated in sids order (see determineChunks), the ith set's chunks
		// form a contiguous run of counts[i] entries within partials.
		groups = make([][]Set[F], len(sids))
	)
	//
	for i, offset := 0, uint(0); i < len(sids); i++ {
		groups[i] = partials[offset : offset+counts[i]]
		offset += counts[i]
	}
	// reduce phase: merge the chunks of each set in parallel
	merged := array.ParallelMap(sids, func(i uint, sid SetId) Set[F] {
		return mergeSets(groups[i])
	})
	//
	for i, sid := range sids {
		context[sid.String()] = merged[i]
	}
	//
	stats.Log(fmt.Sprintf("Building context (%d sets, %d chunks; parallel)", len(sids), len(chunks)))
	//
	return contextImpl[F]{context}
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
func determineChunks[F field.Element[F]](sids []SetId, tr trace.Trace[F], sc AnySchema[F]) ([]setChunk, []uint) {
	var (
		heights = make([]uint, len(sids))
		total   uint
	)
	// Determine height of every set, and the total number of rows overall.
	for i, sid := range sids {
		heights[i] = setHeight(sid, tr, sc)
		total += heights[i]
	}
	//
	var (
		size   = determineChunkSize(total)
		chunks []setChunk
		counts = make([]uint, len(sids))
	)
	//
	for i, height := range heights {
		if height == 0 {
			chunks = append(chunks, setChunk{sids[i], 0, 0})
			counts[i] = 1

			continue
		}
		//
		for start := uint(0); start < height; start += size {
			chunks = append(chunks, setChunk{sids[i], start, min(start+size, height)})
			counts[i]++
		}
	}
	//
	return chunks, counts
}

// setHeight determines the number of candidate (static or dynamic) rows from
// which the given set is constructed.
func setHeight[F field.Element[F]](id SetId, tr trace.Trace[F], sc AnySchema[F]) uint {
	scModule := sc.Module(id.Module())
	//
	if scModule.IsStatic() {
		return uint(len(scModule.StaticContents()))
	}
	//
	return tr.Module(id.Module()).Height()
}

// mergeSets combines one or more partial sets (constructed from disjoint
// row-range chunks of the same underlying set) into a single set.
func mergeSets[F field.Element[F]](partials []Set[F]) Set[F] {
	// Common case: set was constructed as a single chunk.
	if len(partials) == 1 {
		return partials[0]
	}
	//
	var size uint
	//
	for _, p := range partials {
		size += p.Size()
	}
	//
	merged := hash.NewSet[hash.Array[F]](size >> 4)
	//
	for _, p := range partials {
		for v := range p.Iter() {
			merged.Insert(v)
		}
	}
	//
	return merged
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
func buildSetChunk[F field.Element[F]](c setChunk, tr trace.Trace[F], sc AnySchema[F]) Set[F] {
	scModule := sc.Module(c.id.Module())
	//
	if scModule.IsStatic() {
		return buildStaticSetChunk(c, scModule)
	}
	//
	return buildDynamicSetChunk(c, tr.Module(c.id.Module()))
}

func buildStaticSetChunk[F field.Element[F]](c setChunk, sm Module[F]) Set[F] {
	var (
		buffer   = make([]F, c.id.Width())
		contents = sm.StaticContents()[c.start:c.end]
		data     = hash.NewSet[hash.Array[F]](uint(len(contents)) >> 4)
	)
	// Insert all selected rows within this chunk
	for _, row := range contents {
		if isStaticSelected(c.id, row) {
			// Read each register of this vector
			readStaticRegisters(c.id, row, buffer)
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

func buildDynamicSetChunk[F field.Element[F]](c setChunk, trModule trace.Module[F]) Set[F] {
	var (
		buffer = make([]F, c.id.Width())
		data   = hash.NewSet[hash.Array[F]]((c.end - c.start) >> 4)
	)
	//
	for i := c.start; i < c.end; i++ {
		if isSelected(i, c.id, trModule) {
			// Read each register of this vector
			readRegisters(i, c.id, trModule, buffer)
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
