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
	"math/big"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
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
func scanShiftAmountWidths[W word.Word[W]](modules []Module) map[shiftKey]uint {
	result := make(map[shiftKey]uint)

	for _, mod := range modules {
		fn, ok := mod.(*WordFunction)
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

				switch insn.OpCode() {
				case opcode.BIT_SHL:
					t := insn.(*instruction.WordTypeB)
					op, targetID, amountID = t.OpCode(), t.Target, t.RightSource
				case opcode.BIT_SHR:
					t := insn.(*instruction.WordTypeB)
					op, targetID, amountID = t.OpCode(), t.Target, t.RightSource
				default:
					continue
				}

				origWidth, _ := maxBitwidthOf(regs, targetID)
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
func newShlHelper[W word.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) Module {
	var padding big.Int

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = register.NewInput("arg2", amtWidth, padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width
	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	zeroReg := b.newComputedNamed(amtWidth)
	b.emit(instruction.UintConst(zeroReg, zero))

	// if n == 0: return a
	b.emit(instruction.NewSkipIf(opcode.NEQ, n, zeroReg, 2))
	b.emit(instruction.UintAdd(out, []register.Id{a}, zero))
	b.emit(instruction.NewReturn())

	// doubled = 2*a mod 2^width: strip the top bit via Destruct, add low+low.
	// low < 2^(width-1) so low+low < 2^width — no IntAdd overflow.
	low := b.newComputedNamed(width - 1)
	carry := b.newComputedNamed(1)
	b.emit(instruction.UintDestruct[W](register.NewVector(low, carry), a))
	doubled := b.newComputedNamed(width)
	b.emit(instruction.UintAdd(doubled, []register.Id{low, low}, zero))

	n1 := b.newComputedNamed(amtWidth)
	b.emit(instruction.UintSub(n1, []register.Id{n}, one))
	b.emit(instruction.NewCall(selfID, []register.Id{doubled, n1}, []register.Id{out}))
	b.emit(instruction.NewReturn())

	return function.New(helperName(key), false, b.regs(), []VectorInstruction{{Codes: b.code}})
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
func newShrHelper[W word.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) Module {
	var padding big.Int

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = register.NewInput("arg2", amtWidth, padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width
	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	zeroReg := b.newComputedNamed(amtWidth)
	b.emit(instruction.UintConst(zeroReg, zero))

	// if n == 0: return a
	b.emit(instruction.NewSkipIf(opcode.NEQ, n, zeroReg, 2))
	b.emit(instruction.UintAdd(out, []register.Id{a}, zero))
	b.emit(instruction.NewReturn())

	// floor(a/2) via Destruct: split a into [lsb:u1, rest:u(width-1)].
	// rest holds the upper (width-1) bits of a, i.e. floor(a/2), with no
	// field arithmetic — works for any field modulus.
	lsb := b.newComputedNamed(1)
	rest := b.newComputedNamed(width - 1)
	b.emit(instruction.UintDestruct[W](register.NewVector(lsb, rest), a))
	// Zero-extend rest from u(width-1) to u(width); safe since rest < 2^(width-1).
	half := b.newComputedNamed(width)
	b.emit(instruction.UintAdd(half, []register.Id{rest}, zero))
	n1 := b.newComputedNamed(amtWidth)
	b.emit(instruction.UintSub(n1, []register.Id{n}, one))
	b.emit(instruction.NewCall(selfID, []register.Id{half, n1}, []register.Id{out}))
	b.emit(instruction.NewReturn())

	return function.New(helperName(key), false, b.regs(), []VectorInstruction{{Codes: b.code}})
}
