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
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerBitwise rewrites VM-level bitwise bytecodes into CALLs to helper
// functions. The helper modules are appended to the returned program.
// We assume this lowering happens BEFORE vectorization and register splitting.
func LowerBitwise[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		out          = slices.Clone(program.Modules())
		amountWidths = scanShiftAmountWidths[W](out)
		helpers      = newBitwiseHelpers[W](uint(len(out)), amountWidths)
	)

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerBitwiseFunction(fn, helpers)
		}
	}

	return descriptor.NewProgram(program.Field(), append(out, helpers.modules()...)...)
}

func lowerBitwiseFunction[W word.Word[W]](fn *descriptor.Function[W], helpers *bitwiseHelpers[W],
) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = newRegAllocator(fn.Registers())
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return lowerBitwiseCode(b, alloc, helpers)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.IsNative(), nvecs)
}

func lowerBitwiseCode[W word.Word[W]](
	b Bytecode[W],
	registers *regAllocator[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	//
	bw, ok := b.(*bytecode.Bitwise)
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	switch bw.Op {
	case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
		return lowerBitwiseAndOrXor(bw, registers, helpers)
	case bytecode.OP_NOT:
		return inlineBitwiseNot[W](bw, registers)
	case bytecode.OP_SHL, bytecode.OP_SHR:
		return lowerBitwiseShlShr(bw, helpers)
	default:
		return []Bytecode[W]{b}
	}
}

func lowerBitwiseAndOrXor[W word.Word[W]](
	b *bytecode.Bitwise,
	registers *regAllocator[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	origWidth, isPowerOfTwo := maxBitwidthOf(registers.Registers(), b.Uses()...)
	//
	p := origWidth
	if !isPowerOfTwo {
		p = util_math.NextPowerOfTwo(origWidth)
	}
	// After BinarizeBitwise, any non-identity constant has been
	// materialised as a source register, so we can drop the constant
	// argument here: at the (possibly widened) helper width the original
	// identity mask is redundant because the cast already zero-extends
	// inputs.
	id := helpers.ensure(b.Op, p, 2)
	//
	return []Bytecode[W]{
		bytecode.CallFun(uint16(id), bytecode.CallFlags{},
			[]bytecode.RegisterId{b.Left, b.Right}, []bytecode.RegisterId{b.Target}),
	}
}

func lowerBitwiseShlShr[W word.Word[W]](
	b *bytecode.Bitwise,
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	var (
		// NOTE: bitwidth of shift (e.g. "x << y") determined by width of first
		// argument only (i.e. "x").
		id = helpers.ensure(b.Op, uint(b.Bitwidth), 2)
	)
	//
	return []Bytecode[W]{
		bytecode.CallFun(uint16(id), bytecode.CallFlags{},
			[]bytecode.RegisterId{b.Left, b.Right}, []bytecode.RegisterId{b.Target}),
	}
}

// inlineBitwiseNot emits ~x as (MASK - x) directly into the caller's bytecode
// stream, where MASK = 2^width - 1.  No helper module is created.
func inlineBitwiseNot[W word.Word[W]](b *bytecode.Bitwise, registers *regAllocator[W]) []Bytecode[W] {
	var (
		width, _ = maxBitwidthOf(registers.Registers(), b.Uses()...)
		maskBig  = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width), big.NewInt(1))
		zeroW    W
		mask     = zeroW.SetBigInt(maskBig)
		zero     W
	)

	maskReg := registers.Allocate("", util.Some(width))

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

type bitwiseHelperKey struct {
	op    bytecode.Operation
	width uint
	arity int
}

type bitwiseHelpers[W word.Word[W]] struct {
	baseID       uint
	ids          map[bitwiseHelperKey]uint
	items        []descriptor.Module[W]
	amountWidths map[shiftKey]uint
}

func newBitwiseHelpers[W word.Word[W]](
	baseID uint, amountWidths map[shiftKey]uint,
) *bitwiseHelpers[W] {
	return &bitwiseHelpers[W]{
		baseID:       baseID,
		ids:          make(map[bitwiseHelperKey]uint),
		amountWidths: amountWidths,
	}
}

// shiftAmountWidth returns the canonical shift-amount register width for a
// given (opcode, value-width) pair: the maximum seen across all call sites,
// defaulting to valueWidth if no entry was recorded.
func (p *bitwiseHelpers[W]) shiftAmountWidth(op bytecode.Operation, valueWidth uint) uint {
	if w, ok := p.amountWidths[shiftKey{op: op, width: valueWidth}]; ok {
		return w
	}

	return valueWidth
}

func (p *bitwiseHelpers[W]) modules() []descriptor.Module[W] {
	return p.items
}

func (p *bitwiseHelpers[W]) ensure(op bytecode.Operation, width uint, arity int) uint {
	key := bitwiseHelperKey{
		op:    op,
		width: width,
		arity: arity,
	}

	if id, ok := p.ids[key]; ok {
		return id
	}

	// SHL/SHR are self-recursive: pre-register the ID before the factory runs
	// so any re-entrant ensure call for the same key resolves correctly.
	if op == bytecode.OP_SHL || op == bytecode.OP_SHR {
		id := p.baseID + uint(len(p.items))
		p.ids[key] = id

		amtWidth := p.shiftAmountWidth(op, width)

		var mod descriptor.Module[W]
		if op == bytecode.OP_SHL {
			mod = newShlHelper[W](key, id, amtWidth)
		} else {
			mod = newShrHelper[W](key, id, amtWidth)
		}

		p.items = append(p.items, mod)

		return id
	}

	// AND/OR/XOR: the factory may recursively call ensure for sub-helpers,
	// which appends them to p.items.  The current module must occupy the slot
	// AFTER all its sub-helpers (callees before callers), so its ID is derived
	// from len(p.items) only after the factory returns.
	mod := newDecomposedNaryHelper[W](p, key)

	id := p.baseID + uint(len(p.items))
	p.items = append(p.items, mod)
	p.ids[key] = id

	return id
}

// newDecomposedNaryHelper builds a helper module for bitwise AND/OR/XOR using
// recursive halving.  Each module body is O(arity) instructions: it splits
// every source into two half-wide pieces, calls a single half-wide sub-helper
// for each piece, and recombines.  The body is independent of any caller-side
// constant — non-identity constants are materialised as register sources by
// BinarizeBitwise before lowering reaches here, so a single helper per (op,
// width, arity) is sufficient and shared across all call sites.
func newDecomposedNaryHelper[W word.Word[W]](
	helpers *bitwiseHelpers[W],
	key bitwiseHelperKey,
) descriptor.Module[W] {
	b := newHelperBuilder[W](key.width, key.arity)

	out := b.output
	zero := word.Const64[W](0)

	// TODO: see https://github.com/LFDT-Lineth/zkc/issues/1747
	// we will want to stop before width == 1 to reduce the number of tiny modules.
	if key.width == 1 {
		// Base case: single-bit operation.  Seed agg with the op's identity
		// (1 for AND, 0 for OR/XOR) then fold each source in via the
		// appropriate pairwise combinator.
		agg := b.newComputed("agg")

		if key.op == bytecode.OP_AND {
			one := word.Const64[W](1)
			b.emit(bytecode.LoadConst(agg, one))
		} else {
			b.emit(bytecode.LoadConst(agg, zero))
		}

		for _, inp := range b.inputs {
			agg = b.combineBit(key.op, agg, inp)
		}

		b.emit(bytecode.Move[W](out, agg))
	} else {
		// Recursive case: low and high halves share the same sub-helper
		// because the body no longer depends on a caller-side constant.
		half := key.width / 2
		subID := helpers.ensure(key.op, half, key.arity)

		lowSrcs := make([]bytecode.RegisterId, key.arity)
		highSrcs := make([]bytecode.RegisterId, key.arity)

		for i, arg := range b.inputs {
			lo := b.newComputedNamed(half)
			hi := b.newComputedNamed(half)
			// UintDestruct: split arg into [lo, hi] (little-endian limbs).
			b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{lo, hi}, []bytecode.RegisterId{arg}))
			lowSrcs[i] = lo
			highSrcs[i] = hi
		}

		resLow := b.newComputedNamed(half)
		resHigh := b.newComputedNamed(half)

		b.emit(bytecode.CallFun(uint16(subID), bytecode.CallFlags{}, lowSrcs, []bytecode.RegisterId{resLow}))
		b.emit(bytecode.CallFun(uint16(subID), bytecode.CallFlags{}, highSrcs, []bytecode.RegisterId{resHigh}))

		b.emit(bytecode.Concat([]bytecode.RegisterId{out}, []bytecode.RegisterId{resLow, resHigh}))
	}

	b.emit(bytecode.NewRet())

	return descriptor.NewFunction(helperName(key), b.regs(), false,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}

type helperBuilder[W word.Word[W]] struct {
	width   uint
	inputs  []bytecode.RegisterId
	output  bytecode.RegisterId
	base    []descriptor.Register[W]
	code    []Bytecode[W]
	nextTmp uint
}

func newHelperBuilder[W word.Word[W]](width uint, arity int) *helperBuilder[W] {
	var (
		padding W
		base    = make([]descriptor.Register[W], 0, arity+1)
		inputs  = make([]bytecode.RegisterId, arity)
	)

	for i := 0; i < arity; i++ {
		inputs[i] = bytecode.RegisterId(i)
		base = append(base, descriptor.NewRegister(register.INPUT_REGISTER,
			fmt.Sprintf("arg%d", i+1), util.Some(width), padding))
	}

	output := bytecode.RegisterId(arity)

	base = append(base, descriptor.NewRegister(register.OUTPUT_REGISTER, "out", util.Some(width), padding))

	return &helperBuilder[W]{
		width:  width,
		inputs: inputs,
		output: output,
		base:   base,
	}
}

func (p *helperBuilder[W]) regs() []descriptor.Register[W] {
	return p.base
}

func (p *helperBuilder[W]) emit(insn Bytecode[W]) {
	p.code = append(p.code, insn)
}

func (p *helperBuilder[W]) newComputed(prefix string) bytecode.RegisterId {
	return p.newComputedWidth(prefix, p.width)
}

func (p *helperBuilder[W]) newComputedWidth(prefix string, width uint) bytecode.RegisterId {
	var padding W

	id := bytecode.RegisterId(len(p.base))
	name := fmt.Sprintf("%s%d", prefix, p.nextTmp)
	p.base = append(p.base, descriptor.NewRegister(register.COMPUTED_REGISTER, name, util.Some(width), padding))
	p.nextTmp++

	return id
}

func (p *helperBuilder[W]) newComputedNamed(width uint) bytecode.RegisterId {
	var padding W

	id := bytecode.RegisterId(len(p.base))
	name := fmt.Sprintf("$%d", p.nextTmp)
	p.base = append(p.base, descriptor.NewRegister(register.COMPUTED_REGISTER, name, util.Some(width), padding))
	p.nextTmp++

	return id
}

func (p *helperBuilder[W]) combineBit(op bytecode.Operation, lhs, rhs bytecode.RegisterId) bytecode.RegisterId {
	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	switch op {
	case bytecode.OP_AND:
		res := p.newComputed("and")
		p.emit(bytecode.MulConst(res, []bytecode.RegisterId{lhs, rhs}, one))

		return res
	case bytecode.OP_OR:
		// a + (1-a)*b avoids the intermediate overflow of (a+b) when a=b=1
		oneReg := p.newComputed("or_one")
		p.emit(bytecode.LoadConst(oneReg, one))

		na := p.newComputed("or_na")
		p.emit(bytecode.SubConst(na, []bytecode.RegisterId{oneReg, lhs}, zero))

		prod := p.newComputed("or_prod")
		p.emit(bytecode.MulConst(prod, []bytecode.RegisterId{na, rhs}, one))

		res := p.newComputed("or")
		p.emit(bytecode.AddConst(res, []bytecode.RegisterId{lhs, prod}, zero))

		return res
	case bytecode.OP_XOR:
		// a*(1-b) + (1-a)*b avoids intermediate overflow when a=b=1
		oneReg := p.newComputed("xor_one")
		p.emit(bytecode.LoadConst(oneReg, one))

		nb := p.newComputed("xor_nb")
		p.emit(bytecode.SubConst(nb, []bytecode.RegisterId{oneReg, rhs}, zero))

		na := p.newComputed("xor_na")
		p.emit(bytecode.SubConst(na, []bytecode.RegisterId{oneReg, lhs}, zero))

		l := p.newComputed("xor_l")
		p.emit(bytecode.MulConst(l, []bytecode.RegisterId{lhs, nb}, one))

		r := p.newComputed("xor_r")
		p.emit(bytecode.MulConst(r, []bytecode.RegisterId{na, rhs}, one))

		res := p.newComputed("xor")
		p.emit(bytecode.AddConst(res, []bytecode.RegisterId{l, r}, zero))

		return res
	default:
		panic(fmt.Sprintf("unsupported bit combine opcode: %d", op))
	}
}

func helperName(key bitwiseHelperKey) string {
	var op string

	switch key.op {
	case bytecode.OP_AND:
		op = "and"
	case bytecode.OP_OR:
		op = "or"
	case bytecode.OP_XOR:
		op = "xor"
	case bytecode.OP_NOT:
		op = "not"
	case bytecode.OP_SHL:
		op = "shl"
	case bytecode.OP_SHR:
		op = "shr"
	default:
		op = "unknown"
	}

	return fmt.Sprintf("$bit_%s_u%d", op, key.width)
}
