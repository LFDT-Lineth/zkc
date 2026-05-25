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

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Word --- see documentation on vm.WordMachine
type Word[W word.Word[W]] = Base[W, instruction.Word, WordExecutor[W]]

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
func (p WordExecutor[W]) Execute(insn instruction.Word, frame []W, regs []register.Register) (err error) {
	//nolint
	switch insn.OpCode() {
	// ==============================================================
	// Arithmetic Instructions
	// ==============================================================
	case opcode.INT_ADD:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeAdd(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.INT_DIV:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeDiv(insn.Target, insn.Sources, frame, regs)
		// Fall thru
	case opcode.INT_MUL:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeMul(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.INT_REM:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeRem(insn.Target, insn.Sources, frame, regs)
		// Fall thru
	case opcode.INT_SUB:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeSub(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.INT_ADDMOD_P:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeFieldAdd(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru
	case opcode.INT_SUBMOD_P:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeFieldSub(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru
	case opcode.INT_MULMOD_P:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeFieldMul(insn.Target, insn.Sources, insn.Constant, p.modulus, frame)
		// Fall thru

	// ==============================================================
	// Bitwise Instructions
	// ==============================================================
	case opcode.BIT_AND:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeAnd(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.BIT_NOT:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeNot(insn.Target, insn.Sources, frame, regs)
		// Fall thru
	case opcode.BIT_OR:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeOr(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.BIT_XOR:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeXor(insn.Target, insn.Sources, insn.Constant, frame, regs)
		// Fall thru
	case opcode.BIT_SHL:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeShl(insn.Target, insn.Sources, frame, regs)
		// Fall thru
	case opcode.BIT_SHR:
		insn := insn.(*instruction.WordTypeB[W])
		err = executeShr(insn.Target, insn.Sources, frame, regs)
		// Fall thru
	case opcode.BIT_CONCAT:
		insn := insn.(*instruction.WordTypeA[W])
		err = executeConcat(insn.Target, insn.Sources, frame, regs)
		// Fall thru

	// ==============================================================
	// Field Instructions (executable in word machine)
	// ==============================================================
	case opcode.HINT_DIVISION:
		insn := insn.(*instruction.FieldHint)
		err = executeDivHint(insn.Targets, insn.Sources, frame, regs)
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

func executeAdd[W word.Word[W]](target register.Vector, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		val      = constant
		bitwidth = register.WidthOfRegisters(regs, target.Registers())
		overflow bool
	)
	//
	for _, arg := range sources {
		val, overflow = val.Add(bitwidth, frame[arg.Unwrap()])
		//
		if overflow {
			return errors.New("executeAdd arithmetic overflow")
		}
	}
	//
	storeAcross(target, val, frame, regs)
	//
	return nil
}

func executeMul[W word.Word[W]](target register.Vector, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		zero     W
		val      W = constant
		bitwidth   = register.WidthOfRegisters(regs, target.Registers())
		overflow bool
	)
	//
outer:
	for i, arg := range sources {
		val, overflow = val.Mul(bitwidth, frame[arg.Unwrap()])
		//
		if overflow {
			// sanity check for zero
			for _, jth := range sources[i:] {
				if frame[jth.Unwrap()].Cmp(zero) == 0 {
					val = zero
					break outer
				}
			}
			// overflow is real
			return errors.New("executeMul arithmetic overflow")
		}
	}
	//
	storeAcross(target, val, frame, regs)
	//
	return nil
}

func executeSub[W word.Word[W]](target register.Vector, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		val       W
		bitwidth  = register.WidthOfRegisters(regs, target.Registers())
		underflow bool
	)
	//
	for i, arg := range sources {
		if i == 0 {
			val = frame[arg.Unwrap()]
		} else {
			if val, underflow = val.Sub(bitwidth, frame[arg.Unwrap()]); underflow {
				return errors.New("arithmetic underflow")
			}
		}
	}
	// Subtract constant
	if val, underflow = val.Sub(bitwidth, constant); underflow {
		return errors.New("arithmetic underflow")
	}
	//
	storeAcross(target, val, frame, regs)
	//
	return nil
}

// executeFieldAdd computes the field sum of the source registers and the
// given constant, storing the result in the target register.  Reduction is
// performed implicitly within the field's bandwidth — the underlying word
// type is responsible for wrapping at the field's prime characteristic.
func executeFieldAdd[W word.Word[W]](target register.Id, sources []register.Id, constant, modulus W, frame []W) error {
	//
	for _, arg := range sources {
		constant = constant.AddMod(frame[arg.Unwrap()], modulus)
	}
	//
	frame[target.Unwrap()] = constant
	//
	return nil
}

// executeFieldSub computes the chained field difference of the source
// registers minus the given constant, storing the result in the target
// register.
func executeFieldSub[W word.Word[W]](target register.Id, sources []register.Id, constant, modulus W, frame []W) error {
	//
	var val W
	//
	for i, arg := range sources {
		if i == 0 {
			val = frame[arg.Unwrap()]
		} else {
			val = val.SubMod(frame[arg.Unwrap()], modulus)
		}
	}
	//
	frame[target.Unwrap()] = val.SubMod(constant, modulus)
	//
	return nil
}

// executeFieldMul computes the field product of the source registers and
// the given constant, storing the result in the target register.
func executeFieldMul[W word.Word[W]](target register.Id, sources []register.Id, constant, modulus W, frame []W) error {
	//
	var (
		val W = constant
	)
	//
	for _, arg := range sources {
		val = val.MulMod(frame[arg.Unwrap()], modulus)
	}
	//
	frame[target.Unwrap()] = val
	//
	return nil
}

func executeDiv[W word.Word[W]](target register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		bitwidth = regs[target.Unwrap()].Width()
		dividend = frame[sources[0].Unwrap()]
		divisor  = frame[sources[1].Unwrap()]
	)
	//
	if divisor.BigInt().Sign() == 0 {
		return errors.New("division by zero")
	}
	//
	frame[target.Unwrap()] = dividend.Div(bitwidth, divisor)
	//
	return nil
}

func executeRem[W word.Word[W]](target register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		bitwidth = regs[target.Unwrap()].Width()
		dividend = frame[sources[0].Unwrap()]
		divisor  = frame[sources[1].Unwrap()]
	)
	//
	if divisor.BigInt().Sign() == 0 {
		return errors.New("division by zero")
	}
	//
	frame[target.Unwrap()] = dividend.Rem(bitwidth, divisor)
	//
	return nil
}

// ==============================================================
// Bitwise Instructions
// ==============================================================

func executeAnd[W word.Word[W]](target register.Id, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		val      W = constant
		bitwidth   = regs[target.Unwrap()].Width()
	)
	//
	for _, arg := range sources {
		val = val.And(bitwidth, frame[arg.Unwrap()])
	}
	//
	frame[target.Unwrap()] = val
	//
	return nil
}
func executeOr[W word.Word[W]](target register.Id, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		val      W = constant
		bitwidth   = regs[target.Unwrap()].Width()
	)
	//
	for _, arg := range sources {
		val = val.Or(bitwidth, frame[arg.Unwrap()])
	}
	//
	frame[target.Unwrap()] = val
	//
	return nil
}

func executeXor[W word.Word[W]](target register.Id, sources []register.Id, constant W, frame []W,
	regs []register.Register) error {
	//
	var (
		val      W = constant
		bitwidth   = regs[target.Unwrap()].Width()
	)
	//
	for _, arg := range sources {
		val = val.Xor(bitwidth, frame[arg.Unwrap()])
	}
	//
	frame[target.Unwrap()] = val
	//
	return nil
}

func executeNot[W word.Word[W]](target register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		bitwidth = regs[target.Unwrap()].Width()
		arg      = frame[sources[0].Unwrap()]
	)
	//
	frame[target.Unwrap()] = arg.Not(bitwidth)
	//
	return nil
}

// ==============================================================
// Shift Instructions
// ==============================================================

func executeShl[W word.Word[W]](target register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		bitwidth = regs[target.Unwrap()].Width()
		lhs      = frame[sources[0].Unwrap()]
		rhs      = frame[sources[1].Unwrap()]
	)
	//
	frame[target.Unwrap()] = lhs.Shl(bitwidth, rhs)
	//
	return nil
}

func executeShr[W word.Word[W]](target register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		bitwidth = regs[target.Unwrap()].Width()
		lhs      = frame[sources[0].Unwrap()]
		rhs      = frame[sources[1].Unwrap()]
	)
	//
	frame[target.Unwrap()] = lhs.Shr(bitwidth, rhs)
	//
	return nil
}

// ==============================================================
// Hint Instructions (executable in word machine)
// ==============================================================

// executeDivHint computes quotient and remainder for a division hint.
// targets[0] = sources[0] / sources[1], targets[1] = sources[0] % sources[1].
func executeDivHint[W word.Word[W]](targets []register.Id, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		qWidth   = regs[targets[0].Unwrap()].Width()
		rWidth   = regs[targets[1].Unwrap()].Width()
		dividend = frame[sources[0].Unwrap()]
		divisor  = frame[sources[1].Unwrap()]
		one      W
	)
	//
	one = one.SetUint64(1)
	//
	if divisor.BigInt().Sign() == 0 {
		return errors.New("division by zero")
	}
	//
	q := dividend.Div(qWidth, divisor)
	r := dividend.Rem(rWidth, divisor)
	w, _ := divisor.Sub(rWidth, r)
	w, _ = w.Sub(rWidth, one)
	//
	frame[targets[0].Unwrap()] = q
	frame[targets[1].Unwrap()] = r
	frame[targets[2].Unwrap()] = w
	//
	return nil
}

// ==============================================================
// Misc Instructions
// ==============================================================

func executeConcat[W word.Word[W]](target register.Vector, sources []register.Id, frame []W,
	regs []register.Register) error {
	//
	var (
		val       W
		offset    uint64
		registers = target.Registers()
		width     = register.WidthOfRegisters(regs, registers)
	)
	//
	for _, reg := range sources {
		// determine register width
		var (
			reg_width = regs[reg.Unwrap()].Width()
			reg_val   = frame[reg.Unwrap()]
		)
		// Merge bits from value at the correct position
		val = val.Or(width, reg_val.Shl64(width, offset))
		// Update width accumulate
		offset += uint64(reg_width)
	}
	//
	storeAcross(target, val, frame, regs)
	//
	return nil
}

func storeAcross[W word.Word[W]](targets register.Vector, val W, frame []W, regs []register.Register) {
	var tRegIds = targets.Registers()
	//
	if targets.Len() == 1 {
		var id = tRegIds[0].Unwrap()
		//
		frame[id] = val
	} else {
		for _, rid := range targets.Registers() {
			var (
				id    = rid.Unwrap()
				width = regs[id].Width()
			)
			//
			frame[id] = val.Slice(width)
			//
			val = val.Shr64(uint64(width))
		}
	}
}
