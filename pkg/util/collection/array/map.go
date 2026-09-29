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
package array

import (
	"runtime"
	"sync"
)

// ParallelApply invokes fn once for every element in a given array, purely
// for its side effects, parallelising the work using a worker pool whose size
// is bounded by the number of available CPUs.
//
// This is the side-effecting counterpart to ParallelMap, for cases where no
// result slice is needed.
func ParallelApply[T any](items []T, fn func(uint, T)) {
	var (
		n = len(items)
		// Determine number of workers.
		workers = min(n, runtime.NumCPU())
		// worker pool
		wg sync.WaitGroup
		// Channel from which workers pull indices to process.
		ch = make(chan int, workers)
	)
	// Start workers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		//
		go func() {
			defer wg.Done()
			// Process indices until channel is closed.
			for i := range ch {
				fn(uint(i), items[i])
			}
		}()
	}
	// Distribute work.
	for i := range n {
		ch <- i
	}
	// Signal workers to stop.
	close(ch)
	// Wait for all workers to finish.
	wg.Wait()
}

// ParallelMap behaves exactly like Map, mapping every element in a given array
// to a corresponding element of another type, but parallelises the work using a
// worker pool whose size is bounded by the number of available CPUs, making it
// safe to use even with large input slices or relatively cheap mapper
// functions.
//
// Map (in util.go) provides the sequential alternative.
func ParallelMap[T any, S any](items []T, mapper func(uint, T) S) []S {
	var (
		results = make([]S, len(items))
		n       = len(items)
		// Determine number of workers.
		workers = min(n, runtime.NumCPU())
		// worker pool
		wg sync.WaitGroup
		// Channel from which workers pull indices to process.
		ch = make(chan int, workers)
	)
	// Start workers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		//
		go func() {
			defer wg.Done()
			// Process indices until channel is closed.
			for i := range ch {
				results[i] = mapper(uint(i), items[i])
			}
		}()
	}
	// Distribute work.
	for i := range n {
		ch <- i
	}
	// Signal workers to stop.
	close(ch)
	// Wait for all workers to finish.
	wg.Wait()
	//
	return results
}

// ParallelReduce reduces an array of items to a single item using a given
// reduction function.  The reduction function is expected to be associative.
//
// The work is split into contiguous chunks, one per worker (bounded by the
// number of available CPUs).  Each chunk is reduced sequentially, and the
// per-chunk results are then reduced sequentially in order.  Since chunk order
// is preserved, associativity of the reduction function is sufficient for the
// result to be deterministic.  An empty array reduces to the zero value of T.
func ParallelReduce[T any](items []T, reducer func(T, T) T) T {
	var (
		n = len(items)
		// Determine number of workers.
		workers = min(n, runtime.NumCPU())
		// Per-worker partial results.
		partials = make([]T, workers)
		// worker pool
		wg sync.WaitGroup
	)
	//
	if n == 0 {
		var dummy T
		return dummy
	}
	// Start workers, each reducing a contiguous chunk of items.
	for w := range workers {
		var (
			start = w * n / workers
			end   = (w + 1) * n / workers
		)
		//
		wg.Add(1)
		//
		go func() {
			defer wg.Done()
			//
			acc := items[start]
			//
			for _, item := range items[start+1 : end] {
				acc = reducer(acc, item)
			}
			//
			partials[w] = acc
		}()
	}
	// Wait for all workers to finish.
	wg.Wait()
	// Combine partial results.
	result := partials[0]
	//
	for _, p := range partials[1:] {
		result = reducer(result, p)
	}
	//
	return result
}

// ParallelMapReduce applies a ParallelMap over a given array of items, followed
// by a ParallelReduce to produce a single result.
func ParallelMapReduce[S, T any](items []S, mapping func(uint, S) T, reducer func(T, T) T) T {
	return ParallelReduce(ParallelMap(items, mapping), reducer)
}
