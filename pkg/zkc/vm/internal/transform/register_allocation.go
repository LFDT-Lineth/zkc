// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.
//
// SPDX-License-Identifier: Apache-2.0
package transform

import (
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Function provides a useful alias
type Function[W word.Word[W]] = descriptor.Function[W]

// ProgramPoint provides a convenient alias
type ProgramPoint = descriptor.ProgramPoint

// AllocateRegisters applies register allocation to each function in a given
// program.  In this context, register allocation is about identifying registers
// which can be allocated to the same column.  Consider the following simple
// program:
//
//	fn f(x'1:u16,x'0:u16,y'1:u16,y'0:u16) -> (r'1:u16,r'0:u16)
//	         ...
//	         var c$11:u1
//	         var c$12:u1
//	 [00]  ...
//	       skip_if $4 == 0 4
//	       add c$11::r'0 = x'0+0x1
//	       add r'1 = x'1+c$11
//	       check r'1:u16
//	       skip 3
//	       add c$12::r'0 = y'0+0x1
//	       add r'1 = y'1+c$12
//	       check r'1:u16
//	       ret
//
// In this case, the two registers `c$11` and `c$12` are written on different
// branches, and neither is live where the other is.  Hence, they can be
// coalesced (i.e. merged together) and, subsequently, allocated to the same
// column.
//
// Two registers can be coalesced only when they do not "interfere" with each
// other.  Interference arises in three ways:
//
// (1) Structural.  Inputs interfere with every register, as do any two outputs,
// and any two registers of different bitwidths or padding.  Likewise, special
// cases are employed for Dispatch bytecodes and RegisterVectors.
//
// (2) Definitional.  A register written at a point where another may already
// have been written in the same vector interferes with it, since coalescing
// them would introduce a write-after-write conflict.
//
// (3) Liveness.  Two registers live at the same program point interfere with
// each other.  For this purpose, the registers written by a bytecode are also
// considered live on entry to it.  This ensures a register which is written but
// never subsequently read cannot overwrite one still live.
//
// Registers are then coalesced greedily: each register (in order) is merged
// into the lowest-numbered register group it does not interfere with (if any).
// This is simple, but not necessarily optimal.
//
// NOTE: this transform must run after register splitting, and before lookup
// accesses are flattened.  Call and memory lookups read their operands on the
// current row (i.e. after every write in the vector), hence FlattenLookupAccess
// snapshots any operand written later in the same vector.  Coalescing after
// that could reintroduce such writes.
func AllocateRegisters[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		//
		out = make([]descriptor.Module[W], len(mods))
	)
	//
	for i, ith := range mods {
		// check for function
		if f, ok := ith.(*Function[W]); ok {
			ith = allocateFunctionRegisters(f)
		}
		//
		out[i] = ith
	}
	//
	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), out...)
}

func allocateFunctionRegisters[W word.Word[W]](f *Function[W]) *Function[W] {
	var (
		// Construct the interference graph which clarifies which registers can
		// be coalesced.
		graph = determineInterferenceGraph(*f)
		// Apply knowledge of interference to determine an allocation.
		allocation = determineRegisterAllocation(*f, graph)
	)
	// Apply the register allocation
	return applyRegisterAllocation(*f, allocation)
}

// Apply a given register allocation to a function f, producing a semantically
// equivalent function which uses potentially fewer registers.  The allocation
// is a mapping from each register in f to another register in f.  For example,
// if a register r maps to itself then, after allocation, it will be retained.
// In contrast, if a register r maps to another register q then, after
// allocation, register r will be merged into q.
func applyRegisterAllocation[W word.Word[W]](f Function[W], allocation []uint) *Function[W] {
	var (
		mapping   = make([]uint16, f.Width())
		registers []descriptor.Register[W]
		vectors   = make([]bytecode.Vector[W], len(f.Vectors()))
	)
	// Build the mapping
	for i, j := range allocation {
		var ith = f.Register(uint16(i))
		//
		if uint(i) == j {
			var ni = uint16(len(registers))
			// Retain register
			registers = append(registers, ith)
			// Map register to new position
			mapping[i] = ni
		} else {
			// j < i (by construction)
			mapping[i] = mapping[j]
		}
	}
	// Apply the mapping
	for i, vec := range f.Vectors() {
		vectors[i] = vec.Map(func(_ uint, b bytecode.Bytecode[W]) []bytecode.Bytecode[W] {
			return []Bytecode[W]{substituteRegisters(b, mapping)}
		})
	}
	//
	return descriptor.NewFunction(f.Name(), registers, f.Kind(), f.Effects(), vectors)
}

func determineRegisterAllocation[W word.Word[W]](f Function[W], ig InterferenceGraph) []uint {
	//
outer:
	for u := range f.Width() {
		// Following holds from order in which registers allocated.
		util.Assert(ig.IsRoot(u), "invalid register group")
		// Look for least register which does not interfere with u.
		for v := range u {
			if ig.IsRoot(v) && !ig.HaveInterference(v, u) {
				// Merge u to v
				ig.Coalesce(v, u)
				// Done
				continue outer
			}
		}
	}
	//
	return ig.roots
}

func determineInterferenceGraph[W word.Word[W]](f Function[W]) InterferenceGraph {
	var ig = NewInterferenceGraph(f.Width())
	// Record interference between parameters / returns
	addStructuralInterference(f, &ig)
	// Record interference based on definitions
	addDefinitionalInterference(f, &ig)
	// Record interference based on liveness
	addLivenessInterference(f, &ig)
	//
	return ig
}

// Add interference arising from structural information between two registers.
// For example, if two registers have different bitwidths, then they cannot be
// coalesced.
func addStructuralInterference[W word.Word[W]](f Function[W], ig *InterferenceGraph) {
	for i := range f.Width() {
		var ith = f.Register(RegisterId(i))
		//
		for j := i + 1; j < f.Width(); j++ {
			var jth = f.Register(RegisterId(j))

			if hasStructuralInterference(ith, jth) {
				ig.AddInterference(i, j)
			}
		}
	}
	// Ensure dispatch registers are never coalesced
	for _, vec := range f.Vectors() {
		for _, bc := range vec.Bytecodes {
			if d, ok := bc.(*bytecode.Dispatch[W]); ok {
				// Prevent every bit register in a dispatch from coalescing with
				// any other register.
				for _, c := range d.Cases {
					ig.PreventCoalescing(uint(c.Bit))
				}
				// Prevent default register in dispatch from coalescing with any other register.
				ig.PreventCoalescing(uint(d.Default))
			}
		}
	}
	// Ensure (for now) that register vectors are never coalesced (as this could
	// potentially break their ordering).  See #2255.
	for _, v := range f.Vectors() {
		for _, bc := range v.Bytecodes {
			for _, rv := range extractRegisterVectors(bc) {
				if rv.Len > 1 {
					ig.PreventCoalescingVec(rv)
				}
			}
		}
	}
}

// Add interference arising from definitional information between registers.
// Specifically, a register written at a point where another may already have
// been written in the same vector interferes with it, since coalescing them
// would introduce a write-after-write conflict.
func addDefinitionalInterference[W word.Word[W]](f Function[W], ig *InterferenceGraph) {
	for _, vec := range f.Vectors() {
		// Compute definitional information
		var writes = vec.WriteMap()
		//
		for i, b := range vec.Bytecodes {
			var st = writes.StateOf(uint(i))
			// Prevent coalescing of registers when this would result in a
			// Write-After-Write conflict in the given vector.
			for _, d := range b.Definitions() {
				// Registers which may already have been written in this vector
				for it := st.MaybeWrites(); it.HasNext(); {
					if y := it.Next(); y != uint(d) {
						ig.AddInterference(uint(d), y)
					}
				}
			}
		}
	}
}

// Add interference arising from liveness information between two registers.
// Specifically, if two registers are live at the same time they cannot be
// coalesced.
func addLivenessInterference[W word.Word[W]](f Function[W], ig *InterferenceGraph) {
	var (
		// Construct initial live set information
		liveSets = liveVariablesAnalysis(f)
		// Convert into range information for efficient querying
		liveRanges = liveSets.ToRanges()
	)
	// Post-process liveness information to account for variables which are
	// defined but never used.
	for i, vec := range f.Vectors() {
		for j, bc := range vec.Bytecodes {
			var pp = ProgramPoint{Macro: uint(i), Micro: uint(j)}
			// Mark all registers defined by this bytecode as live at the
			// corresponding program point.
			for _, r := range bc.Definitions() {
				liveRanges.MarkLive(r, pp)
			}
		}
	}
	// Add interference based on available liveness information.  Specifically,
	// two registers cannot be coalesced if they are "live" at the same time.
	for i := range f.Width() {
		for j := i + 1; j < f.Width(); j++ {
			if liveRanges.HaveOverlap(RegisterId(i), RegisterId(j)) {
				ig.AddInterference(i, j)
			}
		}
	}
}

// Determine whether two registers "obviously" interfere with each other.  For
// example, if one is a parameter or both are returns, or they have different
// bitwidths, etc.
func hasStructuralInterference[W word.Word[W]](r1, r2 descriptor.Register[W]) bool {
	if r1.IsInput() {
		return true
	} else if r1.IsOutput() && r2.IsOutput() {
		return true
	} else if r1.Bitwidth() != r2.Bitwidth() {
		return true
	} else if r1.Padding().Cmp(r2.Padding()) != 0 {
		return true
	}
	//
	return false
}

// Extract all RegisterVectors used as operands within a given bytecode.
func extractRegisterVectors[W word.Word[W]](b Bytecode[W]) []bytecode.RegisterVector {
	var vecs []bytecode.RegisterVector
	//
	switch bc := b.(type) {
	case *bytecode.Bitwise[W]:
		if bc.Right.IsRegisterVector() {
			vecs = append(vecs, bc.Right.AsRegisterVector())
		}
	case *bytecode.Debug[W]:
		return bc.Sources
	case *bytecode.DivRem[W]:
		if bc.Divisor.IsRegisterVector() {
			vecs = append(vecs, bc.Divisor.AsRegisterVector())
		}
	case *bytecode.Fail[W]:
		return bc.Sources
	case *bytecode.Intrinsic[W]:
		vecs = append(vecs, bc.Targets...)
		//
		for _, op := range bc.Sources {
			if op.IsRegisterVector() {
				vecs = append(vecs, op.AsRegisterVector())
			}
		}
		//
	case *bytecode.SkipIf[W]:
		if bc.Right.IsRegisterVector() {
			vecs = append(vecs, bc.Right.AsRegisterVector())
		}
		//
		vecs = append(vecs, bc.Left)
	}
	//
	return vecs
}

// InterferenceGraph captures information about when two registers "interfere"
// with each other.  Specifically, if two registers interfere then they cannot
// be coalesced.
type InterferenceGraph struct {
	edges []bit.Set
	// Identifies the leader of each group
	roots []uint
}

// NewInterferenceGraph constructs a new interference graph with n vertices.
func NewInterferenceGraph(n uint) InterferenceGraph {
	var roots = make([]uint, n)
	//
	for i := range roots {
		roots[i] = uint(i)
	}
	//
	return InterferenceGraph{make([]bit.Set, n), roots}
}

// IsRoot returns true if the given register is the current root of some
// register group.
func (p *InterferenceGraph) IsRoot(x uint) bool {
	return p.roots[x] == x
}

// AddInterference records that two register groups interfere with each other.
// For simplicity, both arguments must be their respective register group roots.
func (p *InterferenceGraph) AddInterference(g1, g2 uint) {
	util.Assert(p.IsRoot(g1), "invalid register group root")
	util.Assert(p.IsRoot(g2), "invalid register group root")
	p.edges[g1].Insert(g2)
	p.edges[g2].Insert(g1)
}

// PreventCoalescing prevents a given register group from coalescing with
// anything else.
func (p *InterferenceGraph) PreventCoalescing(g uint) {
	for i := range uint(len(p.edges)) {
		if p.IsRoot(i) {
			p.AddInterference(g, i)
		}
	}
}

// PreventCoalescingVec prevents all registers in a given vector from coalescing
// with anything else.
func (p *InterferenceGraph) PreventCoalescingVec(v bytecode.RegisterVector) {
	for i := range uint(v.Len) {
		p.PreventCoalescing(uint(v.Base) + i)
	}
}

// Coalesce one register group (g2) into another (g1) in the graph to ensure
// interference edges for all are retained.  For simplicity, both arguments must
// be their respective register group roots.
func (p *InterferenceGraph) Coalesce(g1, g2 uint) {
	util.Assert(g1 < g2, "must coalesce into lower group")
	util.Assert(!p.HaveInterference(g1, g2), "cannot coalesce interfering groups")
	// Insert all edges of g2 into g1
	p.edges[g1].Union(p.edges[g2])
	// Remap all edges involving g2
	for iter := p.edges[g2].Iter(); iter.HasNext(); {
		var n = iter.Next()
		p.edges[n].Remove(g2)
		p.edges[n].Insert(g1)
	}
	// Finalise
	p.edges[g2] = bit.Set{}
	p.roots[g2] = g1
}

// HaveInterference checks whether two register groups interfere with each
// other, or not.  For simplicity, both arguments must be their respective
// register group roots.
func (p *InterferenceGraph) HaveInterference(g1, g2 uint) bool {
	util.Assert(p.IsRoot(g1), "invalid register group root")
	util.Assert(p.IsRoot(g2), "invalid register group root")
	//
	return p.edges[g1].Contains(g2)
}

// ============================================================================
// Live Ranges
// ============================================================================

// LiveRanges represents the liveness information in a form that is suitable for
// efficient overlap queries.
type LiveRanges struct {
	offsets []uint
	// live ranges for each register
	ranges []bit.Set
}

// NewLiveRanges constructs a set of empty live ranges for a given function.
// The intention is that these are then populated via the MarkLive function.
func NewLiveRanges[W word.Word[W]](f descriptor.Function[W]) LiveRanges {
	var (
		offsets = make([]uint, len(f.Vectors()))
		offset  uint
	)
	//
	for i, v := range f.Vectors() {
		offsets[i] = offset
		offset += uint(len(v.Bytecodes))
	}
	//
	return LiveRanges{offsets, make([]bit.Set, f.Width())}
}

// HaveOverlap returns true if the live ranges of the given registers
// intersect.
func (p *LiveRanges) HaveOverlap(r1, r2 RegisterId) bool {
	var (
		r1_range = p.ranges[r1]
		r2_range = p.ranges[r2]
	)
	// Check for any intersection
	return r1_range.Intersects(r2_range)
}

// MarkLive marks a given variable as live at a given program point.
func (p *LiveRanges) MarkLive(r RegisterId, pp ProgramPoint) {
	var index = p.offsets[pp.Macro] + pp.Micro
	p.ranges[r].Insert(index)
}

// ============================================================================
// Live Sets
// ============================================================================

// LiveSets records, for each program point (i.e. bytecode) in a function, the
// set of registers live immediately before it.  Two registers have overlapping
// live ranges if some program point has both in its live set.
type LiveSets[W word.Word[W]] struct {
	fun descriptor.Function[W]
	// live sets for each program point
	sets map[ProgramPoint]bit.Set
}

// ToRanges converts the live variables information into live range information.
func (p *LiveSets[W]) ToRanges() LiveRanges {
	var ranges = NewLiveRanges(p.fun)
	// Insert all information
	for pp, set := range p.sets {
		for iter := set.Iter(); iter.HasNext(); {
			var r = RegisterId(iter.Next())
			ranges.MarkLive(r, pp)
		}
	}
	//
	return ranges
}

// Get the live set associated with a given program point
func (p *LiveSets[W]) Get(pc descriptor.ProgramPoint) bit.Set {
	return p.sets[pc]
}

// Join a live set into that associated with a given program point.
func (p *LiveSets[W]) Join(pc descriptor.ProgramPoint, liveset bit.Set) bool {
	var (
		set     = p.sets[pc]
		changed = set.Union(liveset)
	)
	// Update information
	p.sets[pc] = set
	//
	return changed
}

// ============================================================================
// Live Variables Analysis
// ============================================================================

// Compute the set of registers live before each bytecode in the given function.
func liveVariablesAnalysis[W word.Word[W]](f Function[W]) LiveSets[W] {
	// Records the registers live before each bytecode.
	var (
		liveness = LiveSets[W]{f, make(map[ProgramPoint]bit.Set)}
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
					set = propagateLiveness(pp, ith[j-1], liveness, f)
				)
				// Merge in sets, and record whether anything changed
				changed = liveness.Join(pp, set) || changed
			}
		}
	}
	//
	return liveness
}

// Propagate liveness information backwards through the given bytecode.
func propagateLiveness[W word.Word[W]](pp ProgramPoint, bc Bytecode[W], liveness LiveSets[W], f Function[W]) bit.Set {
	var (
		// Resulting live information
		set bit.Set
		// Determine next logical bytecode
		next = pp.Skip(0)
	)
	//
	switch bc := bc.(type) {
	case *bytecode.Fail[W]:
		// Do nothing
	case *bytecode.Ret[W]:
		var numInputsOutputs = f.NumInputs() + f.NumOutputs()
		// Construct liveness for function
		for r := f.NumInputs(); r < numInputsOutputs; r++ {
			set.Insert(r)
		}
	case *bytecode.Jmp[W]:
		// Determine jump target
		target := ProgramPoint{Macro: uint(bc.Target), Micro: 0}
		// Extract set at that point
		set.Union(liveness.Get(target))
	case *bytecode.Skip[W]:
		// Determine target bytecode
		var target = pp.Skip(uint(bc.Skip))
		// Source liveness information
		set.Union(liveness.Get(target))
	case *bytecode.SkipIf[W]:
		// Determine next logical bytecode
		var target = pp.Skip(uint(bc.Skip))
		// Include liveness from false branch
		set.Union(liveness.Get(next))
		// Include liveness from true branch
		set.Union(liveness.Get(target))
	case *bytecode.Switch[W]:
		for _, c := range bc.Cases {
			// Determine target bytecode
			var target = pp.Skip(uint(c.Skip))
			// Source liveness information
			set.Union(liveness.Get(target))
		}
		// Source liveness from following bytecode
		set.Union(liveness.Get(next))
	case *bytecode.Dispatch[W]:
		for _, c := range bc.Cases {
			// Determine target bytecode
			var target = pp.Skip(uint(c.Skip))
			// Source liveness information
			set.Union(liveness.Get(target))
		}
		// Source liveness from following bytecode
		set.Union(liveness.Get(next))
	default:
		// Source liveness from following bytecode
		set.Union(liveness.Get(next))
	}
	// Remove definitions
	for _, d := range bc.Definitions() {
		set.Remove(uint(d))
	}
	// Add uses
	for _, d := range bc.Uses() {
		set.Insert(uint(d))
	}
	// Done
	return set
}
