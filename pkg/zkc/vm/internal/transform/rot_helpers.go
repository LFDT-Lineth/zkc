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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ensureRot returns the module id of the rotation helper for values of the
// given width, entered at level amtWidth of the chain, creating any missing
// levels.  op selects the direction: OP_SHL rotates left, OP_SHR right.
// Unlike shifts, rotations need no guard: callers guarantee amtWidth <=
// shiftChainDepth(width), and for the power-of-two widths recognised by
// scanRotations every amount value is then in range.
func (p *shiftHelpers[W]) ensureRot(op bytecode.Operation, width uint, amtWidth uint) uint {
	key := shiftHelperKey{op: op, width: width, amtWidth: amtWidth, rot: true}

	if id, ok := p.ids[key]; ok {
		return id
	}

	var subID uint
	if amtWidth > 1 {
		subID = p.ensureRot(op, width, amtWidth-1)
	}

	id := p.baseID + uint(len(p.items))
	p.ids[key] = id
	p.items = append(p.items, newRotLevelHelper[W](key, subID))

	return id
}

// rotHelperName is the module name of a rotation helper: the direction, the
// value width and the amount (arg2) width.
func rotHelperName(key shiftHelperKey) string {
	dir := "rotl"
	if key.op == bytecode.OP_SHR {
		dir = "rotr"
	}

	return fmt.Sprintf("$bit_%s_u%d_u%d", dir, key.width, key.amtWidth)
}

// newRotLevelHelper builds level j (= key.amtWidth) of the rotation chain for
// values of width w (= key.width), mirroring newShiftLevelHelper:
//
// let bit:u1, low:u(j-1) = n
//
//	level_j(a, n:u_j)  =  level_{j-1}(bit == 0 ? a : a rot 2^(j-1), nlow)
//	level_1(a, n:u1)   =  n == 0 ? a : a rot 1
//
// where "a rot k" is a rotation by the constant k, realised purely by a
// Destruct followed by a swapped Concat (see rotByConst) — cheaper than a
// constant shift, which additionally zero-fills.  subID is the module id of
// level j-1; it is ignored when j == 1.
func newRotLevelHelper[W word.Word[W]](key shiftHelperKey, subID uint) descriptor.Module[W] {
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
		// out = a rotated by 1
		b.emitAll(rotByConst(b, key.op, out, a, 1))
		b.emit(bytecode.NewRet[W]())
	} else {
		shift := uint(1) << (level - 1)
		// Destruct n into [nlow:u(level-1), bit:u1] (little-endian).
		nlow := b.newComputedWidth("$nlow", level-1)
		bit := b.newComputedWidth("$bit", 1)
		b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{nlow, bit}, []bytecode.RegisterId{n}))
		// next = bit == 0 ? a : a rotated by 2^(level-1)
		next := b.newComputedWidth("$next", width)
		rotated := rotByConst(b, key.op, next, a, shift)
		b.emit(bytecode.NewSkipIf(bytecode.CONDITION_NEQ, 2,
			bytecode.NewRegisterVector(bit),
			bytecode.NewConstantOperand(zero)))
		b.emit(bytecode.AddConst(next, []bytecode.RegisterId{a}, zero))
		b.emit(bytecode.NewSkip[W](uint16(len(rotated))))
		b.emitAll(rotated)
		// return level_{j-1}(next, nlow)
		b.emit(bytecode.CallFun[W](uint16(subID), []bytecode.RegisterId{next, nlow}, []bytecode.RegisterId{out}))
		b.emit(bytecode.NewRet[W]())
	}

	return descriptor.NewFunction(rotHelperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}

// rotByConst returns the codes computing "target = a rot shift" for a
// constant amount in (0, width), allocating temporaries on the builder but
// NOT emitting (so the caller can size a Skip over the sequence).  A
// rotate-left by s splits a into [lo:u(w-s), hi:u_s] (little-endian) and
// reassembles target = lo : hi with hi in the low bits; a rotate-right by
// shift is a rotate-left by width - shift.
func rotByConst[W word.Word[W]](b *helperBuilder[W], op bytecode.Operation,
	target, a bytecode.RegisterId, shift uint,
) []Bytecode[W] {
	var (
		width = b.width
		s     = shift
	)

	if op == bytecode.OP_SHR {
		s = width - shift
	}

	var (
		lo = b.newComputedWidth("$lo", width-s)
		hi = b.newComputedWidth("$hi", s)
	)

	return []Bytecode[W]{
		bytecode.AddVec[W]([]bytecode.RegisterId{lo, hi}, []bytecode.RegisterId{a}),
		bytecode.AssignV[W]([]bytecode.RegisterId{target}, hi, lo),
	}
}
