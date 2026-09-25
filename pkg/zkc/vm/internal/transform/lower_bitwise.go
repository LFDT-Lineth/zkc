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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerBitwise rewrites VM-level NOT and SHL/SHR bytecodes: NOT is inlined as
// (MASK - x), while SHL/SHR become CALLs to helper functions whose modules are
// appended to the returned program.  AND/OR/XOR are left untouched here and are
// lowered after register splitting instead (see LowerOrXorAnd).
//
// We assume this lowering happens BEFORE vectorization and register splitting.
func LowerBitwise[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		out     = slices.Clone(program.Modules())
		helpers = newShiftHelpers[W](uint(len(out)), scanShiftParams(out))
	)

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerBitwiseFunction(fn, func(b Bytecode[W], alloc split.Allocator[W]) []Bytecode[W] {
				return lowerBitwiseCode(b, alloc, helpers)
			})
		}
	}

	return descriptor.NewProgram(
		program.Field(),
		program.MaxStaticHeight(),
		append(out, helpers.modules()...)...)
}

// lowerBitwiseFunction rewrites each bytecode of a function via codeFn,
// threading a per-function register allocator (for any temporaries codeFn
// introduces).  The helper registry, if any, is captured by the codeFn closure
// — this driver is shared by LowerBitwise and LowerOrXorAnd, whose registries
// differ.
func lowerBitwiseFunction[W word.Word[W]](fn *descriptor.Function[W],
	codeFn func(Bytecode[W], split.Allocator[W]) []Bytecode[W],
) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return codeFn(b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

func lowerBitwiseCode[W word.Word[W]](
	b Bytecode[W],
	registers split.Allocator[W],
	helpers *shiftHelpers[W],
) []Bytecode[W] {
	//
	bw, ok := b.(*bytecode.Bitwise[W])
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	switch bw.Op {
	case bytecode.OP_NOT:
		return inlineBitwiseNot(bw, registers)
	case bytecode.OP_SHL, bytecode.OP_SHR:
		return lowerBitwiseShlShr(bw, registers, helpers)
	default:
		// AND/OR/XOR are lowered after register splitting; see LowerOrXorAnd.
		return []Bytecode[W]{b}
	}
}

// lowerBitwiseShlShr rewrites a SHL/SHR into a call to the shared cascade,
// which operates at helpers.maxWidth() and returns both directions as
// (out_shl, out_shr).  A partial call binds only the requested direction and
// discards the other.
//
// A call site narrower than maxWidth needs no explicit widening.
//
// Narrowing the result back down is only necessary for SHL.  Binding a maxWidth
// output into a w-bit target likewise pads the high limbs to zero, which
// asserts the result fits in w bits:
//
//   - For SHR that assertion always holds — the operand is zero above bit w, so
//     the shifted result is too — making the direct binding both sound and free.
//   - For SHL the discarded high bits are legitimately non-zero, the result must
//     therefore come back at maxWidth and be destructed.
func lowerBitwiseShlShr[W word.Word[W]](
	b *bytecode.Bitwise[W],
	registers split.Allocator[W],
	helpers *shiftHelpers[W],
) []Bytecode[W] {
	var (
		// NOTE: the shift amount is always a register (constant operands are
		// only supported for AND/OR/XOR).
		amount = b.Right.AsRegister()
		// NOTE: bitwidth of shift (e.g. "x << y") determined by width of first
		// argument only (i.e. "x").
		amtWidth = registers.Registers()[amount].Bitwidth().Unwrap()
		width    = uint(b.Bitwidth)
		maxWidth = helpers.maxWidth()
		id       = helpers.ensureShift(amtWidth)
		// Only a narrowing SHL has to route its result through a wider register.
		truncate = b.Op == bytecode.OP_SHL && width < maxWidth
	)
	//
	result := b.Target
	if truncate {
		result = registers.Allocate("raw_shl_res", util.Some(maxWidth))
	}
	// Bind only the requested direction, discarding the other.
	var returns []bytecode.RegisterId

	switch b.Op {
	case bytecode.OP_SHL:
		returns = []bytecode.RegisterId{result, bytecode.DISCARD}
	case bytecode.OP_SHR:
		returns = []bytecode.RegisterId{bytecode.DISCARD, result}
	default:
		panic("unsupported opcode")
	}
	//
	code := []Bytecode[W]{bytecode.CallFun[W](uint16(id),
		[]bytecode.RegisterId{b.Left, amount}, returns)}
	// Truncate by destructing off the high bits (little-endian, so the target
	// takes the low width bits).
	if truncate {
		high := registers.Allocate("shl_res_high", util.Some(maxWidth-width))
		code = append(code, bytecode.AddVec[W](
			[]bytecode.RegisterId{b.Target, high}, []bytecode.RegisterId{result}))
	}

	return code
}

// inlineBitwiseNot emits ~x as (MASK - x) directly into the caller's bytecode
// stream, where MASK = 2^width - 1.  No helper module is created.
func inlineBitwiseNot[W word.Word[W]](b *bytecode.Bitwise[W], registers split.Allocator[W]) []Bytecode[W] {
	var (
		width, _ = maxBitwidthOf(registers.Registers(), b.Uses()...)
		zero     W
		mask     = zero.Not(width)
	)

	maskReg := registers.Allocate("", util.Some(width))
	// TODO: CSUB, see: https://github.com/LFDT-Lineth/zkc/issues/2062
	return []Bytecode[W]{
		bytecode.LoadConst(maskReg, mask),
		bytecode.SubConst(b.Target, []bytecode.RegisterId{maskReg, b.Left}, zero),
	}
}

func maxBitwidthOf[W word.Word[W]](regs []descriptor.Register[W], targets ...bytecode.RegisterId) (uint, bool) {
	var w uint
	//
	for _, src := range targets {
		reg := regs[src]

		if reg.IsNative() {
			panic("unexpected native register in bitwise lowering")
		} else if reg.Bitwidth().Unwrap() == 0 {
			panic(fmt.Sprintf("zero-width register: %s", reg.Name()))
		}
		//
		w = max(w, reg.Bitwidth().Unwrap())
	}

	return w, w&(w-1) == 0
}

// bitwiseOpName is the short name used in helper module names for a bitwise
// operation.  SHL/SHR have no entry: both directions are served by a single
// merged cascade whose name is fixed (see shiftHelperName).
func bitwiseOpName(op bytecode.Operation) string {
	switch op {
	case bytecode.OP_AND:
		return "and"
	case bytecode.OP_OR:
		return "or"
	case bytecode.OP_XOR:
		return "xor"
	case bytecode.OP_NOT:
		return "not"
	default:
		panic(fmt.Sprintf("unexpected op: %v", op))
	}
}
