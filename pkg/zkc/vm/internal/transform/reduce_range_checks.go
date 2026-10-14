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
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

// ReduceRangeChecks attempts to safely reduce the number of registers marked as
// "safe" (i.e. as requiring a "range check").  There are a variety of
// situations in which a register can be marked as unsafe without compromising
// overall security of the system.  For example, registers which are always
// assigned constants do not need range checks.  Likewise, a register "r" which
// is always assigned directly from a safe register (e.g. "r = x")  does not
// require a range check (i.e. because safety of "r" follows from that of "x" in
// this case).  However, in other situations, safety must still be enforced. For
// example, if "r" is assigned from an arithmetic expression (e.g. "r = x + 1")
// then its safety does not follow from that of "x".
//
// The present algorithm works in an "assume until proven" style.  Specifically,
// it initially marks all registers as unsafe and then scans all bytecodes
// whilst updating the assignment accordingly.  Specifically, when it encounters
// an instruction whose target register(s) cannot be assumed safe, it marks
// those registers as being safe (i.e. as needing a range check).  This process
// continues until all bytecodes have been examined exactly once.
//
// NOTE: in principle, this algorithm guarantees arguments are never marked as
// safe (i.e. since they are never assigned).  However, to ensure soundness,
// such registers are still marked safe (for now).  In particular, they can be
// marked unsafe again once checkcast instructions give rise to conditional
// lookups --- see #2246.
//
// Some assumptions / exceptions are currently respected by this algorithm (and
// which could be lifted in the future).  In particular, the algorithm currently
// assumes that: (1) native functions require range checks on all declared
// input/output registers; (2) that the registers used in RAM, ROM, WOM, SROM
// modules are range checked.
func ReduceRangeChecks[W Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods = program.Modules()
		//
		out = make([]descriptor.Module[W], len(mods))
	)
	//
	for mid, ith := range mods {
		// apply reduction to non-native functions only.
		if f, ok := ith.(*Function[W]); ok && !f.IsNative() {
			var env = program.EnvironmentOf(uint16(mid))
			//
			ith = reduceFunctionRangeChecks(f, env)
		}
		//
		out[mid] = ith
	}
	//
	return descriptor.NewProgram(program.Field(), program.MaxStaticHeight(), out...)
}

func reduceFunctionRangeChecks[W Word[W]](f *Function[W], env Environment[W]) *Function[W] {
	var (
		// identifies registers which must be safe.
		safeRegs bit.Set
	)
	// Iterate all bytecodes in all vectors
	for _, bv := range f.Vectors() {
		for _, bc := range bv.Bytecodes {
			safeRegs = reviseSafetyAssignment(safeRegs, env, bc)
		}
	}
	// Construct revised register set
	registers := array.Map(f.Registers(), func(i uint, r Register[W]) Register[W] {
		// Determine whether safety required or not
		//
		// TODO: include zero-width registers here is a temporary fix until
		// constant registers are available (#2236)
		var safety = safeRegs.Contains(i) || r.IsInput() || r.IsZeroWidth()
		// Construct identical register with corresponding safety.
		return bytecode.NewRegister(r.Kind().WithSafety(safety), r.Name(), r.Bitwidth(), r.Padding())
	})
	// Done
	return descriptor.NewFunction(f.Name(), registers, f.Kind(), f.Effects(), f.Vectors())
}

func reviseSafetyAssignment[W Word[W]](safeRegs bit.Set, env Environment[W], code Bytecode[W]) bit.Set {
	switch bc := code.(type) {
	// ========================================================================
	// Non-mutating Bytecodes.  These do not assign target registers and can be
	// safely ignored.
	// ========================================================================
	case *bytecode.CheckCast[W]:
	case *bytecode.Debug[W]:
	case *bytecode.Dispatch[W]:
	case *bytecode.Fail[W]:
	case *bytecode.Jmp[W]:
	case *bytecode.Ret[W]:
	case *bytecode.Skip[W]:
	case *bytecode.SkipIf[W]:
	case *bytecode.Switch[W]:
	// ========================================================================
	// Mutating Bytecodes.  These assign target registers and must be examined.
	// ========================================================================
	case *bytecode.Arith[W]:
		return reviseArithSafety(safeRegs, env, bc)
	case *bytecode.Call[W]:
		var callee = env.Module(bc.Target).Unwrap().(*Function[W])
		//
		return reviseAssignmentsSafety(safeRegs, env, bc.Returns, callee.Outputs())
	case *bytecode.Cat[W]:
		return reviseCatSafety(safeRegs, env, bc)
	case *bytecode.FieldToUint[W]:
		return reviseFieldCastSafety(safeRegs, env, bc)
	case *bytecode.ReadWrite[W]:
		var callee = env.Module(bc.Id).Unwrap().(*descriptor.Memory[W])
		// Only revise assignment for reads
		if !bc.Write {
			return reviseAssignmentsSafety(safeRegs, env, bc.Data, callee.Outputs())
		}
	case *bytecode.Intrinsic[W]:
		return reviseIntrinsicSafety(safeRegs, env, bc)
	// ========================================================================
	// Miscellaneous special cases
	// ========================================================================
	case *bytecode.FieldArith[W]:
		// NOTE: since native registers are always considered unsafe, this
		// instruction cannot impact the assignment of safe registers.
	case *bytecode.UintToField[W]:
		// NOTE: since native registers are always considered unsafe, this
		// instruction cannot impact the assignment of safe registers.
	case *bytecode.Bitwise[W]:
		// NOTE: this cannot arise in the normal case of things, primarily
		// because bitwise operations are compiled out before this point.
		// However, this might not happen if the lowering is disabled (e.g. for
		// debugging) and, instead, we want a simple (but safe) approach here.
		return markAll(safeRegs, bc.Target)
	case *bytecode.DivMod[W]:
		// NOTE: this cannot arise in the normal case of things, primarily
		// because divmod operations are compiled out before this point.
		// However, this might not happen if the lowering is disabled (e.g. for
		// debugging) and, instead, we want a simple (but safe) approach here.
		return markAll(safeRegs, bc.Quotient, bc.Remainder)
	default:
		// Determine type of bytecode
		var tn = reflect.TypeOf(bc).String()
		// Return somewhat useful error
		panic(fmt.Sprintf("unexpected bytecode encountered: %s", tn))
	}
	//
	return safeRegs
}

func reviseArithSafety[W Word[W]](safeRegs bit.Set, env Environment[W], b *bytecode.Arith[W]) bit.Set {
	var (
		bits    util.Option[uint]
		sources = b.Source
	)
	//
	switch b.Op {
	case bytecode.OP_ADD:
		bits = descriptor.CalculateAddBitwidth(sources, b.Constant, env)
	case bytecode.OP_MUL:
		bits = descriptor.CalculateMulBitwidth(sources, b.Constant, env)
	case bytecode.OP_SUB:
		// Subtraction can always underflow.
		bits = util.Some[uint](math.MaxUint)
	default:
		panic("unknown arithmetic instruction")
	}
	//
	return reviseAssignmentSafety(safeRegs, env, b.Target, bits)
}

func reviseCatSafety[W Word[W]](safeRegs bit.Set, env Environment[W], b *bytecode.Cat[W]) bit.Set {
	var (
		// Determine size of rhs
		bits = descriptor.BitwidthOf(env, b.Sources...)
	)
	//
	return reviseAssignmentSafety(safeRegs, env, b.Targets, bits)
}

func reviseFieldCastSafety[W Word[W]](safeRegs bit.Set, env Environment[W], b *bytecode.FieldToUint[W],
) bit.Set {
	var (
		// Determine size of rhs
		bits = util.Some(env.Field().BitLen())
	)
	//
	return reviseAssignmentSafety(safeRegs, env, b.Target, bits)
}

func reviseIntrinsicSafety[W Word[W]](safeRegs bit.Set, _ Environment[W], b *bytecode.Intrinsic[W]) bit.Set {
	util.Assert(b.Op == bytecode.DIV_HINT, "unexpected intrinsic bytecode")
	// NOTE: the DIV_HINT intrinsic is a hint, meaning its assigned registers
	// must be treated as requiring safety checks.
	return markAll(safeRegs, b.Definitions()...)
}

func reviseAssignmentsSafety[W Word[W]](safeRegs bit.Set, env Environment[W], lhs []RegisterId, rhs []Register[W],
) bit.Set {
	//
	for i := range lhs {
		safeRegs = reviseAssignmentSafety(safeRegs, env, lhs[i:i+1], rhs[i].Bitwidth())
	}
	//
	return safeRegs
}

// Update register safety after assigning some number of bits on the right-hand
// side (RHS) across some number of registers on the left-hand side (LHS).
func reviseAssignmentSafety[W Word[W]](safeRegs bit.Set, env Environment[W], lhs []RegisterId,
	rhs util.Option[uint]) bit.Set {
	// Destructuring assignments always force their target registers to require
	// safety checks.
	if len(lhs) == 1 && safeAssignment(env, lhs[0], rhs) {
		return safeRegs
	}
	// Mark all registers as safe (hence requiring safety checks)
	return markAll(safeRegs, lhs...)
}

func safeAssignment[W Word[W]](rmap Environment[W], lhs RegisterId, rhs util.Option[uint]) bool {
	var lhsWidth util.Option[uint]
	//
	if lhs == bytecode.DISCARD {
		// Discarded registers do not bind to real columns, hence no check
		// required.
		return true
	} else if lhsWidth = rmap.Register(lhs).Bitwidth(); lhsWidth.IsEmpty() {
		// Native registers never have safety checks.
		return true
	}
	// Sanity check
	util.Assert(rhs.HasValue(), "native and uint registers cannot mix")
	// Final check to determine whether assignment can overflow lhs.
	return lhsWidth.Unwrap() >= rhs.Unwrap()
}

func markAll(safeRegs bit.Set, regs ...RegisterId) bit.Set {
	// Mark all registers as safe (hence requiring safety checks)
	for _, lv := range regs {
		safeRegs.Insert(uint(lv))
	}
	//
	return safeRegs
}
