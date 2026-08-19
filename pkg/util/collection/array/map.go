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
