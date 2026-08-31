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
package builder

import (
	"fmt"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceExpansion expands a given trace according to a given schema. More
// specifically, that means computing the actual values for any assignments.
// This is done using a straightforward sequential algorithm.
func TraceExpansion[F field.Element[F]](config Config, schema sc.AnySchema[F], tr trace.Shard[F],
) (trace.Shard[F], error) {
	//
	var (
		err error
	)
	//
	if config.Parallel {
		// Run (parallel) trace expansion
		tr, err = ParallelTraceExpansion(config.BatchSize, schema, tr)
	} else {
		tr, err = SequentialTraceExpansion(schema, tr)
	}
	//
	return tr, err
}

// SequentialTraceExpansion expands a given trace according to a given schema.
// More specifically, that means computing the actual values for any
// assignments.  This is done using a straightforward sequential algorithm.
func SequentialTraceExpansion[F field.Element[F]](schema sc.AnySchema[F], tr trace.Shard[F]) (trace.Shard[F], error) {
	var (
		err      error
		expander = NewExpander(schema.Width(), schema.Assignments())
		modules  = extractModules(tr)
	)
	// Allocate new trace from expanded modules
	tr = trace.NewShard(modules)
	// Compute each assignment in turn
	for !expander.Done() {
		var cols []array.Array[F]
		// Get next assignment
		ith := expander.Next(1)[0]
		// Compute ith assignment(s)
		if cols, err = ith.Compute(tr, schema); err != nil {
			return tr, err
		}
		// Fill all computed columns
		fillComputedColumns(ith.RegistersWritten(), cols, modules)
	}
	// Done
	return tr, nil
}

// ParallelTraceExpansion performs trace expansion using concurrently executing
// jobs.  The chosen algorithm operates in waves, rather than using an
// continuous approach.  This is for two reasons: firstly, the latter would
// require locks that would slow down evaluation performance; secondly, the vast
// majority of jobs are run in the very first wave.
func ParallelTraceExpansion[F field.Element[F]](batchsize uint, schema sc.AnySchema[F], tr trace.Shard[F],
) (trace.Shard[F], error) {
	var (
		batchNum = 0
		//
		expander = NewExpander(schema.Width(), schema.Assignments())
		modules  = extractModules(tr)
	)
	// Allocate new trace from expanded modules
	tr = trace.NewShard(modules)
	// Iterate until all assignments processed.
	for !expander.Done() {
		var (
			stats = util.NewPerfStats()
			batch = expander.Next(batchsize)
		)
		// Process all assignments in this wave in parallel using a worker pool.
		results := array.ParallelMap(batch, func(_ uint, ith sc.Assignment[F]) columnBatch[F] {
			cols, err := ith.Compute(tr, schema)
			return columnBatch[F]{ith.RegistersWritten(), cols, err}
		})
		// Check for errors and fill computed columns into the trace.
		for _, r := range results {
			if r.err != nil {
				// Fail immediately
				return tr, r.err
			}
			//
			fillComputedColumns(r.targets, r.columns, modules)
		}
		// Log stats about this batch
		stats.Log(fmt.Sprintf("Expansion batch %d (remaining %d)", batchNum, expander.Count()))
		// Increment batch
		batchNum++
	}
	// Done
	return tr, nil
}

// extractModules copies out the modules underlying a given trace, so they can
// be progressively updated (via fillComputedColumns) as expansion proceeds.
func extractModules[F field.Element[F]](tr trace.Shard[F]) []trace.Module[F] {
	modules := make([]trace.Module[F], tr.Width())
	//
	for i := range modules {
		modules[i] = tr.RawModule(uint(i))
	}
	//
	return modules
}

// Fill a set of columns with their computed results.  The column index is that
// of the first column in the sequence, and subsequent columns are index
// consecutively.
func fillComputedColumns[F field.Element[F]](refs []register.Ref, cols []array.Array[F], modules []trace.Module[F]) {
	// Add all columns
	for i, ref := range refs {
		var (
			mid = ref.Module()
			rid = ref.Column().Unwrap()
			col = cols[i]
		)
		// Expand it, recording the updated module.
		modules[mid] = modules[mid].Expand(rid, col)
	}
}

// Result from given computation.
type columnBatch[F field.Element[F]] struct {
	// Target registers for this batch
	targets []register.Ref
	// The computed columns in this batch.
	columns []array.Array[F]
	// An error (should one arise)
	err error
}
