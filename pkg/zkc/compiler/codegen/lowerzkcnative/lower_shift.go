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
package lowerzkcnative

import (
	"math/big"
	"math/bits"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// shiftKey identifies a shift helper by opcode and value width.
type shiftKey struct {
	opcode instruction.OpCode
	width  uint
}

// scanShiftAmountWidths scans all Boot functions and returns, for each
// (opcode, value-width) pair, the maximum shift-amount register width seen
// across all call sites.  The helper's arg2 is built with this width so every
// call site can pass its amount register with an upcast (never a downcast).
func scanShiftAmountWidths[W vm.Word[W]](modules []vm.Module) map[shiftKey]uint {
	result := make(map[shiftKey]uint)

	for _, mod := range modules {
		fn, ok := mod.(*vm.WordFunction)
		if !ok {
			continue
		}

		regs := fn.RegisterMap()

		for _, vec := range fn.Code() {
			for _, insn := range vec.Codes {
				var (
					op                 instruction.OpCode
					targetID, amountID register.Id
				)

				switch t := insn.(type) {
				case *instruction.BitShl[W]:
					op, targetID, amountID = t.OpCode(), t.Target, t.Sources[1]
				case *instruction.BitShr[W]:
					op, targetID, amountID = t.OpCode(), t.Target, t.Sources[1]
				default:
					continue
				}

				origWidth, _ := lowerableWidth(regs, targetID)
				key := shiftKey{opcode: op, width: origWidth}
				amountWidth := regs.Register(amountID).Width()

				if existing, seen := result[key]; !seen || amountWidth > existing {
					result[key] = amountWidth
				}
			}
		}
	}

	return result
}

// bitwiseShiftCall emits a call to a SHL/SHR helper module.  The value
// register is passed directly (its width matches the helper's arg1 by
// construction).  The amount register is cast up to amtWidth when its width
// is narrower (upcasting never fails); a call site with the widest amount type
// passes it directly without any cast.
func bitwiseShiftCall(
	id uint,
	target, value, amount register.Id,
	amtWidth uint,
	registers RegisterAllocator,
) []vm.WordInstruction {
	if registers.Register(amount).Width() == amtWidth {
		return []vm.WordInstruction{
			instruction.NewCall(id, []register.Id{value, amount}, []register.Id{target}),
		}
	}

	wAmount := registers.Allocate("", amtWidth)

	return []vm.WordInstruction{
		instruction.NewCast(wAmount, amount, amtWidth),
		instruction.NewCall(id, []register.Id{value, wAmount}, []register.Id{target}),
	}
}

// newShlHelper builds a self-recursive module for left shift:
//
//	shl(a, 0)    = a
//	shl(a, n>=w) = 0
//	shl(a, n)    = shl(2*a mod 2^w, n-1)
//
// Doubling is done as low(a) + low(a) where low(a) = Destruct(a)[0:width-1].
// This avoids IntAdd overflow since low(a) < 2^(width-1), so 2*low(a) < 2^width.
// amtWidth is the register width of arg2 (the shift amount); it equals the
// maximum shift-amount width seen across all call sites for this value width.
// selfID must be the module slot that will be assigned to this module.
func newShlHelper[W vm.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) vm.Module {
	var padding big.Int

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = register.NewInput("arg2", amtWidth, padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width

	zero := vm.Uint64[W](0)
	one := vm.Uint64[W](1)
	wmax := vm.Uint64[W](uint64(width - 1))

	// wmaxReg needs enough bits to hold wmax = width-1; nWide needs enough bits
	// to hold any shift amount n (up to 2^amtWidth-1).  Take the larger of the
	// two so a single width covers both registers.
	wmaxWidth := amtWidth
	if needed := uint(bits.Len(width - 1)); needed > wmaxWidth {
		wmaxWidth = needed
	}

	if wmaxWidth == 0 {
		wmaxWidth = 1
	}

	zeroReg := b.newComputedNamed(amtWidth)
	wmaxReg := b.newComputedNamed(wmaxWidth)
	b.emit(instruction.NewIntAdd(zeroReg, nil, zero))
	b.emit(instruction.NewIntAdd(wmaxReg, nil, wmax))

	// if n == 0: return a
	b.emit(instruction.NewSkipIf(opcode.NEQ, n, zeroReg, 2))
	b.emit(instruction.NewIntAdd(out, []register.Id{a}, zero))
	b.emit(instruction.NewReturn())

	// if n >= width: return 0
	// Zero-extend n to wmaxWidth so both sides of LTEQ share the same register width.
	nWide := b.newComputedNamed(wmaxWidth)
	b.emit(instruction.NewIntAdd(nWide, []register.Id{n}, zero))
	b.emit(instruction.NewSkipIf(opcode.LTEQ, nWide, wmaxReg, 2))
	b.emit(instruction.NewIntAdd(out, nil, zero))
	b.emit(instruction.NewReturn())

	// doubled = 2*a mod 2^width: strip the top bit via Destruct, add low+low.
	// low < 2^(width-1) so low+low < 2^width — no IntAdd overflow.
	low := b.newComputedNamed(width - 1)
	b.emit(instruction.NewDestruct([]register.Id{low}, a))
	doubled := b.newComputedNamed(width)
	b.emit(instruction.NewIntAdd(doubled, []register.Id{low, low}, zero))

	n1 := b.newComputedNamed(amtWidth)
	b.emit(instruction.NewIntSub(n1, []register.Id{n}, one))
	b.emit(instruction.NewCall(selfID, []register.Id{doubled, n1}, []register.Id{out}))
	b.emit(instruction.NewReturn())

	return vm.NewFunction(helperName(key), false, b.regs(), []vectorInstruction{{Codes: b.code}})
}

// newShrHelper builds a self-recursive module for logical right shift:
//
//	shr(a, 0)    = a
//	shr(a, n>=w) = 0
//	shr(a, n)    = shr(floor(a/2), n-1)
//
// floor(a/2) via Destruct: split a into [lsb:u1, rest:u(width-1)].
// rest holds the upper (width-1) bits of a, i.e. floor(a/2), with no
// field arithmetic — works for any field modulus.
// amtWidth is the register width of arg2; see newShlHelper for details.
// selfID must be the module slot that will be assigned to this module.
func newShrHelper[W vm.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) vm.Module {
	var padding big.Int

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = register.NewInput("arg2", amtWidth, padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width

	zero := vm.Uint64[W](0)
	one := vm.Uint64[W](1)
	wmax := vm.Uint64[W](uint64(width - 1))

	// wmaxReg needs enough bits to hold wmax = width-1; nWide needs enough bits
	// to hold any shift amount n (up to 2^amtWidth-1).  Take the larger of the
	// two so a single width covers both registers.
	wmaxWidth := amtWidth
	if needed := uint(bits.Len(width - 1)); needed > wmaxWidth {
		wmaxWidth = needed
	}

	if wmaxWidth == 0 {
		wmaxWidth = 1
	}

	zeroReg := b.newComputedNamed(amtWidth)
	wmaxReg := b.newComputedNamed(wmaxWidth)
	b.emit(instruction.NewIntAdd(zeroReg, nil, zero))
	b.emit(instruction.NewIntAdd(wmaxReg, nil, wmax))

	// if n == 0: return a
	b.emit(instruction.NewSkipIf(opcode.NEQ, n, zeroReg, 2))
	b.emit(instruction.NewIntAdd(out, []register.Id{a}, zero))
	b.emit(instruction.NewReturn())

	// if n >= width: return 0
	// Zero-extend n to wmaxWidth so both sides of LTEQ share the same register width.
	nWide := b.newComputedNamed(wmaxWidth)
	b.emit(instruction.NewIntAdd(nWide, []register.Id{n}, zero))
	b.emit(instruction.NewSkipIf(opcode.LTEQ, nWide, wmaxReg, 2))
	b.emit(instruction.NewIntAdd(out, nil, zero))
	b.emit(instruction.NewReturn())

	// floor(a/2) via Destruct: split a into [lsb:u1, rest:u(width-1)].
	// rest holds the upper (width-1) bits of a, i.e. floor(a/2), with no
	// field arithmetic — works for any field modulus.
	lsb := b.newComputedNamed(1)
	rest := b.newComputedNamed(width - 1)
	b.emit(instruction.NewDestruct([]register.Id{lsb, rest}, a))
	// Zero-extend rest from u(width-1) to u(width); safe since rest < 2^(width-1).
	half := b.newComputedNamed(width)
	b.emit(instruction.NewIntAdd(half, []register.Id{rest}, zero))
	n1 := b.newComputedNamed(amtWidth)
	b.emit(instruction.NewIntSub(n1, []register.Id{n}, one))
	b.emit(instruction.NewCall(selfID, []register.Id{half, n1}, []register.Id{out}))
	b.emit(instruction.NewReturn())

	return vm.NewFunction(helperName(key), false, b.regs(), []vectorInstruction{{Codes: b.code}})
}
