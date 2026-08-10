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
package transform

import (
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

// This file holds the dataflow analysis half of timestamp threading: a
// forward analysis (over the dfa framework) computing, for every position of
// one row, the version of each effect's stamp.  Following static single
// assignment form, the stamp of one memory at a program point is identified
// by a version number: the k-th movement (memory access or effectful call)
// executed on a path advances the version from k-1 to k.  A version names a
// register — version zero is the row's entry register, the highest version
// live at an exit is the canonical register, every other version a fresh
// temporary — so, unlike a position-tagged scheme, the analysis result needs
// no later resolution step: the rewrite sweep in thread_timestamps.go binds
// (effect, version) pairs to registers as it walks the row.

// stampVersion records, symbolically, the current timestamp of one memory at
// a given program point.
type stampVersion struct {
	// v counts the movements (memory accesses and effectful calls) executed
	// on the longest path from the row's entry to this point; the k-th
	// movement advances the version from k-1 to k.
	v uint
	// lit marks a value which is the literal constant 1 + v rather than the
	// content of a register: the entry state of "main", which counts from one
	// and stays literal until the first effectful call.
	lit bool
	// canon marks a value already materialised in the effect's canonical
	// register: the entry state of every row after the first, and the state
	// after the pre-branch canonicalisation in front of a jump table.
	canon bool
}

// bump returns the value advanced by one movement.  Literality is preserved
// (a constant plus one is a constant); canonicity is not (the advanced value
// lives in the next version's register).
func (v stampVersion) bump() stampVersion {
	return stampVersion{v: v.v + 1, lit: v.lit}
}

// join combines the values carried by two paths.  Agreeing values pass
// through; otherwise the merged version is the largest incoming one and the
// rewrite sweep equalises the deficient paths (see mergeAt).  Literality
// never survives a disagreement (the constants differ); canonicity survives
// only when every path is canonical.
func (v stampVersion) join(o stampVersion) stampVersion {
	if v == o {
		return v
	}
	//
	return stampVersion{v: max(v.v, o.v), canon: v.canon && o.canon}
}

// String returns a debug rendering of this value.
func (v stampVersion) String() string {
	switch {
	case v.canon:
		return fmt.Sprintf("canon@%d", v.v)
	case v.lit:
		return fmt.Sprintf("lit(%d)", v.v+1)
	default:
		return fmt.Sprintf("v%d", v.v)
	}
}

// stamps is the dataflow state of the timestamp analysis: one version per
// threaded effect, in effect order.  The zero value (nil vals) is bottom,
// i.e. an unreachable position.
type stamps struct {
	vals []stampVersion
}

// isBottom reports whether this state denotes an unreachable position.
func (p stamps) isBottom() bool {
	return p.vals == nil
}

// with returns a copy of this state with the given effect's value replaced.
func (p stamps) with(x int, v stampVersion) stamps {
	vals := slices.Clone(p.vals)
	vals[x] = v
	//
	return stamps{vals}
}

// Join implementation for the dfa.State interface.
func (p stamps) Join(o stamps) stamps {
	vals := make([]stampVersion, len(p.vals))
	//
	for x := range vals {
		vals[x] = p.vals[x].join(o.vals[x])
	}
	//
	return stamps{vals}
}

// String implementation for the dfa.State interface (debugging only).
func (p stamps) String(register.Map) string {
	if p.isBottom() {
		return "⊥"
	}
	//
	var builder strings.Builder
	//
	builder.WriteString("{")
	//
	for x, v := range p.vals {
		if x != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(v.String())
	}
	//
	builder.WriteString("}")
	//
	return builder.String()
}

// stampArcKind classifies the arcs of the intra-row control-flow graph.  The
// distinction determines where the rewrite sweep can place instructions
// executing exactly on one arc (see mergeAt).
type stampArcKind uint8

const (
	// fallArc is the fall-through into the next position.
	fallArc stampArcKind = iota
	// uncondArc is the edge of an unconditional skip: instructions inserted
	// just before the skip execute exactly on that edge.
	uncondArc
	// condArc is a taken edge of a conditional (or multiway) skip: its source
	// cannot host edge-only instructions (they would also run on the
	// fall-through path), so equalisation lands in an edge block at the merge
	// point, and the skip is retargeted to it.
	condArc
)

// stampArc is one arc of the intra-row control-flow graph together with the
// symbolic state carried along it.
type stampArc struct {
	source, target uint
	// caseIdx identifies which case of a multiway (switch / dispatch) skip the
	// arc belongs to; zero otherwise.
	caseIdx int
	kind    stampArcKind
	state   stamps
}

// analyseStamps runs the dataflow analysis over one row, returning the joined
// symbolic state entering every position, the arcs into every position, and
// the skip-landing table.  All three carry one sentinel slot for the end of
// the row, which skips may land on and a live path may fall into.
func (t *threader[W]) analyseStamps(insns []Bytecode[W], entry stamps,
) (dfa.Result[stamps], [][]stampArc, []bool) {
	var (
		n     = len(insns)
		lands = t.skipLandings(insns)
		// codes extends the row by one nil sentinel, giving the end of the row
		// a position of its own in the analysis.
		codes    = make([]Bytecode[W], n+1)
		incoming = make([][]stampArc, n+1)
	)
	//
	copy(codes, insns)
	//
	transfer := func(offset uint, _ Bytecode[W], state stamps) []dfa.Transfer[stamps] {
		var (
			arcs = t.stampArcs(insns, lands, int(offset), state)
			out  = make([]dfa.Transfer[stamps], len(arcs))
		)
		//
		for j, a := range arcs {
			incoming[a.target] = append(incoming[a.target], a)
			out[j] = dfa.NewTransfer(a.state, a.target)
		}
		//
		return out
	}
	//
	return dfa.Construct(entry, codes, transfer), incoming, lands
}

// stampArcs is the transfer function of the analysis: it returns the arcs
// leaving position i, each carrying the symbolic state along it.  A bottom
// entry state denotes an unreachable position, which transfers nothing (its
// skips never execute).
func (t *threader[W]) stampArcs(insns []Bytecode[W], lands []bool, i int, entry stamps) []stampArc {
	if entry.isBottom() || i == len(insns) {
		return nil
	}
	// Equalise the incoming paths at a merge point.
	state := t.normaliseEntry(entry, lands[i], t.isExitAt(insns, i))
	//
	fall := func(st stamps) []stampArc {
		return []stampArc{{source: uint(i), target: uint(i + 1), kind: fallArc, state: st}}
	}
	//
	switch insn := insns[i].(type) {
	case *bytecode.Jmp[W], *bytecode.Ret[W], *bytecode.Fail[W]:
		// Control-flow terminators: no fall-through within the row.
		return nil
	case *bytecode.ReadWrite[W]:
		// The k-th movement executed advances the stamp to version k.
		if x := t.effectIndex(insn.Id); x >= 0 {
			state = state.with(x, state.vals[x].bump())
		}
		//
		return fall(state)
	case *bytecode.Call[W]:
		return fall(t.callArcState(insn, state))
	case *bytecode.Skip[W]:
		return []stampArc{{source: uint(i), target: uint(i) + 1 + uint(insn.Skip), kind: uncondArc, state: state}}
	case *bytecode.SkipIf[W]:
		state = t.branchState(insns, i, state)
		//
		return append([]stampArc{
			{source: uint(i), target: uint(i) + 1 + uint(insn.Skip), kind: condArc, state: state},
		}, fall(state)...)
	case *bytecode.Switch[W]:
		state = t.branchState(insns, i, state)
		arcs := make([]stampArc, 0, len(insn.Cases)+1)
		//
		for ci, c := range insn.Cases {
			arcs = append(arcs,
				stampArc{source: uint(i), target: uint(i) + 1 + uint(c.Skip), caseIdx: ci, kind: condArc, state: state})
		}
		//
		return append(arcs, fall(state)...)
	case *bytecode.Dispatch[W]:
		state = t.branchState(insns, i, state)
		arcs := make([]stampArc, 0, len(insn.Cases)+1)
		//
		for ci, c := range insn.Cases {
			arcs = append(arcs,
				stampArc{source: uint(i), target: uint(i) + 1 + uint(c.Skip), caseIdx: ci, kind: condArc, state: state})
		}
		//
		return append(arcs, fall(state)...)
	default:
		return fall(state)
	}
}

// normaliseEntry applies the merge-point discipline to the joined entry state
// of position i.  In SSA terms, equalising a merge point plays the role of
// inserting a phi node: each incoming path materialises the shared register
// with its own value (see mergeAt).  At a position with incoming skip edges
// the joined state stands as-is — the merged version's register is made good
// on every deficient path by mergeAt — except at an exit, where every path
// instead leaves the stamp in the canonical register, so the state entering
// the exit is canonical.  This is the single decision point shared by the
// analysis and the rewrite sweep, so the two cannot drift.
func (t *threader[W]) normaliseEntry(entry stamps, hasEdges, isExit bool) stamps {
	if !hasEdges || !isExit {
		return entry
	}
	//
	vals := slices.Clone(entry.vals)
	//
	for x, v := range vals {
		vals[x] = stampVersion{v: v.v, canon: true}
	}
	//
	return stamps{vals}
}

// branchState applies the branch-point treatment to the state carried by every
// arc leaving a conditional (or multiway) skip.  When the remainder of the row
// is a pure jump table (the if-goto / dispatch shape), the canonical stamps
// are materialised once, before the branch, covering every exit.  Otherwise
// the state passes through untouched: a version is a register on every path,
// so — unlike a base-plus-offset state — it needs no normalisation, and the
// guarded region's accesses are reconciled at its merge point by
// normaliseEntry / mergeAt.
func (t *threader[W]) branchState(insns []Bytecode[W], i int, state stamps) stamps {
	if !jumpTableFollows(insns, i) {
		return state
	}
	//
	vals := slices.Clone(state.vals)
	//
	for x, v := range vals {
		vals[x] = stampVersion{v: v.v, canon: true}
	}
	//
	return stamps{vals}
}

// callArcState applies a call's effect to the state: each read-write memory
// the callee declares advances to its next version, whose register receives
// the updated stamp directly from the call (see threadCall).
func (t *threader[W]) callArcState(call *bytecode.Call[W], state stamps) stamps {
	callee, ok := t.mods[call.Target].(*descriptor.Function[W])
	if !ok || callee.IsNative() {
		return state
	}
	// Effects are read-write memories by construction (linker-enforced).
	effects := callee.Effects()
	if len(effects) == 0 {
		return state
	}
	//
	vals := slices.Clone(state.vals)
	//
	for _, e := range effects {
		x := t.effectIndex(e)
		if x < 0 {
			// Guaranteed by the type-checker: callee effects are a subset of
			// the caller's.
			panic(fmt.Sprintf("caller lacks a stamp for memory %d", e))
		}
		// The callee's stamp is dynamic, so literality is lost.
		vals[x] = stampVersion{v: vals[x].v + 1}
	}
	//
	return stamps{vals}
}

// skipLandings returns, for each position (including the end-of-row sentinel),
// whether some skip edge from a reachable position lands there.  Edges in dead
// code are ignored: they never execute, so they carry no stamp.  Reachability
// needs no fixpoint since every intra-row edge is forward.
func (t *threader[W]) skipLandings(insns []Bytecode[W]) []bool {
	var (
		n     = len(insns)
		reach = make([]bool, n+1)
		lands = make([]bool, n+1)
	)
	//
	reach[0] = true
	//
	land := func(i int, skip uint16) {
		target := i + 1 + int(skip)
		reach[target] = true
		lands[target] = true
	}
	//
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		//
		switch insn := insns[i].(type) {
		case *bytecode.Jmp[W], *bytecode.Ret[W], *bytecode.Fail[W]:
		case *bytecode.Skip[W]:
			land(i, insn.Skip)
		case *bytecode.SkipIf[W]:
			land(i, insn.Skip)
			//
			reach[i+1] = true
		case *bytecode.Switch[W]:
			for _, c := range insn.Cases {
				land(i, c.Skip)
			}
			//
			reach[i+1] = true
		case *bytecode.Dispatch[W]:
			for _, c := range insn.Cases {
				land(i, c.Skip)
			}
			//
			reach[i+1] = true
		default:
			reach[i+1] = true
		}
	}
	//
	return lands
}

// isExitAt reports whether position i is an exit which must be entered with
// the stamps in their canonical registers on every path: a jump, a non-main
// return (main binds no stamp outputs), or the end of the row.  A fail
// aborts, so it does not force this.
func (t *threader[W]) isExitAt(insns []Bytecode[W], i int) bool {
	if i == len(insns) {
		return true
	}
	//
	switch insns[i].(type) {
	case *bytecode.Jmp[W]:
		return true
	case *bytecode.Ret[W]:
		return !t.isMain
	}
	//
	return false
}

// effectIndex returns the position of the given memory in this function's
// threaded effects, or -1 when it is not threaded (e.g. a read-only memory).
func (t *threader[W]) effectIndex(e descriptor.ModuleId) int {
	for x, eff := range t.effects {
		if eff == e {
			return x
		}
	}
	//
	return -1
}
