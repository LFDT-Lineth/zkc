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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Shift splits a bitwise SHL or SHR instruction into a single WIDE_SHL /
// WIDE_SHR intrinsic operating over the limb vectors of its operands.  Unlike
// AND, OR and XOR — which are bit-position-aligned and therefore split limb-wise
// (see Bitwise) — a shift moves bits across limb boundaries and so cannot be
// decomposed into independent per-limb shifts.  Instead the (possibly
// multi-limb) value, shift amount and result are handed to the corresponding
// wide intrinsic, whose executor reconstructs each value via big.Int arithmetic
// (see loadIntrinsicOperand / storeIntrinsicResult).  For example, splitting
//
// > x = y << z            (all u16)
//
// into u8 limbs (x=x1::x0, etc.) yields
//
// > x1;x0 = wide_shl(y1;y0, z1;z0)
//
// where each operand is the limb vector of the original register (ordered
// most-significant limb first, matching ApplyLimbsMap).
func Shift[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.Bitwise[W]) []Bytecode[W] {
	var op bytecode.Operation
	//
	switch insn.Op {
	case bytecode.OP_SHL:
		op = bytecode.WIDE_SHL
	case bytecode.OP_SHR:
		op = bytecode.WIDE_SHR
	default:
		panic("expected shift operation")
	}
	//
	var (
		target = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Target)...)
		value  = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Left)...)
		amount = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Right)...)
	)
	//
	return []Bytecode[W]{bytecode.NewIntrinsic[W](op,
		[]bytecode.RegisterVector{target},
		[]bytecode.RegisterVector{value, amount})}
}
