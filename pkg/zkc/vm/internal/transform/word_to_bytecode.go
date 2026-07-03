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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode provides a convenient alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// BytecodeVector provides a convenient alias
type BytecodeVector[W word.Word[W]] = bytecode.Vector[W]

// WordToBytecodeMachine compiles a word machine into a bytecode sequence which
// can be executed by an interpreter.
func WordToBytecodeMachine[W word.Word[W]](wm *machine.Word[W]) *interpreter.Interpreter[W] {
	var (
		program = WordToBytecodeProgram(wm)
	)
	//
	return interpreter.New(program, wm.Executor().Modulus())
}

// WordToBytecodeProgram compiles the various components of a word machine into
// a bytecode program.  It first derives a signature table from the machine's
// modules, then lowers each module against those signatures.
func WordToBytecodeProgram[W word.Word[W]](wm *machine.Word[W]) descriptor.Program[W] {
	var (
		mods       = wm.Modules()
		signatures = make([]descriptor.Module[W], len(mods))
		compiler   = &bytecodeCompiler[W]{nil}
	)
	// Derive signatures (body-less for functions; complete for memories).
	for i, m := range mods {
		signatures[i] = compiler.compileWordSignature(m)
	}
	//
	return descriptor.NewProgram(WordToBytecodeModules(signatures, mods...)...)
}

// WordToBytecodeModules lowers a set of word modules into descriptor modules,
// resolving call / memory references against the supplied signature table.  The
// signature table must be index-aligned with the module ids used by Call /
// MemRead / MemWrite instructions; it is what allows references to forward (or
// self) modules to be resolved before all module bodies have been compiled.
// This is the generalized core used both by WordToBytecodeProgram and by the
// per-function lowering invoked from codegen.
func WordToBytecodeModules[W word.Word[W]](signatures []descriptor.Module[W], modules ...Module,
) []descriptor.Module[W] {
	var (
		out      = make([]descriptor.Module[W], len(modules))
		compiler = &bytecodeCompiler[W]{signatures}
	)
	//
	for i, m := range modules {
		out[i] = compiler.compileWordModule(uint(i), m)
	}
	//
	return out
}

type bytecodeCompiler[W word.Word[W]] struct {
	// signatures holds the descriptor signature of every module in the program,
	// indexed by module id.  It is used to resolve the foreign modules referenced
	// by call / memory instructions (see compileCall / compileMemRead /
	// compileMemWrite).
	signatures []descriptor.Module[W]
}

// compileWordSignature builds a descriptor signature for a word module: a
// body-less function (registers + native flag only) or a complete memory.  The
// signature carries everything needed to resolve a reference to this module from
// another module's instructions.
func (p *bytecodeCompiler[W]) compileWordSignature(m Module) descriptor.Module[W] {
	switch t := m.(type) {
	case *WordFunction:
		registers := descriptor.FromRegisters[W](t.Registers()...)
		return descriptor.NewFunction[W](t.Name(), registers, t.IsNative(), nil)
	case memory.Memory[W]:
		return p.compileWordMemory(t)
	default:
		panic("todo")
	}
}

func (p *bytecodeCompiler[W]) compileWordModule(mid uint, m Module) descriptor.Module[W] {
	switch t := m.(type) {
	case *WordFunction:
		return p.compileWordFunction(t)
	case memory.Memory[W]:
		return p.compileWordMemory(t)
	default:
		panic("todo")
	}
}

func (p *bytecodeCompiler[W]) compileWordMemory(m memory.Memory[W]) descriptor.Module[W] {
	//
	var (
		registers = descriptor.FromRegisters[W](m.Registers()...)
		contents  []W
	)
	// Extract contents for static memories only
	if m.Kind().IsStatic() {
		contents = m.Contents()
	}
	//
	return descriptor.NewMemory(m.Name(), registers, m.Kind(), contents)
}

func (p *bytecodeCompiler[W]) compileWordFunction(f *WordFunction) descriptor.Module[W] {
	var vectors []BytecodeVector[W]
	//
	for _, vec := range f.Code() {
		// Compile vector instruction into a bytecode vector as required.
		vectors = append(vectors, p.compileWordVector(vec, f))
	}
	//
	registers := descriptor.FromRegisters[W](f.Registers()...)
	//
	return descriptor.NewFunction[W](f.Name(), registers, f.IsNative(), vectors)
}

func (p *bytecodeCompiler[W]) compileWordVector(vec VectorInstruction, f *WordFunction) BytecodeVector[W] {
	var packets [][]Bytecode[W]
	//
	for _, insn := range vec.Codes {
		// Compile instruction into sequence of bytecodes as required.
		bytecode := p.compileWordInstruction(insn, f)
		//
		packets = append(packets, bytecode)
	}
	//
	bytecodes := flatternPackets(packets)
	//
	return bytecode.NewVector(bytecodes...)
}

func (p *bytecodeCompiler[W]) compileWordInstruction(insn WordInstruction, f *WordFunction) []Bytecode[W] {
	var code Bytecode[W]
	//
	switch insn.OpCode() {
	// Base instructions are word-type-agnostic and translate verbatim.
	case opcode.CALL:
		return p.compileCall(insn.(*instruction.Call), false, f)
	case opcode.UNCONDITIONAL_CALL:
		return p.compileCall(&instruction.Call{OpIo: insn.(*instruction.UnconditionalCall).OpIo}, true, f)
	case opcode.DEBUG:
		code = bytecode.NewDebug(insn.(*instruction.Debug).Chunks)
	case opcode.FAIL:
		code = bytecode.NewFail(insn.(*instruction.Fail).Chunks)
	case opcode.JUMP:
		code = p.compileJump(insn.(*instruction.Jump))
	case opcode.MEMORY_READ:
		return p.compileMemRead(insn.(*instruction.MemRead), f)
	case opcode.MEMORY_WRITE:
		return p.compileMemWrite(insn.(*instruction.MemWrite), f)
	case opcode.RETURN:
		code = bytecode.NewRet()
	case opcode.SKIP:
		return p.compileSkip(insn.(*instruction.Skip))
	case opcode.SKIP_IF:
		code = p.compileSkipIf(insn.(*instruction.SkipIf))
	case opcode.SKIP_MULTI:
		code = p.compileMultiwaySkip(insn.(*instruction.MultiwaySkip))
	case opcode.HINT_DIVISION:
		code = p.compileDivHint(insn.(*instruction.FieldHint))
	case opcode.INT_ADD:
		return p.compileAdd(insn.(*instruction.WordTypeA[W]), f)
	case opcode.INT_SUB:
		return p.compileSub(insn.(*instruction.WordTypeA[W]), f)
	case opcode.INT_MUL:
		return p.compileMul(insn.(*instruction.WordTypeA[W]), f)
	case opcode.BIT_CONCAT:
		code = p.compileConcat(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_DIV:
		return p.compileDivRem(insn.(*instruction.WordTypeB), encoding.DIV, f)
	case opcode.INT_REM:
		return p.compileDivRem(insn.(*instruction.WordTypeB), encoding.REM, f)
	case opcode.BIT_AND:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OP_AND, f)
	case opcode.BIT_NOT:
		code = p.compileNot(insn.(*instruction.WordTypeB))
	case opcode.BIT_OR:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OP_OR, f)
	case opcode.BIT_XOR:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OP_XOR, f)
	case opcode.BIT_SHL:
		code = p.compileShift(insn.(*instruction.WordTypeB), true)
	case opcode.BIT_SHR:
		code = p.compileShift(insn.(*instruction.WordTypeB), false)
	case opcode.INT_ADDMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), bytecode.OP_ADDMOD_P)
	case opcode.INT_SUBMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), bytecode.OP_SUBMOD_P)
	case opcode.INT_MULMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), bytecode.OP_MULMOD_P)
	default:
		panic(fmt.Sprintf("unknown instruction opcode (0x%x)", insn.OpCode()))
	}
	//
	return []Bytecode[W]{code}
}

func (p *bytecodeCompiler[W]) compileAdd(insn *instruction.WordTypeA[W], f *WordFunction) []Bytecode[W] {
	var (
		acc = func(lhs, rhs *big.Int) { lhs.Add(lhs, rhs) }
		// Initialise max value
		max      = CalculateMaximumValue(insn.Constant, insn.Sources, f.RegisterMap(), acc)
		bitwidth = util.MapOption(max, func(x big.Int) uint { return uint(x.BitLen()) })
		//
		code = []Bytecode[W]{bytecode.AddVecConst(toRegs(insn.Target.Registers()), toRegs(insn.Sources), insn.Constant)}
	)
	// Check whether cast check is required (or not).
	return append(code, AddCheckCast[W](f.RegisterMap(), insn.Target, bitwidth)...)
}

func (p *bytecodeCompiler[W]) compileMul(insn *instruction.WordTypeA[W], f *WordFunction) []Bytecode[W] {
	var (
		acc = func(lhs, rhs *big.Int) { lhs.Mul(lhs, rhs) }
		// Initialise max value
		max      = CalculateMaximumValue(insn.Constant, insn.Sources, f.RegisterMap(), acc)
		bitwidth = util.MapOption(max, func(x big.Int) uint { return uint(x.BitLen()) })
		//
		code = []Bytecode[W]{bytecode.MulVecConst(toRegs(insn.Target.Registers()), toRegs(insn.Sources), insn.Constant)}
	)
	// Check whether cast check is required (or not).
	return append(code, AddCheckCast[W](f.RegisterMap(), insn.Target, bitwidth)...)
}

func (p *bytecodeCompiler[W]) compileSub(insn *instruction.WordTypeA[W], f *WordFunction) []Bytecode[W] {
	var (
		acc = func(lhs, rhs *big.Int) { lhs.Add(lhs, rhs) }
		// Initialise max value
		lhsMax = CalculateMaximumValue(insn.Constant, insn.Sources[:1], f.RegisterMap(), acc).Unwrap()
		rhsMax = CalculateMaximumValue(insn.Constant, insn.Sources[1:], f.RegisterMap(), acc).Unwrap()
		//
		code = []Bytecode[W]{bytecode.SubVecConst(toRegs(insn.Target.Registers()), toRegs(insn.Sources), insn.Constant)}
	)
	// Subtract one for rhs to account correctly for negative values.  This is
	// because negative values do not need to encode zero and, hence, can
	// account for one additional value.
	lhsMax.Sub(&lhsMax, big.NewInt(1))
	// Calculate bitwidth, including an additional bit for the sign.
	bitwidth := util.Some(1 + max(uint(lhsMax.BitLen()), uint(rhsMax.BitLen())))

	return append(code, AddCheckCast[W](f.RegisterMap(), insn.Target, bitwidth)...)
}

// compileFieldArith emits a modular field-arithmetic bytecode (ADDMOD_P,
// SUBMOD_P or MULMOD_P).  Unlike the integer arithmetic forms, the result is
// always reduced modulo the prime characteristic and so fits within the (native)
// target register; hence no cast check is ever required, matching the slow
// machine (executeFieldAdd / executeFieldSub / executeFieldMul).
func (p *bytecodeCompiler[W]) compileFieldArith(insn *instruction.WordTypeF[W], op bytecode.Operation) Bytecode[W] {
	return bytecode.NewFieldArith(op, toReg(insn.Target), toRegs(insn.Sources), insn.Constant)
}

func (p *bytecodeCompiler[W]) compileConcat(insn *instruction.WordTypeA[W]) Bytecode[W] {
	if insn.Constant.Cmp64(0) != 0 {
		panic("constant given for bit concatenation")
	}
	// CAT keeps source and target vectors in low-limb-first register order.
	return bytecode.Concat(toRegs(insn.Target.Registers()), toRegs(insn.Sources))
}

func (p *bytecodeCompiler[W]) compileCall(insn *instruction.Call, unconditional bool, f *WordFunction) []Bytecode[W] {
	var (
		callee = p.signatures[insn.Id]
		mid    = util.Cast[bytecode.ModuleId](bytecode.ModuleId(insn.Id))
	)
	// sanity chewcks
	checkOperands(insn.Arguments...)
	checkOperands(insn.Returns...)
	// Check whether cast checks are required for arguments wider than their
	// receiving parameter registers, matching the slow machine (frameCopyTo).
	code := AddOutgoingCheckCasts[W](f.RegisterMap(), insn.Arguments, callee.Inputs())
	// Add the call instruction.  Checkpointing is applied later (see
	// Program.AddCheckPoint), so only the unconditional flag is set here.
	code = append(code, bytecode.CallFun(mid, bytecode.CallFlags{Unconditional: unconditional},
		toRegs(insn.Arguments), toRegs(insn.Returns)))
	// Check whether cast checks are required for returns wider than their
	// receiving target registers, matching the slow machine (frameCopyFrom).
	return append(code, AddIncomingCheckCasts[W](f.RegisterMap(), callee.Outputs(), insn.Returns)...)
}

func (p *bytecodeCompiler[W]) compileNot(insn *instruction.WordTypeB) Bytecode[W] {
	var bitwidth = util.Cast[uint16](insn.Bitwidth)
	// NOT uses only the left source; WordTypeB duplicates it as the right source.
	return bytecode.NewBitwise(bytecode.OP_NOT, toReg(insn.Target), toReg(insn.LeftSource),
		toReg(insn.RightSource), bitwidth)
}

func (p *bytecodeCompiler[W]) compileBitwise(insn *instruction.WordTypeB, op bytecode.Operation, f *WordFunction,
) []Bytecode[W] {
	// The bytecode records the operation width (matching compileShift / compileNot),
	// so it is self-describing; op selects the bytecode operation (bytecode.AND / OR / XOR).
	code := []Bytecode[W]{bytecode.NewBitwise(op, toReg(insn.Target), toReg(insn.LeftSource), toReg(insn.RightSource),
		util.Cast[uint16](insn.Bitwidth))}
	// A cast check is required when the result is written to a register narrower
	// than the operation width.
	return append(code, BitwidthCheckCast[W](f.RegisterMap(), insn.Target, insn.Bitwidth)...)
}

func (p *bytecodeCompiler[W]) compileDivHint(insn *instruction.FieldHint) Bytecode[W] {
	// The only hint form currently generated is the division hint produced by
	// LowerDivisions, which assigns quotient, remainder and range witness from
	// a dividend and divisor.
	if len(insn.Targets) != 3 || len(insn.Sources) != 2 {
		panic("unsupported hint form")
	}
	//
	return bytecode.NewHint(bytecode.DIV_HINT, toRegisterVectors(insn.Targets), toRegisterVectors(insn.Sources))
}

func (p *bytecodeCompiler[W]) compileDivRem(insn *instruction.WordTypeB, op uint32, f *WordFunction) []Bytecode[W] {
	// LeftSource is the dividend; RightSource is the divisor.  op selects the
	// bytecode operation (bytecode.DIV / REM).
	code := []Bytecode[W]{bytecode.NewDivRem(op, toReg(insn.Target), toReg(insn.LeftSource), toReg(insn.RightSource))}
	// Check whether cast check is required (or not).
	return append(code, BitwidthCheckCast[W](f.RegisterMap(), insn.Target, insn.Bitwidth)...)
}

func (p *bytecodeCompiler[W]) compileShift(insn *instruction.WordTypeB, shl bool) Bytecode[W] {
	var (
		op       bytecode.Operation = bytecode.OP_SHL
		bitwidth                    = util.Cast[uint16](insn.Bitwidth)
	)
	//
	if !shl {
		op = bytecode.OP_SHR
	}
	// LeftSource is the value shifted; RightSource holds the shift amount.  The
	// bitwidth masks the result of a left shift and is ignored by a right shift.
	return bytecode.NewBitwise(op, toReg(insn.Target), toReg(insn.LeftSource), toReg(insn.RightSource), bitwidth)
}

func (p *bytecodeCompiler[W]) compileJump(insn *instruction.Jump) Bytecode[W] {
	return bytecode.Jump(bytecode.Address(insn.Immediate))
}

func (p *bytecodeCompiler[W]) compileMemRead(insn *instruction.MemRead, f *WordFunction) []Bytecode[W] {
	var (
		mem = p.signatures[insn.Id].(*descriptor.Memory[W])
		mid = util.Cast[bytecode.ModuleId](insn.Id)
	)
	// A single read bytecode suffices regardless of the memory kind (ROM, static
	// ROM, RAM, paged RAM): the kind is recovered from the environment at encode
	// time.
	code := []Bytecode[W]{bytecode.NewMemRead(mid, toRegs(insn.Arguments), toRegs(insn.Returns))}
	// Check whether cast checks are required (or not).  Values read from a
	// memory whose data registers are wider than the receiving registers must
	// be checked, matching the slow machine which validates every register
	// write (frame.Store).
	return append(code, AddIncomingCheckCasts[W](f.RegisterMap(), mem.Outputs(), insn.Returns)...)
}

func (p *bytecodeCompiler[W]) compileMemWrite(insn *instruction.MemWrite, f *WordFunction) []Bytecode[W] {
	var (
		mem = p.signatures[insn.Id].(*descriptor.Memory[W])
		mid = util.Cast[bytecode.ModuleId](insn.Id)
	)
	// Check whether cast checks are required (or not).  Values written from
	// registers wider than the memory's data registers must be checked before
	// the write, matching the slow machine (executeMemWrite).
	code := AddOutgoingCheckCasts[W](f.RegisterMap(), insn.Returns, mem.Outputs())
	// A single write bytecode suffices regardless of the memory kind (write-once,
	// RAM, paged RAM): the kind is recovered from the environment at encode time.
	return append(code, bytecode.NewMemWrite(mid, toRegs(insn.Arguments), toRegs(insn.Returns)))
}

func (p *bytecodeCompiler[W]) compileSkip(insn *instruction.Skip) []Bytecode[W] {
	var (
		skip  = util.Cast[uint16](insn.Skip)
		codes []Bytecode[W]
	)
	//
	if insn.Skip != 0 {
		codes = append(codes, bytecode.NewSkip(skip))
	}
	//
	return codes
}

func (p *bytecodeCompiler[W]) compileSkipIf(insn *instruction.SkipIf) Bytecode[W] {
	var (
		skip = util.Cast[uint16](insn.Skip)
		l    = insn.Left
		r    = insn.Right
	)
	//
	return bytecode.NewSkipIfVec(insn.Cond, skip, l, r)
}

// compileMultiwaySkip translates a multiway skip into a single SMW bytecode.
// Each dispatch case targets the micro-code its skip would reach (pos.micro +
// skip + 1) -- exactly as compileSkip / compileSkipIf resolve their targets --
// so a match transfers control to that case's jump, while a non-match falls
// through to the following (default) jump.
func (p *bytecodeCompiler[W]) compileMultiwaySkip(insn *instruction.MultiwaySkip) Bytecode[W] {
	var cases = make([]bytecode.SwitchCase[W], len(insn.Cases))
	//
	for i, c := range insn.Cases {
		var (
			skip  = util.Cast[uint16](c.Skip)
			value = word.Const64[W](uint64(c.Value))
		)
		//
		cases[i] = bytecode.SwitchCase[W]{Value: value, Skip: skip}
	}
	//
	return bytecode.MultiwaySkip(toReg(insn.Source), cases)
}

// AddIncomingCheckCasts emits a CHECKCAST for every target register which is
// narrower than the corresponding source register, where sources are values
// arriving in this frame from another module (e.g. a memory's data registers,
// or a callee's return registers).  This mirrors the width check the slow
// machine performs on every register write (frame.Store / frameCopyFrom).  The
// targets are resolved against the given register map (the frame's own
// registers).
func AddIncomingCheckCasts[W word.Word[W]](regmap register.Map, sources []descriptor.Register[W],
	targets []register.Id) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, target := range targets {
		var (
			src = sources[i]
			dst = regmap.Register(target)
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Bitwidth().Unwrap() > dst.Width()) {
			width := util.Cast[uint16](dst.Width())
			codes = append(codes, bytecode.NewCheckCast(toReg(target), width))
		}
	}
	//
	return codes
}

// AddOutgoingCheckCasts emits a CHECKCAST for every source register in this
// frame which is wider than the register receiving its value in another module
// (e.g. a memory's data registers, or a callee's parameter registers).  This
// mirrors the width check the slow machine performs on memory writes
// (executeMemWrite) and call arguments (frameCopyTo).  The sources are resolved
// against the given register map (the frame's own registers).
func AddOutgoingCheckCasts[W word.Word[W]](regmap register.Map, sources []register.Id,
	targets []descriptor.Register[W]) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, source := range sources {
		var (
			src = regmap.Register(source)
			dst = targets[i]
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Width() > dst.Bitwidth().Unwrap()) {
			width := util.Cast[uint16](dst.Bitwidth().Unwrap())
			codes = append(codes, bytecode.NewCheckCast(toReg(source), width))
		}
	}
	//
	return codes
}

// AddCheckCast adds a checkcast instruction if the bitwidth of the right-hand
// side does not fit within the target register(s), resolving widths against the
// given register map.
func AddCheckCast[W word.Word[W]](regmap register.Map, lhs register.Vector, rhs util.Option[uint]) []Bytecode[W] {
	var (
		isNative = isNative(regmap, lhs)
		codes    []Bytecode[W]
	)
	// Add case if either: (i) the rhs has no specific bitwidth; or (2) the
	// bitwidth of the rhs overflows the lhs.
	if !isNative && (rhs.IsEmpty() || lhs.BitWidth(regmap) < rhs.Unwrap()) {
		var (
			last      = lhs.Last()
			lastWidth = util.Cast[uint16](regmap.Register(last).Width())
		)
		// yes
		codes = append(codes, bytecode.NewCheckCast(toReg(last), lastWidth))
	}
	//
	return codes
}

// BitwidthCheckCast adds a checkcast instruction when the target register is
// narrower than the given operation bitwidth, resolving the target width against
// the register map.  This is the cast rule shared by bitwise (AND/OR/XOR) and
// division/remainder operations.
func BitwidthCheckCast[W word.Word[W]](regmap register.Map, target register.Id, opBitwidth uint) []Bytecode[W] {
	var targetWidth = base.RegisterBitwidth(regmap, target)
	//
	if targetWidth < opBitwidth {
		return []Bytecode[W]{bytecode.NewCheckCast(toReg(target), util.Cast[uint16](targetWidth))}
	}
	//
	return nil
}

// CalculateMaximumValue computes the number of bits required to hold the largest
// value the right-hand side of an arithmetic instruction can produce.  Starting
// from the instruction's constant, it folds in the maximum value of each source
// register via acc (addition for INT_ADD, multiplication for INT_MUL) and
// returns the bit-length of that worst-case total.  The caller (addCheckCast)
// uses this to decide whether the result can overflow its target register and
// hence whether a cast check must be inserted.
//
// None is returned when any source is a native (field-typed) register.  Such a
// register has no fixed bitwidth — it can hold any field element up to the prime
// modulus — so the RHS has no finite width bound and could always overflow a
// fixed-width target.  Returning None therefore signals "unbounded", which
// addCheckCast treats as a guaranteed overflow and always emits a cast check.
// This also avoids calling Width() on a native register, which panics.
func CalculateMaximumValue[W word.Word[W]](constant W, regs []register.Id, regmap register.Map,
	acc func(*big.Int, *big.Int)) util.Option[big.Int] {
	//
	var rhsMaxVal big.Int
	// Initialise max value
	rhsMaxVal.Set(constant.BigInt())
	// Determine maximum expressible value
	for _, rod := range regs {
		reg := regmap.Register(rod)
		// Check for native registers
		if reg.IsNative() {
			return util.None[big.Int]()
		}
		// Accumulate maximum register value
		acc(&rhsMaxVal, maxValueOf(reg.Width()))
	}
	//
	return util.Some(rhsMaxVal)
}

func isNative(mapping register.Map, vec register.Vector) bool {
	if vec.Len() != 1 {
		return false
	}
	//
	var reg = mapping.Register(vec.Last())
	//
	return reg.IsNative()
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

// flatternPackets concatenates an array of "bytecode packets" into a single
// flat array of bytecodes.  Each packet holds the bytecodes compiled from one
// word instruction, and a single word instruction may expand to zero, one or
// several bytecodes (e.g. an arithmetic operation followed by a cast check).
//
// The key challenge is that branching instructions (Skip / SkipIf) encode their
// targets as a number of *packets* to skip: at the word-instruction level one
// instruction occupies exactly one position, so skips are counted in those
// positions.  Once packets are flattened these counts are wrong, because each
// intervening packet no longer occupies a single slot.  Each skip count must
// therefore be rewritten into the equivalent number of *bytecodes*.
//
// This proceeds in two passes.  The first records, for every packet, the
// bytecode offset at which it begins (mapping[i]).  The second copies each
// bytecode into the output, recomputing the skip count of any Skip / SkipIf via
// calculateSkip against that mapping.
func flatternPackets[W word.Word[W]](packets [][]Bytecode[W]) (bytecodes []Bytecode[W]) {
	var (
		// One extra slot holds the end sentinel: mapping[len(packets)] is the
		// total bytecode length, i.e. the offset just past the last packet.  A
		// skip which falls off the end of the vector targets this position (see
		// calculateSkip), exactly as the word machine treats a skip past the end
		// as terminating the vector / falling through to the next one.
		mapping = make([]uint, len(packets)+1)
		offset  uint
	)
	// Build the mapping
	for i, p := range packets {
		mapping[i] = offset
		offset += uint(len(p))
	}
	// Record the end sentinel (total bytecode length).
	mapping[len(packets)] = offset
	//
	offset = 0
	// Apply the mapping
	for i, p := range packets {
		for _, b := range p {
			switch s := b.(type) {
			case *bytecode.Skip:
				// Construct clone with recalculated skip
				b = bytecode.NewSkip(calculateSkip(uint(i), offset, s.Skip, mapping))
			case *bytecode.SkipIf:
				// Construct clone with recalculated skip
				b = &bytecode.SkipIf{
					Skip:  calculateSkip(uint(i), offset, s.Skip, mapping),
					Left:  s.Left,
					Right: s.Right,
					Op:    s.Op,
				}
			case *bytecode.Switch[W]:
				// Recalculate every case's skip, mirroring Skip / SkipIf above.
				// Under the current lowering this is a no-op, since a multiway
				// skip is always immediately followed by a contiguous table of
				// single-bytecode jumps (see compileDispatch), so no packet in
				// any case's skip range can expand.  It is done here regardless
				// so flattening stays correct should that layout ever change.
				ncases := make([]bytecode.SwitchCase[W], len(s.Cases))
				for j, c := range s.Cases {
					ncases[j] = bytecode.SwitchCase[W]{
						Value: c.Value,
						Skip:  calculateSkip(uint(i), offset, c.Skip, mapping),
					}
				}
				//
				b = &bytecode.Switch[W]{Source: s.Source, Cases: ncases}
			default:
			}
			//
			bytecodes = append(bytecodes, b)
			offset++
		}
	}
	//
	return bytecodes
}

// calculateSkip rewrites a skip count expressed in packets into the equivalent
// count expressed in flattened bytecodes.  Its parameters are:
//
//	cc      -- index of the packet containing this skip instruction.
//	ncc     -- offset of this skip instruction within the flattened bytecodes.
//	skip    -- original skip count, measured in packets.
//	mapping -- maps each packet index to its starting bytecode offset, with a
//	           final end sentinel at mapping[len-1] holding the total length.
//
// A skip of n packets from packet cc lands on packet cc+n+1 (the +1 steps past
// the skip itself).  That target packet is translated to its starting bytecode
// offset via mapping, and the new skip count is the distance from this
// instruction's own bytecode position (ncc) to that offset, again less one for
// the skip itself.
//
// A skip whose target reaches or passes the end of the vector falls off the end
// — a legitimate vector terminator (the word machine treats it as falling
// through to the next vector).  Such a target is clamped to the end sentinel, so
// the flattened skip lands just past the last bytecode.  This mirrors the gogen
// toReg / toRegs convert the schema register.Id values carried by word
// instructions into the bytecode-layer RegisterId expected by the bytecode
// constructors (the inverse of toId / toIds in bytecode_to_word.go).
func toReg(id register.Id) bytecode.RegisterId {
	return util.Cast[bytecode.RegisterId](id.Unwrap())
}

func toRegs(ids []register.Id) []bytecode.RegisterId {
	var regs = make([]bytecode.RegisterId, len(ids))
	//
	for i, id := range ids {
		regs[i] = toReg(id)
	}
	//
	return regs
}

// toRegisterVectors converts a list of word-machine register vectors into the
// corresponding bytecode register vectors, preserving the per-operand grouping
// (each value may span several limb registers after splitting).
func toRegisterVectors(vecs []register.Vector) []bytecode.RegisterVector {
	var out = make([]bytecode.RegisterVector, len(vecs))
	//
	for i, v := range vecs {
		out[i] = bytecode.NewRegisterVector(toRegs(v.Registers())...)
	}
	//
	return out
}

// emitter's skipTarget, which clamps micro >= vecLen to the next vector.
func calculateSkip(cc, ncc uint, skip uint16, mapping []uint) uint16 {
	// Original target packet of this skip
	var target = cc + uint(skip) + 1
	// Clamp a fall-off-the-end target to the end sentinel (last mapping entry).
	if target >= uint(len(mapping)) {
		target = uint(len(mapping)) - 1
	}
	// Bytecode offset at which that target packet begins
	var ntarget = mapping[target]
	// Recompute skip relative to the flattened bytecode position
	var nskip = ntarget - ncc - 1
	//
	return util.Cast[uint16](nskip)
}
