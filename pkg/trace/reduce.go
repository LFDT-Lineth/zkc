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
package trace

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Reduce combines a sequence of aligned traces into a single trace.  Two traces
// are aligned when they have the same width and modules sharing a name also
// share the same module index and matching register (and hence limb)
// descriptors.  For each module, the rows of every input trace are
// concatenated, in trace order, into the corresponding module of the result.
func Reduce[F field.Element[F]](traces []Trace[F]) Trace[F] {
	if len(traces) == 0 {
		return nil
	}
	// Aligned traces all share the same width, so the first trace determines
	// the module structure of the result.
	width := traces[0].Width()
	modules := make([]*CompactModule[F], width)
	//
	for mid := range width {
		var descriptor = traces[0].Module(mid).Descriptor()
		//
		modules[mid] = reduceModule(mid, descriptor, traces)
	}
	//
	return NewArray(modules)
}

// ParallelReduce behaves exactly like Reduce, combining a sequence of aligned
// traces into a single trace, but reduces each module of the result
// concurrently using a worker pool.
func ParallelReduce[F field.Element[F]](traces []Trace[F]) Trace[F] {
	if len(traces) == 0 {
		return nil
	}
	//
	width := traces[0].Width()
	descriptors := make([]ModuleDescriptor, width)
	//
	for mid := range width {
		descriptors[mid] = traces[0].Module(mid).Descriptor()
	}
	//
	modules := array.ParallelMap(descriptors, func(mid uint, descriptor ModuleDescriptor) *CompactModule[F] {
		return reduceModule(mid, descriptor, traces)
	})
	//
	return NewArray(modules)
}

// reduceModule concatenates the rows of the module at a given index across all
// traces.  The module name and descriptor are taken from the first trace, which
// aligned traces guarantee match those of every other trace.
func reduceModule[F field.Element[F]](mid uint, descriptor ModuleDescriptor, traces []Trace[F]) *CompactModule[F] {
	// Check whether replicating
	if descriptor.Replicated {
		return reduceReplicatedModule(mid, descriptor, traces)
	}
	//
	acc := InitCompactModule[F](descriptor)
	//
	for i := range traces {
		acc.Join(traces[i].Module(mid))
	}
	//
	return acc
}

func reduceReplicatedModule[F field.Element[F]](mid uint, descriptor ModuleDescriptor, traces []Trace[F],
) *CompactModule[F] {
	var (
		winner Module[F]
		height uint
		acc    = InitCompactModule[F](descriptor)
	)
	// Find tallest module
	for i := range traces {
		ith := traces[i].Module(mid)
		// TODO: find a better joining strategy
		if height < ith.Height() {
			winner = ith
			height = ith.Height()
		}
	}
	// Construct new module
	acc.Join(winner)
	// Done
	return acc
}
