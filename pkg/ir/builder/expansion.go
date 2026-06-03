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
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceExpansion expands a given trace according to a given schema. More
// specifically, that means computing the actual values for any assignments.
// This is done using a straightforward sequential algorithm.
func TraceExpansion[F field.Element[F]](parallel bool, batchsize uint, schema sc.AnySchema[F],
	trace *tr.ArrayTrace[F]) error {
	//
	var (
		err error
		// Start timer
		stats = util.NewPerfStats()
	)
	//
	if parallel {
		// Run (parallel) trace expansion
		err = ParallelTraceExpansion(batchsize, schema, trace)
	} else {
		err = SequentialTraceExpansion(schema, trace)
	}
	// Log stats
	stats.Log("Trace expansion")
	//
	return err
}

// SequentialTraceExpansion expands a given trace according to a given schema.
// More specifically, that means computing the actual values for any
// assignments.  This is done using a straightforward sequential algorithm.
func SequentialTraceExpansion[F field.Element[F]](schema sc.AnySchema[F], trace *trace.ArrayTrace[F]) error {
	var (
		err      error
		expander = NewExpander(schema.Width(), schema.Assignments())
	)
	// Compute each assignment in turn
	for !expander.Done() {
		var cols []array.MutArray[F]
		// Get next assignment
		ith := expander.Next(1)[0]
		// Compute ith assignment(s)
		if cols, err = ith.Compute(trace, schema); err != nil {
			return err
		}
		// Fill all computed columns
		fillComputedColumns(ith.RegistersWritten(), cols, trace)
	}
	// Done
	return nil
}

// ParallelTraceExpansion performs trace expansion using concurrently executing
// jobs.  The chosen algorithm operates in waves, rather than using an
// continuous approach.  This is for two reasons: firstly, the latter would
// require locks that would slow down evaluation performance; secondly, the vast
// majority of jobs are run in the very first wave.
func ParallelTraceExpansion[F field.Element[F]](batchsize uint, schema sc.AnySchema[F], trace *tr.ArrayTrace[F]) error {
	var (
		batchNum = 0
		// Construct a communication channel for errors.
		ch = make(chan columnBatch[F], batchsize)
		//
		expander = NewExpander[F](schema.Width(), schema.Assignments())
	)
	// Iterate until all assignments processed.
	for !expander.Done() {
		var (
			stats = util.NewPerfStats()
			batch = expander.Next(batchsize)
		)
		// Dispatch next batch of assignments.
		dispatchReadyAssignments(batch, schema, trace, ch)
		//
		batches := make([]columnBatch[F], len(batch))
		// Collect all the results
		for i := range len(batch) {
			batches[i] = <-ch
			// Read from channel
			if batches[i].err != nil {
				// Fail immediately
				return batches[i].err
			}
		}
		// Once we get here, all go rountines are complete and we are sequential
		// again.
		for _, r := range batches {
			fillComputedColumns(r.targets, r.columns, trace)
		}
		// Log stats about this batch
		stats.Log(fmt.Sprintf("Expansion batch %d (remaining %d)", batchNum, expander.Count()))
		// Increment batch
		batchNum++
	}
	// Done
	return nil
}

// Dispatch the given set of assignments with results being fed back into the
// shared channel.
func dispatchReadyAssignments[F field.Element[F]](batch []sc.Assignment[F], schema sc.AnySchema[F],
	trace *tr.ArrayTrace[F], ch chan columnBatch[F]) {
	// Dispatch each assignment in the batch
	for _, ith := range batch {
		// Dispatch!
		go func(targets []register.Ref) {
			cols, err := ith.Compute(trace, schema)
			// Send outcome back
			ch <- columnBatch[F]{targets, cols, err}
		}(ith.RegistersWritten())
	}
}

// Fill a set of columns with their computed results.  The column index is that
// of the first column in the sequence, and subsequent columns are index
// consecutively.
func fillComputedColumns[F field.Element[F]](refs []register.Ref, cols []array.MutArray[F], trace *tr.ArrayTrace[F]) {
	var resized bit.Set
	// Add all columns
	for i, ref := range refs {
		var (
			rid    = ref.Column().Unwrap()
			module = trace.RawModule(ref.Module())
			col    = cols[i]
		)
		// Looks good
		if module.FillColumn(rid, col) {
			// Register module as being resized.
			resized.Insert(ref.Module())
		}
	}
	// Finalise resized modules
	for iter := resized.Iter(); iter.HasNext(); {
		module := trace.RawModule(iter.Next())
		module.Resize()
	}
}

// Result from given computation.
type columnBatch[F field.Element[F]] struct {
	// Target registers for this batch
	targets []register.Ref
	// The computed columns in this batch.
	columns []array.MutArray[F]
	// An error (should one arise)
	err error
}
