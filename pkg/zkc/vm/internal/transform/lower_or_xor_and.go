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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerOrXorAnd rewrites VM-level bitwise AND/OR/XOR bytecodes into either a
// static-table lookup (a memory read) or a CALL to a recursive helper function,
// appending the helper / table modules to the returned program.
//
// Unlike LowerBitwise (which handles NOT and SHL/SHR before splitting), this
// pass runs AFTER register splitting: SplitRegisters has already broken each
// wide AND/OR/XOR into limb-wide bitwise bytecodes (see split.Bitwise), so the
// helpers built here operate at the (narrow) limb width.
//
// It must run before AddRangeConstraints (so the freshly introduced registers
// are range-checked) and the CALLs it introduces must subsequently be flattened.
func LowerOrXorAnd[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		out = slices.Clone(program.Modules())
		// maxStaticWidth is floor(log2(maxStaticHeight)); a bitwise table indexes
		// two w-bit operands, so it fits only when 2w <= maxStaticWidth.
		bitwiseStaticWidth = util_math.FloorLog2(program.MaxStaticHeight()) / 2
		helpers            = newBitwiseHelpers[W](uint(len(out)), bitwiseStaticWidth)
	)
	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerBitwiseFunction(fn, func(b Bytecode[W], alloc split.Allocator[W]) []Bytecode[W] {
				return lowerOrXorAndCode(b, alloc, helpers)
			})
		}
	}
	//
	return descriptor.NewProgram(program.Field(),
		program.MaxStaticHeight(), append(out, helpers.modules()...)...)
}

// bitwiseHelperKey identifies an AND/OR/XOR helper module.
type bitwiseHelperKey struct {
	op    bytecode.Operation
	width uint
	arity int
}

// bitwiseHelpers is the registry of AND/OR/XOR helper modules built by
// LowerOrXorAnd: static truth tables for small widths, recursive halving
// helpers above.  SHL/SHR use the separate shiftHelpers registry (see
// lower_shift.go).
type bitwiseHelpers[W word.Word[W]] struct {
	baseID uint
	ids    map[bitwiseHelperKey]uint
	items  []descriptor.Module[W]
	// bitwiseStaticWidth is the largest width for which an AND/OR/XOR operation
	// is realised as a static lookup table rather than a recursive helper (see
	// ensureNary).
	bitwiseStaticWidth uint
}

func newBitwiseHelpers[W word.Word[W]](baseID uint, bitwiseStaticWidth uint) *bitwiseHelpers[W] {
	return &bitwiseHelpers[W]{
		baseID:             baseID,
		ids:                make(map[bitwiseHelperKey]uint),
		bitwiseStaticWidth: bitwiseStaticWidth,
	}
}

func (p *bitwiseHelpers[W]) modules() []descriptor.Module[W] {
	return p.items
}

func helperName(key bitwiseHelperKey) string {
	return fmt.Sprintf("$bit_%s_u%d", bitwiseOpName(key.op), key.width)
}

// isTableWidth reports whether an AND/OR/XOR operation of the given width is
// realised as a static lookup table (rather than being inlined or lowered to a
// recursive helper).
// Single-bit operations are excluded: they are cheap
// arithmetic and are inlined directly.
func (p *bitwiseHelpers[W]) isTableWidth(width uint) bool {
	return width >= 2 && width <= p.bitwiseStaticWidth
}

// ensureNary returns the module id of the AND/OR/XOR helper for (op, width,
// arity), creating it on first use, together with a flag indicating whether
// that module is a static table (invoked by a memory read) rather than a
// recursive helper function (invoked by a call).
func (p *bitwiseHelpers[W]) ensureNary(op bytecode.Operation, width uint, arity int) (uint, bool) {
	key := bitwiseHelperKey{op: op, width: width, arity: arity}
	//
	if id, ok := p.ids[key]; ok {
		return id, p.isTableWidth(width)
	}
	// Base case: a static table enumerating every (a, b) pair.  Tables have no
	// sub-dependencies, so the id can be assigned immediately.
	if p.isTableWidth(width) {
		id := p.baseID + uint(len(p.items))
		p.items = append(p.items, newBitwiseTable[W](op, width))
		p.ids[key] = id
		//
		return id, true
	}
	// Recursive case: the factory may recursively call ensureNary for its
	// sub-helpers, which appends them to p.items.  The current module must occupy
	// the slot AFTER all its sub-helpers (callees before callers), so its id is
	// derived from len(p.items) only after the factory returns.
	mod := newDecomposedNaryHelper(p, key)
	id := p.baseID + uint(len(p.items))
	p.items = append(p.items, mod)
	p.ids[key] = id
	//
	return id, false
}

func lowerOrXorAndCode[W word.Word[W]](
	b Bytecode[W],
	registers split.Allocator[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	//
	bw, ok := b.(*bytecode.Bitwise[W])
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	switch bw.Op {
	case bytecode.OP_AND, bytecode.OP_OR, bytecode.OP_XOR:
		return lowerBitwiseAndOrXor(bw, registers, helpers)
	default:
		return []Bytecode[W]{b}
	}
}

func lowerBitwiseAndOrXor[W word.Word[W]](
	b *bytecode.Bitwise[W],
	registers split.Allocator[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	// Materialise a constant right operand into a register, since both the
	// unit-arithmetic and table paths below operate on registers only.
	right, pre := materialiseRight(b, registers)
	//
	origWidth, isPowerOfTwo := maxBitwidthOf(registers.Registers(), b.Left, right)
	//
	p := origWidth
	if !isPowerOfTwo {
		p = util_math.NextPowerOfTwo(origWidth)
	}
	// Single-bit operations are cheap arithmetic (a&b = a*b, etc.);
	// inline them rather than calling a (static or not) table.
	if p == 1 {
		return append(pre, emitUnitBitwise(b.Op, registers, b.Target, b.Left, right)...)
	}
	// Both operands are now registers, so lowering only needs the
	// (power-of-two) operand width p: a table read when p is small enough,
	// otherwise a recursive helper.
	id, isTable := helpers.ensureNary(b.Op, p, 2)
	//
	return append(pre,
		invokeNary[W](id, isTable, []bytecode.RegisterId{b.Left, right}, b.Target),
	)
}

// materialiseRight returns the right operand of a bitwise instruction as a
// register, loading a constant operand into a freshly allocated register of
// the instruction's width.  This is the single chokepoint through which
// constant operands are materialised, so that a future pass can deduplicate
// repeated constants here.
func materialiseRight[W word.Word[W]](b *bytecode.Bitwise[W], registers split.Allocator[W],
) (bytecode.RegisterId, []Bytecode[W]) {
	if !b.Right.IsConstant() {
		return b.Right.AsRegister(), nil
	}
	//
	reg := registers.Allocate("", util.Some(uint(b.Bitwidth)))
	//
	return reg, []Bytecode[W]{bytecode.LoadConst(reg, b.Right.AsConstant())}
}

// emitUnitBitwise inlines a single-bit AND/OR/XOR of left and right into target,
// using the same overflow-free arithmetic identities as combineBit (a&b = a*b;
// a|b = a + (1-a)*b; a^b = a*(1-b) + (1-a)*b).  Temporaries are allocated in the
// enclosing function.
func emitUnitBitwise[W word.Word[W]](op bytecode.Operation, registers split.Allocator[W],
	target, left, right bytecode.RegisterId) []Bytecode[W] {
	var (
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		bit  = util.Some[uint](1)
	)
	//
	switch op {
	case bytecode.OP_AND:
		return []Bytecode[W]{bytecode.MulConst(target, []bytecode.RegisterId{left, right}, one)}
	case bytecode.OP_OR:
		oneReg := registers.Allocate("", bit)
		na := registers.Allocate("", bit)
		prod := registers.Allocate("", bit)
		//
		return []Bytecode[W]{
			bytecode.LoadConst(oneReg, one),
			bytecode.SubConst(na, []bytecode.RegisterId{oneReg, left}, zero),
			bytecode.MulConst(prod, []bytecode.RegisterId{na, right}, one),
			bytecode.AddConst(target, []bytecode.RegisterId{left, prod}, zero),
		}
	case bytecode.OP_XOR:
		oneReg := registers.Allocate("", bit)
		nb := registers.Allocate("", bit)
		na := registers.Allocate("", bit)
		l := registers.Allocate("", bit)
		r := registers.Allocate("", bit)
		//
		return []Bytecode[W]{
			bytecode.LoadConst(oneReg, one),
			bytecode.SubConst(nb, []bytecode.RegisterId{oneReg, right}, zero),
			bytecode.SubConst(na, []bytecode.RegisterId{oneReg, left}, zero),
			bytecode.MulConst(l, []bytecode.RegisterId{left, nb}, one),
			bytecode.MulConst(r, []bytecode.RegisterId{na, right}, one),
			bytecode.AddConst(target, []bytecode.RegisterId{l, r}, zero),
		}
	default:
		panic(fmt.Sprintf("unsupported unit bitwise operation: %d", op))
	}
}

// invokeNary emits the bytecode invoking an AND/OR/XOR helper: a memory read
// when the helper is a static table (see ensureNary), otherwise a call.
func invokeNary[W word.Word[W]](id uint, isTable bool, sources []bytecode.RegisterId,
	target bytecode.RegisterId) Bytecode[W] {
	if isTable {
		// Table read: the source operands are the (a, b) address lines and the
		// result is the single data line.
		return bytecode.NewMemRead[W](uint16(id), sources, []bytecode.RegisterId{target})
	}
	//
	return bytecode.CallFun[W](uint16(id), sources, []bytecode.RegisterId{target})
}

// newDecomposedNaryHelper builds a helper module for bitwise AND/OR/XOR using
// recursive halving.  Each module body is O(arity) instructions: it splits
// every source into two half-wide pieces, invokes a single half-wide sub-helper
// (a static table or a narrower recursive helper) for each piece, and
// recombines.  The body depends only on (op, width, arity) — operands are always
// registers — so a single helper per key is sufficient and shared across all
// call sites.
func newDecomposedNaryHelper[W word.Word[W]](
	helpers *bitwiseHelpers[W],
	key bitwiseHelperKey,
) descriptor.Module[W] {
	b := newHelperBuilder[W](key.width, key.arity)

	out := b.output
	zero := word.Const64[W](0)

	// NOTE: with a non-degenerate maxStaticHeight this recursion bottoms out in a
	// static table (see ensureNary), so the width==1 arithmetic base below is
	// reached only when bitwiseStaticWidth==0 (a tiny maxStaticHeight that admits
	// no bitwise table at all).
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

		b.emit(bytecode.Assign[W](out, agg))
	} else {
		// Recursive case: low and high halves share the same sub-helper
		// (which may be a static table or a narrower recursive helper) because
		// the body no longer depends on a caller-side constant.
		half := key.width / 2
		subID, isTable := helpers.ensureNary(key.op, half, key.arity)

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

		b.emit(invokeNary[W](subID, isTable, lowSrcs, resLow))
		b.emit(invokeNary[W](subID, isTable, highSrcs, resHigh))

		b.emit(bytecode.AssignV[W]([]bytecode.RegisterId{out}, resLow, resHigh))
	}

	b.emit(bytecode.NewRet[W]())

	return descriptor.NewFunction(helperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}

// newBitwiseTable builds a static read-only table for a single bitwise
// operation of the given width: a memory with two address lines (a, b) and one
// data line holding "a op b".  A read of this table (see invokeNary) becomes a
// lookup asserting (a, b, result) is a valid row of the operation's truth
// table.  The table enumerates every (a, b) pair, so it has 2^(2*width) rows;
// the caller (ensureNary) only builds it when 2^(2*width) <= maxStaticHeight.
//
// The row index encodes the operands as a (high) followed by b (low), matching
// how the interpreter decodes address lines from the memory geometry (see
// decodeAddress), so a = row >> width and b = row mod 2^width.
func newBitwiseTable[W word.Word[W]](op bytecode.Operation, width uint) descriptor.Module[W] {
	var (
		padding  W
		mask     = (uint64(1) << width) - 1
		rows     = uint64(1) << (2 * width)
		contents = make([]W, rows)
		regs     = []descriptor.Register[W]{
			descriptor.NewRegister(register.INPUT_REGISTER, "a", util.Some(width), padding),
			descriptor.NewRegister(register.INPUT_REGISTER, "b", util.Some(width), padding),
			descriptor.NewRegister(register.OUTPUT_REGISTER, "r", util.Some(width), padding),
		}
	)
	//
	for row := uint64(0); row < rows; row++ {
		var (
			a = row >> width
			b = row & mask
			r uint64
			w W
		)
		//
		switch op {
		case bytecode.OP_AND:
			r = a & b
		case bytecode.OP_OR:
			r = a | b
		case bytecode.OP_XOR:
			r = a ^ b
		default:
			panic(fmt.Sprintf("unsupported bitwise table operation: %d", op))
		}
		//
		contents[row] = w.SetUint64(r)
	}
	//
	return descriptor.NewMemory(helperName(bitwiseHelperKey{op: op, width: width}),
		descriptor.PRIVATE_STATIC_MEMORY, util.None[uint](), regs, contents)
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

func (p *helperBuilder[W]) emitAll(insns []Bytecode[W]) {
	p.code = append(p.code, insns...)
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
