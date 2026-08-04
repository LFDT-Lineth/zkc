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

package split

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DivRem splits a DIV or REM instruction into a single WIDE_DIV / WIDE_REM
// intrinsic operating over the limb vectors of its operands.  Like a shift (see
// Shift), a division cannot be decomposed into independent per-limb operations
// because carries and borrows cross limb boundaries.  Instead the (possibly
// multi-limb) dividend, divisor and result are handed to the corresponding wide
// intrinsic, whose executor reconstructs each value via big.Int arithmetic (see
// loadIntrinsicOperand / storeIntrinsicResult).  For example, splitting
//
// > x = y / z            (all u16)
//
// into u8 limbs (x=x1::x0, etc.) yields
//
// > x1;x0 = wide_div(y1;y0, z1;z0)
//
// where each operand is the limb vector of the original register (ordered
// most-significant limb first, matching ApplyLimbsMap).
func DivRem[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	insn *bytecode.DivRem[W]) []Bytecode[W] {
	//
	var op bytecode.Operation
	//
	switch insn.Opcode {
	case encoding.DIV:
		op = bytecode.WIDE_DIV
	case encoding.REM:
		op = bytecode.WIDE_REM
	default:
		panic("expected division operation")
	}
	//
	var (
		target   = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Target)...)
		dividend = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Dividend)...)
		// Divisor operand (and any loads required to materialise it).
		loads, divisor, divisorLen = Operand(mapping, alloc, insn.Divisor)
	)
	// Check whether splitting actually required
	if target.Len == 1 && dividend.Len == 1 && divisorLen == 1 {
		if divisor.IsConstant() {
			// No, splitting not technically required
			return []Bytecode[W]{bytecode.NewDivRemConst(insn.Opcode,
				target.Base, dividend.Base, divisor.AsConstant())}
		}
		// No, splitting not technically required
		return append(loads, bytecode.NewDivRem[W](insn.Opcode,
			target.Base, dividend.Base, divisor.AsRegister()))
	}
	// Yes, splitting is actually required.
	return append(loads, bytecode.NewIntrinsic(op,
		[]bytecode.RegisterVector{target},
		[]bytecode.Operand[W]{
			bytecode.NewRegisterVectorOperand[W](dividend),
			divisor,
		}))
}

// Operand splits an operand into limbs: a register operand becomes the
// limb vector of its constituent registers, whilst a constant operand stays a
// single (unsplit) constant when it fits the register width — the invariant
// being that constant operands are always single-limb.  A constant too wide
// for a single limb is instead materialised into freshly allocated registers
// via the returned load bytecodes, which must precede the consuming
// instruction.
func Operand[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	operand bytecode.Operand[W]) (loads []Bytecode[W], out bytecode.Operand[W], n uint16) {
	//
	if !operand.IsConstant() {
		var vec = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, operand.AsRegisters()...)...)
		//
		return nil, bytecode.NewRegisterVectorOperand[W](vec), vec.Len
	}
	//
	var constant = operand.AsConstant()
	//
	if constant.FitsWithin(mapping.RegisterWidth()) {
		return nil, operand, 1
	}
	// Constant too wide for a single limb: materialise it into registers.
	var vec bytecode.RegisterVector
	//
	loads, vec = materialiseConstant(alloc, constant, mapping.RegisterWidth())
	//
	return loads, bytecode.NewRegisterVectorOperand[W](vec), vec.Len
}

// materialiseConstant loads a constant too wide for a single limb into freshly
// allocated (consecutive) registers of the given width, returning the loading
// bytecodes together with the resulting register vector (most-significant limb
// first, matching ApplyLimbsMap).
func materialiseConstant[W word.Word[W]](alloc Allocator[W], constant W, width uint,
) ([]Bytecode[W], bytecode.RegisterVector) {
	// Slice the constant into limbs, least-significant first.
	var limbs = []W{constant.Slice(width)}
	//
	for acc := constant.Shr64(uint64(width)); acc.Cmp64(0) > 0; acc = acc.Shr64(uint64(width)) {
		limbs = append(limbs, acc.Slice(width))
	}
	// Reverse into most-significant-first order.
	slices.Reverse(limbs)
	//
	var (
		total = uint(constant.BigInt().BitLen())
		regs  = make([]bytecode.RegisterId, len(limbs))
		loads = make([]Bytecode[W], len(limbs))
	)
	//
	for i, limb := range limbs {
		// Every limb spans the full width except the (leftover) most
		// significant one.
		w := width
		//
		if i == 0 {
			w = total - width*uint(len(limbs)-1)
		}
		//
		regs[i] = alloc.Allocate("", util.Some(w))
		loads[i] = bytecode.LoadConst(regs[i], limb)
	}
	//
	return loads, bytecode.NewRegisterVector(regs...)
}
