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
	"math"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// InlineFunctions constructs an equivalent set of modules in which every call to
// one of the named functions has been inlined at its call site, and the named
// function modules removed.  Removing modules shifts the identifiers of those
// which follow, hence module identifiers within Call / MemRead / MemWrite
// instructions are remapped accordingly.
//
// Inlining a call site replaces the Call instruction with the callee's body,
// where every callee register is realised by a caller register.  Where
// possible, callee inputs / outputs are aliased directly to the corresponding
// argument / return registers of the call; otherwise, a fresh (caller-local)
// shadow register is allocated, along with a copy of the argument register
// into the shadowed input at entry (resp. of the shadowed output into the
// return register at exit).  Such copies enforce the same dynamic width
// checks as entering / leaving the callee's stack frame did, hence aliasing
// additionally requires identically shaped registers (see buildShadowMap for
// the exact conditions).
//
// Output aliasing assumes the callee never reads an output before assigning
// it, and assigns every output before returning.  Both are guaranteed for
// compiler-generated functions (see validate.ControlFlow); machines built by
// other means which violate them may observe the return register's previous
// value where a true call would have observed the callee's initial (zero)
// output.
//
// This transform must be applied before vectorisation, since it splits the
// vector containing a call at the call site.  It panics on: an unknown or
// duplicate name; a native function; the entry function "main"; or (mutual)
// recursion amongst the named functions.
func InlineFunctions[W word.Word[W]](modules []Module, names []string) []Module {
	var targets []uint
	//
	modules = slices.Clone(modules)
	targets = resolveInlineTargets(modules, names)
	// Inline named functions in callee-first order.  At each step, pick a
	// target whose body no longer calls any unprocessed target; hence, by the
	// time a target is inlined into its callers, its body is fully inlined
	// itself.  If no such target exists, the named functions are (mutually)
	// recursive.
	for remaining := slices.Clone(targets); len(remaining) > 0; {
		var (
			index  = findInlinableTarget(modules, remaining)
			callee = modules[remaining[index]].(*WordFunction)
		)
		//
		for i, m := range modules {
			if fn, ok := m.(*WordFunction); ok && fn != callee && !fn.IsNative() {
				modules[i] = inlineAllCalls[W](fn, remaining[index], callee)
			}
		}
		//
		remaining = slices.Delete(remaining, index, index+1)
	}
	// Remove now-dead targets, remapping module identifiers.
	return removeModules(modules, targets)
}

// resolveInlineTargets maps each name to its module identifier, sanity
// checking that every name identifies a distinct function which can actually
// be inlined (and removed).
func resolveInlineTargets(modules []Module, names []string) []uint {
	var targets = make([]uint, len(names))
	//
	for i, name := range names {
		index := slices.IndexFunc(modules, func(m Module) bool { return m.Name() == name })
		//
		switch {
		case index < 0:
			panic(fmt.Sprintf("cannot inline unknown function \"%s\"", name))
		case name == "main":
			panic("cannot inline entry function \"main\"")
		case slices.Contains(targets[:i], uint(index)):
			panic(fmt.Sprintf("duplicate inlined function \"%s\"", name))
		}
		//
		if fn, ok := modules[index].(*WordFunction); !ok {
			panic(fmt.Sprintf("cannot inline non-function \"%s\"", name))
		} else if fn.IsNative() {
			panic(fmt.Sprintf("cannot inline native function \"%s\"", name))
		}
		//
		targets[i] = uint(index)
	}
	//
	return targets
}

// findInlinableTarget returns the index (within remaining) of a target whose
// body contains no calls to any remaining target, panicking if every target
// calls another (i.e. the named functions are recursive).
func findInlinableTarget(modules []Module, remaining []uint) int {
	for i, id := range remaining {
		if !callsAny(modules[id].(*WordFunction), remaining) {
			return i
		}
	}
	// Cycle detected
	var names = make([]string, len(remaining))
	//
	for i, id := range remaining {
		names[i] = modules[id].Name()
	}
	//
	panic(fmt.Sprintf("cannot inline recursive function(s) %s", strings.Join(names, ", ")))
}

// callsAny checks whether the body of a given function calls any of the given
// modules.
func callsAny(fn *WordFunction, ids []uint) bool {
	for _, v := range fn.Code() {
		for _, insn := range v.Codes {
			if call, ok := insn.(*instruction.Call); ok && slices.Contains(ids, call.Id) {
				return true
			}
		}
	}
	//
	return false
}

// inlineAllCalls inlines every call to a given callee within the body of a
// given function, returning the (possibly unchanged) result.
func inlineAllCalls[W word.Word[W]](fn *WordFunction, calleeId uint, callee *WordFunction) *WordFunction {
	var (
		alloc   = register.NewAllocator[int](fn.RegisterMap())
		code    = slices.Clone(fn.Code())
		changed = false
	)
	// Splice call sites one at a time.  Since the callee's body contains no
	// calls to the callee (recursion is rejected upfront), each splice strictly
	// reduces the number of matching call sites.
	for {
		pc, k, call := findCall(code, calleeId)
		//
		if call == nil {
			break
		}
		//
		code = inlineCallSite[W](code, pc, k, call, callee, alloc)
		changed = true
	}
	//
	if !changed {
		return fn
	}
	//
	return function.New(fn.Name(), fn.IsNative(), alloc.Registers(), code)
}

// findCall locates the first call to a given callee, returning the enclosing
// vector index, the position within that vector and the call itself (or nil if
// there is none).
func findCall(code []VectorInstruction, calleeId uint) (uint, uint, *instruction.Call) {
	for i, v := range code {
		for j, insn := range v.Codes {
			if call, ok := insn.(*instruction.Call); ok && call.Id == calleeId {
				return uint(i), uint(j), call
			}
		}
	}
	//
	return 0, 0, nil
}

// inlineCallSite splices the body of the callee into the caller's code in
// place of the call at position k within vector pc.  The enclosing vector is
// split around the call site, giving the following layout:
//
//	[0 .. pc)                                  unchanged (jumps remapped)
//	[pc]        v_pre  = codes[:k] ++ argument copies
//	[pc+1 .. exitPC)   callee body (registers shadowed, jumps rebased,
//	                   returns becoming jumps to exitPC)
//	[exitPC]    v_post = output copies ++ codes[k+1:]
//	[exitPC+1 ..]                              unchanged (jumps remapped)
//
// Since the callee body occupies len(callee.Code())+1 additional vectors, all
// jump targets beyond pc within the original code are shifted accordingly.
func inlineCallSite[W word.Word[W]](code []VectorInstruction, pc, k uint, call *instruction.Call,
	callee *WordFunction, alloc RegisterAllocator) []VectorInstruction {
	//
	var (
		codes  = code[pc].Codes
		nBody  = uint(len(callee.Code()))
		exitPC = pc + 1 + nBody
		// Inserting v_pre, body and v_post in place of one vector shifts
		// subsequent vectors down by this amount.
		delta = nBody + 1
	)
	// Sanity check the call site can actually be split.
	checkCallSite(codes, k, callee)
	// Map callee registers onto caller registers, aliasing inputs / outputs
	// with the call's argument / return registers where possible (and
	// allocating fresh shadows otherwise).
	shadows := buildShadowMap(call, callee, alloc)
	// Construct entry vector (argument copies)
	vPre := buildEntryVector[W](codes[:k], shadows.entryCopies)
	// Construct callee body
	body := buildInlinedBody[W](callee, shadows.registers, pc+1, exitPC)
	// Construct exit vector (output copies)
	vPost := buildExitVector[W](codes[k+1:], shadows.exitCopies)
	// Splice, remapping all jumps within original caller vectors (including
	// v_pre / v_post, whose codes originate from vector pc).
	ncode := make([]VectorInstruction, 0, uint(len(code))+delta)
	//
	for _, v := range code[:pc] {
		ncode = append(ncode, remapJumps(v, pc, delta))
	}
	//
	ncode = append(ncode, remapJumps(vPre, pc, delta))
	ncode = append(ncode, body...)
	ncode = append(ncode, remapJumps(vPost, pc, delta))
	//
	for _, v := range code[pc+1:] {
		ncode = append(ncode, remapJumps(v, pc, delta))
	}
	//
	return ncode
}

// checkCallSite ensures a given call site can be inlined.  Specifically, no
// skip before the call may cross over it, since such a skip cannot survive
// splitting the enclosing vector at the call site.  Note that a skip targeting
// the call itself is fine, as this lands on the argument copies (i.e. the call
// entry) after splitting.
func checkCallSite(codes []WordInstruction, k uint, callee *WordFunction) {
	if len(callee.Code()) == 0 {
		panic(fmt.Sprintf("cannot inline function \"%s\" with empty body", callee.Name()))
	}
	//
	for j := range k {
		var skip uint
		//
		switch insn := codes[j].(type) {
		case *instruction.Skip:
			skip = insn.Skip
		case *instruction.SkipIf:
			skip = insn.Skip
		default:
			continue
		}
		// Determine target of this skip
		if j+skip+1 > k {
			panic(fmt.Sprintf(
				"cannot inline call to \"%s\" guarded by skip (inlining must be applied before vectorisation)",
				callee.Name()))
		}
	}
}

// shadowMap describes how callee registers are realised within the caller at
// a given call site.  Every callee register maps to a caller register: either
// the corresponding argument / return register of the call itself (where this
// is provably equivalent), or a freshly allocated shadow (in which case a
// corresponding entry / exit copy is recorded).
type shadowMap struct {
	// registers maps each callee register to its caller-local realisation.
	registers []register.Id
	// entryCopies records (shadowed input, argument) pairs to be copied on
	// entry to the inlined body.
	entryCopies []registerCopy
	// exitCopies records (return register, shadowed output) pairs to be
	// copied on exit from the inlined body.
	exitCopies []registerCopy
}

// registerCopy records a register-to-register assignment.
type registerCopy struct {
	target, source register.Id
}

// buildShadowMap maps each callee register onto a caller register at a given
// call site.  Wherever possible, inputs / outputs are aliased directly to the
// call's argument / return registers, eliding the corresponding copy:
//
// An input can be aliased provided the callee never writes it (guaranteed for
// compiler-generated functions, which cannot write parameters) since the
// argument register then remains stable throughout the body.
//
// An output can be aliased provided its return register neither duplicates
// another return register (returning from a stack frame is last-wins, whereas
// direct writes would interleave), nor aliases an argument register which was
// itself aliased (the body could then clobber that argument whilst still
// reading it).
//
// In both cases, aliasing additionally requires the two registers to have
// identical shape: the elided copy performed a dynamic width check, which is
// vacuous exactly when the value already resides in a register of the same
// width.  Anything which cannot be aliased (including all temporaries) gets a
// fresh (computed) shadow register of the same shape.
func buildShadowMap(call *instruction.Call, callee *WordFunction, alloc RegisterAllocator) shadowMap {
	var (
		shadows    = shadowMap{registers: make([]register.Id, callee.Width())}
		written    = writtenRegisters(callee)
		numInputs  = callee.NumInputs()
		numOutputs = callee.NumOutputs()
		callerRegs = alloc.Registers()
		elidedArgs []register.Id
	)
	//
	for i, r := range callee.Registers() {
		var (
			index = uint(i)
			alias = register.UnusedId()
		)
		// Determine whether this register can be aliased.
		if index < numInputs {
			arg := call.Arguments[index]
			//
			if !written.Contains(index) && sameShape(callerRegs[arg.Unwrap()], r) {
				alias = arg
			}
		} else if index < numInputs+numOutputs {
			var (
				j         = index - numInputs
				ret       = call.Returns[j]
				duplicate = slices.Contains(call.Returns[:j], ret) || slices.Contains(call.Returns[j+1:], ret)
			)
			//
			if sameShape(callerRegs[ret.Unwrap()], r) && !duplicate && !slices.Contains(elidedArgs, ret) {
				alias = ret
			}
		}
		//
		if alias.IsUsed() {
			shadows.registers[i] = alias
			//
			if index < numInputs {
				elidedArgs = append(elidedArgs, alias)
			}
			//
			continue
		}
		// Allocate a fresh shadow of the same shape.
		var width uint = math.MaxUint
		//
		if !r.IsNative() {
			width = r.Width()
		}
		//
		shadows.registers[i] = alloc.Allocate(callee.Name()+"_"+r.Name(), width)
		// Record the corresponding entry / exit copy.
		if index < numInputs {
			shadows.entryCopies = append(shadows.entryCopies,
				registerCopy{shadows.registers[i], call.Arguments[index]})
		} else if j := index - numInputs; j < numOutputs {
			// Where the same register receives several outputs, retain only
			// the last copy (matching the last-wins semantics of returning
			// from a stack frame) since sequential copies would conflict.
			if !slices.Contains(call.Returns[j+1:], call.Returns[j]) {
				shadows.exitCopies = append(shadows.exitCopies,
					registerCopy{call.Returns[j], shadows.registers[i]})
			}
		}
	}
	//
	return shadows
}

// writtenRegisters returns the set of registers written anywhere within the
// body of a given function.
func writtenRegisters(fn *WordFunction) bit.Set {
	var written bit.Set
	//
	for _, v := range fn.Code() {
		for _, insn := range v.Codes {
			for _, r := range insn.Definitions() {
				written.Insert(r.Unwrap())
			}
		}
	}
	//
	return written
}

// sameShape checks whether two registers have identical shape, i.e. are both
// native, or both have the same declared width.
func sameShape(a, b register.Register) bool {
	if a.IsNative() || b.IsNative() {
		return a.IsNative() && b.IsNative()
	}
	//
	return a.Width() == b.Width()
}

// buildEntryVector constructs the vector replacing the front portion of the
// vector enclosing the call site.  This retains all codes preceding the call,
// followed by copies of the argument registers into the shadowed callee
// inputs.  Such copies enforce the same dynamic width checks as entering the
// callee's stack frame did.
func buildEntryVector[W word.Word[W]](codes []WordInstruction, copies []registerCopy) VectorInstruction {
	var ncodes = slices.Clone(codes)
	//
	for _, c := range copies {
		ncodes = append(ncodes, instruction.UintAssign[W](c.target, c.source))
	}
	// Vectors must be non-empty in order to execute.
	if len(ncodes) == 0 {
		ncodes = append(ncodes, &instruction.Skip{Skip: 0})
	}
	//
	return instruction.NewVector(ncodes...)
}

// buildExitVector constructs the vector replacing the back portion of the
// vector enclosing the call site.  This copies the shadowed callee outputs
// into the call's return registers, followed by all codes succeeding the
// call.  Placing the copies here (rather than at each return site within the
// body) ensures they are emitted exactly once.
func buildExitVector[W word.Word[W]](codes []WordInstruction, copies []registerCopy) VectorInstruction {
	var ncodes []WordInstruction
	//
	for _, c := range copies {
		ncodes = append(ncodes, instruction.UintAssign[W](c.target, c.source))
	}
	//
	ncodes = append(ncodes, codes...)
	// Vectors must be non-empty in order to execute.
	if len(ncodes) == 0 {
		ncodes = append(ncodes, &instruction.Skip{Skip: 0})
	}
	//
	return instruction.NewVector(ncodes...)
}

// buildInlinedBody instantiates the callee's body at a given call site.  All
// registers are substituted for their caller-local shadows; internal jumps are
// rebased onto the caller's program counter; and returns become jumps to the
// exit vector (which performs the output copies).
func buildInlinedBody[W word.Word[W]](callee *WordFunction, shadows []register.Id, base, exitPC uint,
) []VectorInstruction {
	var body = make([]VectorInstruction, len(callee.Code()))
	//
	for i, v := range callee.Code() {
		body[i] = v.Map(func(_ uint, insn WordInstruction) []WordInstruction {
			switch insn := insn.(type) {
			case *instruction.Return:
				return []WordInstruction{instruction.NewJump(exitPC)}
			case *instruction.Jump:
				return []WordInstruction{instruction.NewJump(base + insn.Immediate)}
			default:
				return []WordInstruction{substituteRegisters[W](insn, shadows)}
			}
		})
	}
	//
	return body
}

// substituteRegisters reconstructs a given instruction with every register
// substituted according to a given mapping.  Unused register identifiers (e.g.
// marking the register-constant variant of skip_if) are retained as is.
func substituteRegisters[W word.Word[W]](insn WordInstruction, sub []register.Id) WordInstruction {
	switch insn := insn.(type) {
	case *instruction.Call:
		return instruction.NewCall(insn.Id, substituteIds(insn.Arguments, sub), substituteIds(insn.Returns, sub))
	case *instruction.MemRead:
		return instruction.NewMemRead(insn.Id, substituteIds(insn.Arguments, sub), substituteIds(insn.Returns, sub))
	case *instruction.MemWrite:
		return instruction.NewMemWrite(insn.Id, substituteIds(insn.Arguments, sub), substituteIds(insn.Returns, sub))
	case *instruction.Debug:
		return instruction.NewDebug(substituteChunks(insn.Chunks, sub)...)
	case *instruction.Fail:
		return instruction.NewFail(substituteChunks(insn.Chunks, sub)...)
	case *instruction.Skip:
		return insn
	case *instruction.SkipIf:
		var (
			left  = register.NewVector(substituteIds(insn.Left.Registers(), sub)...)
			right = register.NewVector(substituteIds(insn.Right.Registers(), sub)...)
		)
		//
		return instruction.NewSkipIfVec(insn.Cond, left, right, insn.Skip)
	case *instruction.FieldHint:
		return instruction.NewFieldHint(substituteIds(insn.Targets, sub), substituteIds(insn.Sources, sub))
	case *instruction.WordTypeA[W]:
		target := register.NewVector(substituteIds(insn.Target.Registers(), sub)...)
		return instruction.NewWordTypeA(insn.Op, target, substituteIds(insn.Sources, sub), insn.Constant)
	case *instruction.WordTypeB:
		return instruction.NewWordTypeB(insn.Op, insn.Bitwidth, substituteId(insn.Target, sub),
			substituteId(insn.LeftSource, sub), substituteId(insn.RightSource, sub))
	case *instruction.WordTypeF[W]:
		return instruction.NewWordTypeF(insn.Op, substituteId(insn.Target, sub),
			substituteIds(insn.Sources, sub), insn.Constant)
	default:
		panic(fmt.Sprintf("unexpected instruction in inlined body (0x%x)", insn.OpCode()))
	}
}

func substituteChunks(chunks []instruction.FormattedChunk, sub []register.Id) []instruction.FormattedChunk {
	var nchunks = make([]instruction.FormattedChunk, len(chunks))
	//
	for i, chunk := range chunks {
		nchunks[i] = instruction.FormattedChunk{
			Text:     chunk.Text,
			Format:   chunk.Format,
			Argument: register.NewVector(substituteIds(chunk.Argument.Registers(), sub)...),
		}
	}
	//
	return nchunks
}

func substituteId(id register.Id, sub []register.Id) register.Id {
	if !id.IsUsed() {
		return id
	}
	//
	return sub[id.Unwrap()]
}

func substituteIds(ids []register.Id, sub []register.Id) []register.Id {
	var nids = make([]register.Id, len(ids))
	//
	for i, id := range ids {
		nids[i] = substituteId(id, sub)
	}
	//
	return nids
}

// remapJumps shifts all jump targets beyond a given program counter down by a
// given amount, accounting for the vectors inserted by inlining.  A target of
// pc itself is retained, since the entry vector occupies that position in the
// new layout.
func remapJumps(v VectorInstruction, pc, delta uint) VectorInstruction {
	var (
		ncodes  = make([]WordInstruction, len(v.Codes))
		changed = false
	)
	//
	for i, insn := range v.Codes {
		if jmp, ok := insn.(*instruction.Jump); ok && jmp.Immediate > pc {
			ncodes[i] = instruction.NewJump(jmp.Immediate + delta)
			changed = true
		} else {
			ncodes[i] = insn
		}
	}
	//
	if !changed {
		return v
	}
	//
	return instruction.NewVector(ncodes...)
}

// removeModules removes the given target modules, remapping the module
// identifiers embedded within the instructions of all remaining functions
// accordingly.
func removeModules(modules []Module, targets []uint) []Module {
	var (
		idMap = make([]uint, len(modules))
		kept  []Module
	)
	//
	for i, m := range modules {
		if slices.Contains(targets, uint(i)) {
			idMap[i] = math.MaxUint
		} else {
			idMap[i] = uint(len(kept))
			kept = append(kept, m)
		}
	}
	//
	for i, m := range kept {
		if fn, ok := m.(*WordFunction); ok {
			kept[i] = remapModuleIds(fn, idMap)
		}
	}
	//
	return kept
}

// remapModuleIds reconstructs a given function with all module identifiers
// substituted according to a given mapping.
func remapModuleIds(fn *WordFunction, idMap []uint) *WordFunction {
	var (
		code    = make([]VectorInstruction, len(fn.Code()))
		changed = false
	)
	//
	for i, v := range fn.Code() {
		var ncodes = make([]WordInstruction, len(v.Codes))
		//
		for j, insn := range v.Codes {
			ncodes[j] = remapModuleId(insn, idMap)
			changed = changed || ncodes[j] != insn
		}
		//
		code[i] = instruction.NewVector(ncodes...)
	}
	//
	if !changed {
		return fn
	}
	//
	return function.New(fn.Name(), fn.IsNative(), fn.Registers(), code)
}

func remapModuleId(insn WordInstruction, idMap []uint) WordInstruction {
	var id uint
	// Extract module identifier (if applicable)
	switch insn := insn.(type) {
	case *instruction.Call:
		id = idMap[insn.Id]
	case *instruction.MemRead:
		id = idMap[insn.Id]
	case *instruction.MemWrite:
		id = idMap[insn.Id]
	default:
		return insn
	}
	// Sanity check no residual references to removed modules.
	if id == math.MaxUint {
		panic("residual reference to inlined function")
	}
	// Reconstruct instruction (where necessary)
	switch insn := insn.(type) {
	case *instruction.Call:
		if id != insn.Id {
			return instruction.NewCall(id, insn.Arguments, insn.Returns)
		}
	case *instruction.MemRead:
		if id != insn.Id {
			return instruction.NewMemRead(id, insn.Arguments, insn.Returns)
		}
	case *instruction.MemWrite:
		if id != insn.Id {
			return instruction.NewMemWrite(id, insn.Arguments, insn.Returns)
		}
	}
	//
	return insn
}
