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
	"math/bits"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// shiftKey identifies a shift helper by operation and value width.
type shiftKey struct {
	op    bytecode.Operation
	width uint
}

// shiftChainDepth returns ceil(log2(width)): the number of levels in the
// barrel-shifter chain for a value of the given width.  A shift amount of
// shiftChainDepth(w) bits is sufficient to express every in-range shift
// (0 .. w-1); any amount with a set bit above that yields zero.
func shiftChainDepth(width uint) uint {
	return uint(bits.Len(width - 1))
}

// scanShiftAmountWidths scans all functions and returns, for each (operation,
// value-width) pair, the maximum shift-amount register width seen across all
// call sites.  This width is only consumed by the guard module.
func scanShiftAmountWidths[W word.Word[W]](modules []descriptor.Module[W]) map[shiftKey]uint {
	result := make(map[shiftKey]uint)

	for _, mod := range modules {
		fn, ok := mod.(*descriptor.Function[W])
		if !ok {
			continue
		}

		regs := fn.Registers()

		for _, vec := range fn.Vectors() {
			for _, insn := range vec.Bytecodes {
				bw, ok := insn.(*bytecode.Bitwise[W])
				if !ok {
					continue
				}

				switch bw.Op {
				case bytecode.OP_SHL, bytecode.OP_SHR:
					// NOTE: only the source register width is used, not the target
					key := shiftKey{op: bw.Op, width: uint(bw.Bitwidth)}
					amountWidth := regs[bw.Right].Bitwidth().Unwrap()

					if existing, seen := result[key]; !seen || amountWidth > existing {
						result[key] = amountWidth
					}
				}
			}
		}
	}

	return result
}

// shiftHelperKey identifies a SHL/SHR helper module.
type shiftHelperKey struct {
	op    bytecode.Operation
	width uint
	// amtWidth is the width of the helper's arg2 (amount) register: level j of
	// a barrel chain has amtWidth == j, while a guard carries the widest amount
	// width seen across its call sites (always > the chain depth, so guard and
	// level keys never collide).
	amtWidth uint
}

// shiftHelpers is the registry of SHL/SHR helper modules built by
// LowerBitwise: the barrel-chain levels plus (when some call site's amount
// register is wider than the chain) a guard per (op, value-width).
type shiftHelpers[W word.Word[W]] struct {
	baseID       uint
	ids          map[shiftHelperKey]uint
	items        []descriptor.Module[W]
	amountWidths map[shiftKey]uint
}

func newShiftHelpers[W word.Word[W]](baseID uint, amountWidths map[shiftKey]uint) *shiftHelpers[W] {
	return &shiftHelpers[W]{
		baseID:       baseID,
		ids:          make(map[shiftHelperKey]uint),
		amountWidths: amountWidths,
	}
}

func (p *shiftHelpers[W]) modules() []descriptor.Module[W] {
	return p.items
}

// shiftAmountWidth returns the width of the guard's arg2 for a given (opcode,
// value-width) pair: the maximum amount register width seen across all call
// sites, defaulting to valueWidth if no entry was recorded.
func (p *shiftHelpers[W]) shiftAmountWidth(op bytecode.Operation, valueWidth uint) uint {
	if w, ok := p.amountWidths[shiftKey{op: op, width: valueWidth}]; ok {
		return w
	}

	return valueWidth
}

// ensureShift returns the module id of the shift (SHL/SHR) helper a call site
// with the given value and amount register widths should invoke, creating any
// missing modules.  Call sites whose amount fits the barrel chain (amtWidth <=
// ceil(log2(width))) enter the chain directly at their own level; wider call
// sites go through the guard, which zeroes out-of-range amounts first.
func (p *shiftHelpers[W]) ensureShift(op bytecode.Operation, width uint, amtWidth uint) uint {
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
func (p *shiftHelpers[W]) ensureShiftLevel(op bytecode.Operation, width uint, level uint) uint {
	key := shiftHelperKey{op: op, width: width, amtWidth: level}

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
func (p *shiftHelpers[W]) ensureShiftGuard(op bytecode.Operation, width uint) uint {
	key := shiftHelperKey{op: op, width: width, amtWidth: p.shiftAmountWidth(op, width)}

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

// shiftHelperName is the module name of a shift helper: the operation, the
// value width and the amount (arg2) width.
func shiftHelperName(key shiftHelperKey) string {
	return fmt.Sprintf("$bit_%s_u%d_u%d", bitwiseOpName(key.op), key.width, key.amtWidth)
}

// newShiftLevelHelper builds level j (= key.amtWidth) of the barrel-shifter
// chain for SHL/SHR over values of width w (= key.width):
//
// let bit:u1, low:u(j-1) = n
//
//	level_j(a, n:u_j)  =  level_{j-1}(bit == 0 ? a : a<op>2^(j-1), nlow)
//	level_1(a, n:u1)   =  n == 0 ? a : a<op>1
//
// The conditional is a skip diamond selecting `next`; the call to level j-1
// is unconditional, so each level contains exactly one call site.
//
// where a<op>k is a shift by the constant k, realised purely by Destruct (for
// SHR: drop the low k bits and zero-extend the rest) or Destruct + Concat (for
// SHL: drop the high k bits and append k zero bits) — no field arithmetic, so
// this works for any field modulus.
// subID is the module id of level j-1; it is ignored when j == 1.
func newShiftLevelHelper[W word.Word[W]](key shiftHelperKey, subID uint) descriptor.Module[W] {
	var padding W

	b := newHelperBuilder[W](key.width, 2)
	b.base[1] = descriptor.NewRegister(register.INPUT_REGISTER, "arg2", util.Some(key.amtWidth), padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width
	level := key.amtWidth
	zero := word.Const64[W](0)

	if level == 1 {
		// if n == 0: return a
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
			bytecode.NewRegisterVector(n),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(out, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.NewRet[W]())
		// out = a shifted by 1
		b.emitAll(shiftByConst(b, key.op, out, a, 1))
		b.emit(bytecode.NewRet[W]())
	} else {
		shift := uint(1) << (level - 1)
		// Destruct n into [nlow:u(level-1), bit:u1] (little-endian).
		nlow := b.newComputedWidth("$nlow", level-1)
		bit := b.newComputedWidth("$bit", 1)
		b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{nlow, bit}, []bytecode.RegisterId{n}))
		// next = bit == 0 ? a : a shifted by 2^(level-1)
		next := b.newComputedWidth("$next", width)
		shifted := shiftByConst(b, key.op, next, a, shift)
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
			bytecode.NewRegisterVector(bit),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(next, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.NewSkip[W](uint16(len(shifted))))
		b.emitAll(shifted)
		// return level_{j-1}(next, nlow)
		b.emit(bytecode.CallFun[W](uint16(subID), []bytecode.RegisterId{next, nlow}, []bytecode.RegisterId{out}))
		b.emit(bytecode.NewRet[W]())
	}

	return descriptor.NewFunction(shiftHelperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}

// shiftByConst returns the codes computing "target = a op shift" for a
// constant shift amount in (0, width), allocating any temporaries on the
// builder but NOT emitting (so the caller can size a Skip over the sequence):
//
//	SHR: Destruct a into [drop:u_shift, high:u(width-shift)]; target = high
//	     (zero-extended via AddConst, since high < 2^(width-shift)).
//	SHL: Destruct a into [low:u(width-shift), drop:u_shift]; target = zeros:low
//	     via Concat with a constant-zero register in the low bits.
func shiftByConst[W word.Word[W]](b *helperBuilder[W], op bytecode.Operation,
	target, a bytecode.RegisterId, shift uint,
) []Bytecode[W] {
	var (
		width = b.width
		zero  = word.Const64[W](0)
		drop  = b.newComputedWidth("$drop", shift)
		keep  = b.newComputedWidth("$keep", width-shift)
	)

	switch op {
	case bytecode.OP_SHR:
		// Destruct a into [drop, keep] (little-endian): keep = a >> shift.
		return []Bytecode[W]{
			bytecode.AddVec[W]([]bytecode.RegisterId{drop, keep}, []bytecode.RegisterId{a}),
			bytecode.AddConst(target, []bytecode.RegisterId{keep}, zero),
		}
	case bytecode.OP_SHL:
		zeros := b.newComputedWidth("$zeros", shift)
		// Destruct a into [keep, drop] (little-endian): keep = a mod 2^(width-shift),
		// then target = keep : zeros, i.e. (a << shift) mod 2^width.
		return []Bytecode[W]{
			bytecode.AddVec[W]([]bytecode.RegisterId{keep, drop}, []bytecode.RegisterId{a}),
			bytecode.LoadConst(zeros, zero),
			bytecode.AssignV[W]([]bytecode.RegisterId{target}, zeros, keep),
		}
	default:
		panic("expected shift operation")
	}
}

// newShiftGuardHelper builds the entry module for shift call sites whose
// amount register is wider than the level chain (amtWidth > k where k =
// shiftChainDepth(width)):
//
//	guard(a, n:u_amtWidth)  =  vhigh != 0 ? 0 : level_k(a, vlow)   where vhigh:vlow = n
//
// Any amount with a bit set above the low k bits is at least 2^k >= width and
// so shifts everything out.  Amounts in [width, 2^k) — possible when width is
// not a power of two — need no special handling: the level chain strips more
// bits than the value has and naturally yields zero.  For width == 1 there are
// no levels (k == 0) and the guard degenerates to "n == 0 ? a : 0"; levelID is
// ignored in that case.
func newShiftGuardHelper[W word.Word[W]](key shiftHelperKey, levelID uint) descriptor.Module[W] {
	var padding W

	amtWidth := key.amtWidth

	b := newHelperBuilder[W](key.width, 2)
	b.base[1] = descriptor.NewRegister(register.INPUT_REGISTER, "arg2", util.Some(amtWidth), padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	depth := shiftChainDepth(key.width)
	zero := word.Const64[W](0)

	if depth == 0 {
		// width == 1: if n == 0: return a
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
			bytecode.NewRegisterVector(n),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(out, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.NewRet[W]())
	} else {
		// Destruct n into [vlow:u_depth, vhigh:u(amtWidth-depth)] (little-endian).
		vlow := b.newComputedWidth("$vlow", depth)
		vhigh := b.newComputedWidth("$vhigh", amtWidth-depth)
		b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{vlow, vhigh}, []bytecode.RegisterId{n}))
		// if vhigh == 0: return level_depth(a, vlow)
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
			bytecode.NewRegisterVector(vhigh),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.CallFun[W](uint16(levelID), []bytecode.RegisterId{a, vlow}, []bytecode.RegisterId{out}))
		b.emit(bytecode.NewRet[W]())
	}
	// out = 0
	b.emit(bytecode.LoadConst(out, zero))
	b.emit(bytecode.NewRet[W]())

	return descriptor.NewFunction(shiftHelperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}
