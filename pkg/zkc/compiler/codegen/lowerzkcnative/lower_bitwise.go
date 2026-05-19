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
package lowerzkcnative

import (
	"fmt"
	"math/big"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

type vectorInstruction = vm.Vector[vm.WordInstruction]

// LowerBitwise rewrites VM-level bitwise micro-instructions into CALLs to
// helper functions. The helper modules are appended to the returned module
// slice.
// We assume this lowering happens BEFORE vectorization and register splitting
func LowerBitwise[W vm.Word[W]](modules []vm.Module) []vm.Module {
	var (
		out          = append([]vm.Module{}, modules...)
		amountWidths = scanShiftAmountWidths[W](out)
		helpers      = newBitwiseHelpers[W](uint(len(out)), amountWidths)
	)

	for i, mod := range out {
		if fn, ok := mod.(*vm.WordFunction); ok {
			out[i] = lowerBitwiseFunction(fn, helpers)
		}
	}

	return append(out, helpers.modules()...)
}

func lowerBitwiseFunction[W vm.Word[W]](fn *vm.WordFunction, helpers *bitwiseHelpers[W],
) *vm.WordFunction {
	var (
		code      = fn.Code()
		ncode     = make([]vectorInstruction, len(code))
		registers = append([]register.Register{}, fn.Registers()...)
	)

	for i, insn := range code {
		ncodes := lowerBitwiseCodes(insn.Codes, &registers, helpers)
		ncode[i] = vectorInstruction{Codes: ncodes}
	}

	return vm.NewFunction(fn.Name(), fn.IsNative(), registers, ncode)
}

func lowerBitwiseCodes[W vm.Word[W]](
	codes []vm.WordInstruction,
	registers *[]register.Register,
	helpers *bitwiseHelpers[W],
) []vm.WordInstruction {
	ncodes := make([]vm.WordInstruction, 0, len(codes))

	for _, code := range codes {
		ncodes = append(ncodes, lowerBitwiseCode(code, registers, helpers)...)
	}

	return ncodes
}

func lowerBitwiseCode[W vm.Word[W]](
	code vm.WordInstruction,
	registers *[]register.Register,
	helpers *bitwiseHelpers[W],
) []vm.WordInstruction {
	if !isBitwiseOpcode(code.OpCode()) {
		return []vm.WordInstruction{code}
	}

	origWidth, isPowerOfTwo := lowerableWidth(*registers, code.Definitions()[0])

	p := origWidth
	if !isPowerOfTwo {
		p = nextPowerOfTwo(origWidth)
	}

	switch t := code.(type) {
	case *instruction.BitAnd[W]:
		// After BinarizeBitwise, any non-identity constant has been
		// materialised as a source register, so we can drop the constant
		// argument here: at the (possibly widened) helper width the original
		// identity mask is redundant because the cast already zero-extends
		// inputs.
		id := helpers.ensure(t.OpCode(), p, len(t.Sources))
		return bitwiseCall(id, t.Target, t.Sources, origWidth, p, registers)
	case *instruction.BitOr[W]:
		id := helpers.ensure(t.OpCode(), p, len(t.Sources))
		return bitwiseCall(id, t.Target, t.Sources, origWidth, p, registers)
	case *instruction.BitXor[W]:
		id := helpers.ensure(t.OpCode(), p, len(t.Sources))
		return bitwiseCall(id, t.Target, t.Sources, origWidth, p, registers)
	case *instruction.BitNot[W]:
		// Inline ~x as (MASK - x) directly in the caller; no helper module needed.
		return bitwiseInlineNot[W](t.Target, t.Sources[0], origWidth, registers)
	case *instruction.BitShl[W]:
		id := helpers.ensure(t.OpCode(), origWidth, len(t.Sources))
		amtWidth := helpers.shiftAmountWidth(t.OpCode(), origWidth)

		return bitwiseShiftCall(id, t.Target, t.Sources[0], t.Sources[1], amtWidth, registers)
	case *instruction.BitShr[W]:
		id := helpers.ensure(t.OpCode(), origWidth, len(t.Sources))
		amtWidth := helpers.shiftAmountWidth(t.OpCode(), origWidth)

		return bitwiseShiftCall(id, t.Target, t.Sources[0], t.Sources[1], amtWidth, registers)
	default:
		panic(fmt.Sprintf("unexpected non-bitwise opcode: %d", code.OpCode()))
	}
}

// bitwiseCall emits a call to a bitwise helper module.  When origWidth equals p
// (already a power of two) the call is direct with no temporaries.  Otherwise
// each source is zero-extended from origWidth to p bits via Cast before the
// call, and the p-wide result is truncated back to origWidth bits afterwards.
func bitwiseCall(
	id uint,
	target register.Id,
	sources []register.Id,
	origWidth, p uint,
	registers *[]register.Register,
) []vm.WordInstruction {
	if origWidth == p {
		return []vm.WordInstruction{
			instruction.NewCall(id, append([]register.Id{}, sources...), []register.Id{target}),
		}
	}

	insns := make([]vm.WordInstruction, 0, 2+len(sources))

	pSources := make([]register.Id, len(sources))
	for i, src := range sources {
		pTmp := allocTmp(registers, p)
		insns = append(insns, instruction.NewCast(pTmp, src, p))
		pSources[i] = pTmp
	}

	pResult := allocTmp(registers, p)
	insns = append(insns, instruction.NewCall(id, pSources, []register.Id{pResult}))
	insns = append(insns, instruction.NewCast(target, pResult, origWidth))

	return insns
}

// bitwiseInlineNot emits ~x as (MASK - x) directly into the caller's
// instruction stream, where MASK = 2^width - 1.  No helper module is created.
func bitwiseInlineNot[W vm.Word[W]](
	target, source register.Id,
	width uint,
	registers *[]register.Register,
) []vm.WordInstruction {
	maskBig := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width), big.NewInt(1))

	var zeroW W

	mask := zeroW.SetBigInt(maskBig)
	zero := vm.Uint64[W](0)

	maskReg := allocTmp(registers, width)

	return []vm.WordInstruction{
		instruction.NewIntAdd(maskReg, nil, mask),
		instruction.NewIntSub(target, []register.Id{maskReg, source}, zero),
	}
}

func allocTmp(registers *[]register.Register, width uint) register.Id {
	var padding big.Int

	id := register.NewId(uint(len(*registers)))
	name := fmt.Sprintf("$%d", len(*registers))
	*registers = append(*registers, register.NewComputed(name, width, padding))

	return id
}

func nextPowerOfTwo(w uint) uint {
	p := uint(1)
	for p < w {
		p <<= 1
	}

	return p
}

func lowerableWidth(registers []register.Register, target register.Id) (uint, bool) {
	reg := registers[target.Unwrap()]

	if reg.IsNative() {
		panic("unexpected native register in bitwise lowering")
	}

	w := reg.Width()

	if w == 0 {
		panic(fmt.Sprintf("zero-width register: %s", reg.Name()))
	}

	return w, w&(w-1) == 0
}

type bitwiseHelperKey struct {
	opcode instruction.OpCode
	width  uint
	arity  int
}

type bitwiseHelpers[W vm.Word[W]] struct {
	baseID       uint
	ids          map[bitwiseHelperKey]uint
	items        []vm.Module
	amountWidths map[shiftKey]uint
}

func newBitwiseHelpers[W vm.Word[W]](
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
func (p *bitwiseHelpers[W]) shiftAmountWidth(op instruction.OpCode, valueWidth uint) uint {
	if w, ok := p.amountWidths[shiftKey{opcode: op, width: valueWidth}]; ok {
		return w
	}

	return valueWidth
}

func (p *bitwiseHelpers[W]) modules() []vm.Module {
	return p.items
}

func (p *bitwiseHelpers[W]) ensure(op instruction.OpCode, width uint, arity int) uint {
	key := bitwiseHelperKey{
		opcode: op,
		width:  width,
		arity:  arity,
	}

	if id, ok := p.ids[key]; ok {
		return id
	}

	// SHL/SHR are self-recursive: pre-register the ID before the factory runs
	// so any re-entrant ensure call for the same key resolves correctly.
	if op == opcode.BIT_SHL || op == opcode.BIT_SHR {
		id := p.baseID + uint(len(p.items))
		p.ids[key] = id

		amtWidth := p.shiftAmountWidth(op, width)

		var mod vm.Module
		if op == opcode.BIT_SHL {
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
func newDecomposedNaryHelper[W vm.Word[W]](
	helpers *bitwiseHelpers[W],
	key bitwiseHelperKey,
) vm.Module {
	b := newHelperBuilder[W](key.width, key.arity)

	out := b.output
	zero := vm.Uint64[W](0)

	// TODO: we will want to stop before width == 1 to reduce the number of tiny modules.
	if key.width == 1 {
		// Base case: single-bit operation.  Seed agg with the op's identity
		// (1 for AND, 0 for OR/XOR) then fold each source in via the
		// appropriate pairwise combinator.
		agg := b.newComputed("agg")

		if key.opcode == opcode.BIT_AND {
			one := vm.Uint64[W](1)
			b.emit(instruction.NewIntAdd(agg, nil, one))
		} else {
			b.emit(instruction.NewIntAdd(agg, nil, zero))
		}

		for _, inp := range b.inputs {
			agg = b.combineBit(key.opcode, agg, inp)
		}

		b.emit(instruction.NewIntAdd(out, []register.Id{agg}, zero))
	} else {
		// Recursive case: low and high halves share the same sub-helper
		// because the body no longer depends on a caller-side constant.
		half := key.width / 2
		subID := helpers.ensure(key.opcode, half, key.arity)

		lowSrcs := make([]register.Id, key.arity)
		highSrcs := make([]register.Id, key.arity)

		for i, arg := range b.inputs {
			lo := b.newComputedNamed(half)
			hi := b.newComputedNamed(half)
			b.emit(instruction.NewDestruct([]register.Id{lo, hi}, arg))
			lowSrcs[i] = lo
			highSrcs[i] = hi
		}

		resLow := b.newComputedNamed(half)
		resHigh := b.newComputedNamed(half)

		b.emit(instruction.NewCall(subID, lowSrcs, []register.Id{resLow}))
		b.emit(instruction.NewCall(subID, highSrcs, []register.Id{resHigh}))

		b.emit(instruction.NewBitConcat[W](out, []register.Id{resLow, resHigh}))
	}

	b.emit(instruction.NewReturn())

	return vm.NewFunction(helperName(key), false, b.regs(), []vectorInstruction{{Codes: b.code}})
}

type helperBuilder[W vm.Word[W]] struct {
	width   uint
	inputs  []register.Id
	output  register.Id
	base    []register.Register
	code    []vm.WordInstruction
	nextTmp uint
}

func newHelperBuilder[W vm.Word[W]](width uint, arity int) *helperBuilder[W] {
	var (
		padding big.Int
		base    = make([]register.Register, 0, arity+1)
		inputs  = make([]register.Id, arity)
	)

	for i := 0; i < arity; i++ {
		inputs[i] = register.NewId(uint(i))
		base = append(base, register.NewInput(fmt.Sprintf("arg%d", i+1), width, padding))
	}

	output := register.NewId(uint(arity))

	base = append(base, register.NewOutput("out", width, padding))

	return &helperBuilder[W]{
		width:  width,
		inputs: inputs,
		output: output,
		base:   base,
	}
}

func (p *helperBuilder[W]) regs() []register.Register {
	return p.base
}

func (p *helperBuilder[W]) emit(insn vm.WordInstruction) {
	p.code = append(p.code, insn)
}

func (p *helperBuilder[W]) newComputed(prefix string) register.Id {
	return p.newComputedWidth(prefix, p.width)
}

func (p *helperBuilder[W]) newComputedWidth(prefix string, width uint) register.Id {
	var padding big.Int

	id := register.NewId(uint(len(p.base)))
	name := fmt.Sprintf("%s%d", prefix, p.nextTmp)
	p.base = append(p.base, register.NewComputed(name, width, padding))
	p.nextTmp++

	return id
}

func (p *helperBuilder[W]) newComputedNamed(width uint) register.Id {
	var padding big.Int

	id := register.NewId(uint(len(p.base)))
	name := fmt.Sprintf("$%d", p.nextTmp)
	p.base = append(p.base, register.NewComputed(name, width, padding))
	p.nextTmp++

	return id
}

func (p *helperBuilder[W]) combineBit(op instruction.OpCode, lhs, rhs register.Id) register.Id {
	zero := vm.Uint64[W](0)
	one := vm.Uint64[W](1)

	switch op {
	case opcode.BIT_AND:
		res := p.newComputed("and")
		p.emit(instruction.NewIntMul(res, []register.Id{lhs, rhs}, one))

		return res
	case opcode.BIT_OR:
		// a + (1-a)*b avoids the intermediate overflow of (a+b) when a=b=1
		oneReg := p.newComputed("or_one")
		p.emit(instruction.NewIntAdd(oneReg, nil, one))

		na := p.newComputed("or_na")
		p.emit(instruction.NewIntSub(na, []register.Id{oneReg, lhs}, zero))

		prod := p.newComputed("or_prod")
		p.emit(instruction.NewIntMul(prod, []register.Id{na, rhs}, one))

		res := p.newComputed("or")
		p.emit(instruction.NewIntAdd(res, []register.Id{lhs, prod}, zero))

		return res
	case opcode.BIT_XOR:
		// a*(1-b) + (1-a)*b avoids intermediate overflow when a=b=1
		oneReg := p.newComputed("xor_one")
		p.emit(instruction.NewIntAdd(oneReg, nil, one))

		nb := p.newComputed("xor_nb")
		p.emit(instruction.NewIntSub(nb, []register.Id{oneReg, rhs}, zero))

		na := p.newComputed("xor_na")
		p.emit(instruction.NewIntSub(na, []register.Id{oneReg, lhs}, zero))

		l := p.newComputed("xor_l")
		p.emit(instruction.NewIntMul(l, []register.Id{lhs, nb}, one))

		r := p.newComputed("xor_r")
		p.emit(instruction.NewIntMul(r, []register.Id{na, rhs}, one))

		res := p.newComputed("xor")
		p.emit(instruction.NewIntAdd(res, []register.Id{l, r}, zero))

		return res
	default:
		panic(fmt.Sprintf("unsupported bit combine opcode: %d", op))
	}
}

func isBitwiseOpcode(op instruction.OpCode) bool {
	switch op {
	case opcode.BIT_AND, opcode.BIT_OR, opcode.BIT_XOR,
		opcode.BIT_NOT, opcode.BIT_SHL, opcode.BIT_SHR:
		return true
	default:
		return false
	}
}

func helperName(key bitwiseHelperKey) string {
	var op string

	switch key.opcode {
	case opcode.BIT_AND:
		op = "and"
	case opcode.BIT_OR:
		op = "or"
	case opcode.BIT_XOR:
		op = "xor"
	case opcode.BIT_NOT:
		op = "not"
	case opcode.BIT_SHL:
		op = "shl"
	case opcode.BIT_SHR:
		op = "shr"
	default:
		op = "unknown"
	}

	return fmt.Sprintf("$bit_%s_u%d", op, key.width)
}
