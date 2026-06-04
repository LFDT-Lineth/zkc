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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// WordToBytecodeMachine compiles a word machine into a bytecode sequence which
// can be executed by an interpreter.
func WordToBytecodeMachine[W word.Word[W]](wm *machine.Word[W]) *bytecode.Interpreter[W] {
	var (
		program = WordToBytecodeProgram(wm)
	)
	//
	return bytecode.NewInterpreter[W](program)
}

// WordToBytecodeProgram compiles the various components of a word machine into
// a bytecode program.
func WordToBytecodeProgram[W word.Word[W]](wm *machine.Word[W]) bytecode.Program {
	var (
		encoder bytecode.Encoder[Label]
	)
	//
	for i, m := range wm.Modules() {
		if f, ok := m.(*WordFunction); ok {
			//
			compileWordFunction[W](&encoder, uint(i), f)
		}
	}
	//
	return encoder.Encode()
}

func compileWordFunction[W word.Word[W]](encoder *bytecode.Encoder[Label], fid uint, f *WordFunction) {
	// mark entry point of this function
	encoder.MarkLabel(Label{fid, 0, 0})
	encoder.MarkSymbol(f.Name())
	//
	for i, vec := range f.Code() {
		for j, insn := range vec.Codes {
			var label = Label{fid, uint(i), uint(j)}
			// Mark instruction position in case it is the target of a skip or
			// jump instruction.
			encoder.MarkLabel(label)
			// Compile instruction into sequence of bytecodes as required.
			compileWordInstruction[W](encoder, label, insn)
		}
	}
}

func compileWordInstruction[W word.Word[W]](encoder *bytecode.Encoder[Label], pos Label, insn WordInstruction) {
	switch insn.OpCode() {
	// Base instructions are word-type-agnostic and translate verbatim.
	case opcode.CALL:
		panic("todo")
	case opcode.DEBUG:
		panic("todo")
	case opcode.FAIL:
		encoder.Fail()
	case opcode.JUMP:
		compileJump(encoder, pos, insn.(*instruction.Jump))
	case opcode.MEMORY_READ:
		panic("todo")
	case opcode.MEMORY_WRITE:
		panic("todo")
	case opcode.RETURN:
		encoder.Ret(0, 0)
	case opcode.SKIP:
		compileSkip(encoder, pos, insn.(*instruction.Skip))
	case opcode.SKIP_IF:
		compileSkipIf(encoder, pos, insn.(*instruction.SkipIf))
	case opcode.HINT_DIVISION:
		panic("todo")
	case opcode.INT_ADD:
		compileAdd(encoder, insn.(*instruction.WordTypeA[W]))
	case opcode.INT_SUB:
		panic("todo")
	case opcode.INT_MUL:
		panic("todo")
	case opcode.BIT_CONCAT:
		panic("todo")
	case opcode.INT_DIV:
		panic("todo")
	case opcode.INT_REM:
		panic("todo")
	case opcode.BIT_AND:
		panic("todo")
	case opcode.BIT_NOT:
		panic("todo")
	case opcode.BIT_OR:
		panic("todo")
	case opcode.BIT_XOR:
		panic("todo")
	case opcode.BIT_SHL:
		panic("todo")
	case opcode.BIT_SHR:
		panic("todo")
	case opcode.INT_ADDMOD_P:
		panic("todo")
	case opcode.INT_SUBMOD_P:
		panic("todo")
	case opcode.INT_MULMOD_P:
		panic("todo")
	default:
		panic(fmt.Sprintf("unknown instruction opcode (0x%x)", insn.OpCode()))
	}
}

func compileAdd[W word.Word[W]](encoder *bytecode.Encoder[Label], insn *instruction.WordTypeA[W]) {
	//nolint
	if insn.Target.Len() != 1 || insn.Constant.Cmp64(0) != 0 {
		panic("todo")
	} else if len(insn.Sources) == 2 {
		encoder.Add(insn.Sources[0], insn.Sources[1], insn.Target.AsRegister())
	} else if len(insn.Sources) == 1 {
		encoder.Move(insn.Sources[0], insn.Target.AsRegister())
	} else {
		panic("todo")
	}
}

func compileJump(encoder *bytecode.Encoder[Label], pos Label, insn *instruction.Jump) {
	encoder.Jmp(Label{pos.fun, insn.Immediate, 0})
}

func compileSkip(encoder *bytecode.Encoder[Label], pos Label, insn *instruction.Skip) {
	var (
		target = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
	)
	encoder.Jmp(target)
}

func compileSkipIf(encoder *bytecode.Encoder[Label], pos Label, insn *instruction.SkipIf) {
	var (
		target = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
	)
	// TODO: sort out vectored skip if
	encoder.JmpIf(target, insn.Cond, insn.Left.AsRegister(), insn.Right.AsRegister())
}

// Label uniquely identifies an instruction within a given module.
type Label struct {
	// Function identifier
	fun uint
	// Vector instruction
	macro uint
	// Vector position
	micro uint
}
