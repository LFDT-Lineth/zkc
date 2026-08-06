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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
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
	//
	var (
		op       bytecode.Operation
		targetId bytecode.RegisterId
	)
	//
	switch {
	case insn.Quotient.HasValue() && insn.Remainder.HasValue():
		// Combined divmod only exists between codegen and LowerDivisions in
		// tracing mode; splitting never sees it (fast-mode codegen emits a
		// DIV / REM pair instead).
		panic("divmod must be lowered before splitting")
	case insn.Quotient.HasValue():
		op, targetId = bytecode.WIDE_DIV, insn.Quotient.Unwrap()
	case insn.Remainder.HasValue():
		op, targetId = bytecode.WIDE_REM, insn.Remainder.Unwrap()
	default:
		panic("division without quotient or remainder")
	}
	//
	var (
		target              = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, targetId)...)
		dividend            = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, insn.Dividend)...)
		divisor, divisorLen = Operand(mapping, insn.Divisor)
	)
	// Check whether splitting actually required
	if target.Len == 1 && dividend.Len == 1 && divisorLen == 1 {
		var quotient, remainder = util.None[bytecode.RegisterId](), util.None[bytecode.RegisterId]()
		// Rebuild the (single) target option over the limb-mapped register.
		if insn.Quotient.HasValue() {
			quotient = util.Some(target.Base)
		} else {
			remainder = util.Some(target.Base)
		}
		// No, splitting not technically required
		return []Bytecode[W]{bytecode.NewDivRem(quotient, remainder, dividend.Base, divisor)}
	}
	// Yes, splitting is actually required.
	return []Bytecode[W]{bytecode.NewIntrinsic(op,
		[]bytecode.RegisterVector{target},
		[]bytecode.Operand[W]{
			bytecode.NewRegisterVectorOperand[W](dividend),
			divisor,
		})}
}

// Operand splits an operand into limbs: a register operand becomes the limb
// vector of its constituent registers, whilst a constant operand always stays
// a single (unsplit) value, regardless of its width — like an arithmetic
// immediate, it never occupies a register.  Its consumers work on the value
// directly (the intrinsic executors reconstruct full values anyway), so it
// need not fit the register width; only the multi-limb constant form is
// forbidden (see Intrinsic.Validate).
func Operand[W word.Word[W]](mapping descriptor.LimbsMap[W], operand bytecode.Operand[W],
) (out bytecode.Operand[W], n uint16) {
	//
	if operand.IsConstant() {
		return operand, 1
	}
	//
	var vec = bytecode.NewRegisterVector(ApplyLimbsMap(mapping, operand.AsRegisters()...)...)
	//
	return bytecode.NewRegisterVectorOperand[W](vec), vec.Len
}
