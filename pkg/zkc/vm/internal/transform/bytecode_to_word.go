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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// BytecodeProgramToWord decompiles a bytecode program descriptor back into an
// equivalent word machine.  It is the inverse of WordToBytecodeProgram: where
// that lowering compiles each word instruction into a packet of bytecodes
// (inserting the cast checks the word machine performs implicitly, and rewriting
// branch offsets from instruction units into bytecode units), this reverses the
// process.  The cast-check bytecodes are dropped (the word machine re-validates
// every register write itself, so they are redundant) and the surviving
// bytecodes map one-to-one back onto word instructions, with branch offsets
// rewritten back into instruction units.
//
// The resulting machine is behaviourally equivalent to the original, though the
// round trip is not bit-identical: information discarded during lowering (e.g.
// the distinction between conditional and unconditional calls, and the
// checkpoint flag on calls) is not recovered.  The bytecode descriptor does not
// carry the surrounding field, so the prime modulus -- needed when executing
// native field instructions -- is supplied separately.
func BytecodeProgramToWord[W word.Word[W]](p descriptor.Program[W]) *machine.Word[W] {
	var (
		mods      = p.Modules()
		modules   = make([]Module, len(mods))
		callstack machine.CallStack[W, instruction.Word]
		modulus   W
	)
	// Derive the prime field modulus from the word configuration.
	modulus = modulus.SetBigInt(p.Field().Modulus())
	// Decompile each module back into its word-machine form.
	for i, m := range mods {
		modules[i] = decompileModule(m)
	}
	//
	return machine.NewWordFromModulus(modulus, callstack, modules...)
}

func decompileModule[W word.Word[W]](m descriptor.Module[W]) Module {
	switch m := m.(type) {
	case *descriptor.Function[W]:
		return decompileFunction(m)
	case *descriptor.Memory[W]:
		return decompileMemory(m)
	default:
		panic(fmt.Sprintf("unknown descriptor module \"%s\"", m.Name()))
	}
}

func decompileFunction[W word.Word[W]](f *descriptor.Function[W]) *WordFunction {
	var (
		registers = f.Registers()
		regs      = decompileRegisters(registers)
		code      = make([]VectorInstruction, len(f.Vectors()))
	)
	// Decompile each bytecode vector back into a word instruction vector.
	for i, vec := range f.Vectors() {
		code[i] = decompileVector(vec, registers)
	}
	//
	return function.New(f.Name(), f.IsNative(), regs, code)
}

// decompileVector turns a bytecode vector back into a word instruction vector.
// First, the cast-check bytecodes are dropped via Map (which automatically
// rewrites the branch offsets of any skip to account for the removed
// bytecodes), reversing the packet expansion performed during lowering.  The
// surviving bytecodes then map one-to-one onto word instructions, so their
// (now instruction-unit) branch offsets carry over verbatim.
func decompileVector[W word.Word[W]](vec bytecode.Vector[W], regs []descriptor.Register[W]) VectorInstruction {
	// Drop cast checks, rewriting skip offsets as needed.
	cleaned := vec.Map(func(_ uint, b bytecode.Bytecode[W]) []bytecode.Bytecode[W] {
		if _, ok := b.(*bytecode.CheckCast[W]); ok {
			return nil
		}
		//
		return []bytecode.Bytecode[W]{b}
	})
	//
	insns := make([]WordInstruction, len(cleaned.Bytecodes))
	//
	for i, b := range cleaned.Bytecodes {
		insns[i] = decompileBytecode(b, regs)
	}
	//
	return instruction.NewVector(insns...)
}

//nolint:gocyclo
func decompileBytecode[W word.Word[W]](b bytecode.Bytecode[W], regs []descriptor.Register[W]) WordInstruction {
	switch b := b.(type) {
	case *bytecode.Arith[W]:
		return decompileArith(b)
	case *bytecode.Bitwise[W]:
		return decompileBitwise(b)
	case *bytecode.Call[W]:
		// The checkpoint flag has no word-instruction representation and is
		// dropped; the unconditional flag selects between the two call forms.
		if b.Flags.Unconditional {
			return instruction.NewUnconditionalCall(uint(b.Target), toIds(b.Arguments), toIds(b.Returns))
		}
		//
		return instruction.NewCall(uint(b.Target), toIds(b.Arguments), toIds(b.Returns))
	case *bytecode.Cat[W]:
		return instruction.BitConcatV[W](toVector(b.Targets), toIds(b.Sources))
	case *bytecode.CheckCast[W]:
		// Cast checks are dropped by decompileVector before reaching here.
		panic("unexpected cast check")
	case *bytecode.Debug[W]:
		return instruction.NewDebug(decompileChunks(b.Chunks, b.Sources)...)
	case *bytecode.Hint[W]:
		return instruction.NewFieldHint(registerVectorsToVectors(b.Targets), registerVectorsToVectors(b.Sources))
	case *bytecode.DivRem[W]:
		return decompileDivRem(b, regs)
	case *bytecode.Fail[W]:
		return instruction.NewFail(decompileChunks(b.Chunks, b.Sources)...)
	case *bytecode.FieldArith[W]:
		return decompileFieldArith(b)
	case *bytecode.Jmp[W]:
		return instruction.NewJump(uint(b.Target))
	case *bytecode.ReadWrite[W]:
		return decompileReadWrite(b)
	case *bytecode.Ret[W]:
		return instruction.NewReturn()
	case *bytecode.Skip[W]:
		return &instruction.Skip{Skip: uint(b.Skip)}
	case *bytecode.SkipIf[W]:
		return instruction.NewSkipIfVec(b.Op, toVector(b.Left.Registers()), toVector(b.Right.Registers()), uint(b.Skip))
	case *bytecode.Switch[W]:
		return decompileSwitch(b)
	default:
		panic("unknown bytecode")
	}
}

func decompileArith[W word.Word[W]](b *bytecode.Arith[W]) WordInstruction {
	var (
		target  = toVector(b.Target)
		sources = toIds(b.Source)
	)
	//
	switch b.Op {
	case bytecode.OP_ADD:
		return instruction.UintAddV(target, sources, b.Constant)
	case bytecode.OP_SUB:
		return instruction.UintSubV(target, sources, b.Constant)
	case bytecode.OP_MUL:
		return instruction.UintMulV(target, sources, b.Constant)
	default:
		panic("unknown arithmetic operation")
	}
}

func decompileBitwise[W word.Word[W]](b *bytecode.Bitwise[W]) WordInstruction {
	var (
		bitwidth = uint(b.Bitwidth)
		target   = toId(b.Target)
		left     = toId(b.Left)
		right    = toId(b.Right)
	)
	//
	switch b.Op {
	case bytecode.OP_AND:
		return instruction.BitAnd(bitwidth, target, left, right)
	case bytecode.OP_OR:
		return instruction.BitOr(bitwidth, target, left, right)
	case bytecode.OP_XOR:
		return instruction.BitXor(bitwidth, target, left, right)
	case bytecode.OP_NOT:
		// NOT uses only the left source (it is duplicated as the right source).
		return instruction.BitNot(bitwidth, target, left)
	case bytecode.OP_SHL:
		return instruction.BitShl(bitwidth, target, left, right)
	case bytecode.OP_SHR:
		return instruction.BitShr(bitwidth, target, left, right)
	default:
		panic("unknown bitwise operation")
	}
}

func decompileDivRem[W word.Word[W]](b *bytecode.DivRem[W], regs []descriptor.Register[W]) WordInstruction {
	var (
		// Like the bitwise operations, the bytecode does not record the operation
		// width; it is the width of the (uniform) operands, recovered from the
		// dividend register, and needed so re-lowering re-inserts any cast check.
		bitwidth = regs[b.Dividend].Bitwidth().UnwrapOr(0)
		target   = toId(b.Target)
		dividend = toId(b.Dividend)
		divisor  = toId(b.Divisor)
	)
	//
	switch b.Opcode {
	case encoding.DIV:
		return instruction.UintDiv[W](bitwidth, target, dividend, divisor)
	case encoding.REM:
		return instruction.UintRem(bitwidth, target, dividend, divisor)
	default:
		panic("unknown division operation")
	}
}

func decompileFieldArith[W word.Word[W]](b *bytecode.FieldArith[W]) WordInstruction {
	var (
		target  = toId(b.Target)
		sources = toIds(b.Sources)
	)
	//
	switch b.Op {
	case bytecode.OP_ADDMOD_P:
		return instruction.UintAddModP(target, sources, b.Constant)
	case bytecode.OP_SUBMOD_P:
		return instruction.UintSubModP(target, sources, b.Constant)
	case bytecode.OP_MULMOD_P:
		return instruction.UintMulModP(target, sources, b.Constant)
	default:
		panic("unknown field operation")
	}
}

func decompileReadWrite[W word.Word[W]](b *bytecode.ReadWrite[W]) WordInstruction {
	var (
		id      = uint(b.Id)
		address = toIds(b.Address)
		data    = toIds(b.Data)
	)
	//
	if b.Write {
		return instruction.NewMemWrite(id, address, data)
	}
	//
	return instruction.NewMemRead(id, address, data)
}

func decompileSwitch[W word.Word[W]](b *bytecode.Switch[W]) WordInstruction {
	var cases = make([]instruction.DispatchCase, len(b.Cases))
	//
	for i, c := range b.Cases {
		cases[i] = instruction.DispatchCase{Value: uint(c.Value.Uint64()), Skip: uint(c.Skip)}
	}
	//
	return instruction.NewMultiwaySkip(toId(b.Source), cases...)
}

// decompileChunks reconstructs the formatted (debug / fail) message chunks from
// the bytecode form, re-pairing each chunk that carries a format with the next
// source register vector (mirroring how the chunks and sources were separated
// during lowering, see bytecode.NewDebug / bytecode.NewFail).
func decompileChunks(chunks []bytecode.FormattedChunk, sources []bytecode.RegisterVector) []instruction.FormattedChunk {
	var (
		result = make([]instruction.FormattedChunk, len(chunks))
		next   = 0
	)
	//
	for i, c := range chunks {
		var argument register.Vector
		// A chunk consumes a source vector exactly when it carries a value to
		// format, matching the slow machine's executeFormattedChunks.
		if c.Format.HasFormat() {
			argument = toVector(sources[next].Registers())
			next++
		}
		//
		result[i] = instruction.FormattedChunk{Text: c.Text, Format: c.Format, Argument: argument}
	}
	//
	return result
}

func decompileMemory[W word.Word[W]](m *descriptor.Memory[W]) Module {
	var (
		name     = m.Name()
		geometry = m.Geometry()
		kind     = m.Kind()
	)
	//
	switch {
	case kind.IsStatic():
		return memory.NewStatic(name, kind.IsPublic(), geometry, m.StaticContents()...)
	case kind.IsReadOnly():
		return memory.NewReadOnly(name, kind.IsPublic(), geometry)
	case kind.IsWriteOnly():
		return memory.NewWriteOnce(name, kind.IsPublic(), geometry)
	case kind.IsPaged():
		return memory.NewPagedRandomAccess(name, geometry)
	case kind.IsReadWrite():
		return memory.NewRandomAccess(name, geometry)
	default:
		panic(fmt.Sprintf("unknown memory kind for \"%s\"", name))
	}
}

// decompileRegisters reconstructs the schema registers of a module from their
// descriptor form.
func decompileRegisters[W word.Word[W]](regs []descriptor.Register[W]) []register.Register {
	var result = make([]register.Register, len(regs))
	//
	for i, r := range regs {
		result[i] = decompileRegister(r)
	}
	//
	return result
}

func decompileRegister[W word.Word[W]](r descriptor.Register[W]) register.Register {
	var (
		kind    = registerKind(r)
		padding = *r.Padding().BigInt()
	)
	// Native registers have no fixed bitwidth.
	if r.IsNative() {
		return register.NewNative(kind, r.Name(), padding)
	}
	//
	return register.New(kind, r.Name(), r.Bitwidth().Unwrap(), padding)
}

func registerKind[W word.Word[W]](r descriptor.Register[W]) register.Type {
	switch {
	case r.IsInput():
		return register.INPUT_REGISTER
	case r.IsOutput():
		return register.OUTPUT_REGISTER
	default:
		return register.COMPUTED_REGISTER
	}
}

// ============================================================================
// Register helpers
// ============================================================================

func toId(r bytecode.RegisterId) register.Id {
	return register.NewId(uint(r))
}

func toIds(regs []bytecode.RegisterId) []register.Id {
	var ids = make([]register.Id, len(regs))
	//
	for i, r := range regs {
		ids[i] = toId(r)
	}
	//
	return ids
}

// registerVectorsToVectors converts each bytecode register vector into the
// corresponding word-machine register vector, preserving the per-operand
// grouping (each value may span several limb registers after splitting).
func registerVectorsToVectors(vecs []bytecode.RegisterVector) []register.Vector {
	var out = make([]register.Vector, len(vecs))
	//
	for i, v := range vecs {
		out[i] = toVector(v.Registers())
	}
	//
	return out
}

func toVector(regs []bytecode.RegisterId) register.Vector {
	return register.NewVector(toIds(regs)...)
}
