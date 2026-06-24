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
// a bytecode program.
func WordToBytecodeProgram[W word.Word[W]](wm *machine.Word[W]) descriptor.Program[W] {
	var (
		modules  = make([]descriptor.Module[W], len(wm.Modules()))
		compiler = &bytecodeCompiler[W]{wm, modules}
	)
	// translate functions
	for i, m := range wm.Modules() {
		modules[i] = compiler.compileWordModule(uint(i), m)
	}
	//
	return descriptor.NewProgram(0, modules...)
}

type bytecodeCompiler[W word.Word[W]] struct {
	machine *machine.Word[W]
	modules []descriptor.Module[W]
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
	return descriptor.NewMemory(m.Name(), registers, m.Kind(), m.Geometry(), contents)
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
		return p.compileCall(insn.(*instruction.Call), f)
	case opcode.UNCONDITIONAL_CALL:
		return p.compileCall(&instruction.Call{OpIo: insn.(*instruction.UnconditionalCall).OpIo}, f)
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
		code = p.compileSub(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_MUL:
		return p.compileMul(insn.(*instruction.WordTypeA[W]), f)
	case opcode.BIT_CONCAT:
		code = p.compileConcat(insn.(*instruction.WordTypeA[W]))
	case opcode.INT_DIV:
		return p.compileDivRem(insn.(*instruction.WordTypeB), encoding.DIV, f)
	case opcode.INT_REM:
		return p.compileDivRem(insn.(*instruction.WordTypeB), encoding.REM, f)
	case opcode.BIT_AND:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.AND, f)
	case opcode.BIT_NOT:
		code = p.compileNot(insn.(*instruction.WordTypeB))
	case opcode.BIT_OR:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.OR, f)
	case opcode.BIT_XOR:
		return p.compileBitwise(insn.(*instruction.WordTypeB), bytecode.XOR, f)
	case opcode.BIT_SHL:
		code = p.compileShift(insn.(*instruction.WordTypeB), true)
	case opcode.BIT_SHR:
		code = p.compileShift(insn.(*instruction.WordTypeB), false)
	case opcode.INT_ADDMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), encoding.ADDMOD_P)
	case opcode.INT_SUBMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), encoding.SUBMOD_P)
	case opcode.INT_MULMOD_P:
		code = p.compileFieldArith(insn.(*instruction.WordTypeF[W]), encoding.MULMOD_P)
	default:
		panic(fmt.Sprintf("unknown instruction opcode (0x%x)", insn.OpCode()))
	}
	//
	return []Bytecode[W]{code}
}

func (p *bytecodeCompiler[W]) compileAdd(insn *instruction.WordTypeA[W], f *WordFunction) []Bytecode[W] {
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
	code := []Bytecode[W]{bytecode.AddVecConst(insn.Target.Registers(), insn.Sources, insn.Constant)}
	// Check whether cast check is required (or not).
	return append(code, p.addCheckCast(insn.Target, &rhsMaxVal, f)...)
}

func (p *bytecodeCompiler[W]) compileMul(insn *instruction.WordTypeA[W], f *WordFunction) []Bytecode[W] {
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
	code := []Bytecode[W]{bytecode.MulVecConst(insn.Target.Registers(), insn.Sources, insn.Constant)}
	// Check whether cast check is required (or not).
	return append(code, p.addCheckCast(insn.Target, &rhsMaxVal, f)...)
}
func (p *bytecodeCompiler[W]) compileSub(insn *instruction.WordTypeA[W]) Bytecode[W] {
	return bytecode.SubVecConst(insn.Target.Registers(), insn.Sources, insn.Constant)
}

// compileFieldArith emits a modular field-arithmetic bytecode (ADDMOD_P,
// SUBMOD_P or MULMOD_P).  Unlike the integer arithmetic forms, the result is
// always reduced modulo the prime characteristic and so fits within the (native)
// target register; hence no cast check is ever required, matching the slow
// machine (executeFieldAdd / executeFieldSub / executeFieldMul).
func (p *bytecodeCompiler[W]) compileFieldArith(insn *instruction.WordTypeF[W], op uint32) Bytecode[W] {
	return bytecode.NewFieldArith(op, insn.Target, insn.Sources, insn.Constant)
}

func (p *bytecodeCompiler[W]) compileConcat(insn *instruction.WordTypeA[W]) Bytecode[W] {
	if insn.Constant.Cmp64(0) != 0 {
		panic("constant given for bit concatenation")
	}
	// CAT keeps source and target vectors in low-limb-first register order.
	return bytecode.Concat(insn.Target.Registers(), insn.Sources)
}

func (p *bytecodeCompiler[W]) compileCall(insn *instruction.Call, f *WordFunction) []Bytecode[W] {
	var (
		callee = p.machine.Module(insn.Id).(*WordFunction)
		mid    = util.Cast[bytecode.ModuleId](bytecode.ModuleId(insn.Id))
	)
	// sanity chewcks
	checkOperands(insn.Arguments...)
	checkOperands(insn.Returns...)
	// Check whether cast checks are required for arguments wider than their
	// receiving parameter registers, matching the slow machine (frameCopyTo).
	code := p.addOutgoingCheckCasts(insn.Arguments, callee.Inputs(), f)
	// Add the call instruction
	code = append(code, bytecode.CallFun(mid, false, insn.Arguments, insn.Returns))
	// Check whether cast checks are required for returns wider than their
	// receiving target registers, matching the slow machine (frameCopyFrom).
	return append(code, p.addIncomingCheckCasts(callee.Outputs(), insn.Returns, f)...)
}

func (p *bytecodeCompiler[W]) compileNot(insn *instruction.WordTypeB) Bytecode[W] {
	var bitwidth = util.Cast[uint16](insn.Bitwidth)
	// NOT uses only the left source; WordTypeB duplicates it as the right source.
	return bytecode.NewBitwise(bytecode.NOT, insn.Target, insn.LeftSource, insn.RightSource, bitwidth)
}

func (p *bytecodeCompiler[W]) compileBitwise(insn *instruction.WordTypeB, op bytecode.BitwiseOp, f *WordFunction,
) []Bytecode[W] {
	//
	var bitwidth = util.Cast[uint16](base.RegisterBitwidth(f.RegisterMap(), insn.Target))
	// op selects the bytecode operation (bytecode.AND / OR / XOR).
	code := []Bytecode[W]{bytecode.NewBitwise(op, insn.Target, insn.LeftSource, insn.RightSource, bitwidth)}
	// Check whether cast check is required (or not).
	if uint(bitwidth) < insn.Bitwidth {
		// yes
		code = append(code, bytecode.NewCheckCast(insn.Target, bitwidth))
	}
	//
	return code
}

func (p *bytecodeCompiler[W]) compileDivHint(insn *instruction.FieldHint) Bytecode[W] {
	// The only hint form currently generated is the division hint produced by
	// LowerDivisions, which assigns quotient, remainder and range witness from
	// a dividend and divisor.
	if len(insn.Targets) != 3 || len(insn.Sources) != 2 {
		panic("unsupported hint form")
	}
	//
	return bytecode.NewDivHint(
		insn.Targets[0], insn.Targets[1], insn.Targets[2], insn.Sources[0], insn.Sources[1])
}

func (p *bytecodeCompiler[W]) compileDivRem(insn *instruction.WordTypeB, op uint32, f *WordFunction) []Bytecode[W] {
	var bitwidth = util.Cast[uint16](base.RegisterBitwidth(f.RegisterMap(), insn.Target))
	// LeftSource is the dividend; RightSource is the divisor.  op selects the
	// bytecode operation (bytecode.DIV / REM).
	code := []Bytecode[W]{bytecode.NewDivRem(op, insn.Target, insn.LeftSource, insn.RightSource)}
	// Check whether cast check is required (or not).
	if uint(bitwidth) < insn.Bitwidth {
		// yes
		code = append(code, bytecode.NewCheckCast(insn.Target, bitwidth))
	}
	//
	return code
}

func (p *bytecodeCompiler[W]) compileShift(insn *instruction.WordTypeB, shl bool) Bytecode[W] {
	var (
		op       bytecode.BitwiseOp = bytecode.SHL
		bitwidth                    = util.Cast[uint16](insn.Bitwidth)
	)
	//
	if !shl {
		op = bytecode.SHR
	}
	// LeftSource is the value shifted; RightSource holds the shift amount.  The
	// bitwidth masks the result of a left shift and is ignored by a right shift.
	return bytecode.NewBitwise(op, insn.Target, insn.LeftSource, insn.RightSource, bitwidth)
}

func (p *bytecodeCompiler[W]) compileJump(insn *instruction.Jump) Bytecode[W] {
	return bytecode.Jump(bytecode.Address(insn.Immediate))
}

func (p *bytecodeCompiler[W]) compileMemRead(insn *instruction.MemRead, f *WordFunction) []Bytecode[W] {
	var (
		mem  = p.machine.Module(insn.Id).(memory.Memory[W])
		mid  = util.Cast[bytecode.ModuleId](insn.Id)
		code []Bytecode[W]
	)
	//
	switch mem.(type) {
	case *memory.ReadOnly[W]:
		code = []Bytecode[W]{bytecode.ReadRom(mid, insn.Arguments, insn.Returns)}
	case *memory.StaticReadOnly[W]:
		code = []Bytecode[W]{bytecode.ReadStaticRom(mid, insn.Arguments, insn.Returns)}
	case *memory.RandomAccess[W]:
		code = []Bytecode[W]{bytecode.ReadRam(mid, insn.Arguments, insn.Returns)}
	case *memory.PagedRandomAccess[W]:
		code = []Bytecode[W]{bytecode.ReadPagedRam(mid, insn.Arguments, insn.Returns)}
	default:
		panic("unknown memory type")
	}
	// Check whether cast checks are required (or not).  Values read from a
	// memory whose data registers are wider than the receiving registers must
	// be checked, matching the slow machine which validates every register
	// write (frame.Store).
	return append(code, p.addIncomingCheckCasts(mem.Geometry().DataRegisters(), insn.Returns, f)...)
}

func (p *bytecodeCompiler[W]) compileMemWrite(insn *instruction.MemWrite, f *WordFunction) []Bytecode[W] {
	var (
		mem  = p.machine.Module(insn.Id).(memory.Memory[W])
		code []Bytecode[W]
		mid  = util.Cast[bytecode.ModuleId](insn.Id)
	)
	// Check whether cast checks are required (or not).  Values written from
	// registers wider than the memory's data registers must be checked before
	// the write, matching the slow machine (executeMemWrite).
	code = p.addOutgoingCheckCasts(insn.Returns, mem.Geometry().DataRegisters(), f)
	//
	switch mem.(type) {
	case *memory.WriteOnce[W]:
		code = append(code, bytecode.WriteWom(mid, insn.Arguments, insn.Returns))
	case *memory.RandomAccess[W]:
		code = append(code, bytecode.WriteRam(mid, insn.Arguments, insn.Returns))
	case *memory.PagedRandomAccess[W]:
		code = append(code, bytecode.WritePagedRam(mid, insn.Arguments, insn.Returns))
	default:
		panic("unknown memory type")
	}
	//
	return code
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
	return bytecode.MultiwaySkip(insn.Source, cases)
}

// addIncomingCheckCasts emits a CHECKCAST for every target register which is
// narrower than the corresponding source register, where sources are values
// arriving in this frame from another module (e.g. a memory's data registers,
// or a callee's return registers).  This mirrors the width check the slow
// machine performs on every register write (frame.Store / frameCopyFrom).
func (p *bytecodeCompiler[W]) addIncomingCheckCasts(sources []register.Register, targets []register.Id,
	f *WordFunction) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, target := range targets {
		var (
			src = sources[i]
			dst = f.Register(target)
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Width() > dst.Width()) {
			width := util.Cast[uint16](dst.Width())
			codes = append(codes, bytecode.NewCheckCast(target, width))
		}
	}
	//
	return codes
}

// addOutgoingCheckCasts emits a CHECKCAST for every source register in this
// frame which is wider than the register receiving its value in another module
// (e.g. a memory's data registers, or a callee's parameter registers).  This
// mirrors the width check the slow machine performs on memory writes
// (executeMemWrite) and call arguments (frameCopyTo).
func (p *bytecodeCompiler[W]) addOutgoingCheckCasts(sources []register.Id, targets []register.Register,
	f *WordFunction) []Bytecode[W] {
	var codes []Bytecode[W]
	//
	for i, source := range sources {
		var (
			src = f.Register(source)
			dst = targets[i]
		)
		//
		if !dst.IsNative() && (src.IsNative() || src.Width() > dst.Width()) {
			width := util.Cast[uint16](dst.Width())
			codes = append(codes, bytecode.NewCheckCast(source, width))
		}
	}
	//
	return codes
}

// Add a checkcast instruction if the given value does not fit within the target
// register(s).
func (p *bytecodeCompiler[W]) addCheckCast(target register.Vector, value *big.Int, f *WordFunction) []Bytecode[W] {
	var (
		targetMaxVal = maxValueOf(target.BitWidth(f.RegisterMap()))
		codes        []Bytecode[W]
	)
	//
	if targetMaxVal.Cmp(value) < 0 {
		var (
			last      = target.Last()
			lastWidth = util.Cast[uint16](f.Register(last).Width())
		)
		// yes
		codes = append(codes, bytecode.NewCheckCast(last, lastWidth))
	}
	//
	return codes
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
		mapping = make([]uint, len(packets))
		offset  uint
	)
	// Build the mapping
	for i, p := range packets {
		mapping[i] = offset
		offset += uint(len(p))
	}
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
//	mapping -- maps each packet index to its starting bytecode offset.
//
// A skip of n packets from packet cc lands on packet cc+n+1 (the +1 steps past
// the skip itself).  That target packet is translated to its starting bytecode
// offset via mapping, and the new skip count is the distance from this
// instruction's own bytecode position (ncc) to that offset, again less one for
// the skip itself.
func calculateSkip(cc, ncc uint, skip uint16, mapping []uint) uint16 {
	// Original target packet of this skip
	var target = cc + uint(skip) + 1
	// Bytecode offset at which that target packet begins
	var ntarget = mapping[target]
	// Recompute skip relative to the flattened bytecode position
	var nskip = ntarget - ncc - 1
	//
	return util.Cast[uint16](nskip)
}
