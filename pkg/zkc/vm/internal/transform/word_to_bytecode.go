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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
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
		p.compileCall(insn.(*instruction.Call), f)
	case opcode.DEBUG:
		p.encoder.Add(bytecode.NewDebug())
	case opcode.FAIL:
		p.encoder.Add(bytecode.NewFail())
	case opcode.JUMP:
		p.compileJump(pos, insn.(*instruction.Jump))
	case opcode.MEMORY_READ:
		p.compileMemRead(insn.(*instruction.MemRead), f)
	case opcode.MEMORY_WRITE:
		p.compileMemWrite(insn.(*instruction.MemWrite), f)
	case opcode.RETURN:
		p.encoder.Add(bytecode.NewRet(f.Width(), f.NumInputs()))
	case opcode.SKIP:
		p.compileSkip(pos, insn.(*instruction.Skip))
	case opcode.SKIP_IF:
		p.compileSkipIf(pos, insn.(*instruction.SkipIf))
	case opcode.HINT_DIVISION:
		p.compileDivHint(insn.(*instruction.FieldHint))
	case opcode.INT_ADD:
		p.compileAdd(insn.(*instruction.WordTypeA[W]), f)
	case opcode.INT_SUB:
		p.compileSub(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_MUL:
		p.compileMul(insn.(*instruction.WordTypeA[W]), f)
	case opcode.BIT_CONCAT:
		p.compileConcat(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_DIV:
		p.compileDivRem(insn.(*instruction.WordTypeB), bytecode.DIV, f)
	case opcode.INT_REM:
		p.compileDivRem(insn.(*instruction.WordTypeB), bytecode.REM, f)
	case opcode.BIT_AND:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.AND, f)
	case opcode.BIT_NOT:
		p.compileNot(insn.(*instruction.WordTypeB))
	case opcode.BIT_OR:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OR, f)
	case opcode.BIT_XOR:
		p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.XOR, f)
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

func (p *bytecodeCompiler[W]) compileCall(insn *instruction.Call, f *WordFunction) {
	var (
		callee     = p.machine.Module(insn.Id).(*WordFunction)
		frameWidth = callee.Width()
		// index identifies first instruction of given function.
		index = p.encoder.Label(Label{insn.Id, 0, 0})
	)
	// sanity chewcks
	checkUintIs[uint16](frameWidth)
	checkOperands(insn.Arguments...)
	checkOperands(insn.Returns...)
	// Check whether cast checks are required for arguments wider than their
	// receiving parameter registers, matching the slow machine (frameCopyTo).
	p.addOutgoingCheckCasts(insn.Arguments, callee.Inputs(), f)
	//
	p.encoder.Add(bytecode.CallFun(index, uint16(frameWidth), insn.Arguments, insn.Returns))
	// Check whether cast checks are required for returns wider than their
	// receiving target registers, matching the slow machine (frameCopyFrom).
	p.addIncomingCheckCasts(callee.Outputs(), insn.Returns, f)
}

func (p *bytecodeCompiler[W]) compileNot(insn *instruction.WordTypeB) {
	// NOT uses only the left source; WordTypeB duplicates it as the right source.
	p.encoder.Add(bytecode.NewNot(insn.Target, insn.LeftSource, insn.Bitwidth))
}

func (p *bytecodeCompiler[W]) compileBitwise(insn *instruction.WordTypeB, op uint32, f *WordFunction) {
	var bitwidth uint = base.RegisterBitwidth(f.RegisterMap(), insn.Target)
	// op selects the bytecode operation (bytecode.AND / OR / XOR).
	p.encoder.Add(bytecode.NewBitwise(op, insn.Target, insn.LeftSource, insn.RightSource))
	// Check whether cast check is required (or not).
	if bitwidth < insn.Bitwidth {
		// yes
		p.encoder.Add(bytecode.NewCheckCast(insn.Target, bitwidth))
	}
}

func (p *bytecodeCompiler[W]) compileDivHint(insn *instruction.FieldHint) {
	// The only hint form currently generated is the division hint produced by
	// LowerDivisions, which assigns quotient, remainder and range witness from
	// a dividend and divisor.
	if len(insn.Targets) != 3 || len(insn.Sources) != 2 {
		panic("unsupported hint form")
	}
	//
	p.encoder.Add(bytecode.NewDivHint(
		insn.Targets[0], insn.Targets[1], insn.Targets[2], insn.Sources[0], insn.Sources[1]))
}

func (p *bytecodeCompiler[W]) compileDivRem(insn *instruction.WordTypeB, op uint32, f *WordFunction) {
	var bitwidth uint = base.RegisterBitwidth(f.RegisterMap(), insn.Target)
	// LeftSource is the dividend; RightSource is the divisor.  op selects the
	// bytecode operation (bytecode.DIV / REM).
	p.encoder.Add(bytecode.NewDivRem(op, insn.Target, insn.LeftSource, insn.RightSource))
	// Check whether cast check is required (or not).
	if bitwidth < insn.Bitwidth {
		// yes
		p.encoder.Add(bytecode.NewCheckCast(insn.Target, bitwidth))
	}
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

func (p *bytecodeCompiler[W]) compileMemRead(insn *instruction.MemRead, f *WordFunction) {
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
	case *memory.PagedRandomAccess[W]:
		code = bytecode.ReadPagedRam(mid, insn.Arguments, insn.Returns)
	default:
		panic("unknown memory type")
	}
	//
	p.encoder.Add(code)
	// Check whether cast checks are required (or not).  Values read from a
	// memory whose data registers are wider than the receiving registers must
	// be checked, matching the slow machine which validates every register
	// write (frame.Store).
	p.addIncomingCheckCasts(mem.Geometry().DataRegisters(), insn.Returns, f)
}

func (p *bytecodeCompiler[W]) compileMemWrite(insn *instruction.MemWrite, f *WordFunction) {
	var (
		mem  = p.machine.Module(insn.Id).(memory.Memory[W])
		code bytecode.Bytecode[W]
		mid  = uint16(insn.Id)
	)
	//
	checkModuleId(insn.Id)
	// Check whether cast checks are required (or not).  Values written from
	// registers wider than the memory's data registers must be checked before
	// the write, matching the slow machine (executeMemWrite).
	p.addOutgoingCheckCasts(insn.Returns, mem.Geometry().DataRegisters(), f)
	//
	switch mem.(type) {
	case *memory.WriteOnce[W]:
		code = bytecode.WriteWom(mid, insn.Arguments, insn.Returns)
	case *memory.RandomAccess[W]:
		code = bytecode.WriteRam(mid, insn.Arguments, insn.Returns)
	case *memory.PagedRandomAccess[W]:
		code = bytecode.WritePagedRam(mid, insn.Arguments, insn.Returns)
	default:
		panic("unknown memory type")
	}
	//
	p.encoder.Add(code)
}

// addIncomingCheckCasts emits a CHECKCAST for every target register which is
// narrower than the corresponding source register, where sources are values
// arriving in this frame from another module (e.g. a memory's data registers,
// or a callee's return registers).  This mirrors the width check the slow
// machine performs on every register write (frame.Store / frameCopyFrom).
func (p *bytecodeCompiler[W]) addIncomingCheckCasts(sources []register.Register, targets []register.Id,
	f *WordFunction) {
	//
	for i, target := range targets {
		var (
			src = sources[i]
			dst = f.Register(target)
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Width() > dst.Width()) {
			p.encoder.Add(bytecode.NewCheckCast(target, dst.Width()))
		}
	}
}

// addOutgoingCheckCasts emits a CHECKCAST for every source register in this
// frame which is wider than the register receiving its value in another module
// (e.g. a memory's data registers, or a callee's parameter registers).  This
// mirrors the width check the slow machine performs on memory writes
// (executeMemWrite) and call arguments (frameCopyTo).
func (p *bytecodeCompiler[W]) addOutgoingCheckCasts(sources []register.Id, targets []register.Register,
	f *WordFunction) {
	//
	for i, source := range sources {
		var (
			src = f.Register(source)
			dst = targets[i]
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Width() > dst.Width()) {
			p.encoder.Add(bytecode.NewCheckCast(source, dst.Width()))
		}
	}
}

func (p *bytecodeCompiler[W]) compileSkip(pos Label, insn *instruction.Skip) {
	if insn.Skip != 0 {
		var (
			label = Label{pos.fun, pos.macro, pos.micro + insn.Skip + 1}
			index = p.encoder.Label(label)
		)
		p.encoder.Add(bytecode.Jump(index))
	}
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

func checkUintIs[T uint8 | uint16](value uint) {
	var bound = uint(T(0) - 1)
	//
	if value >= bound {
		panic("wide instructions not supported")
	}
}

func checkOperands(regs ...register.Id) {
	if len(regs) > math.MaxUint8 {
		panic("wide instructions not supported")
	}
	//
	for _, reg := range regs {
		if reg.Unwrap() > math.MaxUint8 {
			panic("wide instructions not supported")
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
