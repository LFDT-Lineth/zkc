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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerDivisions rewrites INT_DIV and INT_REM bytecodes into a non-deterministic
// hint followed by arithmetic validation (see expandDivRem for the emitted
// sequence and the rationale for its structure):
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy  = q * y
//	qyr = qy + r
//	0   = x - qyr            // written into a 0-width register: asserts x == q*y + r
//	rw1 = r + w + 1
//	0   = y - rw1            // written into a 0-width register: asserts y == r + w + 1
//
// NOTE: this transform must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerDivisionFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func lowerDivisionFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return lowerDivisionCode(b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

// lowerDivisionCode replaces a DIV / REM / divmod bytecode with the
// hint+validation sequence (see expandDivRem).  The hint produces both
// quotient and remainder, so whichever the bytecode does not name is written
// to a freshly allocated register; a divmod (both present) thus shares a
// single block for the pair.
func lowerDivisionCode[W word.Word[W]](b Bytecode[W], registers split.Allocator[W]) []Bytecode[W] {
	dr, ok := b.(*bytecode.DivRem[W])
	if !ok {
		return []Bytecode[W]{b}
	}
	//
	var (
		x    = dr.Dividend
		y    = dr.Divisor
		nX   = registers.Register(x).Bitwidth().Unwrap()
		nY   = divisorWidth(y, registers)
		w    = registers.Allocate("", util.Some(nY))
		q, r bytecode.RegisterId
	)
	//
	if !dr.Quotient.HasValue() && !dr.Remainder.HasValue() {
		panic("At least one of quotient or remainder must be defined")
	}
	//
	if dr.Quotient.HasValue() {
		q = dr.Quotient.Unwrap()
	} else {
		q = registers.Allocate("", util.Some(nX))
	}
	//
	if dr.Remainder.HasValue() {
		r = dr.Remainder.Unwrap()
	} else {
		r = registers.Allocate("", util.Some(nY))
	}
	//
	return expandDivRem(q, r, w, x, y, nX, registers)
}

// divisorWidth returns the bitwidth of the given divisor operand: the declared
// register width for a register divisor, or the (minimal) width of the value
// itself for a constant divisor.
func divisorWidth[W word.Word[W]](y bytecode.Operand[W], registers split.Allocator[W]) uint {
	if y.IsConstant() {
		return uint(y.AsConstant().BigInt().BitLen())
	}
	//
	return registers.Register(y.AsRegister()).Bitwidth().Unwrap()
}

// expandDivRem builds the shared hint+validation sequence for both INT_DIV and
// INT_REM, given the (already allocated) quotient q, remainder r and range
// witness w registers.  It emits:
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy  = q * y
//	qyr = qy + r
//	0   = x - qyr                   // asserts x == q*y + r
//	rw1 = r + w + 1
//	0   = y - rw1                   // asserts y == r + w + 1 (i.e. r + w < y, so r < y)
//
// The validity checks are deliberately structured as two-operand differences
// asserted to be zero (x == qyr and y == rw1) rather than as a single
// three-operand subtraction (e.g. x - qy - r == 0).  A three-operand zero
// assertion cannot be split limb-wise without a borrow chain, and for wide
// operands that borrow grows past the field register width.  A two-operand zero
// assertion splits into independent per-limb equalities (see split.Subtraction),
// which needs no borrows.
func expandDivRem[W word.Word[W]](q, r, w, x bytecode.RegisterId, y bytecode.Operand[W], nX uint,
	registers split.Allocator[W]) []Bytecode[W] {
	var (
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("qy", util.Some(nX))
		// qyr = qy + r must hold q*y + r which, by the DIV_HINT semantics
		// (q = x/y, r = x%y), equals the dividend x exactly and so fits nX
		// bits; the extra bit guards the addition's transient carry.
		qyr = registers.Allocate("qy_plus_r", util.Some(nX+1))
		// rw1 = r + w + 1 holds the divisor exactly (nY bits) + 1 to ensure no overflow
		nRW = max(registers.Register(r).Bitwidth().Unwrap(),
			registers.Register(w).Bitwidth().Unwrap()) + 1
		rw1 = registers.Allocate("", util.Some(nRW))
		// TODO: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("zero", util.Some[uint](0))
		z1 = registers.Allocate("zero", util.Some[uint](0))

		mulQY, subZ1 Bytecode[W]
	)
	//
	if y.IsConstant() {
		mulQY = bytecode.MulConst(qy, []bytecode.RegisterId{q}, y.AsConstant())
		subZ1 = bytecode.SubConst(z1, []bytecode.RegisterId{rw1}, y.AsConstant())
	} else {
		mulQY = bytecode.MulConst(qy, []bytecode.RegisterId{q, y.AsRegister()}, one)
		subZ1 = bytecode.SubConst(z1, []bytecode.RegisterId{y.AsRegister(), rw1}, zero)
	}
	//
	return []Bytecode[W]{
		bytecode.NewIntrinsic(bytecode.DIV_HINT,
			[]bytecode.RegisterVector{
				bytecode.NewRegisterVector(q), bytecode.NewRegisterVector(r), bytecode.NewRegisterVector(w),
			},
			[]bytecode.Operand[W]{bytecode.NewRegisterOperand[W](x), y}),
		mulQY,
		bytecode.AddConst(qyr, []bytecode.RegisterId{qy, r}, zero),
		bytecode.SubConst(z0, []bytecode.RegisterId{x, qyr}, zero),
		bytecode.AddConst(rw1, []bytecode.RegisterId{r, w}, one),
		subZ1,
	}
}
