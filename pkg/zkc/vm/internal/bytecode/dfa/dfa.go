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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// RegisterId provides an useful alias
type RegisterId = uint16

// ProgramPoint provides an useful alias
type ProgramPoint = descriptor.ProgramPoint

// Bytecode provides a useful alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// ============================================================================
// Transfer
// ============================================================================

// Transfer is essentially a pair identifying where a given state should be
// propagated during a dataflow analysis.
type Transfer[T any] struct {
	set    T
	target ProgramPoint
}

// NewTransfer constructs a new transfer arc to join a given state into a given
// target.
func NewTransfer[T any](state T, target ProgramPoint) Transfer[T] {
	return Transfer[T]{state, target}
}

// ============================================================================
// FlowSet
// ============================================================================

// FlowSet captures the notion of an abstract "dataflow set" associated with a
// given program point.  A dataflow set essentially contains the information
// gleaned from analysing the given program.
type FlowSet[S any] interface {
	// Join another state into this state, producing a state representing both.
	Join(other S) (S, bool)
	// IsBottom returns true when this flow state represents something which has not ne
	IsBottom() bool
}

// FlowSets provides a generic representation of the "dataflow sets" computed by
// a given dataflow analysis.
type FlowSets[S FlowSet[S]] struct {
	// flow sets for each program point
	sets map[ProgramPoint]S
}

// Get the live set associated with a given program point
func (p *FlowSets[S]) Get(pc ProgramPoint) S {
	return p.sets[pc]
}

// Join a flow set into that associated with a given program point, returning
// true if that resulted in a change (and false otherwise).
func (p *FlowSets[S]) Join(pc ProgramPoint, in S) bool {
	var (
		set     = p.sets[pc]
		changed bool
	)
	// Update information
	p.sets[pc], changed = set.Join(in)
	//
	return changed
}
