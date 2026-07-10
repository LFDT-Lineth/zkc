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
package rtrace

import "sync"

// Reduce combines a sequence of aligned traces into a single trace.  Two traces
// are aligned when they have the same width and modules sharing a name also
// share the same module index and matching register (and hence limb)
// descriptors.  For each module, the rows of every input trace are
// concatenated, in trace order, into the corresponding module of the result.
func Reduce[T any](traces []Trace[T]) *Array[T] {
	if len(traces) == 0 {
		return NewArray[T](nil)
	}
	// Aligned traces all share the same width, so the first trace determines
	// the module structure of the result.
	width := traces[0].Width()
	modules := make([]ArrayModule[T], width)
	//
	for mid := range width {
		modules[mid] = reduceModule(traces, mid)
	}
	//
	return NewArray(modules)
}

// ParallelReduce behaves exactly like Reduce, combining a sequence of aligned
// traces into a single trace, but reduces each module of the result
// concurrently using one goroutine per module.
func ParallelReduce[T any](traces []Trace[T]) *Array[T] {
	if len(traces) == 0 {
		return NewArray[T](nil)
	}
	//
	width := traces[0].Width()
	modules := make([]ArrayModule[T], width)
	//
	var wg sync.WaitGroup
	// Reduce each module independently.  Goroutines write to disjoint slots of
	// the modules slice, so no further synchronisation is required.
	for mid := range width {
		wg.Add(1)
		//
		go func(mid uint) {
			defer wg.Done()
			//
			modules[mid] = reduceModule(traces, mid)
		}(mid)
	}
	//
	wg.Wait()
	//
	return NewArray(modules)
}

// reduceModule concatenates the rows of the module at a given index across all
// traces.  The module name and descriptor are taken from the first trace, which
// aligned traces guarantee match those of every other trace.
func reduceModule[T any](traces []Trace[T], mid uint) ArrayModule[T] {
	var (
		first      = traces[0].Module(mid)
		descriptor = first.Descriptor().Collect()
		rows       [][]T
	)
	//
	for _, tr := range traces {
		rows = append(rows, moduleRows(tr.Module(mid))...)
	}
	//
	return NewArrayModule(first.Name(), descriptor, rows...)
}

// moduleRows extracts the raw row data from a module as a slice of rows, where
// each row is itself a slice holding one value per column.
func moduleRows[T any](module Module[T]) [][]T {
	rows := make([][]T, module.Height())
	//
	for rid := range module.Height() {
		row := module.Row(rid)
		data := make([]T, row.Width())
		//
		for cid := range row.Width() {
			data[cid] = row.Get(cid)
		}
		//
		rows[rid] = data
	}
	//
	return rows
}
