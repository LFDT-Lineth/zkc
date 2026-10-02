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
package dfa

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ForwardTransferFn provides a generic notion of a function which determines,
// for a given incoming flow set, those sets arising from (abstractly) executing
// a given bytecode.
type ForwardTransferFn[W word.Word[W], S any] = func(ProgramPoint, Bytecode[W], S) []Transfer[S]

// ForwardDataFlowAnalysis performs (as the name suggests) a forward data flow
// analysis which iterates until fixed-point is reached.  Care is required to
// ensure that the given flow-set types do not lead not lead to non-termination
// (e.g. have infinite ascending chains, etc) as, at this time, no widening
// operator is supported.
func ForwardDataFlowAnalysis[W word.Word[W], S FlowSet[S]](f descriptor.Function[W],
	fn ForwardTransferFn[W, S], init S) FlowSets[S] {
	// Records the registers live before each bytecode.
	var (
		flowsets = FlowSets[S]{make(map[ProgramPoint]S)}
		changed  = true
	)
	// Iterate to a fixed point
	for changed {
		// Reset changed status
		changed = false
		// Go through each vector, updating the liveness information which holds
		// before each bytecode.
		for i, v := range f.Vectors() {
			var ith = v.Bytecodes
			// Process in reverse order, as this better matches a backwards
			// analysis.
			for j := len(ith); j > 0; j-- {
				var (
					// Construct program point for this bytecode
					pp = ProgramPoint{Macro: uint(i), Micro: uint(j - 1)}
					// Propagate liveness information backwards
					tfs = fn(pp, ith[j-1], flowsets.Get(pp))
				)
				// Merge in sets, and record whether anything changed
				for _, t := range tfs {
					changed = flowsets.Join(t.target, t.set) || changed
				}
			}
		}
	}
	//
	return flowsets
}
