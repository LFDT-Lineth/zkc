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
		out          = slices.Clone(program.Modules())
		amountWidths = scanShiftAmountWidths(out)
		// Shift helpers never use static bitwise tables, so bitwiseStaticWidth=0.
		helpers = newBitwiseHelpers[W](uint(len(out)), amountWidths, 0)
	)

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerBitwiseFunction(fn, helpers, lowerBitwiseCode[W])
		}
	}

	return descriptor.NewProgram(program.Field(), append(out, helpers.modules()...)...)
}

// lowerBitwiseFunction rewrites each bytecode of a function via codeFn, threading
// a per-function register allocator (for any temporaries codeFn introduces) and
// the shared helper registry.
func lowerBitwiseFunction[W word.Word[W]](fn *descriptor.Function[W], helpers *bitwiseHelpers[W],
	codeFn func(Bytecode[W], split.Allocator[W], *bitwiseHelpers[W]) []Bytecode[W],
) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return codeFn(b, alloc, helpers)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

func lowerBitwiseCode[W word.Word[W]](
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
	case bytecode.OP_NOT:
		return inlineBitwiseNot(bw, registers)
	case bytecode.OP_SHL, bytecode.OP_SHR:
		return lowerBitwiseShlShr(bw, registers, helpers)
	default:
		// AND/OR/XOR are lowered after register splitting; see LowerOrXorAnd.
		return []Bytecode[W]{b}
	}
}

func lowerBitwiseShlShr[W word.Word[W]](
	b *bytecode.Bitwise[W],
	registers split.Allocator[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	var (
		// NOTE: bitwidth of shift (e.g. "x << y") determined by width of first
		// argument only (i.e. "x").
		amtWidth = registers.Registers()[b.Right].Bitwidth().Unwrap()
		id       = helpers.ensureShift(b.Op, uint(b.Bitwidth), amtWidth)
	)
	//
	return []Bytecode[W]{
		bytecode.CallFun[W](uint16(id), []bytecode.RegisterId{b.Left, b.Right}, []bytecode.RegisterId{b.Target}),
	}
}

// inlineBitwiseNot emits ~x as (MASK - x) directly into the caller's bytecode
// stream, where MASK = 2^width - 1.  No helper module is created.
func inlineBitwiseNot[W word.Word[W]](b *bytecode.Bitwise[W], registers split.Allocator[W]) []Bytecode[W] {
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
	// amtWidth is the width of a shift helper's arg2 (amount) register: level
	// j of a barrel chain has amtWidth == j, while a guard carries the widest
	// amount width seen across its call sites (always > the chain depth, so
	// guard and level keys never collide).  It is 0 for non-shift helpers.
	amtWidth uint
}

type bitwiseHelpers[W word.Word[W]] struct {
	baseID       uint
	ids          map[bitwiseHelperKey]uint
	items        []descriptor.Module[W]
	amountWidths map[shiftKey]uint
	// bitwiseStaticWidth is the largest width for which an AND/OR/XOR operation
	// is realised as a static lookup table rather than a recursive helper (see
	// ensureNary).  It is 0 for the shift-only helpers built by LowerBitwise.
	bitwiseStaticWidth uint
}

func newBitwiseHelpers[W word.Word[W]](
	baseID uint, amountWidths map[shiftKey]uint, bitwiseStaticWidth uint,
) *bitwiseHelpers[W] {
	return &bitwiseHelpers[W]{
		baseID:             baseID,
		ids:                make(map[bitwiseHelperKey]uint),
		amountWidths:       amountWidths,
		bitwiseStaticWidth: bitwiseStaticWidth,
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

// ensureShift returns the module id of the shift (SHL/SHR) helper a call site
// with the given value and amount register widths should invoke, creating any
// missing modules.  Call sites whose amount fits the barrel chain (amtWidth <=
// ceil(log2(width))) enter the chain directly at their own level; wider call
// sites go through the guard, which zeroes out-of-range amounts first.
// AND/OR/XOR go through ensureNary instead.
func (p *bitwiseHelpers[W]) ensureShift(op bytecode.Operation, width uint, amtWidth uint) uint {
	if op != bytecode.OP_SHL && op != bytecode.OP_SHR {
		panic(fmt.Sprintf("ensureShift: expected shift operation, got %d", op))
	}

	if depth := shiftChainDepth(width); amtWidth <= depth {
		return p.ensureShiftLevel(op, width, amtWidth)
	}

	return p.ensureShiftGuard(op, width)
}

// ensureShiftLevel returns the module id of barrel-chain level `level` for the
// given (op, width), creating it — and, bottom-up, every level below it — on
// first use.  Building level-1 first means each factory receives the id of an
// already-registered callee, so no pre-registration is needed.
func (p *bitwiseHelpers[W]) ensureShiftLevel(op bytecode.Operation, width uint, level uint) uint {
	key := bitwiseHelperKey{op: op, width: width, arity: 2, amtWidth: level}

	if id, ok := p.ids[key]; ok {
		return id
	}

	var subID uint
	if level > 1 {
		subID = p.ensureShiftLevel(op, width, level-1)
	}

	id := p.baseID + uint(len(p.items))
	p.ids[key] = id
	p.items = append(p.items, newShiftLevelHelper[W](key, subID))

	return id
}

// ensureShiftGuard returns the module id of the guard for the given (op,
// width), creating it (and the full level chain beneath it) on first use.
// Its arg2 width is the maximum amount width seen across all call sites, so
// every wide call site can pass its amount register with an upcast.
func (p *bitwiseHelpers[W]) ensureShiftGuard(op bytecode.Operation, width uint) uint {
	key := bitwiseHelperKey{op: op, width: width, arity: 2, amtWidth: p.shiftAmountWidth(op, width)}

	if id, ok := p.ids[key]; ok {
		return id
	}

	var levelID uint

	if depth := shiftChainDepth(width); depth > 0 {
		levelID = p.ensureShiftLevel(op, width, depth)
	}

	id := p.baseID + uint(len(p.items))
	p.ids[key] = id
	p.items = append(p.items, newShiftGuardHelper[W](key, levelID))

	return id
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

	if key.amtWidth > 0 {
		// Shift helper (chain level or guard): suffix with the amount width.
		return fmt.Sprintf("$bit_%s_u%d_u%d", op, key.width, key.amtWidth)
	}

	return fmt.Sprintf("$bit_%s_u%d", op, key.width)
}
