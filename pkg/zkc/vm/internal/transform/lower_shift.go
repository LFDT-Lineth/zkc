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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// shiftChainDepth returns ceil(log2(width)): the number of levels in the
// barrel-shifter chain for a value of the given width.  A shift amount of
// shiftChainDepth(w) bits is sufficient to express every in-range shift
// (0 .. w-1); any amount with a set bit above that yields zero.
func shiftChainDepth(width uint) uint {
	return uint(bits.Len(width - 1))
}

// shiftParams are the whole-program parameters of the shared shift cascade.
type shiftParams struct {
	// maxWidth is the largest value width across all SHL/SHR call sites.
	maxWidth uint
	// maxAmtWidth is the largest shift-amount register width across all call
	// sites.  This width is only consumed by the guard module.
	maxAmtWidth uint
}

// scanShiftParams scans all functions for SHL/SHR bytecodes and returns the
// parameters of the shared cascade, taken across the call sites of *both*
// operations.  A zero maxWidth means the program performs no shifts at all, in
// which case no cascade module is ever built.
func scanShiftParams[W word.Word[W]](modules []descriptor.Module[W]) shiftParams {
	var result shiftParams

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
					result.maxWidth = max(result.maxWidth, uint(bw.Bitwidth))
					result.maxAmtWidth = max(result.maxAmtWidth,
						regs[bw.Right.AsRegister()].Bitwidth().Unwrap())
				}
			}
		}
	}

	return result
}

// shiftHelpers is the registry of the single SHL/SHR cascade built by
// LowerBitwise: the barrel-chain levels plus (when some call site's amount
// register is wider than the chain) one guard.  Every module operates at
// params.maxWidth and returns both shift directions, so this one cascade serves
// every (operation, value-width) combination appearing in the program.
type shiftHelpers[W word.Word[W]] struct {
	baseID uint
	params shiftParams
	// ids maps a module's arg2 (amount) width to its module id.  Level j of the
	// barrel chain has arg2 width j, while the guard carries the widest amount
	// width seen across all call sites — always > the chain depth, since the
	// guard only exists when some call site exceeds it, so guard and level keys
	// never collide.
	ids   map[uint]uint
	items []descriptor.Module[W]
}

func newShiftHelpers[W word.Word[W]](baseID uint, params shiftParams) *shiftHelpers[W] {
	return &shiftHelpers[W]{
		baseID: baseID,
		params: params,
		ids:    make(map[uint]uint),
	}
}

func (p *shiftHelpers[W]) modules() []descriptor.Module[W] {
	return p.items
}

// maxWidth is the width at which the cascade operates: every call site
// zero-extends its value into this width and truncates the result back down.
func (p *shiftHelpers[W]) maxWidth() uint {
	return p.params.maxWidth
}

// ensureShift returns the module id of the cascade entry point a call site with
// the given amount register width should invoke, creating any missing modules.
// Call sites whose amount fits the barrel chain (amtWidth <=
// ceil(log2(maxWidth))) enter the chain directly at their own level; wider call
// sites go through the guard, which zeroes out-of-range amounts first.
func (p *shiftHelpers[W]) ensureShift(amtWidth uint) uint {
	if depth := shiftChainDepth(p.params.maxWidth); amtWidth <= depth {
		return p.ensureShiftLevel(amtWidth)
	}

	return p.ensureShiftGuard()
}

// ensureShiftLevel returns the module id of barrel-chain level `level`,
// creating it — and, bottom-up, every level below it — on first use.  Building
// level-1 first means each factory receives the id of an already-registered
// callee, so no pre-registration is needed.
func (p *shiftHelpers[W]) ensureShiftLevel(level uint) uint {
	if id, ok := p.ids[level]; ok {
		return id
	}

	var subID uint
	if level > 1 {
		subID = p.ensureShiftLevel(level - 1)
	}

	return p.register(level, newShiftLevelHelper[W](p.params.maxWidth, level, subID))
}

// ensureShiftGuard returns the module id of the guard, creating it (and the
// full level chain beneath it) on first use.  Its arg2 width is the maximum
// amount width seen across all call sites, so every wide call site can pass its
// amount register with an upcast.
func (p *shiftHelpers[W]) ensureShiftGuard() uint {
	amtWidth := p.params.maxAmtWidth

	if id, ok := p.ids[amtWidth]; ok {
		return id
	}

	var levelID uint

	if depth := shiftChainDepth(p.params.maxWidth); depth > 0 {
		levelID = p.ensureShiftLevel(depth)
	}

	return p.register(amtWidth, newShiftGuardHelper[W](p.params.maxWidth, amtWidth, levelID))
}

// register assigns the next module id to a freshly built cascade module.
func (p *shiftHelpers[W]) register(amtWidth uint, mod descriptor.Module[W]) uint {
	id := p.baseID + uint(len(p.items))
	p.ids[amtWidth] = id
	p.items = append(p.items, mod)

	return id
}

// shiftHelperName is the module name of a cascade module: the shared value
// width and the amount (arg2) width.  Both directions are computed by the same
// module, hence the direction-agnostic "shf".
func shiftHelperName(maxWidth, amtWidth uint) string {
	return fmt.Sprintf("$bit_shf_u%d_u%d", maxWidth, amtWidth)
}

// shiftHelperBuilder accumulates the registers and code of a cascade module: a
// value input (arg1), an amount input (arg2), one output register per shift
// direction, and any computed temporaries.
type shiftHelperBuilder[W word.Word[W]] struct {
	width   uint
	value   bytecode.RegisterId
	amount  bytecode.RegisterId
	outShl  bytecode.RegisterId
	outShr  bytecode.RegisterId
	base    []descriptor.Register[W]
	code    []Bytecode[W]
	nextTmp uint
}

func newShiftHelperBuilder[W word.Word[W]](width, amtWidth uint) *shiftHelperBuilder[W] {
	var padding W
	//
	return &shiftHelperBuilder[W]{
		width:  width,
		value:  bytecode.RegisterId(0),
		amount: bytecode.RegisterId(1),
		outShl: bytecode.RegisterId(2),
		outShr: bytecode.RegisterId(3),
		base: []descriptor.Register[W]{
			descriptor.NewInputRegister("arg1", util.Some(width), padding),
			descriptor.NewInputRegister("arg2", util.Some(amtWidth), padding),
			descriptor.NewOutputRegister("out_shl", util.Some(width), padding),
			descriptor.NewOutputRegister("out_shr", util.Some(width), padding),
		},
	}
}

func (p *shiftHelperBuilder[W]) regs() []descriptor.Register[W] {
	return p.base
}

func (p *shiftHelperBuilder[W]) emit(insn Bytecode[W]) {
	p.code = append(p.code, insn)
}

func (p *shiftHelperBuilder[W]) emitAll(insns []Bytecode[W]) {
	p.code = append(p.code, insns...)
}

func (p *shiftHelperBuilder[W]) newComputedWidth(prefix string, width uint) bytecode.RegisterId {
	var padding W

	id := bytecode.RegisterId(len(p.base))
	name := fmt.Sprintf("%s%d", prefix, p.nextTmp)
	p.base = append(p.base, descriptor.NewComputedRegister(name, util.Some(width), padding))
	p.nextTmp++

	return id
}

// newShiftLevelHelper builds level j (= level) of the barrel-shifter chain over
// values of width w (= maxWidth).  Each level returns *both* shift directions:
//
// let bit:u1, nlow:u(j-1) = n, and (lo_shl, lo_shr) = level_{j-1}(a, nlow)
//
//	level_j(a, n:u_j)  =  ( bit == 0 ? lo_shl : lo_shl << 2^(j-1),
//	                        bit == 0 ? lo_shr : lo_shr >> 2^(j-1) )
//	level_1(a, n:u1)   =  n == 0 ? (a, a) : (a << 1, a >> 1)
//
// Note the constant shift is applied to the *result* of the recursive call
// rather than to its argument.  Shifting is associative in the amount —
// (a << nlow) << 2^(j-1) == a << (nlow + 2^(j-1)) == a << n, and likewise for
// SHR — so both formulations are correct, but this one lets a single recursive
// call serve both directions.  Pre-shifting instead would need one call per
// direction (the two arguments differ), making a depth-d chain execute 2^d - 1
// calls rather than d.
//
// subID is the module id of level j-1; it is ignored when j == 1.
func newShiftLevelHelper[W word.Word[W]](maxWidth, level, subID uint) descriptor.Module[W] {
	b := newShiftHelperBuilder[W](maxWidth, level)

	a, n := b.value, b.amount
	outShl, outShr := b.outShl, b.outShr
	zero := word.Const64[W](0)

	if level == 1 {
		// if n == 0: return (a, a)
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 3,
			bytecode.NewRegisterVector(n),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(outShl, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.AddConst(outShr, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.NewRet[W]())
		// return (a << 1, a >> 1)
		b.emitAll(shiftByConst(b, bytecode.OP_SHL, outShl, a, 1))
		b.emitAll(shiftByConst(b, bytecode.OP_SHR, outShr, a, 1))
		b.emit(bytecode.NewRet[W]())
	} else {
		shift := uint(1) << (level - 1)
		// Destruct n into [nlow:u(level-1), bit:u1] (little-endian).
		nlow := b.newComputedWidth("$nlow", level-1)
		bit := b.newComputedWidth("$bit", 1)
		b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{nlow, bit}, []bytecode.RegisterId{n}))
		// lo_shl, lo_shr = level_{j-1}(a, nlow)
		loShl := b.newComputedWidth("$lo_shl", maxWidth)
		loShr := b.newComputedWidth("$lo_shr", maxWidth)
		b.emit(bytecode.CallFun[W](uint16(subID),
			[]bytecode.RegisterId{a, nlow}, []bytecode.RegisterId{loShl, loShr}))
		// out_shl = bit == 0 ? lo_shl : lo_shl << 2^(level-1)
		b.emitDiamond(bit, outShl, loShl, shiftByConst(b, bytecode.OP_SHL, outShl, loShl, shift))
		// out_shr = bit == 0 ? lo_shr : lo_shr >> 2^(level-1)
		b.emitDiamond(bit, outShr, loShr, shiftByConst(b, bytecode.OP_SHR, outShr, loShr, shift))
		b.emit(bytecode.NewRet[W]())
	}

	return descriptor.NewFunction(shiftHelperName(maxWidth, level), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}

// emitDiamond emits "target = bit == 0 ? source : <shifted>", where shifted is
// a pre-built code sequence (see shiftByConst) already writing into target.
func (p *shiftHelperBuilder[W]) emitDiamond(bit, target, source bytecode.RegisterId, shifted []Bytecode[W]) {
	zero := word.Const64[W](0)
	// If bit != 0, skip over the copy and its trailing Skip, landing on shifted.
	p.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
		bytecode.NewRegisterVector(bit),
		bytecode.NewConstantOperand(zero)))
	p.emit(bytecode.AddConst(target, []bytecode.RegisterId{source}, zero))
	p.emit(bytecode.NewSkip[W](uint16(len(shifted))))
	p.emitAll(shifted)
}

// shiftByConst returns the codes computing "target = a op shift" for a
// constant shift amount in (0, width), allocating any temporaries on the
// builder but NOT emitting (so the caller can size a Skip over the sequence):
//
//	SHR: Destruct a into [drop:u_shift, high:u(width-shift)]; target = high
//	     (zero-extended via AddConst, since high < 2^(width-shift)).
//	SHL: Destruct a into [low:u(width-shift), drop:u_shift]; target = zeros:low
//	     via Concat with a constant-zero register in the low bits.
func shiftByConst[W word.Word[W]](b *shiftHelperBuilder[W], op bytecode.Operation,
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
// shiftChainDepth(maxWidth)):
//
//	guard(a, n:u_amtWidth)  =  vhigh != 0 ? (0, 0) : level_k(a, vlow)
//
// where vhigh:vlow = n.  Any amount with a bit set above the low k bits is at
// least 2^k >= maxWidth and so shifts everything out, in either direction.
// Amounts in [maxWidth, 2^k) — possible when maxWidth is not a power of two —
// need no special handling: the level chain strips more bits than the value has
// and naturally yields zero.  For maxWidth == 1 there are no levels (k == 0)
// and the guard degenerates to "n == 0 ? (a, a) : (0, 0)"; levelID is ignored
// in that case.
//
// Both of the level call's outputs are consumed here, so unlike a call site
// (see lowerBitwiseShlShr) the guard needs no partial call.
func newShiftGuardHelper[W word.Word[W]](maxWidth, amtWidth, levelID uint) descriptor.Module[W] {
	b := newShiftHelperBuilder[W](maxWidth, amtWidth)

	a, n := b.value, b.amount
	outShl, outShr := b.outShl, b.outShr
	depth := shiftChainDepth(maxWidth)
	zero := word.Const64[W](0)

	if depth == 0 {
		// maxWidth == 1: if n == 0: return (a, a)
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 3,
			bytecode.NewRegisterVector(n),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(outShl, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.AddConst(outShr, []bytecode.RegisterId{a}, zero))
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
		b.emit(bytecode.CallFun[W](uint16(levelID),
			[]bytecode.RegisterId{a, vlow}, []bytecode.RegisterId{outShl, outShr}))
		b.emit(bytecode.NewRet[W]())
	}
	// out_shl, out_shr = 0, 0
	b.emit(bytecode.LoadConst(outShl, zero))
	b.emit(bytecode.LoadConst(outShr, zero))
	b.emit(bytecode.NewRet[W]())

	return descriptor.NewFunction(shiftHelperName(maxWidth, amtWidth), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}
