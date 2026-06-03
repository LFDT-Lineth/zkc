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
	"slices"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Expander encapsulates key state required in order to expand traces safely
// (whether or not this is done sequentially or in parallel).  The intuition
// behind this algorithm is that each assignment will only be run exactly once.
// However, some assignments must be run before others.  For example, if one
// assignment depends upon a column which is computed by another, then the
// latter must go first.
type Expander[F any] struct {
	// Width records the number of modules in the schema.
	width uint
	// Set of assignments yet to run
	worklist []sc.Assignment[F]
	// Records which columns are not ready.
	notReady bit.Set
	// Records set of columns being vertically expanded.
	expanding bit.Set
}

// NewExpander constructs a new trace expander for a given set of assignments.
func NewExpander[F any](width uint, assignments iter.Iterator[sc.Assignment[F]]) Expander[F] {
	var (
		notReady  bit.Set
		expanding bit.Set
		arr       = assignments.Collect()
	)
	// Mark every column which is written by an assignment as not yet ready.
	// All other columns are therefore assumed to be ready.
	for _, ith := range arr {
		for _, ref := range ith.RegistersWritten() {
			notReady.Insert(ref.Index(width))
		}
	}
	//
	for _, ith := range arr {
		for _, ref := range ith.RegistersExpanded() {
			expanding.Insert(ref.Index(width))
		}
	}
	// Done
	return Expander[F]{width, arr, notReady, expanding}
}

// Done indicates whether all assignments have been processed, or not.
func (p *Expander[F]) Done() bool {
	return len(p.worklist) == 0
}

// Count returns the number of assignments remaining to be processed.
func (p *Expander[F]) Count() uint {
	return uint(len(p.worklist))
}

// Next returns at most n assignments which are ready for execution.  An
// assignment is ready for execution if all of the columns on which it depends
// have been processed already.  Observe that all assignments returned here are
// removed from the worklist and will not be returned again.  Specifically, they
// are assumed to have been processed before any subsequent call is made to this
// method.
func (p *Expander[F]) Next(n uint) []sc.Assignment[F] {
	var (
		batch []sc.Assignment[F]
		m     = len(p.worklist)
	)
	// Go through each assignment in turn, pulling out those which are ready.
	// To avoid too much copying, assignments are removed from the worklist by
	// swapping them to the back.
	for i := 0; i < m && n > 0; {
		if p.isReady(i) {
			// Add to batch
			batch = append(batch, p.worklist[i])
			// Decrease remaining assignments
			m--
			// Decrease remaining requested
			n--
			// Swap to back
			p.worklist[i] = p.worklist[m]
		} else {
			i++
		}
	}
	// Remove all those swapped out in one go.
	p.worklist = p.worklist[:m]
	// Update notion of which columns are ready
	for _, ith := range batch {
		for _, ref := range ith.RegistersWritten() {
			p.notReady.Remove(ref.Index(p.width))
		}
	}
	// Sanity check that something was actually removed.
	if len(batch) == 0 {
		panic("trace expansion cannot progress")
	}
	//
	return batch
}

// isReady checks whether a given assignment can be processed (or not).
// Specifically, an assignment cannot be processed if it depends upon a column
// which is not ready (i.e. has not been processed).
func (p *Expander[F]) isReady(i int) bool {
	for _, ref := range p.worklist[i].RegistersRead() {
		// Check whether dependency is ready
		if p.notReady.Contains(ref.Index(p.width)) && !p.isExpandedBy(ref, i) {
			// No, its not.
			return false
		}
	}
	//
	return true
}

// isExpandedBy checks whether a given register is actually being expanded by a
// given assignment.
func (p *Expander[F]) isExpandedBy(ref register.Ref, i int) bool {
	if p.expanding.Contains(ref.Index(p.width)) {
		ith := p.worklist[i]
		// Check whether the given register is actually written by this assignment
		// or not.
		return slices.Contains(ith.RegistersExpanded(), ref)
	}
	//
	return false
}
