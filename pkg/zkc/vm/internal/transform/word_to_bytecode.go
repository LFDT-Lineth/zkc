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
	"math/big"

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
func WordToBytecodeProgram[W word.Word[W]](wm *machine.Word[W]) bytecode.Program[W] {
	var (
		encoder  = bytecode.NewEncoder[W, Label](wm.Modules()...)
		compiler = &bytecodeCompiler[W]{wm, encoder}
	)
	// translate functions
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
	encoder *bytecode.Encoder[W, Label]
}

func (p *bytecodeCompiler[W]) compileWordFunction(fid uint, f *WordFunction) {
	// mark entry point of this function
	p.encoder.MarkLabel(Label{fid, 0, 0})
	p.encoder.MarkModule(fid)
	//
	for i, vec := range f.Code() {
		for j, insn := range vec.Codes {
			var label = Label{fid, uint(i), uint(j)}
			// Mark instruction position in case it is the target of a skip or
			// jump instruction.
			p.encoder.MarkLabel(label)
			// Compile instruction into sequence of bytecodes as required.
			p.compileWordInstruction(label, insn, f)
		}
	}
}

func (p *bytecodeCompiler[W]) compileWordInstruction(pos Label, insn WordInstruction, f *WordFunction) {
	switch insn.OpCode() {
	// Base instructions are word-type-agnostic and translate verbatim.
	case opcode.CALL:
		p.compileCall(insn.(*instruction.Call))
	case opcode.DEBUG:
		p.encoder.Add(bytecode.NewDebug())
	case opcode.FAIL:
		p.encoder.Add(bytecode.NewFail())
	case opcode.JUMP:
		p.compileJump(pos, insn.(*instruction.Jump))
	case opcode.MEMORY_READ:
		p.compileMemRead(insn.(*instruction.MemRead))
	case opcode.MEMORY_WRITE:
		p.compileMemWrite(insn.(*instruction.MemWrite))
	case opcode.RETURN:
		p.encoder.Add(bytecode.NewRet(f.Width()))
	case opcode.SKIP:
		p.compileSkip(pos, insn.(*instruction.Skip))
	case opcode.SKIP_IF:
		p.compileSkipIf(pos, insn.(*instruction.SkipIf))
	case opcode.HINT_DIVISION:
		panic("todo")
	case opcode.INT_ADD:
		p.compileAdd(insn.(*instruction.WordTypeA[W]), f)
	case opcode.INT_SUB:
		p.compileSub(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_MUL:
		p.compileMul(insn.(*instruction.WordTypeA[W]), f)
	case opcode.BIT_CONCAT:
		p.compileConcat(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_DIV:
		panic("todo")
	case opcode.INT_REM:
		panic("todo")
	case opcode.BIT_AND:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.AND)
	case opcode.BIT_NOT:
		p.compileNot(insn.(*instruction.WordTypeB))
	case opcode.BIT_OR:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OR)
	case opcode.BIT_XOR:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.XOR)
	case opcode.BIT_SHL:
		p.compileShift(insn.(*instruction.WordTypeB), bytecode.SHL)
	case opcode.BIT_SHR:
		p.compileShift(insn.(*instruction.WordTypeB), bytecode.SHR)
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

func (p *bytecodeCompiler[W]) compileAdd(insn *instruction.WordTypeA[W], f *WordFunction) {
	var rhsMaxVal big.Int
	// Initialise max value
	rhsMaxVal.Set(insn.Constant.BigInt())
	// Determine maximum expressible value
	for _, reg := range insn.Sources {
		var bitwidth = f.Register(reg).Width()
		// Determine width of source
		rhsMaxVal.Add(&rhsMaxVal, maxValueOf(bitwidth))
	}
	//
	p.encoder.Add(bytecode.AddVecConst(insn.Target.Registers(), insn.Sources, insn.Constant))
	// Check whether cast check is required (or not).
	p.addCheckCast(insn.Target, &rhsMaxVal, f)
}

func (p *bytecodeCompiler[W]) compileMul(insn *instruction.WordTypeA[W], f *WordFunction) {
	var rhsMaxVal big.Int
	// Initialise max value
	rhsMaxVal.Set(insn.Constant.BigInt())
	// Determine maximum expressible value
	for _, reg := range insn.Sources {
		var bitwidth = f.Register(reg).Width()
		// Determine width of source
		rhsMaxVal.Mul(&rhsMaxVal, maxValueOf(bitwidth))
	}
	//
	p.encoder.Add(bytecode.MulVecConst(insn.Target.Registers(), insn.Sources, insn.Constant))
	// Check whether cast check is required (or not).
	p.addCheckCast(insn.Target, &rhsMaxVal, f)
}
func (p *bytecodeCompiler[W]) compileSub(insn *instruction.WordTypeA[W]) {
	// NOTE: should we worry about overflow here?
	p.encoder.Add(bytecode.SubVecConst(insn.Target.Registers(), insn.Sources, insn.Constant))
}

func (p *bytecodeCompiler[W]) compileConcat(insn *instruction.WordTypeA[W]) {
	if insn.Constant.Cmp64(0) != 0 {
		panic("constant given for bit concatenation")
	}
	//
	// CAT keeps source and target vectors in low-limb-first register order.
	p.encoder.Add(bytecode.Concat(insn.Target.Registers(), insn.Sources))
}

func (p *bytecodeCompiler[W]) compileCall(insn *instruction.Call) {
	checkCallModuleId(insn.Id)
	checkCallOperands(insn.Arguments)
	checkCallOperands(insn.Returns)
	//
	// CALL operands stay in caller register numbering.
	p.encoder.Add(bytecode.NewCall(uint16(insn.Id), insn.Arguments, insn.Returns))
}

func (p *bytecodeCompiler[W]) compileNot(insn *instruction.WordTypeB) {
	// NOT uses only the left source; WordTypeB duplicates it as the right source.
	p.encoder.Add(bytecode.NewNot(insn.Target, insn.LeftSource, insn.Bitwidth))
}

func (p *bytecodeCompiler[W]) compileBitwise(insn *instruction.WordTypeB, op uint32) {
	// op selects the bytecode operation (bytecode.AND / OR / XOR).
	p.encoder.Add(bytecode.NewBitwise(op, insn.Target, insn.LeftSource, insn.RightSource))
}

func (p *bytecodeCompiler[W]) compileShift(insn *instruction.WordTypeB, op uint32) {
	// LeftSource is the value shifted; RightSource holds the shift amount.  The
	// bitwidth masks the result of a left shift and is ignored by a right shift.
	p.encoder.Add(bytecode.NewShift(op, insn.Target, insn.LeftSource, insn.RightSource, insn.Bitwidth))
}

func (p *bytecodeCompiler[W]) compileJump(pos Label, insn *instruction.Jump) {
	var (
		index = p.encoder.Label(Label{pos.fun, insn.Immediate, 0})
	)
	p.encoder.Add(bytecode.Jump(index))
}

func (p *bytecodeCompiler[W]) compileMemRead(insn *instruction.MemRead) {
	var (
		mem  = p.machine.Module(insn.Id).(memory.Memory[W])
		mid  = uint16(insn.Id)
		code bytecode.Bytecode[W]
	)
	//
	checkModuleId(insn.Id)
	//
	switch mem.(type) {
	case *memory.ReadOnly[W]:
		code = bytecode.ReadRom(mid, insn.Arguments, insn.Returns)
	case *memory.StaticReadOnly[W]:
		code = bytecode.ReadStaticRom(mid, insn.Arguments, insn.Returns)
	case *memory.RandomAccess[W]:
		code = bytecode.ReadRam(mid, insn.Arguments, insn.Returns)
	case *memory.BiPartiteRandomAccess[W]:
		code = bytecode.ReadBigRam(mid, insn.Arguments, insn.Returns)
	default:
		panic("unknown memory type")
	}
	//
	p.encoder.Add(code)
}

func (p *bytecodeCompiler[W]) compileMemWrite(insn *instruction.MemWrite) {
	var (
		mem  = p.machine.Module(insn.Id).(memory.Memory[W])
		code bytecode.Bytecode[W]
		mid  = uint16(insn.Id)
	)
	//
	checkModuleId(insn.Id)
	//
	switch mem.(type) {
	case *memory.WriteOnce[W]:
		code = bytecode.WriteWom(mid, insn.Arguments, insn.Returns)
	case *memory.RandomAccess[W]:
		code = bytecode.WriteRam(mid, insn.Arguments, insn.Returns)
	case *memory.BiPartiteRandomAccess[W]:
		code = bytecode.WriteBigRam(mid, insn.Arguments, insn.Returns)
	default:
		panic("unknown memory type")
	}
	//
	p.encoder.Add(code)
}

func (p *bytecodeCompiler[W]) compileSkip(pos Label, insn *instruction.Skip) {
	var (
		label = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
		index = p.encoder.Label(label)
	)
	p.encoder.Add(bytecode.Jump(index))
}

func (p *bytecodeCompiler[W]) compileSkipIf(pos Label, insn *instruction.SkipIf) {
	var (
		label = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
		index = p.encoder.Label(label)
		l     = insn.Left
		r     = insn.Right
	)
	//
	p.encoder.Add(bytecode.JumpIfVec(insn.Cond, index, l, r))
}

// Add a checkcast instruction if the given value does not fit within the target
// register(s).
func (p *bytecodeCompiler[W]) addCheckCast(target register.Vector, value *big.Int, f *WordFunction) {
	var (
		targetMaxVal = maxValueOf(target.BitWidth(f.RegisterMap()))
	)
	//
	if targetMaxVal.Cmp(value) < 0 {
		var (
			last      = target.Last()
			lastWidth = f.Register(last).Width()
		)
		// yes
		p.encoder.Add(bytecode.NewCheckCast(last, lastWidth))
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

func checkModuleId(mid uint) {
	if mid > math.MaxUint16 {
		panic("invalid module identifier (too many modules)")
	}
}

func checkCallModuleId(mid uint) {
	if mid > math.MaxUint8 {
		panic("wide call instructions not supported")
	}
}

func checkCallOperands(regs []register.Id) {
	if len(regs) > math.MaxUint8 {
		panic("wide call instructions not supported")
	}
	//
	for _, reg := range regs {
		if reg.Unwrap() > math.MaxUint8 {
			panic("wide call instructions not supported")
		}
	}
}

// MaxValueOf calculates the maximum value that a register of a given bitwidth
// can hold.
func maxValueOf(bitwidth uint) *big.Int {
	var (
		val = big.NewInt(1)
	)
	//
	val.Lsh(val, bitwidth)
	//
	val.Sub(val, big.NewInt(1))
	//
	return val
}
