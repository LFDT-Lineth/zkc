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
// one row, where each effect's stamp symbolically lives.  The rewrite sweep in
// thread_timestamps.go then consumes its results.  The analysis cannot
// allocate registers, so bases created by the threading itself (call returns,
// normalisation and merge temporaries) are identified by the position defining
// them rather than by a register id; the sweep resolves them in the same
// forward order, guaranteeing every base is bound before it is referenced.

// stampBaseKind enumerates the symbolic bases a stamp value can be counted
// from.
type stampBaseKind uint8

const (
	// stampLiteral denotes no base register: the stamp is the literal offset
	// (the entry state of "main", which counts from one).
	stampLiteral stampBaseKind = iota
	// stampInput denotes a stamp-in input register, known before the analysis.
	stampInput
	// stampCanonical denotes the effect's canonical stamp register (the
	// stamp-out output or, for main, a lazily-allocated computed register).
	stampCanonical
	// stampCallOut denotes the temporary receiving the effect's updated stamp
	// from the call at the defining position.
	stampCallOut
	// stampNorm denotes the temporary created by branch-point normalisation at
	// the defining position.
	stampNorm
	// stampMerge denotes the merge register allocated at the defining (landing)
	// position.
	stampMerge
	// stampConflict is the join of disagreeing values (top): the incoming
	// paths carry distinct stamps, which the landing equalises into a merge
	// register.
	stampConflict
)

// stampValue records, symbolically, where the current timestamp of one memory
// lives at a given program point: its value is base + off.  It is the analysis
// counterpart of stampState, which the rewrite sweep obtains by resolving the
// base onto a concrete register.
type stampValue struct {
	kind stampBaseKind
	// reg is the base register (stampInput only).
	reg bytecode.RegisterId
	// pc is the position defining the base (stampCallOut / stampNorm /
	// stampMerge only).
	pc uint
	// off is the constant offset from the base.
	off uint64
}

// bump returns the value advanced by one access.
func (v stampValue) bump() stampValue {
	v.off++
	//
	return v
}

// isPlainRegister reports whether the value is held directly in some register:
// a known base at offset zero.
func (v stampValue) isPlainRegister() bool {
	return v.kind != stampLiteral && v.kind != stampConflict && v.off == 0
}

// String returns a debug rendering of this value.
func (v stampValue) String() string {
	switch v.kind {
	case stampLiteral:
		return fmt.Sprintf("%d", v.off)
	case stampInput:
		return fmt.Sprintf("r%d+%d", v.reg, v.off)
	case stampCanonical:
		return fmt.Sprintf("canon+%d", v.off)
	case stampCallOut:
		return fmt.Sprintf("call@%d+%d", v.pc, v.off)
	case stampNorm:
		return fmt.Sprintf("norm@%d+%d", v.pc, v.off)
	case stampMerge:
		return fmt.Sprintf("merge@%d+%d", v.pc, v.off)
	default:
		return "⊤"
	}
}

// stamps is the dataflow state of the timestamp analysis: one symbolic value
// per threaded effect, in effect order.  The zero value (nil vals) is bottom,
// i.e. an unreachable position.
type stamps struct {
	vals []stampValue
}

// isBottom reports whether this state denotes an unreachable position.
func (p stamps) isBottom() bool {
	return p.vals == nil
}

// with returns a copy of this state with the given effect's value replaced.
func (p stamps) with(x int, v stampValue) stamps {
	vals := slices.Clone(p.vals)
	vals[x] = v
	//
	return stamps{vals}
}

// Join implementation for the dfa.State interface: values agreeing on both
// paths pass through, disagreeing values become stampConflict.
func (p stamps) Join(o stamps) stamps {
	vals := make([]stampValue, len(p.vals))
	//
	for x := range vals {
		if p.vals[x] == o.vals[x] {
			vals[x] = p.vals[x]
		} else {
			vals[x] = stampValue{kind: stampConflict}
		}
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
	state, _ := t.normaliseEntry(i, entry, lands[i], t.isExitAt(insns, i))
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
		// The k-th access executed carries stamp_in + k.
		if x := t.effectIndex(insn.Id); x >= 0 {
			state = state.with(x, state.vals[x].bump())
		}
		//
		return fall(state)
	case *bytecode.Call[W]:
		// A never-returning call is a control-flow terminator: no fall-through,
		// and no updated stamp is received (see threadCall).
		if insn.Never {
			return nil
		}
		//
		return fall(t.callArcState(uint(i), insn, state))
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
// inserting a phi node: each incoming path materialises the shared merge
// register with its own value (see mergeAt).  An effect needs equalising when
// its incoming states disagree
// (stampConflict), when the shared state is not a plain register (a stamp
// consumer at this position inserts on the fall path only, which skip edges
// land past), or -- at an exit -- when it is not already canonical (the exit's
// own insertions are likewise fall-only).  Flagged effects are left in the
// merge register allocated at this position, or the canonical register at an
// exit.  Positions without incoming skip edges need no equalising.  This is
// the single decision point shared by the analysis and the rewrite sweep, so
// the two cannot drift.
func (t *threader[W]) normaliseEntry(i int, entry stamps, hasEdges, isExit bool) (stamps, []bool) {
	need := make([]bool, len(entry.vals))
	//
	if !hasEdges {
		return entry, need
	}
	//
	vals := slices.Clone(entry.vals)
	//
	for x, v := range vals {
		switch {
		case v.kind == stampConflict:
			need[x] = true
		case !v.isPlainRegister():
			need[x] = true
		case isExit && v != (stampValue{kind: stampCanonical}):
			need[x] = true
		default:
			continue
		}
		//
		if isExit {
			vals[x] = stampValue{kind: stampCanonical}
		} else {
			vals[x] = stampValue{kind: stampMerge, pc: uint(i)}
		}
	}
	//
	return stamps{vals}, need
}

// branchState applies the branch-point treatment to the state carried by every
// arc leaving a conditional (or multiway) skip.  When the remainder of the row
// is a pure jump table (the if-goto / dispatch shape), the canonical stamps
// cover every exit; otherwise the state is normalised to a plain register so
// all edges carry the same symbolic value, and the guarded region's accesses
// are reconciled at its merge point by normaliseEntry.
func (t *threader[W]) branchState(insns []Bytecode[W], i int, state stamps) stamps {
	vals := slices.Clone(state.vals)
	//
	if jumpTableFollows(insns, i) {
		for x := range vals {
			vals[x] = stampValue{kind: stampCanonical}
		}
	} else {
		for x, v := range vals {
			if !v.isPlainRegister() {
				vals[x] = stampValue{kind: stampNorm, pc: uint(i)}
			}
		}
	}
	//
	return stamps{vals}
}

// callArcState applies a call's effect to the state: each read-write memory
// the callee declares leaves its updated stamp in the temporary received from
// the call.
func (t *threader[W]) callArcState(pc uint, call *bytecode.Call[W], state stamps) stamps {
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
		//
		vals[x] = stampValue{kind: stampCallOut, pc: pc}
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
		case *bytecode.Call[W]:
			// A never-returning call is a control-flow terminator.
			if !insn.Never {
				reach[i+1] = true
			}
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

// isExitAt reports whether position i is an exit which must be entered with the
// stamps in their canonical registers on every path: a jump, a non-main return
// (main binds no stamp outputs), or the end of the row.  A fail aborts, so it
// does not force this.
func (t *threader[W]) isExitAt(insns []Bytecode[W], i int) bool {
	if i == len(insns) {
		return true
	}
	//
	switch b := insns[i].(type) {
	case *bytecode.Jmp[W]:
		return true
	case *bytecode.Ret[W]:
		return !t.isMain && !b.Done
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
