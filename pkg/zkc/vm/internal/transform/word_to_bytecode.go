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
		program = CompileWordFunctions(wm)
	)
	//
	return bytecode.NewInterpreter[W](program)
}

// CompileWordFunctions compiles a
func CompileWordFunctions[W word.Word[W]](wm *machine.Word[W]) bytecode.Program {
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
	return bytecode.NewProgram(encoder.Encode())
}

func compileWordFunction[W word.Word[W]](encoder *bytecode.Encoder[Label], fid uint, f *WordFunction) {
	// mark entry point of this function
	encoder.Mark(Label{fid, 0, 0})
	//
	for i, vec := range f.Code() {
		for j, insn := range vec.Codes {
			var label = Label{fid, uint(i), uint(j)}
			// Mark instruction position in case it is the target of a skip or
			// jump instruction.
			encoder.Mark(label)
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
		var i = insn.(*instruction.Jump)
		encoder.Jmp(Label{pos.fun, i.Immediate, 0})
	case opcode.MEMORY_READ:
		panic("todo")
	case opcode.MEMORY_WRITE:
		panic("todo")
	case opcode.RETURN:
		panic("todo")
	case opcode.SKIP:
		var (
			i      = insn.(*instruction.Skip)
			target = Label{pos.fun, pos.macro, pos.micro + i.Skip + 1}
		)
		encoder.Jmp(target)
	case opcode.SKIP_IF:
		var (
			i      = insn.(*instruction.SkipIf)
			target = Label{pos.fun, pos.macro, pos.micro + i.Skip + 1}
		)
		// TODO: sort out vectored skip if
		encoder.JmpIf(target, i.Cond, i.Left.AsRegister(), i.Right.AsRegister())
	case opcode.HINT_DIVISION:
		panic("todo")
	case opcode.INT_ADD:
		panic("todo")
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

// Label uniquely identifies an instruction within a given module.
type Label struct {
	// Function identifier
	fun uint
	// Vector instruction
	macro uint
	// Vector position
	micro uint
}
