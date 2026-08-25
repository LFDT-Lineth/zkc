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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// InlineFunctions constructs an equivalent bytecode program in which every call
// to one of the named functions has been inlined at its call site, and the
// named function modules removed.  Removing modules shifts the identifiers of
// those which follow, hence module identifiers within Call / ReadWrite
// bytecodes are remapped accordingly.
//
// Inlining a call site replaces the Call bytecode with the callee's body, where
// every callee register is realised by a caller register.  Where possible,
// callee inputs / outputs are aliased directly to the corresponding argument /
// return registers of the call; otherwise, a fresh (caller-local) shadow
// register is allocated, along with a copy of the argument register into the
// shadowed input at entry (resp. of the shadowed output into the return
// register at exit).  Such copies enforce the same dynamic width checks as
// entering / leaving the callee's stack frame did, hence aliasing additionally
// requires identically shaped registers (see buildShadowMap for the exact
// conditions).
//
// Output aliasing assumes the callee never reads an output before assigning it,
// and assigns every output before returning.  Both are guaranteed for
// compiler-generated functions (see validate.ControlFlow); programs built by
// other means which violate them may observe the return register's previous
// value where a true call would have observed the callee's initial (zero)
// output.
//
// This transform must be applied before vectorisation, since it splits the
// vector containing a call at the call site.  It panics on: an unknown or
// duplicate name; a native function; the entry function "main"; or (mutual)
// recursion amongst the named functions.
func InlineFunctions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		modules = slices.Clone(program.Modules())
		targets = resolveInlineTargets(modules)
	)
	// Inline named functions in callee-first order.  At each step, pick a
	// target whose body no longer calls any unprocessed target; hence, by the
	// time a target is inlined into its callers, its body is fully inlined
	// itself.  If no such target exists, the named functions are (mutually)
	// recursive.
	for remaining := slices.Clone(targets); len(remaining) > 0; {
		var (
			index  = findInlinableTarget(modules, remaining)
			callee = modules[remaining[index]].(*descriptor.Function[W])
		)
		//
		for i, m := range modules {
			if fn, ok := m.(*descriptor.Function[W]); ok && fn != callee && !fn.IsNative() {
				modules[i] = inlineAllCalls[W](fn, remaining[index], callee)
			}
		}
		//
		remaining = slices.Delete(remaining, index, index+1)
	}
	// Remove now-dead targets, remapping module identifiers.
	return descriptor.NewProgram(program.Field(),
		program.MaxStaticHeight(), removeModules(modules, targets)...)
}

// resolveInlineTargets maps each name to its module identifier, sanity checking
// that every name identifies a distinct function which can actually be inlined
// (and removed).
func resolveInlineTargets[W word.Word[W]](modules []descriptor.Module[W]) []uint {
	var targets []uint
	// Find all inlineable targets
	for i, mod := range modules {
		//
		if fun, ok := mod.(*descriptor.Function[W]); ok && fun.Kind().CanInline() {
			targets = append(targets, uint(i))
		}
	}
	//
	return targets
}

// findInlinableTarget returns the index (within remaining) of a target whose
// body contains no calls to any remaining target, panicking if every target
// calls another (i.e. the named functions are recursive).
func findInlinableTarget[W word.Word[W]](modules []descriptor.Module[W], remaining []uint) int {
	for i, id := range remaining {
		if !callsAny(modules[id].(*descriptor.Function[W]), remaining) {
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
func callsAny[W word.Word[W]](fn *descriptor.Function[W], ids []uint) bool {
	for _, v := range fn.Vectors() {
		for _, insn := range v.Bytecodes {
			if call, ok := insn.(*bytecode.Call[W]); ok && slices.Contains(ids, uint(call.Target)) {
				return true
			}
		}
	}
	//
	return false
}

// inlineAllCalls inlines every call to a given callee within the body of a
// given function, returning the (possibly unchanged) result.
func inlineAllCalls[W word.Word[W]](fn *descriptor.Function[W], calleeId uint,
	callee *descriptor.Function[W]) *descriptor.Function[W] {
	//
	var (
		alloc   = split.NewAllocator(fn)
		code    = slices.Clone(fn.Vectors())
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
	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), code)
}

// findCall locates the first call to a given callee, returning the enclosing
// vector index, the position within that vector and the call itself (or nil if
// there is none).
func findCall[W word.Word[W]](code []BytecodeVector[W], calleeId uint) (uint, uint, *bytecode.Call[W]) {
	for i, v := range code {
		for j, insn := range v.Bytecodes {
			if call, ok := insn.(*bytecode.Call[W]); ok && uint(call.Target) == calleeId {
				return uint(i), uint(j), call
			}
		}
	}
	//
	return 0, 0, nil
}

// inlineCallSite splices the body of the callee into the caller's code in place
// of the call at position k within vector pc.  The enclosing vector is split
// around the call site, giving the following layout:
//
//	[0 .. pc)                                  unchanged (jumps remapped)
//	[pc]        v_pre  = codes[:k] ++ argument copies
//	[pc+1 .. exitPC)   callee body (registers shadowed, jumps rebased,
//	                   returns becoming jumps to exitPC)
//	[exitPC]    v_post = output copies ++ codes[k+1:]
//	[exitPC+1 ..]                              unchanged (jumps remapped)
//
// Since the callee body occupies len(callee.Vectors())+1 additional vectors,
// all jump targets beyond pc within the original code are shifted accordingly.
func inlineCallSite[W word.Word[W]](code []BytecodeVector[W], pc, k uint, call *bytecode.Call[W],
	callee *descriptor.Function[W], alloc split.Allocator[W]) []BytecodeVector[W] {
	//
	var (
		codes  = code[pc].Bytecodes
		nBody  = uint(len(callee.Vectors()))
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
	// Jumps amongst the codes preceding the call are remapped up front, rather
	// than at the splice below, since the fall-through jump which
	// buildEntryVector appends targets the inlined body and must not itself be
	// remapped.
	preCodes := remapJumps(bytecode.NewVector(codes[:k]...), pc, delta)
	// Construct entry vector (argument copies), falling through into the body.
	vPre := buildEntryVector[W](preCodes.Bytecodes, shadows.entryCopies, pc+1)
	// Construct callee body
	body := buildInlinedBody[W](callee, shadows.registers, pc+1, exitPC)
	// Construct exit vector (output copies)
	vPost := buildExitVector[W](codes[k+1:], shadows.exitCopies)
	// Splice, remapping all jumps within original caller vectors (including
	// v_post, whose codes originate from vector pc).
	ncode := make([]BytecodeVector[W], 0, uint(len(code))+delta)
	//
	for _, v := range code[:pc] {
		ncode = append(ncode, remapJumps(v, pc, delta))
	}
	//
	ncode = append(ncode, vPre)
	ncode = append(ncode, body...)
	ncode = append(ncode, remapJumps(vPost, pc, delta))
	//
	for _, v := range code[pc+1:] {
		ncode = append(ncode, remapJumps(v, pc, delta))
	}
	//
	return ncode
}

// checkCallSite ensures a given call site can be inlined.  Specifically, no skip
// before the call may cross over it, since such a skip cannot survive splitting
// the enclosing vector at the call site.  Note that a skip targeting the call
// itself is fine, as this lands on the argument copies (i.e. the call entry)
// after splitting.
func checkCallSite[W word.Word[W]](codes []Bytecode[W], k uint, callee *descriptor.Function[W]) {
	if len(callee.Vectors()) == 0 {
		panic(fmt.Sprintf("cannot inline function \"%s\" with empty body", callee.Name()))
	}
	//
	for j := range k {
		switch insn := codes[j].(type) {
		case *bytecode.Skip[W]:
			checkSkipDoesNotCross(j, uint(insn.Skip), k, callee)
		case *bytecode.SkipIf[W]:
			checkSkipDoesNotCross(j, uint(insn.Skip), k, callee)
		case *bytecode.Switch[W]:
			for _, c := range insn.Cases {
				checkSkipDoesNotCross(j, uint(c.Skip), k, callee)
			}
		}
	}
}

// checkSkipDoesNotCross panics when a skip of the given amount, located at
// position j, would jump over the call at position k.
func checkSkipDoesNotCross[W word.Word[W]](j, skip, k uint, callee *descriptor.Function[W]) {
	if j+skip+1 > k {
		panic(fmt.Sprintf(
			"cannot inline call to \"%s\" guarded by skip (inlining must be applied before vectorisation)",
			callee.Name()))
	}
}

// shadowMap describes how callee registers are realised within the caller at a
// given call site.  Every callee register maps to a caller register: either the
// corresponding argument / return register of the call itself (where this is
// provably equivalent), or a freshly allocated shadow (in which case a
// corresponding entry / exit copy is recorded).
type shadowMap struct {
	// registers maps each callee register to its caller-local realisation.
	registers []bytecode.RegisterId
	// entryCopies records (shadowed input, argument) pairs to be copied on entry
	// to the inlined body.
	entryCopies []registerCopy
	// exitCopies records (return register, shadowed output) pairs to be copied
	// on exit from the inlined body.
	exitCopies []registerCopy
}

// registerCopy records a register-to-register assignment.
type registerCopy struct {
	target, source bytecode.RegisterId
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
// itself aliased (the body could then clobber that argument whilst still reading
// it).
//
// In both cases, aliasing additionally requires the two registers to have
// identical shape: the elided copy performed a dynamic width check, which is
// vacuous exactly when the value already resides in a register of the same
// width.  Anything which cannot be aliased (including all temporaries) gets a
// fresh (computed) shadow register of the same shape.
func buildShadowMap[W word.Word[W]](call *bytecode.Call[W], callee *descriptor.Function[W],
	alloc split.Allocator[W]) shadowMap {
	//
	var (
		shadows    = shadowMap{registers: make([]bytecode.RegisterId, callee.Width())}
		written    = writtenRegisters(callee)
		numInputs  = callee.NumInputs()
		numOutputs = callee.NumOutputs()
		callerRegs = alloc.Registers()
		elidedArgs []bytecode.RegisterId
	)
	//
	for i, r := range callee.Registers() {
		var (
			index   = uint(i)
			aliased = false
			alias   bytecode.RegisterId
		)
		// Determine whether this register can be aliased.
		if index < numInputs {
			arg := call.Arguments[index]
			//
			if !written.Contains(index) && sameShape(callerRegs[arg], r) {
				alias, aliased = arg, true
			}
		} else if index < numInputs+numOutputs {
			var (
				j         = index - numInputs
				ret       = call.Returns[j]
				duplicate = slices.Contains(call.Returns[:j], ret) || slices.Contains(call.Returns[j+1:], ret)
			)
			// A discarded return binds no caller register, so it cannot be
			// aliased (it gets a fresh shadow and no exit copy below).
			if ret != bytecode.DISCARD && sameShape(callerRegs[ret], r) && !duplicate &&
				!slices.Contains(elidedArgs, ret) {
				alias, aliased = ret, true
			}
		}
		//
		if aliased {
			shadows.registers[i] = alias
			//
			if index < numInputs {
				elidedArgs = append(elidedArgs, alias)
			}
			//
			continue
		}
		// Allocate a fresh shadow of the same shape.
		shadows.registers[i] = alloc.Allocate(callee.Name()+"_"+r.Name(), r.Bitwidth())
		// Record the corresponding entry / exit copy.
		if index < numInputs {
			shadows.entryCopies = append(shadows.entryCopies,
				registerCopy{shadows.registers[i], call.Arguments[index]})
		} else if j := index - numInputs; j < numOutputs {
			// Where the same register receives several outputs, retain only the
			// last copy (matching the last-wins semantics of returning from a
			// stack frame) since sequential copies would conflict.  A discarded
			// return receives no copy at all.
			if call.Returns[j] != bytecode.DISCARD && !slices.Contains(call.Returns[j+1:], call.Returns[j]) {
				shadows.exitCopies = append(shadows.exitCopies,
					registerCopy{call.Returns[j], shadows.registers[i]})
			}
		}
	}
	//
	return shadows
}

// writtenRegisters returns the set of registers written anywhere within the body
// of a given function.
func writtenRegisters[W word.Word[W]](fn *descriptor.Function[W]) bit.Set {
	var written bit.Set
	//
	for _, v := range fn.Vectors() {
		for _, insn := range v.Bytecodes {
			for _, r := range insn.Definitions() {
				written.Insert(uint(r))
			}
		}
	}
	//
	return written
}

// sameShape checks whether two registers have identical shape, i.e. are both
// native, or both have the same declared width.
func sameShape[W word.Word[W]](a, b descriptor.Register[W]) bool {
	if a.IsNative() || b.IsNative() {
		return a.IsNative() && b.IsNative()
	}
	//
	return a.Bitwidth().Unwrap() == b.Bitwidth().Unwrap()
}

// buildEntryVector constructs the vector replacing the front portion of the
// vector enclosing the call site.  This retains all codes preceding the call,
// followed by copies of the argument registers into the shadowed callee inputs,
// and finally an explicit jump into the callee body (which begins at bodyPC).
// Such copies enforce the same dynamic width checks as entering the callee's
// stack frame did.
//
// Making the fall-through into the body explicit keeps this vector
// self-contained, as validation requires (see bytecode.Vector.Validate) and as
// Vectorize assumes.  It also guarantees the vector is non-empty --- as it must
// be in order to execute --- which matters when the call was the sole code of
// its vector and no argument copies were needed.
func buildEntryVector[W word.Word[W]](codes []Bytecode[W], copies []registerCopy,
	bodyPC uint) BytecodeVector[W] {
	//
	var ncodes = slices.Clone(codes)
	//
	for _, c := range copies {
		ncodes = append(ncodes, bytecode.Assign[W](c.target, c.source))
	}
	// Fall through into the callee body, which immediately follows.  Observe
	// that appending here cannot disturb any skip within codes, since skip
	// targets are relative to their own position.  Indeed, a skip targeting the
	// call itself (which checkCallSite permits) lands on the first argument copy
	// or, when there are none, on this jump --- either way, at the callee's
	// entry.
	ncodes = append(ncodes, bytecode.Jump[W](bytecode.Address(bodyPC)))
	//
	return bytecode.NewVector(ncodes...)
}

// buildExitVector constructs the vector replacing the back portion of the vector
// enclosing the call site.  This copies the shadowed callee outputs into the
// call's return registers, followed by all codes succeeding the call.  Placing
// the copies here (rather than at each return site within the body) ensures they
// are emitted exactly once.
func buildExitVector[W word.Word[W]](codes []Bytecode[W], copies []registerCopy) BytecodeVector[W] {
	var ncodes []Bytecode[W]
	//
	for _, c := range copies {
		ncodes = append(ncodes, bytecode.Assign[W](c.target, c.source))
	}
	//
	ncodes = append(ncodes, codes...)
	// Vectors must be non-empty in order to execute.
	if len(ncodes) == 0 {
		ncodes = append(ncodes, bytecode.NewSkip[W](0))
	}
	//
	return bytecode.NewVector(ncodes...)
}

// buildInlinedBody instantiates the callee's body at a given call site.  All
// registers are substituted for their caller-local shadows; internal jumps are
// rebased onto the caller's program counter; and returns become jumps to the
// exit vector (which performs the output copies).
func buildInlinedBody[W word.Word[W]](callee *descriptor.Function[W], shadows []bytecode.RegisterId,
	base, exitPC uint) []BytecodeVector[W] {
	//
	var body = make([]BytecodeVector[W], len(callee.Vectors()))
	//
	for i, v := range callee.Vectors() {
		body[i] = v.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			switch insn := insn.(type) {
			case *bytecode.Ret[W]:
				return []Bytecode[W]{bytecode.Jump[W](bytecode.Address(exitPC))}
			case *bytecode.Jmp[W]:
				return []Bytecode[W]{bytecode.Jump[W](bytecode.Address(base) + insn.Target)}
			default:
				return []Bytecode[W]{substituteRegisters[W](insn, shadows)}
			}
		})
	}
	//
	return body
}

// substituteRegisters reconstructs a given bytecode with every register
// substituted according to a given mapping.
//
//nolint:gocyclo
func substituteRegisters[W word.Word[W]](insn Bytecode[W], sub []bytecode.RegisterId) Bytecode[W] {
	switch insn := insn.(type) {
	case *bytecode.Arith[W]:
		return bytecode.NewArith(insn.Op, substituteIds(insn.Target, sub), substituteIds(insn.Source, sub), insn.Constant)
	case *bytecode.Bitwise[W]:
		return bytecode.NewBitwise(insn.Op, substituteId(insn.Target, sub), substituteId(insn.Left, sub),
			substituteOperandVector(insn.Right, sub), insn.Bitwidth)
	case *bytecode.FieldArith[W]:
		return bytecode.NewFieldArith(insn.Op, substituteId(insn.Target, sub), substituteIds(insn.Sources, sub),
			insn.Constant)
	case *bytecode.Cat[W]:
		return bytecode.AssignV[W](substituteIds(insn.Targets, sub), substituteIds(insn.Sources, sub)...)
	case *bytecode.UintToField[W]:
		return &bytecode.UintToField[W]{Target: substituteId(insn.Target, sub), Source: substituteIds(insn.Source, sub)}
	case *bytecode.FieldToUint[W]:
		return &bytecode.FieldToUint[W]{Target: substituteIds(insn.Target, sub), Source: substituteId(insn.Source, sub)}
	case *bytecode.Call[W]:
		return bytecode.CallFun[W](insn.Target, substituteIds(insn.Arguments, sub),
			substituteIds(insn.Returns, sub))
	case *bytecode.ReadWrite[W]:
		if insn.Write {
			return bytecode.NewMemWrite[W](insn.Id, substituteIds(insn.Address, sub), substituteIds(insn.Data, sub),
				substituteIds(insn.Stamp, sub))
		}
		//
		return bytecode.NewMemRead[W](insn.Id, substituteIds(insn.Address, sub), substituteIds(insn.Data, sub),
			substituteIds(insn.Stamp, sub))
	case *bytecode.DivRem[W]:
		return &bytecode.DivRem[W]{
			Quotient:  substituteId(insn.Quotient, sub),
			Remainder: substituteId(insn.Remainder, sub),
			Dividend:  substituteId(insn.Dividend, sub),
			Divisor:   substituteOperandVector(insn.Divisor, sub)}
	case *bytecode.Intrinsic[W]:
		return bytecode.NewIntrinsic[W](insn.Op, substituteRegisterVectors(insn.Targets, sub),
			substituteOperandVectors(insn.Sources, sub))
	case *bytecode.CheckCast[W]:
		return bytecode.NewCheckCast[W](substituteId(insn.Target, sub), insn.Bitwidth)
	case *bytecode.Skip[W]:
		return insn
	case *bytecode.Jmp[W], *bytecode.Ret[W]:
		// Register-free; present only when substituting a full function body.
		return insn
	case *bytecode.SkipIf[W]:
		return &bytecode.SkipIf[W]{Op: insn.Op, Skip: insn.Skip,
			Left: substituteRegisterVector(insn.Left, sub), Right: substituteOperandVector(insn.Right, sub)}
	case *bytecode.Switch[W]:
		return bytecode.MultiwaySkip(substituteId(insn.Source, sub), insn.Cases)
	case *bytecode.Dispatch[W]:
		cases := make([]bytecode.DispatchCase, len(insn.Cases))
		//
		for i, c := range insn.Cases {
			cases[i] = bytecode.DispatchCase{Bit: substituteId(c.Bit, sub), Skip: c.Skip}
		}
		//
		return bytecode.NewDispatch[W](cases, substituteId(insn.Default, sub))
	case *bytecode.Debug[W]:
		return &bytecode.Debug[W]{Chunks: insn.Chunks, Sources: substituteRegisterVectors(insn.Sources, sub)}
	case *bytecode.Fail[W]:
		return &bytecode.Fail[W]{Chunks: insn.Chunks, Sources: substituteRegisterVectors(insn.Sources, sub)}
	default:
		panic(fmt.Sprintf("unexpected instruction in inlined body (%T)", insn))
	}
}

func substituteId(id bytecode.RegisterId, sub []bytecode.RegisterId) bytecode.RegisterId {
	return sub[id]
}

func substituteIds(ids []bytecode.RegisterId, sub []bytecode.RegisterId) []bytecode.RegisterId {
	var nids = make([]bytecode.RegisterId, len(ids))
	//
	for i, id := range ids {
		// A discarded binding has no register to substitute.
		if id == bytecode.DISCARD {
			nids[i] = id
		} else {
			nids[i] = sub[id]
		}
	}
	//
	return nids
}

func substituteOperandVector[W word.Word[W]](v bytecode.Operand[W], sub []bytecode.RegisterId,
) bytecode.Operand[W] {
	//
	if v.IsConstant() {
		return v
	}
	//
	rvec := substituteRegisterVector(v.AsRegisterVector(), sub)
	//
	return bytecode.NewRegisterVectorOperand[W](rvec)
}

func substituteOperandVectors[W word.Word[W]](vs []bytecode.Operand[W], sub []bytecode.RegisterId,
) []bytecode.Operand[W] {
	var nvs = make([]bytecode.Operand[W], len(vs))
	//
	for i, v := range vs {
		nvs[i] = substituteOperandVector(v, sub)
	}
	//
	return nvs
}

// substituteRegisterVector reconstructs a register vector with each constituent register
// substituted according to a given mapping.  The substituted registers must
// remain consecutive (a RegisterVector invariant); this holds before register splitting,
// where every such vector is a single register.
func substituteRegisterVector(v bytecode.RegisterVector, sub []bytecode.RegisterId) bytecode.RegisterVector {
	return bytecode.NewRegisterVector(substituteIds(v.Registers(), sub)...)
}

func substituteRegisterVectors(vs []bytecode.RegisterVector, sub []bytecode.RegisterId) []bytecode.RegisterVector {
	var nvs = make([]bytecode.RegisterVector, len(vs))
	//
	for i, v := range vs {
		nvs[i] = substituteRegisterVector(v, sub)
	}
	//
	return nvs
}

// remapJumps shifts all jump targets beyond a given program counter down by a
// given amount, accounting for the vectors inserted by inlining.  A target of pc
// itself is retained, since the entry vector occupies that position in the new
// layout.
func remapJumps[W word.Word[W]](v BytecodeVector[W], pc, delta uint) BytecodeVector[W] {
	var (
		ncodes  = make([]Bytecode[W], len(v.Bytecodes))
		changed = false
	)
	//
	for i, insn := range v.Bytecodes {
		if jmp, ok := insn.(*bytecode.Jmp[W]); ok && uint(jmp.Target) > pc {
			ncodes[i] = bytecode.Jump[W](bytecode.Address(uint(jmp.Target) + delta))
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
	return bytecode.NewVector(ncodes...)
}

// removeModules removes the given target modules, remapping the module
// identifiers embedded within the bytecodes of all remaining functions
// accordingly.
func removeModules[W word.Word[W]](modules []descriptor.Module[W], targets []uint) []descriptor.Module[W] {
	var (
		idMap = make([]uint, len(modules))
		kept  []descriptor.Module[W]
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
		if fn, ok := m.(*descriptor.Function[W]); ok {
			kept[i] = remapModuleIds(fn, idMap)
		}
	}
	//
	return kept
}

// remapModuleIds reconstructs a given function with all module identifiers —
// those embedded in its bytecodes and those of its declared memory effects —
// substituted according to a given mapping.
func remapModuleIds[W word.Word[W]](fn *descriptor.Function[W], idMap []uint) *descriptor.Function[W] {
	var (
		code    = make([]BytecodeVector[W], len(fn.Vectors()))
		effects = fn.Effects()
		changed = false
	)
	//
	for i, v := range fn.Vectors() {
		var ncodes = make([]Bytecode[W], len(v.Bytecodes))
		//
		for j, insn := range v.Bytecodes {
			ncodes[j] = remapModuleId[W](insn, idMap)
			changed = changed || ncodes[j] != insn
		}
		//
		code[i] = bytecode.NewVector(ncodes...)
	}
	// Remap the memory effects (memories are never removed, so every effect
	// survives the mapping).
	remapped := slices.Clone(effects)
	//
	for i, e := range effects {
		remapped[i] = descriptor.ModuleId(idMap[e])
		changed = changed || remapped[i] != e
	}
	//
	effects = remapped
	//
	if !changed {
		return fn
	}
	//
	return descriptor.NewFunction(fn.Name(), fn.Registers(), fn.Kind(), effects, code)
}

func remapModuleId[W word.Word[W]](insn Bytecode[W], idMap []uint) Bytecode[W] {
	var id uint
	// Extract module identifier (if applicable)
	switch insn := insn.(type) {
	case *bytecode.Call[W]:
		id = idMap[insn.Target]
	case *bytecode.ReadWrite[W]:
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
	case *bytecode.Call[W]:
		if id != uint(insn.Target) {
			return bytecode.CallFun[W](bytecode.ModuleId(id), insn.Arguments, insn.Returns)
		}
	case *bytecode.ReadWrite[W]:
		if id != uint(insn.Id) {
			if insn.Write {
				return bytecode.NewMemWrite[W](bytecode.ModuleId(id), insn.Address, insn.Data, insn.Stamp)
			}
			//
			return bytecode.NewMemRead[W](bytecode.ModuleId(id), insn.Address, insn.Data, insn.Stamp)
		}
	}
	//
	return insn
}
