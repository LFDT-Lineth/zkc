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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// stampWidth is the bit-width of a timestamp register.
//
// TODO: source this per-memory from a (not-yet-implemented) stamp-width syntax
// "memory data(u48; addr:u16) -> ..." (issue #2069) rather than using a single
// global default.
const stampWidth uint = 32

// ThreadTimestamps threads a per-memory timestamp through every function which
// declares a read-write memory effect.  For each such function and read-write
// memory M it may access, it:
//
//   - adds an input register "M$stamp" and an output register "M$stamp_out"
//     (the stamp flowing in and back out), placed first in the inputs /
//     outputs.  The entry function "main" is special: it takes no parameters,
//     so it gets no stamp in/out and instead counts from ONE — timestamp zero
//     is reserved for the initial state of an untouched memory cell, so the
//     memory table's ordering constraint (timestamp-read < timestamp-written)
//     holds on the very first access.
//   - forwards the stamp across every call to an effectful callee (by
//     prepending it to the call's arguments and returns);
//   - gives every memory access a distinct timestamp: the k-th access executed
//     carries stamp_in + k, recorded in the access's Stamp operand.
//
// The threading is OFFSET-BASED: rather than incrementing a working register
// once per access (which would conflict when a single row performs several
// accesses), the transform tracks, at every program point, which register
// holds the current stamp together with a constant offset ("the current stamp
// is base + off").  An access at offset zero consumes its base register
// directly, at no instruction cost; a later access on the same row consumes a
// fresh temporary "t = base + off".  The canonical stamp register (the
// M$stamp_out output, or a fresh computed register in main) is written at most
// once per executed path through a row: at a return, before a jump, or when
// falling into the next row.  In particular the common one-line function
//
//	fn f<ram>(x:u16) -> (r:u8) { r = ram[x] }
//
// costs exactly one added instruction:
//
//	read r = ram[ram$stamp; x]
//	add ram$stamp_out = ram$stamp + 1
//
// This is required only for tracing and constraint generation (the run-time
// memory maintains its own clock), so it is applied on the constraint path
// only.  It runs AFTER vectorisation — so a vector is genuinely one trace row,
// and the canonical register is written at most once per executed path through
// it (a second write would have blocked the vectoriser from forming the row in
// the first place, forcing one-line functions into multi-line form) — and
// before register splitting, so the wide stamp registers and their arithmetic
// are split into limbs and range-checked like any other register.
func ThreadTimestamps[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		out  = make([]descriptor.Module[W], len(mods))
	)
	//
	copy(out, mods)
	//
	for i, mod := range mods {
		fn, ok := mod.(*descriptor.Function[W])
		if !ok || fn.IsNative() {
			continue
		}
		//
		effects := rwMemoryEffects(mods, fn)
		if len(effects) == 0 {
			continue
		}
		//
		out[i] = threadFunction(mods, fn, effects)
	}
	//
	return descriptor.NewProgram(program.Field(), out...)
}

// rwMemoryEffects returns the module ids of the read-write memories declared as
// effects of the given function, preserving declaration order.  Effects naming
// a read-only / write-once memory (which need no timestamp) are dropped.
func rwMemoryEffects[W word.Word[W]](mods []descriptor.Module[W],
	fn *descriptor.Function[W]) []descriptor.ModuleId {
	//
	var out []descriptor.ModuleId
	//
	for _, id := range fn.Effects() {
		if mem, ok := mods[id].(*descriptor.Memory[W]); ok && mem.IsReadWrite() {
			out = append(out, id)
		}
	}
	//
	return out
}

// stampState records, symbolically, where the current timestamp of one memory
// lives at a given program point: its value is base + off.  An empty base
// means the literal value off (the entry state of "main", which counts from
// one).
type stampState struct {
	base util.Option[bytecode.RegisterId]
	off  uint64
}

// equals reports whether two states denote the same symbolic value.
func (s stampState) equals(o stampState) bool {
	return s.base == o.base && s.off == o.off
}

// bump returns the state advanced by one access.
func (s stampState) bump() stampState {
	return stampState{s.base, s.off + 1}
}

// threader carries the per-function context of the threading walk.
type threader[W word.Word[W]] struct {
	mods    []descriptor.Module[W]
	alloc   split.Allocator[W]
	effects []descriptor.ModuleId
	// stampIn maps each effect to its stamp-in input register (absent for
	// main, whose stamps count from literal one).
	stampIn map[descriptor.ModuleId]bytecode.RegisterId
	// canonical maps each effect to its canonical stamp register: the
	// stamp-out output register or, for main, a lazily-allocated computed
	// register.  Every row (vector) is entered with the current stamp held in
	// the canonical register at offset zero, except the entry row.
	canonical map[descriptor.ModuleId]bytecode.RegisterId
	isMain    bool
}

// threadFunction rewrites a single function to thread a timestamp for each of
// the given read-write memory effects.
func threadFunction[W word.Word[W]](mods []descriptor.Module[W], fn *descriptor.Function[W],
	effects []descriptor.ModuleId) *descriptor.Function[W] {
	//
	var (
		isMain  = fn.Name() == "main"
		k       = uint(len(effects))
		oldRegs = fn.Registers()
		ni      = fn.NumInputs()
		padding W
		newRegs []descriptor.Register[W]
		t       = &threader[W]{
			mods:      mods,
			effects:   effects,
			stampIn:   map[descriptor.ModuleId]bytecode.RegisterId{},
			canonical: map[descriptor.ModuleId]bytecode.RegisterId{},
			isMain:    isMain,
		}
	)
	// New register layout, stamps first: [stamp-ins, old inputs, stamp-outs,
	// old outputs, old computed].  main takes no parameters, so it gets no
	// stamp in/out; its canonical registers are allocated lazily below.
	if !isMain {
		for _, e := range effects {
			t.stampIn[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.INPUT_REGISTER, stampName(mods, e),
				util.Some(stampWidth), padding))
		}
	}
	//
	newRegs = append(newRegs, oldRegs[:ni]...)
	//
	if !isMain {
		for _, e := range effects {
			t.canonical[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.OUTPUT_REGISTER, stampOutName(mods, e),
				util.Some(stampWidth), padding))
		}
	}
	//
	newRegs = append(newRegs, oldRegs[ni:]...)
	// Old->new register id remap: inserting k stamp inputs shifts old inputs
	// by k; inserting k stamp outputs shifts old outputs and computed by a
	// further k.  main inserts none, so the map is the identity there.
	sub := make([]bytecode.RegisterId, len(oldRegs))
	//
	for x := range oldRegs {
		id := bytecode.RegisterId(x)
		//
		switch {
		case isMain:
			sub[x] = id
		case uint(x) < ni:
			sub[x] = id + bytecode.RegisterId(k)
		default:
			sub[x] = id + bytecode.RegisterId(2*k)
		}
	}
	// Remap the body onto the new register ids.
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
	)
	//
	for vi, vec := range vectors {
		nvecs[vi] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			return []Bytecode[W]{substituteRegisters[W](insn, sub)}
		})
	}
	// Allocate temporaries against the remapped register set.
	remapped := descriptor.NewFunction(fn.Name(), newRegs, fn.Kind(), fn.Effects(), nvecs)
	t.alloc = split.NewAllocator[W](remapped)
	// Determine the initial symbolic state of each effect's stamp: the
	// stamp-in register or, for main, the literal ONE (timestamp zero is
	// reserved for the initial state of an untouched cell, keeping the memory
	// table's strict timestamp ordering satisfiable on the first access).
	entry := map[descriptor.ModuleId]stampState{}
	//
	for _, e := range effects {
		if isMain {
			entry[e] = stampState{util.None[bytecode.RegisterId](), 1}
		} else {
			entry[e] = stampState{util.Some(t.stampIn[e]), 0}
		}
	}
	// The walk assumes every jump lands on a row entered with canonical
	// stamps.  If the entry row is itself a jump target (e.g. the function
	// begins with a loop header), the entry state must be materialised into
	// the canonical registers on a preamble row of its own, and every jump
	// target shifted by one.
	if jumpTargets(nvecs).Contains(0) {
		var seed []Bytecode[W]
		//
		for _, e := range effects {
			seed = append(seed, t.materialise(entry[e], t.getCanonical(e)))
			entry[e] = t.canonicalState(e)
		}
		//
		nvecs = append([]BytecodeVector[W]{bytecode.NewVector(seed...)}, nvecs...)
		//
		for vi, vec := range nvecs {
			nvecs[vi] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
				if jmp, ok := insn.(*bytecode.Jmp[W]); ok {
					return []Bytecode[W]{bytecode.Jump[W](jmp.Target + 1)}
				}
				//
				return []Bytecode[W]{insn}
			})
		}
	}
	// Thread each row.  Every row after the first is entered with canonical
	// stamps (the walk materialises them at each row boundary).
	for vi := range nvecs {
		states := map[descriptor.ModuleId]stampState{}
		//
		for _, e := range effects {
			if vi == 0 {
				states[e] = entry[e]
			} else {
				states[e] = t.canonicalState(e)
			}
		}
		//
		nvecs[vi] = t.threadVector(nvecs[vi], states)
	}
	//
	return descriptor.NewFunction(fn.Name(), t.alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

// canonicalState returns the state "the canonical register, at offset zero".
func (t *threader[W]) canonicalState(e descriptor.ModuleId) stampState {
	return stampState{util.Some(t.getCanonical(e)), 0}
}

// getCanonical returns the canonical stamp register of the given effect,
// allocating it on first use for main (whose canonical registers are ordinary
// computed registers rather than outputs).
func (t *threader[W]) getCanonical(e descriptor.ModuleId) bytecode.RegisterId {
	if r, ok := t.canonical[e]; ok {
		return r
	}
	//
	r := t.alloc.AllocateNamed(stampName(t.mods, e), util.Some(stampWidth))
	t.canonical[e] = r
	//
	return r
}

// materialise returns an instruction storing the symbolic stamp value into the
// given register.
func (t *threader[W]) materialise(s stampState, target bytecode.RegisterId) Bytecode[W] {
	var constant W
	//
	constant = constant.SetUint64(s.off)
	//
	switch {
	case s.base.IsEmpty():
		return bytecode.LoadConst(target, constant)
	case s.off == 0:
		return bytecode.Assign[W](target, s.base.Unwrap())
	default:
		return bytecode.AddConst(target, []bytecode.RegisterId{s.base.Unwrap()}, constant)
	}
}

// stampRegister returns a register holding the symbolic stamp value, together
// with the (possibly empty) instructions computing it: the base register
// itself when the offset is zero, and a fresh temporary otherwise.
func (t *threader[W]) stampRegister(s stampState) (bytecode.RegisterId, []Bytecode[W]) {
	if s.base.HasValue() && s.off == 0 {
		return s.base.Unwrap(), nil
	}
	//
	tmp := t.alloc.Allocate("stamp", util.Some(stampWidth))
	//
	return tmp, []Bytecode[W]{t.materialise(s, tmp)}
}

// edgeKind classifies how a forward (skip) edge can be equalised at a merge
// point.
type edgeKind int

const (
	// uncondEdge is the edge of an unconditional skip: instructions inserted
	// just before the skip execute exactly on that edge.
	uncondEdge edgeKind = iota
	// condEdge is the taken edge of a conditional skip: its source cannot host
	// edge-only instructions (they would also run on the fall-through path),
	// so equalisation lands in an edge block at the merge point, and the skip
	// is retargeted to it.
	condEdge
)

// pendingState records a symbolic state carried into a position by a forward
// (skip) edge, together with the index of the skip instruction it left and —
// for a multiway (switch / dispatch) skip — which of its cases it is.
type pendingState[W word.Word[W]] struct {
	states  map[descriptor.ModuleId]stampState
	source  int
	caseIdx int
	kind    edgeKind
}

// rowInserts collects the instructions materialised around one original
// instruction position during the walk.
type rowInserts[W word.Word[W]] struct {
	// fall executes on the fall-through path only: skips landing here land
	// past it.
	fall []Bytecode[W]
	// edge is the edge block: conditional skip edges retargeted to this
	// position land at its start and run it before continuing.
	edge []Bytecode[W]
	// skipOver indicates a live fall-through path enters this position, which
	// must then jump over the edge block.
	skipOver bool
	// retargets lists the (source index, case index) pairs of the conditional
	// skip edges whose landing moves to the start of the edge block.
	retargets [][2]int
}

// threadVector rewrites one row: it assigns every read-write memory access its
// stamp operand, threads calls to effectful callees, and materialises the
// canonical stamp registers at every exit (return, jump, or fall-through into
// the next row).  The walk is a forward scan over the row's instructions,
// tracking the symbolic stamp states along the fall-through path and across
// intra-row skip edges (conditional regions and the skip-based control flow
// the vectoriser builds when merging lines).
func (t *threader[W]) threadVector(vec BytecodeVector[W], states map[descriptor.ModuleId]stampState,
) BytecodeVector[W] {
	var (
		insns = vec.Bytecodes
		n     = len(insns)
		// inserts[i] = instructions materialised around original instruction i
		// (i == n denotes the end of the row).
		inserts = map[int]*rowInserts[W]{}
		// replace[i] = rebuilt instruction at original index i.
		replace = map[int]Bytecode[W]{}
		// pending[i] = states carried into original index i by skip edges.
		pending = map[int][]pendingState[W]{}
		// live distinguishes a reachable fall-through path from positions only
		// reachable via skip edges.
		live = true
	)
	//
	at := func(i int) *rowInserts[W] {
		if inserts[i] == nil {
			inserts[i] = &rowInserts[W]{}
		}
		//
		return inserts[i]
	}
	//
	for i := 0; i <= n; i++ {
		// Merge skip-carried states into the fall-through state.
		states, live = t.mergeAt(insns, i, states, live, pending, at)
		//
		if i == n {
			// A live path at the end of the row falls into the next row, which
			// is entered with canonical stamps.
			if live {
				at(n).fall = append(at(n).fall, t.canonicalise(states)...)
			}
			//
			continue
		}
		//
		if !live {
			continue
		}
		//
		switch insn := insns[i].(type) {
		case *bytecode.ReadWrite[W]:
			if s, ok := states[insn.Id]; ok {
				reg, pre := t.stampRegister(s)
				at(i).fall = append(at(i).fall, pre...)
				replace[i] = withStamp(insn, reg)
				states[insn.Id] = s.bump()
			}
		case *bytecode.Call[W]:
			if rebuilt, pre := t.threadCall(insn, states); rebuilt != nil {
				at(i).fall = append(at(i).fall, pre...)
				replace[i] = rebuilt
			}
		case *bytecode.Jmp[W]:
			// The jump target is entered with canonical stamps.
			at(i).fall = append(at(i).fall, t.canonicalise(states)...)
			live = false
		case *bytecode.Ret[W]:
			// Bind the stamp-out outputs (main has none and returns nothing).
			if !t.isMain {
				at(i).fall = append(at(i).fall, t.canonicalise(states)...)
			}

			live = false
		case *bytecode.Fail[W]:
			live = false
		case *bytecode.Skip[W]:
			// Unconditional forward edge: the state flows to the landing; the
			// position before the skip can host edge-only instructions.
			t.recordEdge(pending, i, 0, int(insn.Skip), states, uncondEdge)

			live = false
		case *bytecode.SkipIf[W]:
			states = t.threadBranch(insns, i, int(insn.Skip), states, pending, at)
		case *bytecode.Switch[W]:
			for ci, c := range insn.Cases {
				states = t.threadBranchCase(insns, i, ci, int(c.Skip), states, pending, at)
			}
		case *bytecode.Dispatch[W]:
			for ci, c := range insn.Cases {
				states = t.threadBranchCase(insns, i, ci, int(c.Skip), states, pending, at)
			}
		}
	}
	// Rebuild the row, recomputing intra-row skip amounts around insertions.
	return rebuildVector(insns, inserts, replace)
}

// mergeAt reconciles the fall-through state with any skip-carried states at
// position i.  An effect needs equalising when its incoming states disagree,
// or when the merged state is not a plain register (a later access at this
// position must consume a register every incoming path has computed).  Each
// incoming edge then materialises the merge register at a position executed
// exactly on that edge: inline for the fall-through, before the skip for an
// unconditional edge, and in an edge block at the landing for a conditional
// edge.  When the merge point is an exit (return, jump, or the end of the
// row), the merge register is the canonical stamp itself, so the exit needs no
// further materialisation on any path.
func (t *threader[W]) mergeAt(insns []Bytecode[W], i int, states map[descriptor.ModuleId]stampState,
	live bool, pending map[int][]pendingState[W], at func(int) *rowInserts[W],
) (map[descriptor.ModuleId]stampState, bool) {
	//
	incoming := pending[i]
	if len(incoming) == 0 {
		return states, live
	}
	// Conditional edges landing here share a single edge block, so they must
	// all carry the same states.
	var conditional *pendingState[W]
	//
	for p := range incoming {
		if incoming[p].kind != condEdge {
			continue
		}
		//
		if conditional == nil {
			conditional = &incoming[p]
		} else {
			for _, e := range t.effects {
				if !conditional.states[e].equals(incoming[p].states[e]) {
					panic("timestamp threading: conditional skip edges with distinct stamps " +
						"at one merge point (unsupported control flow shape)")
				}
			}
		}
	}
	// When the fall path is dead, adopt the first edge's states as the
	// reference (cloned: the pending entries must stay intact for the per-edge
	// equalisation below).
	if !live {
		states = map[descriptor.ModuleId]stampState{}
		//
		for e, s := range incoming[0].states {
			states[e] = s
		}
	}
	// A merge point at an exit must leave every stamp in its canonical
	// register: instructions the exit would otherwise insert live on the
	// fall-through path only, which skip edges land past.  main's return
	// binds no stamp outputs (and a fail aborts), so neither forces this.
	var force bool
	//
	switch {
	case i == len(insns):
		force = true
	default:
		switch insns[i].(type) {
		case *bytecode.Jmp[W]:
			force = true
		case *bytecode.Ret[W]:
			force = !t.isMain
		}
	}
	//
	for _, e := range t.effects {
		var (
			s = states[e]
			// A merge point must be left holding a plain register: a stamp
			// consumer at this position inserts on the fall path only, which
			// skip edges land past.
			need = s.off != 0 || s.base.IsEmpty()
		)
		//
		if force && !s.equals(t.canonicalState(e)) {
			need = true
		}
		//
		for _, p := range incoming {
			if !p.states[e].equals(s) {
				need = true
			}
		}
		//
		if !need {
			continue
		}
		// Choose the merge register.
		var target bytecode.RegisterId
		//
		if force {
			target = t.getCanonical(e)
		} else {
			target = t.alloc.Allocate("stamp", util.Some(stampWidth))
		}
		//
		merged := stampState{util.Some(target), 0}
		//
		if live && !s.equals(merged) {
			at(i).fall = append(at(i).fall, t.materialise(s, target))
		}
		//
		for _, p := range incoming {
			ps := p.states[e]
			//
			if ps.equals(merged) {
				continue
			}
			//
			switch p.kind {
			case uncondEdge:
				at(p.source).fall = append(at(p.source).fall, t.materialise(ps, target))
			case condEdge:
				// One materialisation per effect in the shared edge block.
				if conditional != nil && p.source == conditional.source {
					at(i).edge = append(at(i).edge, t.materialise(ps, target))
				}
			}
		}
		//
		states[e] = merged
	}
	// Retarget every conditional edge to the edge block (when one exists), and
	// have a live fall path jump over it.
	if len(at(i).edge) != 0 {
		for _, p := range incoming {
			if p.kind == condEdge {
				at(i).retargets = append(at(i).retargets, [2]int{p.source, p.caseIdx})
			}
		}
		//
		at(i).skipOver = live
	}
	//
	return states, true
}

// threadBranch handles a conditional skip.  When the remainder of the row is a
// pure jump table (the if-goto / dispatch shape), the canonical stamps are
// materialised once, before the branch, covering every exit at minimal cost.
// Otherwise (a conditionally-executed region, e.g. a ternary arm or a
// vectorised if-body) the branch's state is normalised to offset zero so both
// edges carry the same symbolic value, and the region's accesses are
// reconciled at its merge point by mergeAt.
func (t *threader[W]) threadBranch(insns []Bytecode[W], i, skip int,
	states map[descriptor.ModuleId]stampState, pending map[int][]pendingState[W],
	at func(int) *rowInserts[W]) map[descriptor.ModuleId]stampState {
	//
	if jumpTableFollows(insns, i) {
		at(i).fall = append(at(i).fall, t.canonicalise(states)...)
		//
		for _, e := range t.effects {
			states[e] = t.canonicalState(e)
		}
		//
		t.recordEdge(pending, i, 0, skip, states, condEdge)
		//
		return states
	}
	// Normalise pending offsets so the bypass edge carries a plain register
	// (accesses inside the guarded region then count from zero).
	for _, e := range t.effects {
		if s := states[e]; s.base.IsEmpty() || s.off != 0 {
			tmp := t.alloc.Allocate("stamp", util.Some(stampWidth))
			at(i).fall = append(at(i).fall, t.materialise(s, tmp))
			states[e] = stampState{util.Some(tmp), 0}
		}
	}
	//
	t.recordEdge(pending, i, 0, skip, states, condEdge)
	//
	return states
}

// threadBranchCase records one case edge of a multiway (switch / dispatch)
// skip, sharing threadBranch's jump-table and normalisation treatments (both
// idempotent across the cases of one instruction: after the first case the
// states are already plain registers).
func (t *threader[W]) threadBranchCase(insns []Bytecode[W], i, caseIdx, skip int,
	states map[descriptor.ModuleId]stampState, pending map[int][]pendingState[W],
	at func(int) *rowInserts[W]) map[descriptor.ModuleId]stampState {
	//
	if jumpTableFollows(insns, i) {
		at(i).fall = append(at(i).fall, t.canonicalise(states)...)
		//
		for _, e := range t.effects {
			states[e] = t.canonicalState(e)
		}
	} else {
		for _, e := range t.effects {
			if s := states[e]; s.base.IsEmpty() || s.off != 0 {
				tmp := t.alloc.Allocate("stamp", util.Some(stampWidth))
				at(i).fall = append(at(i).fall, t.materialise(s, tmp))
				states[e] = stampState{util.Some(tmp), 0}
			}
		}
	}
	//
	t.recordEdge(pending, i, caseIdx, skip, states, condEdge)
	//
	return states
}

// recordEdge registers a forward skip edge from instruction i (case caseIdx of
// a multiway skip; zero otherwise) with the given skip amount, carrying the
// current states.
func (t *threader[W]) recordEdge(pending map[int][]pendingState[W], i, caseIdx, skip int,
	states map[descriptor.ModuleId]stampState, kind edgeKind) {
	//
	var (
		landing = i + 1 + skip
		copied  = map[descriptor.ModuleId]stampState{}
	)
	//
	for e, s := range states {
		copied[e] = s
	}
	//
	pending[landing] = append(pending[landing], pendingState[W]{copied, i, caseIdx, kind})
}

// canonicalise returns the instructions materialising every effect's state
// into its canonical register, skipping effects already canonical.
func (t *threader[W]) canonicalise(states map[descriptor.ModuleId]stampState) []Bytecode[W] {
	var out []Bytecode[W]
	//
	for _, e := range t.effects {
		s := states[e]
		//
		if s.equals(t.canonicalState(e)) {
			continue
		}
		//
		out = append(out, t.materialise(s, t.getCanonical(e)))
	}
	//
	return out
}

// threadCall rewrites a call to an effectful callee: for each read-write
// memory the callee declares, the caller's current stamp is passed as a
// (prepended) argument and the updated stamp received into a fresh temporary.
// Returns nil when the callee needs no threading.
func (t *threader[W]) threadCall(call *bytecode.Call[W], states map[descriptor.ModuleId]stampState,
) (Bytecode[W], []Bytecode[W]) {
	//
	callee, ok := t.mods[call.Target].(*descriptor.Function[W])
	if !ok || callee.IsNative() {
		return nil, nil
	}
	//
	effects := rwMemoryEffects(t.mods, callee)
	if len(effects) == 0 {
		return nil, nil
	}
	//
	var (
		pre     []Bytecode[W]
		args    = make([]bytecode.RegisterId, len(effects))
		returns = make([]bytecode.RegisterId, len(effects))
	)
	//
	for i, e := range effects {
		s, ok := states[e]
		if !ok {
			// Guaranteed by the type-checker: callee effects are a subset of
			// the caller's.
			panic(fmt.Sprintf("caller lacks a stamp for memory %d", e))
		}
		//
		reg, insns := t.stampRegister(s)
		pre = append(pre, insns...)
		args[i] = reg
		// Receive the callee's updated stamp into a fresh temporary.
		returns[i] = t.alloc.Allocate("stamp", util.Some(stampWidth))
		states[e] = stampState{util.Some(returns[i]), 0}
	}
	//
	return bytecode.CallFun[W](call.Target,
		append(args, call.Arguments...),
		append(returns, call.Returns...)), pre
}

// withStamp rebuilds a memory access with the given stamp operand.
func withStamp[W word.Word[W]](rw *bytecode.ReadWrite[W], stamp bytecode.RegisterId) Bytecode[W] {
	if rw.Write {
		return bytecode.NewMemWrite[W](rw.Id, rw.Address, rw.Data, []bytecode.RegisterId{stamp})
	}
	//
	return bytecode.NewMemRead[W](rw.Id, rw.Address, rw.Data, []bytecode.RegisterId{stamp})
}

// jumpTableFollows reports whether every instruction after index i is an
// unconditional jump — the shape the front end emits for if-goto conditions
// and dispatch tables, whose skip edges select which jump runs.
func jumpTableFollows[W word.Word[W]](insns []Bytecode[W], i int) bool {
	for _, insn := range insns[i+1:] {
		if _, ok := insn.(*bytecode.Jmp[W]); !ok {
			return false
		}
	}
	//
	return i+1 < len(insns)
}

// jumpTargets returns the set of vector indices targeted by some jump.
func jumpTargets[W word.Word[W]](vectors []BytecodeVector[W]) *targetSet {
	set := &targetSet{map[uint32]bool{}}
	//
	for _, vec := range vectors {
		for _, insn := range vec.Bytecodes {
			if jmp, ok := insn.(*bytecode.Jmp[W]); ok {
				set.targets[jmp.Target] = true
			}
		}
	}
	//
	return set
}

// targetSet is a set of jump-target vector indices.
type targetSet struct{ targets map[uint32]bool }

// Contains reports whether the given vector index is a jump target.
func (p *targetSet) Contains(v uint32) bool { return p.targets[v] }

// rebuildVector reassembles a row from its original instructions, the
// materialisations around each original index, and the per-index replacements
// — recomputing every intra-row skip amount around the insertions.  Around
// each original position the layout is [fall..., (skip-over), edge block...,
// instruction]: fall instructions execute on the fall-through path only
// (ordinary skips land past them), conditional skips retargeted to this
// position land at the start of the edge block, and a live fall-through path
// jumps over the block.
func rebuildVector[W word.Word[W]](insns []Bytecode[W], inserts map[int]*rowInserts[W],
	replace map[int]Bytecode[W]) BytecodeVector[W] {
	//
	var (
		n = len(insns)
		// insnPos[i] = index of original instruction i in the rebuilt row;
		// insnPos[n] = final length.
		insnPos = make([]int, n+1)
		// edgeStart[i] = index of position i's edge block in the rebuilt row.
		edgeStart = make([]int, n+1)
		// retargeted maps the (source index, case index) of a retargeted
		// conditional skip edge to its landing position.
		retargeted = map[[2]int]int{}
		out        []Bytecode[W]
	)
	//
	for i := 0; i <= n; i++ {
		if ri := inserts[i]; ri != nil {
			out = append(out, ri.fall...)
			//
			if len(ri.edge) != 0 && ri.skipOver {
				out = append(out, bytecode.NewSkip[W](uint16(len(ri.edge))))
			}
			//
			edgeStart[i] = len(out)
			out = append(out, ri.edge...)
			//
			for _, s := range ri.retargets {
				retargeted[s] = i
			}
		} else {
			edgeStart[i] = len(out)
		}
		//
		insnPos[i] = len(out)
		//
		if i == n {
			break
		}
		//
		if r, ok := replace[i]; ok {
			out = append(out, r)
		} else {
			out = append(out, insns[i])
		}
	}
	// Recompute skip amounts: the edge of case c of the skip at original index
	// i, with amount k, lands at original index i+1+k — past that position's
	// fall (and edge) instructions — except when retargeted, in which case it
	// lands at the start of its landing's edge block.  (Plain and conditional
	// skips are their own single case, index zero.)
	newSkip := func(i, c, k int) uint16 {
		landing := insnPos[i+1+k]
		//
		if l, ok := retargeted[[2]int{i, c}]; ok {
			landing = edgeStart[l]
		}
		//
		return uint16(landing - insnPos[i] - 1)
	}
	//
	for i := 0; i < n; i++ {
		switch insn := insns[i].(type) {
		case *bytecode.Skip[W]:
			out[insnPos[i]] = bytecode.NewSkip[W](newSkip(i, 0, int(insn.Skip)))
		case *bytecode.SkipIf[W]:
			out[insnPos[i]] = bytecode.NewSkipIf[W](insn.Op, newSkip(i, 0, int(insn.Skip)), insn.Left, insn.Right)
		case *bytecode.Switch[W]:
			cases := make([]bytecode.SwitchCase[W], len(insn.Cases))
			//
			for c, sc := range insn.Cases {
				cases[c] = bytecode.SwitchCase[W]{Value: sc.Value, Skip: newSkip(i, c, int(sc.Skip))}
			}
			//
			out[insnPos[i]] = bytecode.MultiwaySkip(insn.Source, cases)
		case *bytecode.Dispatch[W]:
			cases := make([]bytecode.DispatchCase, len(insn.Cases))
			//
			for c, dc := range insn.Cases {
				cases[c] = bytecode.DispatchCase{Bit: dc.Bit, Skip: newSkip(i, c, int(dc.Skip))}
			}
			//
			out[insnPos[i]] = bytecode.NewDispatch[W](cases, insn.Default)
		}
	}
	//
	return bytecode.NewVector(out...)
}

// stampName returns the name of the stamp-in register threaded for the given
// memory.
func stampName[W word.Word[W]](mods []descriptor.Module[W], id descriptor.ModuleId) string {
	return mods[id].Name() + "$stamp"
}

// stampOutName returns the name of the stamp-out register threaded for the
// given memory.  NOTE: deliberately not "$stamp'", which would collide with
// the "'k" suffix used for register limbs after splitting.
func stampOutName[W word.Word[W]](mods []descriptor.Module[W], id descriptor.ModuleId) string {
	return mods[id].Name() + "$stamp_out"
}
