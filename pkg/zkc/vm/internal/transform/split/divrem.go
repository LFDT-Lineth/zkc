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
func DivRem[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.DivRem[W]) []Bytecode[W] {
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
		divisor  = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Divisor)...)
	)
	//
	return []Bytecode[W]{bytecode.NewIntrinsic[W](op,
		[]bytecode.RegisterVector{target},
		[]bytecode.RegisterVector{dividend, divisor})}
}
