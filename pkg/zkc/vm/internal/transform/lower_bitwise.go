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
	codeFn func(Bytecode[W], *regAllocator[W], *bitwiseHelpers[W]) []Bytecode[W],
) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = newRegAllocator(fn.Registers())
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return codeFn(b, alloc, helpers)
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
	bw, ok := b.(*bytecode.Bitwise[W])
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	switch bw.Op {
	case bytecode.OP_NOT:
		return inlineBitwiseNot(bw, registers)
	case bytecode.OP_SHL, bytecode.OP_SHR:
		return lowerBitwiseShlShr(bw, helpers)
	default:
		// AND/OR/XOR are lowered after register splitting; see LowerOrXorAnd.
		return []Bytecode[W]{b}
	}
}

func lowerBitwiseShlShr[W word.Word[W]](
	b *bytecode.Bitwise[W],
	helpers *bitwiseHelpers[W],
) []Bytecode[W] {
	var (
		// NOTE: bitwidth of shift (e.g. "x << y") determined by width of first
		// argument only (i.e. "x").
		id = helpers.ensure(b.Op, uint(b.Bitwidth), 2)
	)
	//
	return []Bytecode[W]{
		bytecode.CallFun[W](uint16(id), []bytecode.RegisterId{b.Left, b.Right}, []bytecode.RegisterId{b.Target}),
	}
}

// inlineBitwiseNot emits ~x as (MASK - x) directly into the caller's bytecode
// stream, where MASK = 2^width - 1.  No helper module is created.
func inlineBitwiseNot[W word.Word[W]](b *bytecode.Bitwise[W], registers *regAllocator[W]) []Bytecode[W] {
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

// ensure returns the module id of the shift (SHL/SHR) helper for the given key,
// creating it on first use.  AND/OR/XOR go through ensureNary instead.
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
	//
	panic(fmt.Sprintf("ensure: expected shift operation, got %d", op))
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
