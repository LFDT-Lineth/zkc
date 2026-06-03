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
package machine

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Word --- see documentation on vm.WordMachine
type Word[W word.Word[W]] = Base[W, instruction.Word, WordExecutor[W]]

// WordFrame is a stack frame for word machines.
type WordFrame[W word.Word[W]] = StackFrame[W, instruction.Word]

// NewWord constructs a new empty word machine
func NewWord[W word.Word[W]](field field.Config, modules ...Module) *Word[W] {
	var (
		prime W
		// Construct executor over the given prime modulus
		executor = WordExecutor[W]{prime.SetBigInt(field.Modulus())}
	)
	//
	return NewBase(executor, modules...)
}

// NewWordFromModulus constructs a new empty word machine directly from a given
// prime modulus (already expressed in the target word type).
func NewWordFromModulus[W word.Word[W]](modulus W, modules ...Module) *Word[W] {
	return NewBase(WordExecutor[W]{modulus}, modules...)
}

// ==============================================================
// Word Executor
// ==============================================================

// WordExecutor provides an executor implementation suitable for word
// instruction.
type WordExecutor[W word.Word[W]] struct {
	// Prime modulus is needed only for simulating the execution of native field
	// instructions.
	modulus W
}

// Modulus returns the prime modulus this executor was constructed with.  This
// is primarily of use when deriving a new word machine from an existing one.
func (p WordExecutor[W]) Modulus() W {
	return p.modulus
}

// nolint
func (p *WordExecutor[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(&p.modulus); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *WordExecutor[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	return gobDecoder.Decode(&p.modulus)
}

// Execute implementation for Executor interface.
func (p WordExecutor[W]) Execute(insn instruction.Word, frame WordFrame[W]) (err error) {
	//nolint
	switch insn.OpCode() {
	// ==============================================================
	// Arithmetic Instructions
	// ==============================================================
	case opcode.INT_ADD:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeAdd(insn.Target, insn.Sources, insn.Constant, frame)
		// Fall thru
	case opcode.INT_DIV:
		insn := insn.(*instruction.WordTypeB)
		err = executeDiv(insn, frame)
		// Fall thru
	case opcode.INT_MUL:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeMul(insn.Target, insn.Sources, insn.Constant, frame)
		// Fall thru
	case opcode.INT_REM:
		insn := insn.(*instruction.WordTypeB)
		err = executeRem(insn, frame)
		// Fall thru
	case opcode.INT_SUB:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeSub(insn.Target, insn.Sources, insn.Constant, frame)
		// Fall thru

	// ==============================================================
	// Field Instructions
	// ==============================================================

	case opcode.INT_ADDMOD_P:
		insn := insn.(*instruction.WordTypeF[W])
		err = executeFieldAdd(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru
	case opcode.INT_SUBMOD_P:
		insn := insn.(*instruction.WordTypeF[W])
		err = executeFieldSub(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru
	case opcode.INT_MULMOD_P:
		insn := insn.(*instruction.WordTypeF[W])
		err = executeFieldMul(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru

	// ==============================================================
	// Bitwise Instructions
	// ==============================================================
	case opcode.BIT_AND:
		insn := insn.(*instruction.WordTypeB)
		err = executeAnd(insn, frame)
		// Fall thru
	case opcode.BIT_NOT:
		insn := insn.(*instruction.WordTypeB)
		err = executeNot(insn, frame)
		// Fall thru
	case opcode.BIT_OR:
		insn := insn.(*instruction.WordTypeB)
		err = executeOr(insn, frame)
		// Fall thru
	case opcode.BIT_XOR:
		insn := insn.(*instruction.WordTypeB)
		err = executeXor(insn, frame)
		// Fall thru
	case opcode.BIT_SHL:
		insn := insn.(*instruction.WordTypeB)
		err = executeShl(insn, frame)
		// Fall thru
	case opcode.BIT_SHR:
		insn := insn.(*instruction.WordTypeB)
		err = executeShr(insn, frame)
		// Fall thru
	case opcode.BIT_CONCAT:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeConcat(insn.Target, insn.Sources, frame)
		// Fall thru

	// ==============================================================
	// Field Instructions (executable in word machine)
	// ==============================================================
	case opcode.HINT_DIVISION:
		insn := insn.(*instruction.FieldHint)
		err = executeDivHint(insn.Targets, insn.Sources, frame)
		// Fall thru

	// ==============================================================
	// Misc Instructions
	// ==============================================================

	default:
		return fmt.Errorf("unknown word instruction (0x%x)", insn.OpCode())
	}
	//
	return err
}

// ==============================================================
// Arithmetic Instructions
// ==============================================================

func executeAdd[W word.Word[W]](target register.Vector, sources []register.Id, constant W,
	frame WordFrame[W]) error {
	//
	var (
		val      = constant
		overflow bool
	)
	//
	for _, arg := range sources {
		val, overflow = val.Add(frame.Load(arg))
		//
		if overflow {
			return errors.New("arithmetic overflow")
		}
	}
	//
	return StoreAcross(frame, target, val)
}

func executeMul[W word.Word[W]](target register.Vector, sources []register.Id, constant W,
	frame WordFrame[W]) error {
	//
	var (
		val      W = constant
		overflow bool
	)
	//
	for _, arg := range sources {
		var of bool
		//
		val, of = val.Mul(frame.Load(arg))
		//
		overflow = overflow || of
	}
	//
	if overflow && val.Cmp64(0) != 0 {
		// overflow is real
		return errors.New("arithmetic overflow")
	}
	//
	return StoreAcross(frame, target, val)
}

func executeSub[W word.Word[W]](target register.Vector, sources []register.Id, constant W,
	frame WordFrame[W]) error {
	//
	var (
		val       W
		underflow bool
	)
	//
	for i, arg := range sources {
		ith := frame.Load(arg)
		//
		if i == 0 {
			val = ith
		} else {
			if val, underflow = val.Sub(ith); underflow {
				return errors.New("arithmetic underflow")
			}
		}
	}
	// Subtract constant
	if val, underflow = val.Sub(constant); underflow {
		return errors.New("arithmetic underflow")
	}
	//
	return StoreAcross(frame, target, val)
}

// executeFieldAdd computes the field sum of the source registers and the
// given constant, storing the result in the target register.  Reduction is
// performed implicitly within the field's bandwidth — the underlying word
// type is responsible for wrapping at the field's prime characteristic.
func executeFieldAdd[W word.Word[W]](tgt register.Id, srcs []register.Id, constant, mod W, frame WordFrame[W]) error {
	//
	for _, arg := range srcs {
		constant = constant.AddMod(frame.Load(arg), mod)
	}
	//
	return frame.Store(tgt, constant)
}

// executeFieldSub computes the chained field difference of the source
// registers minus the given constant, storing the result in the target
// register.
func executeFieldSub[W word.Word[W]](tgt register.Id, srcs []register.Id, constant, mod W, frame WordFrame[W]) error {
	//
	var val W
	//
	for i, arg := range srcs {
		ith := frame.Load(arg)
		//
		if i == 0 {
			val = ith
		} else {
			val = val.SubMod(ith, mod)
		}
	}
	//
	return frame.Store(tgt, val.SubMod(constant, mod))
}

// executeFieldMul computes the field product of the source registers and
// the given constant, storing the result in the target register.
func executeFieldMul[W word.Word[W]](tgt register.Id, srcs []register.Id, constant, mod W, frame WordFrame[W]) error {
	//
	var (
		val W = constant
	)
	//
	for _, arg := range srcs {
		val = val.MulMod(frame.Load(arg), mod)
	}
	//
	return frame.Store(tgt, val)
}

func executeDiv[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	//
	var (
		dividend = frame.Load(insn.LeftSource)
		divisor  = frame.Load(insn.RightSource)
	)
	//
	if divisor.Cmp64(0) == 0 {
		return errors.New("division by zero")
	}
	//
	val := dividend.Div(divisor)
	//
	return frame.Store(insn.Target, val)
}

func executeRem[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	//
	var (
		dividend = frame.Load(insn.LeftSource)
		divisor  = frame.Load(insn.RightSource)
	)
	//
	if divisor.Cmp64(0) == 0 {
		return errors.New("division by zero")
	}
	//
	val := dividend.Rem(divisor)
	//
	return frame.Store(insn.Target, val)
}

// ==============================================================
// Bitwise Instructions
// ==============================================================

func executeAnd[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		rhs = frame.Load(insn.RightSource)
		val = lhs.And(rhs)
	)
	//
	return frame.Store(insn.Target, val)
}

func executeOr[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		rhs = frame.Load(insn.RightSource)
		val = lhs.Or(rhs)
	)
	//
	return frame.Store(insn.Target, val)
}

func executeXor[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		rhs = frame.Load(insn.RightSource)
		val = lhs.Xor(rhs)
	)
	//
	return frame.Store(insn.Target, val)
}

func executeNot[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		val = lhs.Not(insn.Bitwidth)
	)
	//
	return frame.Store(insn.Target, val)
}

// ==============================================================
// Shift Instructions
// ==============================================================

func executeShl[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		rhs = frame.Load(insn.RightSource)
		val = lhs.Shl(insn.Bitwidth, rhs)
	)
	//
	return frame.Store(insn.Target, val)
}

func executeShr[W word.Word[W]](insn *instruction.WordTypeB, frame WordFrame[W]) error {
	var (
		lhs = frame.Load(insn.LeftSource)
		rhs = frame.Load(insn.RightSource)
		val = lhs.Shr(rhs)
	)
	//
	return frame.Store(insn.Target, val)
}

// ==============================================================
// Hint Instructions (executable in word machine)
// ==============================================================

// executeDivHint computes quotient and remainder for a division hint.
// targets[0] = sources[0] / sources[1], targets[1] = sources[0] % sources[1].
func executeDivHint[W word.Word[W]](targets []register.Id, sources []register.Id, frame WordFrame[W]) error {
	//
	var (
		dividend = frame.Load(sources[0])
		divisor  = frame.Load(sources[1])
		one      W
		uf2      bool
	)
	//
	one = one.SetUint64(1)
	//
	if divisor.Cmp64(0) == 0 {
		return errors.New("division by zero")
	}
	//
	q := dividend.Div(divisor)
	r := dividend.Rem(divisor)
	w, uf1 := divisor.Sub(r)
	w, uf2 = w.Sub(one)
	//
	if uf1 || uf2 {
		return errors.New("arithmetic underflow")
	}
	// assign q
	if err := frame.Store(targets[0], q); err != nil {
		return err
	}
	// assign r
	if err := frame.Store(targets[1], r); err != nil {
		return err
	}
	// assign w
	if err := frame.Store(targets[2], w); err != nil {
		return err
	}
	//
	return nil
}

// ==============================================================
// Misc Instructions
// ==============================================================

func executeConcat[W word.Word[W]](target register.Vector, sources []register.Id, frame WordFrame[W]) error {
	//
	var val W
	//
	for i := len(sources); i > 0; i = i - 1 {
		// determine register width
		var (
			reg   = sources[i-1]
			width = frame.BitwidthOf(reg)
		)
		//
		val = val.Shl64(uint64(width))
		// Merge bits from value at the correct position
		val = val.Or(frame.Load(reg))
	}
	//
	return StoreAcross(frame, target, val)
}
