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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
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
		compiler = &bytecodeCompiler[W]{machine: wm}
	)
	//
	for i, m := range wm.Modules() {
		if f, ok := m.(*WordFunction); ok {
			//
			compiler.compileWordFunction(uint(i), f)
		}
	}
	//
	return compiler.encoder.Encode()
}

type bytecodeCompiler[W word.Word[W]] struct {
	machine *machine.Word[W]
	encoder bytecode.Encoder[Label]
	memmap  []uint16
}

func (p *bytecodeCompiler[W]) compileWordFunction(fid uint, f *WordFunction) {
	// mark entry point of this function
	p.encoder.MarkLabel(Label{fid, 0, 0})
	p.encoder.MarkSymbol(f.Name())
	//
	for i, vec := range f.Code() {
		for j, insn := range vec.Codes {
			var label = Label{fid, uint(i), uint(j)}
			// Mark instruction position in case it is the target of a skip or
			// jump instruction.
			p.encoder.MarkLabel(label)
			// Compile instruction into sequence of bytecodes as required.
			p.compileWordInstruction(label, insn)
		}
	}
}

func (p *bytecodeCompiler[W]) compileWordInstruction(pos Label, insn WordInstruction) {
	switch insn.OpCode() {
	// Base instructions are word-type-agnostic and translate verbatim.
	case opcode.CALL:
		panic("todo")
	case opcode.DEBUG:
		panic("todo")
	case opcode.FAIL:
		p.encoder.Fail()
	case opcode.JUMP:
		p.compileJump(pos, insn.(*instruction.Jump))
	case opcode.MEMORY_READ:
		p.compileMemRead(insn.(*instruction.MemRead))
	case opcode.MEMORY_WRITE:
		panic("todo")
	case opcode.RETURN:
		p.encoder.Ret(0, 0)
	case opcode.SKIP:
		p.compileSkip(pos, insn.(*instruction.Skip))
	case opcode.SKIP_IF:
		p.compileSkipIf(pos, insn.(*instruction.SkipIf))
	case opcode.HINT_DIVISION:
		panic("todo")
	case opcode.INT_ADD:
		p.compileAdd(insn.(*instruction.WordTypeA[W]))
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

func (p *bytecodeCompiler[W]) compileAdd(insn *instruction.WordTypeA[W]) {
	//nolint
	switch {
	case insn.Target.Len() != 1:
		panic("todo")
	case len(insn.Sources) == 0:
		p.encoder.LoadConst(insn.Constant.BigInt().Bytes(), asReg(insn.Target.AsRegister()))
	case len(insn.Sources) == 1 && insn.Constant.Cmp64(0) == 0:
		var (
			rs = asReg(insn.Sources[0])
			rd = asReg(insn.Target.AsRegister())
		)
		p.encoder.Move(rs, rd)
	default:
		var (
			rs0 = asReg(insn.Sources[0])
			rs1 = asReg(insn.Sources[1])
			rd  = asReg(insn.Target.AsRegister())
		)
		p.encoder.Add(rs0, rs1, rd)
		//
		for i := 2; i < len(insn.Sources); i = i + 1 {
			var rsi = asReg(insn.Sources[i])
			p.encoder.Add(rsi, rd, rd)
		}
		//
		if insn.Constant.Cmp64(0) != 0 {
			p.encoder.AddConst(insn.Constant.BigInt().Bytes(), rd, rd)
		}
	}
}

func (p *bytecodeCompiler[W]) compileJump(pos Label, insn *instruction.Jump) {
	p.encoder.Jmp(Label{pos.fun, insn.Immediate, 0})
}

func (p *bytecodeCompiler[W]) compileMemRead(insn *instruction.MemRead) {
	var (
		args    = insn.Arguments
		returns = insn.Returns
		mem     = p.machine.Module(insn.Id).(memory.Memory[W])
		mid     = p.memmap[insn.Id]
	)
	// sanity checks
	if len(args) != 1 {
		panic("todo: reads with multiple arguments?")
	}
	//
	for i, r := range returns {
		switch {
		case mem.IsReadOnly():
			p.encoder.ReadRom(mid, asReg(args[0]), uint8(i), asReg(r))
		case mem.IsReadWrite():
			p.encoder.ReadRam(mid, asReg(args[0]), uint8(i), asReg(r))
		default:
			panic("todo")
		}
	}
}

func (p *bytecodeCompiler[W]) compileSkip(pos Label, insn *instruction.Skip) {
	var (
		target = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
	)
	p.encoder.Jmp(target)
}

func (p *bytecodeCompiler[W]) compileSkipIf(pos Label, insn *instruction.SkipIf) {
	var (
		target = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
		l      = uint16(insn.Left.AsRegister().Unwrap())
		r      = uint16(insn.Right.AsRegister().Unwrap())
	)
	// TODO: sort out vectored skip if
	p.encoder.JmpIf(target, insn.Cond, l, r)
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

func asReg(rid register.Id) uint16 {
	if rid.Unwrap() >= math.MaxUint16 {
		panic("invalid register")
	}
	//
	return uint16(rid.Unwrap())
}
