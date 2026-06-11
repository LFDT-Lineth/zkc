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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// WordToWordMachine transforms a machine operating over a given word type (W1)
// into an identical machine which operates over a different word type (W2).
// Generally speaking, we are going from a larger word (e.g. word.Uint) to a
// smaller word (e.g. word.Uint64).
//
// The transformation is purely structural: instructions are re-typed but not
// rewritten or lowered, register declarations are preserved verbatim (no
// splitting or width changes), and constants are not reduced modulo the field.
// The source machine's prime modulus is read from its executor and re-expressed
// in W2 so the new machine retains the same field semantics; this means the
// modulus itself must also fit in W2's bandwidth.  ROM/SROM contents are
// converted element-wise; WOM/RAM/Paged memories start empty in the new
// machine, matching their behaviour in the source.
//
// This function will panic if it encounters a register, constant, modulus or
// memory cell which exceeds the bandwidth of W2.  Callers needing to target a
// narrower word size than some source register widths should run
// SplitRegisters first.
func WordToWordMachine[W1 word.Word[W1], W2 word.Word[W2]](wm *machine.Word[W1]) *machine.Word[W2] {
	var (
		w2       W2
		lowering = wordToWord[W1, W2]{w2.Bandwidth()}
		modules  = make([]Module, len(wm.Modules()))
	)
	// Lower each module
	for i, m := range wm.Modules() {
		modules[i] = lowering.lowerModule(m)
	}
	// Re-express the modulus in the target word type
	modulus := lowering.convertConstant(wm.Executor().Modulus())
	// Construct new machine over W2
	return machine.NewWordFromModulus(modulus, modules...)
}

type wordToWord[W1 word.Word[W1], W2 word.Word[W2]] struct {
	// target bandwidth (i.e. bandwidth of W2)
	bandwidth uint
}

func (p wordToWord[W1, W2]) lowerModule(m Module) Module {
	switch m := m.(type) {
	case *function.Function[instruction.Word]:
		return p.lowerFunc(m)
	case memory.Memory[W1]:
		return p.lowerMemory(m)
	default:
		panic(fmt.Sprintf("unknown module \"%s\"", m.Name()))
	}
}

func (p wordToWord[W1, W2]) lowerFunc(fn *WordFunction) *WordFunction {
	var (
		regs  = slices.Clone(fn.Registers())
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
	)
	// Sanity-check register widths against W2.
	checkRegisterWidths(p.bandwidth, regs...)
	// Lower each instruction vector.
	for i, v := range code {
		ncode[i] = p.lowerVector(v)
	}
	//
	return function.New(fn.Name(), fn.IsNative(), regs, ncode)
}

func (p wordToWord[W1, W2]) lowerVector(v VectorInstruction) VectorInstruction {
	var insns = make([]instruction.Word, len(v.Codes))
	//
	for i, c := range v.Codes {
		insns[i] = p.lowerInstruction(c)
	}
	//
	return instruction.NewVector(insns...)
}

func (p wordToWord[W1, W2]) lowerInstruction(insn instruction.Word) instruction.Word {
	switch insn.OpCode() {
	// Base instructions are word-type-agnostic and translate verbatim.
	case opcode.CALL:
		return insn.(*instruction.Call)
	case opcode.DEBUG:
		return insn.(*instruction.Debug)
	case opcode.FAIL:
		return insn.(*instruction.Fail)
	case opcode.JUMP:
		return insn.(*instruction.Jump)
	case opcode.MEMORY_READ:
		return insn.(*instruction.MemRead)
	case opcode.MEMORY_WRITE:
		return insn.(*instruction.MemWrite)
	case opcode.RETURN:
		return insn.(*instruction.Return)
	case opcode.SKIP:
		return insn.(*instruction.Skip)
	case opcode.SKIP_IF:
		return insn.(*instruction.SkipIf)
	case opcode.HINT_DIVISION:
		return insn.(*instruction.FieldHint)
	// Type-A instructions carry a W-typed constant and must be re-typed.
	case opcode.INT_ADD, opcode.INT_SUB, opcode.INT_MUL, opcode.BIT_CONCAT:
		a := insn.(*instruction.WordTypeA[W1])
		return instruction.NewWordTypeA(a.Op, a.Target, a.Sources, p.convertConstant(a.Constant))
	// Type-B instructions are word-type-agnostic.
	case opcode.INT_DIV, opcode.INT_REM,
		opcode.BIT_AND, opcode.BIT_NOT, opcode.BIT_OR, opcode.BIT_XOR,
		opcode.BIT_SHL, opcode.BIT_SHR:
		return insn.(*instruction.WordTypeB)
	// Type-F instructions carry a W-typed constant and must be re-typed.
	case opcode.INT_ADDMOD_P, opcode.INT_SUBMOD_P, opcode.INT_MULMOD_P:
		f := insn.(*instruction.WordTypeF[W1])
		return instruction.NewWordTypeF(f.Op, f.Target, f.Sources, p.convertConstant(f.Constant))
	default:
		panic(fmt.Sprintf("unknown instruction opcode (0x%x)", insn.OpCode()))
	}
}

func (p wordToWord[W1, W2]) lowerMemory(m memory.Memory[W1]) memory.Memory[W2] {
	var regs = slices.Clone(m.Registers())
	// Sanity-check register widths against W2.
	checkRegisterWidths(p.bandwidth, regs...)
	//
	switch m := m.(type) {
	case *memory.StaticReadOnly[W1]:
		contents := p.convertContents(m.Contents())
		return memory.NewStatic(m.Name(), m.IsPublic(), regs, contents...)
	case *memory.ReadOnly[W1]:
		contents := p.convertContents(m.Contents())
		return memory.NewReadOnly(m.Name(), m.IsPublic(), regs, contents...)
	case *memory.WriteOnce[W1]:
		return memory.NewWriteOnce[W2](m.Name(), m.IsPublic(), regs)
	case *memory.RandomAccess[W1]:
		return memory.NewRandomAccess[W2](m.Name(), regs)
	case *memory.PagedRandomAccess[W1]:
		return memory.NewPagedRandomAccess[W2](m.Name(), regs)
	default:
		panic(fmt.Sprintf("unknown memory module \"%s\"", m.Name()))
	}
}

func (p wordToWord[W1, W2]) convertContents(contents []W1) []W2 {
	var out = make([]W2, len(contents))
	//
	for i, c := range contents {
		out[i] = p.convertConstant(c)
	}
	//
	return out
}

func (p wordToWord[W1, W2]) convertConstant(c W1) W2 {
	var w2 W2
	//
	if !c.FitsWithin(p.bandwidth) {
		panic(fmt.Sprintf("constant 0x%s exceeds u%d bandwidth", c.Text(16), p.bandwidth))
	}
	//
	return w2.SetBigInt(c.BigInt())
}
