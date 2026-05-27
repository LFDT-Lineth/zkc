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
package compiler

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"

	"github.com/consensys/go-corset/pkg/corset/ast"
	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/util/file"
)

// Register encapsulates information about a "register" in the underlying
// constraint system.  The rough analogy is that "register allocation" is
// applied to map Corset columns down to HIR columns (a.k.a. registers).  The
// distinction between columns at the Corset level, and registers at the HIR
// level is necessary for two reasons: firstly, one corset column can expand to
// several HIR registers; secondly, register allocation is applied to columns in
// different perspectives of the same module.
type Register struct {
	// Context (i.e. module + multiplier) of this register.
	Context ast.Context
	// Underlying width of this register.
	Bitwidth uint
	// Common padding value
	Padding big.Int
	// Source columns of this register
	Sources []RegisterSource
	// Cached name
	cached_name *string
}

// IsActive determines whether or not this register is "active".  Inactive
// registers should not generally be visible outside of register allocation.
func (r *Register) IsActive() bool {
	return r.Bitwidth != math.MaxUint
}

// IsInput determines whether or not this register represents an input column,
// or not.  NOTE: there is currently an implicit assumption that columns
// allocated to the same register always have the same "visibility" (i.e. are
// either all input, or all computed, etc).
func (r *Register) IsInput() bool {
	// Extract input from first source
	computed := r.Sources[0].Computed
	// Sanity check all sources are consistent.
	for _, ith := range r.Sources {
		if ith.Computed != computed {
			panic("inconsistent register visibility")
		}
	}
	//
	return !computed
}

// Merge two registers together.  This means the source-level columns will be
// allocated to the same underlying HIR column (i.e. register).
func (r *Register) Merge(other *Register) {
	var zero big.Int
	//
	if r.Context != other.Context {
		panic("cannot merge registers from different context")
	}
	//
	r.Bitwidth = max(r.Bitwidth, other.Bitwidth)
	r.Sources = append(r.Sources, other.Sources...)
	// Check whether padding matches
	if r.Padding.Cmp(&other.Padding) != 0 {
		// No, so reset to default
		r.Padding = zero
	}
	// Reset the cached name
	r.cached_name = nil
	// Deactivate other register
	other.Deactivate()
}

// Deactivate marks a given register as no longer being required.  This happens
// when one register is merged into another.
func (r *Register) Deactivate() {
	r.Bitwidth = math.MaxUint
	r.Sources = nil
}

// Name returns the given name for this register.
func (r *Register) Name() string {
	if r.cached_name == nil {
		// Construct registers name
		names := make([]string, len(r.Sources))
		// Sort by perspective name
		slices.SortFunc(r.Sources, func(l RegisterSource, r RegisterSource) int {
			return strings.Compare(l.Perspective(), r.Perspective())
		})
		//
		for i, source := range r.Sources {
			names[i] = constructColumnName(source)
		}
		// Construct register name from list of names
		name := constructRegisterName(names)
		r.cached_name = &name
	}
	//
	return *r.cached_name
}

// Construct a suitable column name from a given register source.  We need to
// include the perspective name here in order to avoid name clashes between
// columns with the same name defined in different columns.
func constructColumnName(source RegisterSource) string {
	name := fmt.Sprintf("%s%s", source.Perspective(), source.Name.Tail())
	// Normalise name
	return strings.ReplaceAll(name, "/", "")
}

// A simple algorithm for joining names together.
func constructRegisterName(names []string) string {
	str := ""
	// Joing them together
	for i, n := range names {
		if i == 0 {
			str = n
		} else {
			str = fmt.Sprintf("%s_xor_%s", str, n)
		}
	}
	//
	return str
}

// RegisterSource provides necessary information about source-level columns
// allocated to a given register.
type RegisterSource struct {
	// Context is a prefix of name which, when they differ, indicates a virtual
	// column (i.e. one which is subject to register allocation).
	Context file.Path
	// Fully qualified (i.e. absolute) Name of source-level column.
	Name file.Path
	// Length Multiplier of source-level column.
	Multiplier uint
	// Underlying bitwidth of the source-level column.
	Bitwidth uint
	// Provability requirement for source-level column.
	MustProve bool
	// Determines whether this is a Computed column.
	Computed bool
	// Determines value used for padding
	Padding big.Int
	// Display modifier
	Display string
}

// IsVirtual indicates whether or not this is a "virtual" column.  That is,
// something which is subject to register allocation (i.e. because it is
// declared in a perspective).
func (p *RegisterSource) IsVirtual() bool {
	return !p.Name.Parent().Equals(p.Context)
}

// Perspective returns the name of the "virtual perspective" in which this
// column exists.
func (p *RegisterSource) Perspective() string {
	return p.Name.Parent().Slice(p.Context.Depth()).String()
}

// RegisterAllocationView provides a view of an environment for the purposes of
// register allocation, such that only registers in this view will be considered
// for allocation.  This is necessary because we must not attempt to allocate
// registers across different modules (indeed, contexts) together. Instead, we
// must allocate registers on a module-by-module basis, etc.
type RegisterAllocationView struct {
	// View of registers available for register allocation.
	registers []uint
	// Parent pointer for register merging.
	env *GlobalEnvironment
}

// Len returns the number of allocated registers.
func (p *RegisterAllocationView) Len() uint {
	return uint(len(p.registers))
}

// Registers returns an iterator over the set of registers in this local
// allocation.
func (p *RegisterAllocationView) Registers() iter.Iterator[uint] {
	return iter.NewArrayIterator(p.registers)
}

// Register accesses information about a specific register in this window.
func (p *RegisterAllocationView) Register(index uint) *Register {
	return &p.env.registers[index]
}

// Merge one register (src) into another (dst).  This will remove the src
// register, and automatically update all column assignments.  Therefore, any
// register identifier can be potenitally invalided by this operation.  This
// will panic if the registers are incompatible (i.e. have different contexts).
func (p *RegisterAllocationView) Merge(dst uint, src uint) {
	target := &p.env.registers[dst]
	source := &p.env.registers[src]
	// Sanity check
	if target.Context != source.Context {
		// Should be unreachable.
		panic("attempting to merge incompatible registers")
	}
	// Update column map
	for _, col := range p.env.ColumnsOf(src) {
		p.env.columnMap[col] = dst
	}
	//
	target.Merge(source)
}

// RegisterAllocation is a generic interface to support different "regsiter
// allocation" algorithms.  More specifically, register allocation is the
// process of allocating columns to their underlying HIR columns (a.k.a
// registers).  This is straightforward when there is a 1-1 mapping from a
// Corset column to an HIR column.  However, this is not always the case.  For
// example, array columns at the Corset level map to multiple columns at the HIR
// level.  Likewise, perspectives allow columns to be reused, meaning that
// multiple columns at the Corset level can be mapped down to just a single
// column at the HIR level.
//
// Notes:
//
// * Arrays.  These are allocated consecutive columns, as determined by their
// "width".  That is, the size of the array.
//
// * Perspectives.  This is where the main challenge lies.  Columns in different
// perspectives can be merged together, but this is only done when they have
// compatible underlying types.
type RegisterAllocation interface {
	// Access the set of registers being considered for allocation.
	Registers() iter.Iterator[uint]
	// Access information about a specific register.
	Register(uint) *Register
	// Merge one register (src) into another (dst).  This marks the source
	// register as inactive, but does not otherwise update the register
	// assignment.  Thus, existing register ids remain valid throughout register
	// allocation.  Once register allocation is complete, inactive registers are
	// then discarded.
	Merge(dst uint, src uint)
}

// DEFAULT_ALLOCATOR determines the register allocation algorithm to use by
// default.
var DEFAULT_ALLOCATOR func(RegisterAllocation) = LegacyAllocator

// LegacyAllocator is the original register allocation algorithm used in Corset.
// This is retained for backwards compatibility reasons, but should eventually
// be dropped.
func LegacyAllocator(allocation RegisterAllocation) {
	sortRegisters(allocation.(*RegisterAllocationView))
	allocator := NewRegisterAllocator(allocation)
	allocator.CompactBy(identicalType)
	allocator.Finalise()
}

// ImprovedAllocator provides an improved register allocation algorithm over the
// legacy allocator.  However, it is not safe to use at this time.
func ImprovedAllocator(allocation RegisterAllocation) {
	sortRegisters(allocation.(*RegisterAllocationView))
	allocator := NewRegisterAllocator(allocation)
	// Always try to compact by type first.
	allocator.CompactBy(identicalType)
	// Try to compact any unproven register
	allocator.CompactBy(unprovenType)
	// try to compact any unproven registers which fit within a proven register.
	allocator.CompactBy(containedUnprovenType)
	//
	allocator.Finalise()
}

func identicalType(lhs *RegisterGroup, rhs *RegisterGroup) bool {
	return lhs.bitwidth == rhs.bitwidth
}

func unprovenType(lhs *RegisterGroup, rhs *RegisterGroup) bool {
	return !lhs.mustProve && !rhs.mustProve
}

func containedUnprovenType(lhs *RegisterGroup, rhs *RegisterGroup) bool {
	return !lhs.mustProve && rhs.mustProve && lhs.bitwidth <= rhs.bitwidth
}

// Sort the registers into alphabetical order.
func sortRegisters(view *RegisterAllocationView) {
	slices.SortFunc(view.registers, func(l, r uint) int {
		lhs := view.Register(l).Sources[0].Name.String()
		rhs := view.Register(r).Sources[0].Name.String()
		// Within perspective sort alphabetically by name
		return strings.Compare(lhs, rhs)
	})
}

// ============================================================================
// GreedyAllocator
// ============================================================================

// AllocationComparator is a binary predicate over register groups used to
// determine when two groups can be merged.
type AllocationComparator = func(*RegisterGroup, *RegisterGroup) bool

// RegisterAllocator is a simple, but reasonably effective high-level approach
// to register allocation.  Essentially, perspective columns are sorted by type
// and then allocated based on their "compatibility".  Here, compatibility
// determines when two registers can be merged (i.e. when they are
// "compatible"_.  This allocator is parameterised over the notion of
// compatibility in order to support different top-level allocation algorithms.
type RegisterAllocator struct {
	// Underlying allocation of registers
	allocation RegisterAllocation
	// Maps perspectives to their "allocation slot"
	perspectives map[string]uint
	// Maps each slot to its perspective string.
	slots []string
	// Allocation matrix, where each innermost array has an entry for each
	// perspective.  The allocation slot for a perspective determines its index
	// within this array.  Entries can be "empty" if they are given MaxUint
	// value.
	allocations []RegisterGroup
}

// NewRegisterAllocator initialises a new greedy sort allocator from an initial
// allocation with a given compatibility function.
func NewRegisterAllocator(allocation RegisterAllocation) *RegisterAllocator {
	// Construct initial allocator
	allocator := RegisterAllocator{
		allocation,
		make(map[string]uint, 0),
		make([]string, 0),
		make([]RegisterGroup, 0),
	}
	// Identify all perspectives
	for iter := allocation.Registers(); iter.HasNext(); {
		regInfo := allocation.Register(iter.Next())
		regSource := regInfo.Sources[0]
		//
		if len(regInfo.Sources) != 1 {
			// This should be unreachable.
			panic("register not associated with unique column")
		} else if regSource.IsVirtual() {
			allocator.allocatePerspective(regSource.Perspective())
		}
	}
	// Initial allocation of perspective registers
	for iter := allocation.Registers(); iter.HasNext(); {
		regIndex := iter.Next()
		regInfo := allocation.Register(regIndex)
		regSource := regInfo.Sources[0]
		//
		if regSource.IsVirtual() {
			perspective := regSource.Perspective()
			allocator.allocateRegister(perspective, regIndex)
		}
	}
	// Done (for now)
	return &allocator
}

// Width returns the current number of non-empty groups.
func (p *RegisterAllocator) Width() uint {
	count := uint(0)
	//
	for i := range p.allocations {
		ith := &p.allocations[i]
		// Ignore allocation if its already been merged into something else.
		if !ith.IsEmpty() {
			count++
		}
	}
	//
	return count
}

// CompactBy Greedily compact the given allocation using a given "compabitility" comparator.
func (p *RegisterAllocator) CompactBy(predicate AllocationComparator) {
	for i := range p.allocations {
		ith := &p.allocations[i]
		// Ignore allocation if its already been merged into something else.
		if !ith.IsEmpty() {
			for j := i + 1; j < len(p.allocations); j++ {
				jth := &p.allocations[j]
				// Can we merge them?
				if !jth.IsEmpty() && ith.Disjoint(jth) && predicate(ith, jth) {
					// Yes!
					ith.Merge(jth)
				}
			}
		}
	}
}

// Finalise the register allocation by merging allocated registers in the same slot.
func (p *RegisterAllocator) Finalise() {
	for _, a := range p.allocations {
		if !a.IsEmpty() {
			// Determine target register for allocation.
			head := a.Target()
			// Merge all other allocated slots into head.
			for _, r := range a.slots {
				if r.register != head {
					p.allocation.Merge(head, r.register)
				}
			}
		}
	}
}

// Allocate a given perspective to a "slot" within the allocation matrix.  If
// the perspective has already been allocated, then do nothing.
func (p *RegisterAllocator) allocatePerspective(perspective string) {
	if _, ok := p.perspectives[perspective]; !ok {
		slot := uint(len(p.perspectives))
		p.perspectives[perspective] = slot
		p.slots = append(p.slots, perspective)
	}
}

// Greedily allocate a given register.
func (p *RegisterAllocator) allocateRegister(perspective string, regIndex uint) {
	// Extract register info
	regInfo := p.allocation.Register(regIndex)
	regWidth := regInfo.Bitwidth
	regProve := false
	// Check for provability
	for _, col := range regInfo.Sources {
		regProve = regProve || col.MustProve
	}
	// Determine perspective slot
	slot := p.perspectives[perspective]
	// Construct empty allocation
	alloc := NewRegisterGroup(regWidth, regProve)
	// Allocate this slot to the specified register
	alloc.Assign(slot, regIndex)
	// Done
	p.allocations = append(p.allocations, alloc)
}

// RegisterSlot represents a register in a given "slot".  The intuition is that
// each perspective is allocated its own slot.  Thus, the goal of register
// allocation is to group compatible registers in different perspectives (i.e.
// slots).
type RegisterSlot struct {
	slot     uint
	register uint
}

// Cmp implements the necessary comparator for register slots.
func (p RegisterSlot) Cmp(other RegisterSlot) int {
	if p.slot < other.slot {
		return -1
	} else if p.slot == other.slot {
		// This should be unreachable.
		panic("multiple registers allocated to same slot")
	}
	//
	return 1
}

// RegisterGroup represents a group of registers which (eventually) will be
// allocated to the same column.  The intuition is that, initially, we begin
// with a single group for each register.  Then, groups are merged together
// according to the high-level allocation algorithm.
type RegisterGroup struct {
	// The bitwidth for all registers in this group.
	bitwidth uint
	// Indicates whether any register allocated to this group must have its type
	// proven.
	mustProve bool
	// Identifies members of this group, where each member is a register
	// allocated to a given slot (i.e. perspective). The intuition is that no
	// two members of this group should be allocated to the same slot (i.e.
	// perspective).  Furthermore, all members of this groups should be
	// compatible (in some sense).
	slots set.AnySortedSet[RegisterSlot]
}

// NewRegisterGroup constructs a new (and empty) register group.
func NewRegisterGroup(bitwidth uint, mustProve bool) RegisterGroup {
	// Create initially empty set of register slots.
	slots := set.NewAnySortedSet[RegisterSlot]()
	//
	return RegisterGroup{
		bitwidth,
		mustProve,
		*slots,
	}
}

// IsEmpty checks whether this group is empty or not.  Groups can become empty
// when they are merged into others.
func (p *RegisterGroup) IsEmpty() bool {
	return len(p.slots) == 0
}

// Available determines whether a given slot has been set already.
func (p *RegisterGroup) Available(slot uint) bool {
	// NOTE: could be made more efficient using a binary search.
	for _, r := range p.slots {
		if r.slot == slot {
			return false
		} else if r.slot > slot {
			// This early termination criteria makes sense as we know that each
			// RegisterSlot is sorted in increasing order of its slot.
			return true
		}
	}
	//
	return true
}

// Assign a register to a given slot in this group.  If the slot is already
// taken, then this will panic.
func (p *RegisterGroup) Assign(slot uint, reg uint) {
	if !p.Available(slot) {
		// Should be unreachable
		panic("attempt to reassign slot")
	}
	//
	p.slots.Insert(RegisterSlot{slot, reg})
}

// Target returns the register with the least index within this allocation.
func (p *RegisterGroup) Target() uint {
	if len(p.slots) == 0 {
		// Should be impossible since each allocation starts with exactly one
		// slot already taken.
		panic("empty allocation encountered")
	}
	//
	var target uint = math.MaxUint
	//
	for _, r := range p.slots {
		target = min(target, r.register)
	}
	//
	return target
}

// Disjoint checks whether this allocation and another are disjoint.  That is,
// they do not have registers allocated to the same slot.
func (p *RegisterGroup) Disjoint(other *RegisterGroup) bool {
	// Check each slot in turn
	i := 0
	j := 0
	//
	for i < len(p.slots) && j < len(other.slots) {
		ith := p.slots[i].slot
		jth := other.slots[j].slot
		//
		if ith == jth {
			// This indicates a given slot has been assigned a register in both
			// groups.  Hence, they are not disjoint.
			return false
		} else if ith < jth {
			i++
		} else {
			j++
		}
	}
	// No slot allocated in both, so they are disjoint.
	return true
}

// Merge another allocation into this allocation.  This leaves the other
// allocation in the unused state (i.e. it will be ignored from now on).
func (p *RegisterGroup) Merge(other *RegisterGroup) {
	// Join their datatypes
	p.bitwidth = max(p.bitwidth, other.bitwidth)
	// If either group contains registers whose type must be proven, then so
	// does this.
	p.mustProve = p.mustProve || other.mustProve
	// Join the two groups (via set union)
	p.slots.InsertSorted(&other.slots)
	// Mark other allocation as empty
	other.slots = nil
}

func (p *RegisterGroup) String() string {
	var builder strings.Builder
	//
	builder.WriteString("{")
	//
	for i, r := range p.slots {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		fmt.Fprintf(&builder, "%d:=%d", r.slot, r.register)
	}
	//
	builder.WriteString("}")
	//
	return builder.String()
}
