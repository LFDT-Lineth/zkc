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

// scanShiftAmountWidths scans all functions and returns, for each (operation,
// value-width) pair, the maximum shift-amount register width seen across all
// call sites.  The helper's arg2 is built with this width so every call site
// can pass its amount register with an upcast (never a downcast).
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
					origWidth, _ := maxBitwidthOf(regs, bw.Target)
					key := shiftKey{op: bw.Op, width: origWidth}
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
func newShlHelper[W word.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) descriptor.Module[W] {
	var padding W

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = descriptor.NewRegister(register.INPUT_REGISTER, "arg2", util.Some(amtWidth), padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width
	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	// if n == 0: return a
	b.emit(bytecode.NewSkipIf[W](bytecode.CONDITION_NEQ, 2,
		bytecode.NewRegisterVector(n),
		bytecode.NewConstantOperand(zero)))
	b.emit(bytecode.AddConst(out, []bytecode.RegisterId{a}, zero))
	b.emit(bytecode.NewRet[W]())

	// doubled = 2*a mod 2^width: strip the top bit via Destruct, add low+low.
	// low < 2^(width-1) so low+low < 2^width — no IntAdd overflow.
	low := b.newComputedNamed(width - 1)
	carry := b.newComputedNamed(1)
	b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{low, carry}, []bytecode.RegisterId{a}))
	doubled := b.newComputedNamed(width)
	b.emit(bytecode.AddConst(doubled, []bytecode.RegisterId{low, low}, zero))

	n1 := b.newComputedNamed(amtWidth)
	b.emit(bytecode.SubConst(n1, []bytecode.RegisterId{n}, one))
	b.emit(bytecode.CallFun[W](uint16(selfID), []bytecode.RegisterId{doubled, n1}, []bytecode.RegisterId{out}))
	b.emit(bytecode.NewRet[W]())

	return descriptor.NewFunction(helperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
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
func newShrHelper[W word.Word[W]](key bitwiseHelperKey, selfID uint, amtWidth uint) descriptor.Module[W] {
	var padding W

	b := newHelperBuilder[W](key.width, key.arity)
	b.base[1] = descriptor.NewRegister(register.INPUT_REGISTER, "arg2", util.Some(amtWidth), padding)

	a, n, out := b.inputs[0], b.inputs[1], b.output
	width := key.width
	zero := word.Const64[W](0)
	one := word.Const64[W](1)

	// if n == 0: return a
	b.emit(bytecode.NewSkipIf[W](bytecode.CONDITION_NEQ, 2,
		bytecode.NewRegisterVector(n),
		bytecode.NewConstantOperand(zero)))
	b.emit(bytecode.AddConst(out, []bytecode.RegisterId{a}, zero))
	b.emit(bytecode.NewRet[W]())

	// floor(a/2) via Destruct: split a into [lsb:u1, rest:u(width-1)].
	// rest holds the upper (width-1) bits of a, i.e. floor(a/2), with no
	// field arithmetic — works for any field modulus.
	lsb := b.newComputedNamed(1)
	rest := b.newComputedNamed(width - 1)
	b.emit(bytecode.AddVec[W]([]bytecode.RegisterId{lsb, rest}, []bytecode.RegisterId{a}))
	// Zero-extend rest from u(width-1) to u(width); safe since rest < 2^(width-1).
	half := b.newComputedNamed(width)
	b.emit(bytecode.AddConst(half, []bytecode.RegisterId{rest}, zero))
	n1 := b.newComputedNamed(amtWidth)
	b.emit(bytecode.SubConst(n1, []bytecode.RegisterId{n}, one))
	b.emit(bytecode.CallFun[W](uint16(selfID), []bytecode.RegisterId{half, n1}, []bytecode.RegisterId{out}))
	b.emit(bytecode.NewRet[W]())

	return descriptor.NewFunction(helperName(key), b.regs(), descriptor.BYTECODE_FUNCTION, nil,
		[]BytecodeVector[W]{bytecode.NewVector(b.code...)})
}
