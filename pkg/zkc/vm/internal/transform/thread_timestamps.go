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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
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
//   - adds an input register "M$stamp" and an output register "M$stamp_out",
//     placed first in the inputs / outputs.  The entry function "main" takes
//     no parameters, so it gets no stamp in/out and instead counts from ONE
//     (timestamp zero is reserved for the initial state of an untouched cell);
//   - forwards the stamp across every call to an effectful callee (prepended
//     to the call's arguments and returns);
//   - gives every access a distinct timestamp: the k-th access executed
//     carries stamp_in + k, recorded in the access's Stamp operand.
//
// The threading is version-based, following SSA form (see #2099): within one
// row, the k-th movement (access or effectful call) executed on a path
// advances the stamp from version k-1 to version k, and each version names
// one register — version zero the row's entry register, the LARGEST version
// live at an exit the canonical stamp register (M$stamp_out, or a fresh
// computed register in main), every other version a fresh temporary.  Paths
// meeting with different versions are equalised by copies into the largest
// incoming version's register, placed on the deficient paths only.  The
// canonical register is thereby written at most once per executed path
// through a row.  In particular the common one-line function
//
//	fn f<ram>(x:u16) -> (r:u8) { r = ram[x] }
//
// costs exactly one added instruction (the advance past its single access is
// the exit's largest version, so it targets the canonical register directly):
//
//	read r = ram[ram$stamp; x]
//	add ram$stamp_out = ram$stamp + 1
//
// Required only for tracing / constraint generation (the run-time memory keeps
// its own clock), so applied on the constraint path only; runs AFTER
// vectorisation (a vector is then genuinely one trace row — a second canonical
// write would have stopped the vectoriser forming the row, forcing one-line
// functions multi-line) and before register splitting (stamps split into limbs
// and are range-checked like any other register).
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
		// Every declared effect names a read-write memory: the linker rejects
		// effects on read-only / write-once memories (see checkSymbolKind in
		// pkg/zkc/compiler/linker.go), so the list is used as-is.
		effects := fn.Effects()
		if len(effects) == 0 {
			continue
		}
		//
		out[i] = threadFunction(mods, fn, effects)
	}
	//
	return descriptor.NewProgram(program.Field(), out...)
}

// stampLoc records where a stamp value concretely lives: a register, or —
// when reg is empty — the literal constant off (the entry state of "main",
// which counts from one).
type stampLoc struct {
	reg util.Option[bytecode.RegisterId]
	off uint64
}

// is reports whether this location is exactly the given register.
func (s stampLoc) is(reg bytecode.RegisterId) bool {
	return s.reg == util.Some(reg)
}

// stampKey identifies one version of one effect's stamp within the current
// row, for the version -> register binding.
type stampKey struct {
	// effect indexes the threader's effects.
	effect int
	// v is the version number.
	v uint
}

// threader carries the per-function context of the threading.
type threader[W word.Word[W]] struct {
	mods    []descriptor.Module[W]
	alloc   split.Allocator[W]
	effects []descriptor.ModuleId
	// stampIn maps each effect to its stamp-in input register (absent for
	// main, whose stamps count from literal one).
	stampIn map[descriptor.ModuleId]bytecode.RegisterId
	// canonical maps each effect to its canonical stamp register: the
	// stamp-out output or, for main, a lazily-allocated computed register.
	// Every row except the entry row is entered with the current stamp held
	// there.
	canonical map[descriptor.ModuleId]bytecode.RegisterId
	// regs binds each (effect, version) of the current row to its register:
	// version zero is prebound to the row's entry register, the largest exit
	// version binds to the canonical register, and every other version gets a
	// fresh temporary on first use (see getReg).  Reset per row.
	regs map[stampKey]bytecode.RegisterId
	// used records the versions whose value the rewrite actually consumed
	// (via resolveLoc); the bump defining an unconsumed version — e.g. the
	// advance past the last access before a fail — is retracted before the
	// row is rebuilt.  Reset per row.
	used map[stampKey]bool
	// kmax records, per effect, the largest version live at any exit of the
	// current row; that version's register is the canonical register, so the
	// final movement before an exit advances the stamp directly into it.
	// Reset per row.
	kmax   []uint
	isMain bool
}

// threadFunction rewrites a single function to thread a timestamp for each of
// the given read-write memory effects.
func threadFunction[W word.Word[W]](mods []descriptor.Module[W], fn *descriptor.Function[W],
	effects []descriptor.ModuleId) *descriptor.Function[W] {
	//
	t := &threader[W]{
		mods:      mods,
		effects:   effects,
		stampIn:   map[descriptor.ModuleId]bytecode.RegisterId{},
		canonical: map[descriptor.ModuleId]bytecode.RegisterId{},
		isMain:    fn.Name() == "main",
	}
	//
	nvecs := t.remapFunction(fn)
	// Initial symbolic state of each effect's stamp: the stamp-in register
	// or, for main, the literal ONE (timestamp zero is reserved for the
	// initial state of an untouched cell).
	entry := make([]stampVersion, len(effects))
	//
	for x := range effects {
		entry[x] = stampVersion{lit: t.isMain}
	}
	//
	nvecs = t.seedEntryRow(nvecs, entry)
	// Thread each row.  Every row after the first is entered with canonical
	// stamps (each row materialises them at its exits).
	for vi := range nvecs {
		vals := make([]stampVersion, len(effects))
		//
		if vi == 0 {
			copy(vals, entry)
		} else {
			for x, e := range effects {
				// Resolve the canonical register eagerly so that (for main,
				// where it is allocated on first use) it precedes the row's
				// temporaries in the register order.
				_ = t.getCanonical(e)
				vals[x] = stampVersion{canon: true}
			}
		}
		//
		nvecs[vi] = t.threadVector(nvecs[vi], stamps{vals})
	}
	//
	return descriptor.NewFunction(fn.Name(), t.alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

// remapFunction lays out the threaded function's registers (initNewRegs),
// builds the old->new register id remap (buildFunctionMapping), then remaps
// the body onto the new ids and prepares the temporary allocator against the
// result.
func (t *threader[W]) remapFunction(fn *descriptor.Function[W]) []BytecodeVector[W] {
	var (
		newRegs = t.initNewRegs(fn)
		sub     = t.buildFunctionMapping(fn)
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
	)
	// Remap the body onto the new register ids.
	for vi, vec := range vectors {
		nvecs[vi] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			return []Bytecode[W]{substituteRegisters[W](insn, sub)}
		})
	}
	// Allocate temporaries against the remapped register set.
	remapped := descriptor.NewFunction(fn.Name(), newRegs, fn.Kind(), fn.Effects(), nvecs)
	t.alloc = split.NewAllocator[W](remapped)
	//
	return nvecs
}

// initNewRegs constructs the threaded register layout, stamps first:
// [stamp-ins, old inputs, stamp-outs, old outputs, old computed] -- recording
// each effect's stamp-in and canonical (stamp-out) register along the way.
// The stamp registers of a function's memories are always allocated
// consecutively, in effect-declaration order (once per group: all stamp-ins,
// then all stamp-outs).  main takes no parameters, so it gets no stamp in/out;
// its canonical registers are instead allocated lazily (see getCanonical).
func (t *threader[W]) initNewRegs(fn *descriptor.Function[W]) []descriptor.Register[W] {
	var (
		oldRegs = fn.Registers()
		ni      = fn.NumInputs()
		padding W
		newRegs []descriptor.Register[W]
	)
	//
	if !t.isMain {
		for _, e := range t.effects {
			t.stampIn[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.INPUT_REGISTER, stampName(t.mods, e),
				util.Some(stampWidth), padding))
		}
	}
	//
	newRegs = append(newRegs, oldRegs[:ni]...)
	//
	if !t.isMain {
		for _, e := range t.effects {
			t.canonical[e] = bytecode.RegisterId(len(newRegs))
			newRegs = append(newRegs, descriptor.NewRegister(register.OUTPUT_REGISTER, stampOutName(t.mods, e),
				util.Some(stampWidth), padding))
		}
	}
	//
	return append(newRegs, oldRegs[ni:]...)
}

// buildFunctionMapping builds the old->new register id remap: inserting k
// stamp inputs shifts old inputs by k; inserting k stamp outputs shifts old
// outputs and computed by a further k.  main inserts none, so the map is the
// identity there.
func (t *threader[W]) buildFunctionMapping(fn *descriptor.Function[W]) []bytecode.RegisterId {
	var (
		k   = uint(len(t.effects))
		ni  = fn.NumInputs()
		sub = make([]bytecode.RegisterId, len(fn.Registers()))
	)
	//
	for x := range sub {
		id := bytecode.RegisterId(x)
		//
		switch {
		case t.isMain:
			sub[x] = id
		case uint(x) < ni:
			sub[x] = id + bytecode.RegisterId(k)
		default:
			sub[x] = id + bytecode.RegisterId(2*k)
		}
	}
	//
	return sub
}

// seedEntryRow handles an entry row which is itself a jump target (e.g. a
// leading loop header).  Every jump lands on a row entered with canonical
// stamps, so the entry state is materialised on a preamble row of its own,
// every jump target shifted by one, and the entry state (updated in place)
// becomes the canonical stamps.  Rows whose entry is not a jump target are
// returned unchanged.
func (t *threader[W]) seedEntryRow(nvecs []BytecodeVector[W], entry []stampVersion) []BytecodeVector[W] {
	if !jumpTargets(nvecs).Contains(0) {
		return nvecs
	}
	//
	var seed []Bytecode[W]
	//
	for x, e := range t.effects {
		loc := stampLoc{off: 1}
		//
		if !t.isMain {
			loc = stampLoc{reg: util.Some(t.stampIn[e])}
		}
		//
		seed = append(seed, t.materialise(loc, t.getCanonical(e)))
		entry[x] = stampVersion{canon: true}
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
	//
	return nvecs
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

// getReg returns the register bound to the given version of the x-th effect's
// stamp: the canonical register for the largest exit version of the row, a
// fresh temporary otherwise (version zero is prebound by beginRow).
func (t *threader[W]) getReg(x int, v uint) bytecode.RegisterId {
	key := stampKey{x, v}
	//
	if r, ok := t.regs[key]; ok {
		return r
	}
	//
	var r bytecode.RegisterId
	//
	if v == t.kmax[x] && v > 0 {
		r = t.getCanonical(t.effects[x])
	} else {
		r = t.alloc.Allocate("stamp", util.Some(stampWidth))
	}
	//
	t.regs[key] = r
	//
	return r
}

// resolveLoc maps the symbolic stamp value of the x-th effect onto its
// concrete location, recording the consumption of its version (which keeps
// the bump defining it alive, see retractBumps).
func (t *threader[W]) resolveLoc(x int, v stampVersion) stampLoc {
	switch {
	case v.canon:
		return stampLoc{reg: util.Some(t.getCanonical(t.effects[x]))}
	case v.lit:
		return stampLoc{off: uint64(v.v) + 1}
	default:
		t.used[stampKey{x, v.v}] = true
		//
		return stampLoc{reg: util.Some(t.getReg(x, v.v))}
	}
}

// materialise returns an instruction storing the given stamp location into the
// given register.
func (t *threader[W]) materialise(s stampLoc, target bytecode.RegisterId) Bytecode[W] {
	if s.reg.HasValue() {
		return bytecode.Assign[W](target, s.reg.Unwrap())
	}
	//
	var constant W
	//
	constant = constant.SetUint64(s.off)
	//
	return bytecode.LoadConst(target, constant)
}

// stampOperand returns a register holding the given stamp value, together
// with the (possibly empty) instructions computing it: the version's register
// directly, or — for a literal — a fresh temporary loaded with the constant.
// The caller places the returned instructions in the position's preEdge
// segment, which executes on every path reaching it (a constant is
// path-independent, so this is sound even at a merge point).
func (t *threader[W]) stampOperand(x int, v stampVersion) (bytecode.RegisterId, []Bytecode[W]) {
	loc := t.resolveLoc(x, v)
	//
	if loc.reg.HasValue() {
		return loc.reg.Unwrap(), nil
	}
	//
	tmp := t.alloc.Allocate("stamp", util.Some(stampWidth))
	//
	return tmp, []Bytecode[W]{t.materialise(loc, tmp)}
}

// rowInserts collects the instructions materialised around one original
// instruction position.  The rebuilt layout is [bumps..., fall..., (skip over
// edge)?, edge..., preEdge..., instruction]: ordinary skips landing here land
// at the start of preEdge, retargeted conditional skips at the start of edge,
// and a live fall-through path runs bumps and fall and then jumps over the
// edge block.
type rowInserts[W word.Word[W]] struct {
	// bumps holds the version-advancing additions of a movement at the
	// previous position (which necessarily falls through to this one); kept
	// separate from fall so that an unconsumed version's bump can be
	// retracted (see retractBumps).
	bumps []Bytecode[W]
	// bumpKeys identifies, in step with bumps, the version each bump defines.
	bumpKeys []stampKey
	// fall executes on the fall-through path only.
	fall []Bytecode[W]
	// edge is the edge block: retargeted conditional skip edges land at its
	// start.
	edge []Bytecode[W]
	// preEdge executes on EVERY path reaching this position.  Used for the
	// (path-independent) literal materialisations feeding an instruction's
	// stamp operand: arrivals landing here directly would miss a fall-only
	// insertion.
	preEdge []Bytecode[W]
	// skipOver indicates a live fall-through path enters this position, which
	// must then jump over the edge block.
	skipOver bool
	// retargets lists the (source index, case index) pairs of the conditional
	// skip edges retargeted to the edge block.
	retargets [][2]int
}

// threadVector rewrites one row in two phases.  First, a forward dataflow
// analysis (analyseStamps) computes the stamp version entering every
// position, together with the state carried along every control-flow arc.
// Then a rewrite sweep walks the row once more: it equalises the incoming
// paths at each merge point (mergeAt), assigns every read-write memory access
// its stamp operand and advances the version (the bump), threads calls to
// effectful callees, and materialises the canonical stamp registers at every
// exit (return, jump, or fall-through into the next row).  Versions bind to
// registers by construction — entry register, canonical register, or fresh
// temporary — so nothing is left to resolve after the sweep, beyond
// retracting the bumps of unconsumed versions.
func (t *threader[W]) threadVector(vec BytecodeVector[W], entry stamps) BytecodeVector[W] {
	var (
		insns = vec.Bytecodes
		n     = len(insns)
		// inserts[i] = instructions materialised around original instruction i
		// (i == n denotes the end of the row).
		inserts = map[int]*rowInserts[W]{}
		// replace[i] = rebuilt instruction at original index i.
		replace = map[int]Bytecode[W]{}
	)
	//
	result, incoming, lands := t.analyseStamps(insns, entry)
	//
	t.beginRow(insns, result, entry)
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
		st := result.StateOf(uint(i))
		// Positions never reached (on any path) are copied verbatim.
		if st.isBottom() {
			continue
		}
		// Equalise the incoming paths at a merge point.
		states := t.normaliseEntry(st, lands[i], t.isExitAt(insns, i))
		t.mergeAt(i, states, incoming[i], at)
		//
		if i == n {
			// A path reaching the end of the row falls into the next row,
			// which is entered with canonical stamps.
			at(n).fall = append(at(n).fall, t.canonicalise(states)...)
			//
			continue
		}
		//
		t.threadInstruction(insns, i, states, at, replace)
	}
	//
	t.retractBumps(inserts)
	// Rebuild the row, recomputing intra-row skip amounts around insertions.
	return rebuildVector(insns, inserts, replace)
}

// beginRow resets the per-row threading state: the version -> register
// binding (version zero prebound to the row's entry register, when that is a
// plain register) and, per effect, the largest version live at any exit —
// whose register is the canonical stamp, so the final movement before an exit
// advances the stamp directly into it.
func (t *threader[W]) beginRow(insns []Bytecode[W], result dfa.Result[stamps], entry stamps) {
	t.regs = map[stampKey]bytecode.RegisterId{}
	t.used = map[stampKey]bool{}
	t.kmax = make([]uint, len(t.effects))
	//
	for x, v := range entry.vals {
		if !v.lit && !v.canon {
			t.regs[stampKey{x, 0}] = t.stampIn[t.effects[x]]
		}
	}
	//
	for i := 0; i <= len(insns); i++ {
		st := result.StateOf(uint(i))
		//
		if st.isBottom() || !t.isExitAt(insns, i) {
			continue
		}
		//
		for x, v := range st.vals {
			t.kmax[x] = max(t.kmax[x], v.v)
		}
	}
}

// retractBumps drops the bump of every version the rewrite never consumed
// (e.g. the advance past the last access on a path ending in a fail), so a
// movement whose updated stamp is never observed costs nothing.
func (t *threader[W]) retractBumps(inserts map[int]*rowInserts[W]) {
	for _, ri := range inserts {
		var (
			bumps []Bytecode[W]
			keys  []stampKey
		)
		//
		for j, key := range ri.bumpKeys {
			if t.used[key] {
				bumps = append(bumps, ri.bumps[j])
				keys = append(keys, key)
			}
		}
		//
		ri.bumps, ri.bumpKeys = bumps, keys
	}
}

// threadInstruction rewrites the instruction at position i against the (already
// merged) state entering it: a read-write access gains its stamp operand and
// advances the version, a call to an effectful callee is threaded, an exit
// (jump, or return outside main) materialises the canonical stamps its landing
// assumes, and a branch in front of a jump table canonicalises once for every
// exit of the table (rewriteBranch).  Instructions indifferent to stamps pass
// through untouched.
func (t *threader[W]) threadInstruction(insns []Bytecode[W], i int, states stamps,
	at func(int) *rowInserts[W], replace map[int]Bytecode[W]) {
	//
	switch insn := insns[i].(type) {
	case *bytecode.ReadWrite[W]:
		if x := t.effectIndex(insn.Id); x >= 0 {
			s := states.vals[x]
			reg, pre := t.stampOperand(x, s)
			at(i).preEdge = append(at(i).preEdge, pre...)
			replace[i] = withStamp(insn, reg)
			// Advance to the next version: its register receives the current
			// stamp plus one.  A literal advances for free (the next version
			// is a constant too); every path through the access falls through
			// to position i+1, so the bump is placed there.
			if !s.lit {
				var one W
				//
				one = one.SetUint64(1)
				next := stampKey{x, s.v + 1}
				at(i + 1).bumps = append(at(i+1).bumps,
					bytecode.AddConst(t.getReg(x, next.v), []bytecode.RegisterId{reg}, one))
				at(i + 1).bumpKeys = append(at(i+1).bumpKeys, next)
			}
		}
	case *bytecode.Call[W]:
		if rebuilt, pre := t.threadCall(insn, states); rebuilt != nil {
			at(i).preEdge = append(at(i).preEdge, pre...)
			replace[i] = rebuilt
		}
	case *bytecode.Jmp[W]:
		// The jump target is entered with canonical stamps.
		at(i).fall = append(at(i).fall, t.canonicalise(states)...)
	case *bytecode.Ret[W]:
		// Bind the stamp-out outputs (main has none and returns nothing).
		if !t.isMain {
			at(i).fall = append(at(i).fall, t.canonicalise(states)...)
		}
	case *bytecode.SkipIf[W]:
		t.rewriteBranch(insns, i, states, at)
	case *bytecode.Switch[W]:
		t.rewriteBranch(insns, i, states, at)
	case *bytecode.Dispatch[W]:
		t.rewriteBranch(insns, i, states, at)
	}
}

// mergeAt places the equalising materialisations at a merge point.  Each
// incoming path whose stamp is not already in the merged version's register
// materialises it at a position executed exactly on that path: inline for the
// fall-through, at the source's preEdge for an unconditional edge, in an edge
// block at the landing for a conditional edge.  At an exit (return, jump, end
// of row) the target is the canonical stamp itself, so the exit needs no
// further materialisation on any path.
func (t *threader[W]) mergeAt(i int, merged stamps, incoming []stampArc,
	at func(int) *rowInserts[W]) {
	//
	fall, skips, conditional := classifyArcs(incoming)
	//
	if len(skips) == 0 {
		return
	}
	//
	for x, e := range t.effects {
		m := merged.vals[x]
		// A literal is path-independent: its consumers materialise the
		// constant themselves.
		if m.lit {
			continue
		}
		// The merge register: the canonical stamp at an exit (or when every
		// path is already canonical), the merged version's register otherwise.
		var target bytecode.RegisterId
		//
		if m.canon {
			target = t.getCanonical(e)
		} else {
			target = t.getReg(x, m.v)
		}
		//
		if fall != nil {
			if loc := t.resolveLoc(x, fall.state.vals[x]); !loc.is(target) {
				at(i).fall = append(at(i).fall, t.materialise(loc, target))
			}
		}
		//
		for _, p := range skips {
			loc := t.resolveLoc(x, p.state.vals[x])
			//
			if loc.is(target) {
				continue
			}
			//
			switch p.kind {
			case uncondArc:
				// Into the source's preEdge segment: every arrival there
				// carries the same (already merged) state, but a skip landing
				// directly on the source would bypass a fall-only insertion.
				at(int(p.source)).preEdge = append(at(int(p.source)).preEdge, t.materialise(loc, target))
			case condArc:
				// One materialisation per effect in the shared edge block.
				if p.source == conditional.source {
					at(i).edge = append(at(i).edge, t.materialise(loc, target))
				}
			}
		}
	}
	// Retarget every conditional edge to the edge block (when one exists), and
	// have a live fall path jump over it.
	if len(at(i).edge) != 0 {
		for _, p := range skips {
			if p.kind == condArc {
				at(i).retargets = append(at(i).retargets, [2]int{int(p.source), p.caseIdx})
			}
		}
		//
		at(i).skipOver = fall != nil
	}
}

// classifyArcs partitions the arcs into a merge point: the (at most one)
// fall-through arc, the skip arcs, and — among the latter — the representative
// conditional arc.  Conditional edges landing at one merge point share a
// single edge block, so they must all carry the same states; disagreeing
// conditional edges are an unsupported control-flow shape.
func classifyArcs(incoming []stampArc) (fall *stampArc, skips []stampArc, conditional *stampArc) {
	for j := range incoming {
		if incoming[j].kind == fallArc {
			fall = &incoming[j]
		} else {
			skips = append(skips, incoming[j])
		}
	}
	//
	for j := range skips {
		if skips[j].kind != condArc {
			continue
		}
		//
		if conditional == nil {
			conditional = &skips[j]
		} else if !stateEquals(conditional.state, skips[j].state) {
			panic("timestamp threading: conditional skip edges with distinct stamps " +
				"at one merge point (unsupported control flow shape)")
		}
	}
	//
	return fall, skips, conditional
}

// stateEquals reports whether two states carry identical values for every
// effect.
func stateEquals(p, o stamps) bool {
	for x := range p.vals {
		if p.vals[x] != o.vals[x] {
			return false
		}
	}
	//
	return true
}

// rewriteBranch handles a conditional (or multiway) skip.  When the remainder
// of the row is a pure jump table, the canonical stamps are materialised once,
// before the branch, covering every exit (see branchState).  Otherwise nothing
// is placed here: a version is a register on every path, so all outgoing edges
// carry the same symbolic value as-is, and the guarded region's accesses are
// reconciled at its merge point by mergeAt.
func (t *threader[W]) rewriteBranch(insns []Bytecode[W], i int, states stamps,
	at func(int) *rowInserts[W]) {
	//
	if jumpTableFollows(insns, i) {
		at(i).fall = append(at(i).fall, t.canonicalise(states)...)
	}
}

// canonicalise returns the instructions materialising every effect's state
// into its canonical register, skipping effects already there.
func (t *threader[W]) canonicalise(states stamps) []Bytecode[W] {
	var out []Bytecode[W]
	//
	for x, e := range t.effects {
		var (
			target = t.getCanonical(e)
			loc    = t.resolveLoc(x, states.vals[x])
		)
		//
		if loc.is(target) {
			continue
		}
		//
		out = append(out, t.materialise(loc, target))
	}
	//
	return out
}

// threadCall rewrites a call to an effectful callee: for each read-write
// memory the callee declares, the caller's current stamp is passed as a
// (prepended) argument and the updated stamp received directly into the next
// version's register.  Returns nil when the callee needs no threading.
func (t *threader[W]) threadCall(call *bytecode.Call[W], states stamps,
) (Bytecode[W], []Bytecode[W]) {
	//
	callee, ok := t.mods[call.Target].(*descriptor.Function[W])
	if !ok || callee.IsNative() {
		return nil, nil
	}
	// Effects are read-write memories by construction (linker-enforced).
	effects := callee.Effects()
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
	for j, e := range effects {
		x := t.effectIndex(e)
		if x < 0 {
			// Guaranteed by the type-checker: callee effects are a subset of
			// the caller's.
			panic(fmt.Sprintf("caller lacks a stamp for memory %d", e))
		}
		//
		s := states.vals[x]
		reg, insns := t.stampOperand(x, s)
		pre = append(pre, insns...)
		args[j] = reg
		// Receive the callee's updated stamp into the next version's register.
		returns[j] = t.getReg(x, s.v+1)
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
// each original position the layout is [bumps..., fall..., (skip-over), edge
// block..., preEdge..., instruction]: bump and fall instructions execute on
// the fall-through path only (ordinary skips land past them, at the start of
// preEdge), conditional skips retargeted to this position land at the start
// of the edge block, and a live fall-through path jumps over the block.
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
		// preEdgeStart[i] = index of position i's preEdge segment (the landing
		// anchor of ordinary skips) in the rebuilt row.
		preEdgeStart = make([]int, n+1)
		// retargeted maps the (source index, case index) of a retargeted
		// conditional skip edge to its landing position.
		retargeted = map[[2]int]int{}
		out        []Bytecode[W]
	)
	//
	for i := 0; i <= n; i++ {
		if ri := inserts[i]; ri != nil {
			out = append(out, ri.bumps...)
			out = append(out, ri.fall...)
			//
			if len(ri.edge) != 0 && ri.skipOver {
				out = append(out, bytecode.NewSkip[W](uint16(len(ri.edge))))
			}
			//
			edgeStart[i] = len(out)
			out = append(out, ri.edge...)
			preEdgeStart[i] = len(out)
			out = append(out, ri.preEdge...)
			//
			for _, s := range ri.retargets {
				retargeted[s] = i
			}
		} else {
			edgeStart[i] = len(out)
			preEdgeStart[i] = len(out)
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
	// i, with amount k, lands at original index i+1+k — at the start of that
	// position's preEdge segment (past its fall-only instructions) — except
	// when retargeted, in which case it lands at the start of its landing's
	// edge block.  (Plain and conditional skips are their own single case,
	// index zero.)
	newSkip := func(i, c, k int) uint16 {
		landing := preEdgeStart[i+1+k]
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
