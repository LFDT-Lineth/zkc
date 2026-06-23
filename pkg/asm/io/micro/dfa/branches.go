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
	"cmp"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
)

// BranchCondition abstracts the possible conditions under which a given branch
// is taken.
type BranchCondition = logical.Proposition[BranchId, BranchEquality]

// FALSE represents an unreachable path
var FALSE BranchCondition = logical.Truth[BranchId, BranchEquality](false)

// TRUE represents an path which is always reached
var TRUE BranchCondition = logical.Truth[BranchId, BranchEquality](true)

// BranchConjunction represents the conjunction of two paths
type BranchConjunction = logical.Conjunction[BranchId, BranchEquality]

// BranchEquality represents an atomic branch equality
type BranchEquality = logical.Equality[BranchId]

// BranchTransferFunction represents a transfer function over branch state.
type BranchTransferFunction[I any] func(offset uint, code I, state Branch) []Transfer[Branch]

// ============================================================================

// Branch adapts a branch condition to be an instance of State.
type Branch struct {
	Condition BranchCondition
}

// Join implementation for State interface
func (p Branch) Join(st Branch) Branch {
	return Branch{p.Condition.Or(st.Condition)}
}

// String implementation for State interface
func (p Branch) String(mapping register.Map) string {
	return p.Condition.String(func(rid BranchId) string {
		var name = mapping.Register(rid.Id).Name()
		//
		if rid.Forwarding {
			return name
		}
		//
		return fmt.Sprintf("'%s", name)
	})
}

// ============================================================================

// BranchId represents a set of one or more registers which can additionally
// indicate whether forwarding is active or not.  Forwarding indicates that the
// register(s) were previously assigned in the given instruction and, hence,
// need to be "forwarded" to the point where they are used.
type BranchId struct {
	// First underlying register in group
	Id register.Id
	// Number of registers in group
	Width uint
	// Indication of whether Forwarding is active or not.
	Forwarding bool
}

// NewBranchId constructs a new branch id from a group of one (or more)
// consecutively assigned registers.
func NewBranchId(forwarding bool, regs ...register.Id) BranchId {
	var first = regs[0].Unwrap()
	// Sanity check all registers in the vector are allocated in the expected
	// order (i.e. consecutively, starting from the least significant limb).
	for i := range len(regs) {
		expected := register.NewId(first + uint(i))
		//
		if regs[i] != expected {
			panic("invalid register group")
		}
	}
	//
	return BranchId{
		regs[0], uint(len(regs)), forwarding,
	}
}

// Cmp implementation of the logical.Variable interface
func (p BranchId) Cmp(o BranchId) int {
	if p.Forwarding == o.Forwarding {
		if c := p.Id.Cmp(o.Id); c != 0 {
			return c
		}
		//
		return cmp.Compare(p.Width, o.Width)
	} else if p.Forwarding {
		return 1
	}
	//
	return -1
}

// Get the ith id in this group as a (singleton) group.
func (p BranchId) Get(i uint) BranchId {
	if i >= p.Width {
		panic("invalid group member")
	}
	//
	rid := register.NewId(p.Id.Unwrap() + i)
	//
	return BranchId{rid, 1, p.Forwarding}
}

// Registers returns the set of registers in this group.
func (p BranchId) Registers() []register.Id {
	var (
		first = p.Id.Unwrap()
		regs  = make([]register.Id, p.Width)
	)
	//
	for i := range p.Width {
		regs[i] = register.NewId(first + i)
	}
	//
	return regs
}

// String implementation of the logical.Variable interface
func (p BranchId) String() string {
	var (
		first = p.Id.Unwrap()
		last  = first + p.Width - 1
		id    = fmt.Sprintf("{%d...%d}", first, last)
	)
	//
	if p.Forwarding {
		return id
	}
	//
	return fmt.Sprintf("'%s", id)
}
