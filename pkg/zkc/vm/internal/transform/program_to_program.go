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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ProgramToProgram transforms a bytecode program operating over a given word
// type (W1) into an identical program which operates over a different word type
// (W2).  Generally speaking, we are going from a larger word (e.g. word.Uint) to
// a smaller word (e.g. word.Uint64).  This is the program-level analogue of
// WordToWordMachine.
//
// The transformation is purely structural: bytecodes are re-typed but not
// rewritten or lowered, register declarations are preserved verbatim (no
// splitting or width changes), and constants are not reduced modulo the field.
// Only bytecodes and registers carrying a W-typed value (arithmetic constants,
// dispatch values and register padding) are re-expressed in W2; all others are
// word-type-agnostic and carry over unchanged.  Static memory contents are
// converted element-wise; non-static memories carry no contents in either
// representation.
//
// This function will panic if it encounters a register, constant or memory cell
// which exceeds the bandwidth of W2.  Callers needing to target a narrower word
// size than some source register widths should run SplitRegisters first.
func ProgramToProgram[W1 word.Word[W1], W2 word.Word[W2]](p descriptor.Program[W1]) descriptor.Program[W2] {
	var (
		w2       W2
		lowering = programToProgram[W1, W2]{w2.Bandwidth()}
		modules  = make([]descriptor.Module[W2], len(p.Modules()))
	)
	// Lower each module
	for i, m := range p.Modules() {
		modules[i] = lowering.lowerModule(m)
	}
	// Construct new program over W2
	return descriptor.NewProgram(p.Field(), modules...)
}

type programToProgram[W1 word.Word[W1], W2 word.Word[W2]] struct {
	// target bandwidth (i.e. bandwidth of W2)
	bandwidth uint
}

func (p programToProgram[W1, W2]) lowerModule(m descriptor.Module[W1]) descriptor.Module[W2] {
	switch m := m.(type) {
	case *descriptor.Function[W1]:
		return p.lowerFunction(m)
	case *descriptor.Memory[W1]:
		return p.lowerMemory(m)
	default:
		panic(fmt.Sprintf("unknown module \"%s\"", m.Name()))
	}
}

func (p programToProgram[W1, W2]) lowerFunction(fn *descriptor.Function[W1]) *descriptor.Function[W2] {
	var (
		regs    = p.lowerRegisters(fn.Registers())
		vectors = fn.Vectors()
		nvecs   = make([]bytecode.Vector[W2], len(vectors))
	)
	// Lower each bytecode vector.
	for i, v := range vectors {
		nvecs[i] = p.lowerVector(v)
	}
	//
	return descriptor.NewFunction(fn.Name(), regs, fn.Kind(), fn.Effects(), nvecs)
}

func (p programToProgram[W1, W2]) lowerMemory(m *descriptor.Memory[W1]) *descriptor.Memory[W2] {
	var (
		regs     = p.lowerRegisters(m.Registers())
		contents []W2
	)
	// Only static memories carry (fixed) contents.
	if m.IsStatic() {
		contents = p.convertContents(m.StaticContents())
	}
	//
	return descriptor.NewMemory(m.Name(), regs, m.Kind(), contents)
}

func (p programToProgram[W1, W2]) lowerRegisters(regs []descriptor.Register[W1]) []descriptor.Register[W2] {
	var out = make([]descriptor.Register[W2], len(regs))
	//
	for i, r := range regs {
		// Sanity-check register width against W2.
		p.checkRegisterWidth(r)
		//
		out[i] = descriptor.NewRegister(r.Kind(), r.Name(), r.Bitwidth(), p.convertConstant(r.Padding()))
	}
	//
	return out
}

func (p programToProgram[W1, W2]) lowerVector(v bytecode.Vector[W1]) bytecode.Vector[W2] {
	var codes = make([]bytecode.Bytecode[W2], len(v.Bytecodes))
	//
	for i, b := range v.Bytecodes {
		codes[i] = p.lowerBytecode(b)
	}
	//
	return bytecode.NewVector(codes...)
}

func (p programToProgram[W1, W2]) lowerBytecode(b bytecode.Bytecode[W1]) bytecode.Bytecode[W2] {
	switch b := b.(type) {
	// Word-typed bytecodes carry a W-typed constant and must be re-typed.
	case *bytecode.Arith[W1]:
		return bytecode.NewArith(b.Op, b.Target, b.Source, p.convertConstant(b.Constant))
	case *bytecode.FieldArith[W1]:
		return bytecode.NewFieldArith(b.Op, b.Target, b.Sources, p.convertConstant(b.Constant))
	case *bytecode.Switch[W1]:
		return bytecode.MultiwaySkip(b.Source, p.convertCases(b.Cases))
	// All remaining bytecodes carry no W-typed value, but their type is
	// parameterised over W nonetheless, so each must be re-expressed as its W2
	// instantiation (copying its word-agnostic fields verbatim).
	case *bytecode.Bitwise[W1]:
		return &bytecode.Bitwise[W2]{Op: b.Op, Target: b.Target, Left: b.Left, Right: b.Right, Bitwidth: b.Bitwidth}
	case *bytecode.Call[W1]:
		return &bytecode.Call[W2]{Target: b.Target, Arguments: b.Arguments, Returns: b.Returns}
	case *bytecode.Cat[W1]:
		return &bytecode.Cat[W2]{Targets: b.Targets, Sources: b.Sources}
	case *bytecode.UintToField[W1]:
		return &bytecode.UintToField[W2]{Target: b.Target, Source: b.Source}
	case *bytecode.FieldToUint[W1]:
		return &bytecode.FieldToUint[W2]{Target: b.Target, Source: b.Source}
	case *bytecode.CheckCast[W1]:
		return &bytecode.CheckCast[W2]{Bitwidth: b.Bitwidth, Target: b.Target}
	case *bytecode.Debug[W1]:
		return &bytecode.Debug[W2]{Chunks: b.Chunks, Sources: b.Sources}
	case *bytecode.DivRem[W1]:
		return &bytecode.DivRem[W2]{Opcode: b.Opcode, Target: b.Target, Dividend: b.Dividend, Divisor: b.Divisor}
	case *bytecode.Fail[W1]:
		return &bytecode.Fail[W2]{Chunks: b.Chunks, Sources: b.Sources}
	case *bytecode.Intrinsic[W1]:
		return &bytecode.Intrinsic[W2]{Op: b.Op, Targets: b.Targets, Sources: b.Sources}
	case *bytecode.Jmp[W1]:
		return &bytecode.Jmp[W2]{Target: b.Target}
	case *bytecode.ReadWrite[W1]:
		if b.Write {
			return bytecode.NewMemWrite[W2](b.Id, b.Address, b.Data, b.Stamp)
		}
		//
		return bytecode.NewMemRead[W2](b.Id, b.Address, b.Data, b.Stamp)
	case *bytecode.Ret[W1]:
		return &bytecode.Ret[W2]{}
	case *bytecode.Skip[W1]:
		return &bytecode.Skip[W2]{Skip: b.Skip}
	case *bytecode.SkipIf[W1]:
		right := p.convertOperandVector(b.Right)
		return &bytecode.SkipIf[W2]{Skip: b.Skip, Left: b.Left, Right: right, Op: b.Op}
	case *bytecode.Dispatch[W1]:
		return &bytecode.Dispatch[W2]{Cases: b.Cases, Default: b.Default}
	default:
		panic("unknown bytecode")
	}
}

func (p programToProgram[W1, W2]) convertCases(cases []bytecode.SwitchCase[W1]) []bytecode.SwitchCase[W2] {
	var out = make([]bytecode.SwitchCase[W2], len(cases))
	//
	for i, c := range cases {
		out[i] = bytecode.SwitchCase[W2]{Value: p.convertConstant(c.Value), Skip: c.Skip}
	}
	//
	return out
}

func (p programToProgram[W1, W2]) convertContents(contents []W1) []W2 {
	var out = make([]W2, len(contents))
	//
	for i, c := range contents {
		out[i] = p.convertConstant(c)
	}
	//
	return out
}

func (p programToProgram[W1, W2]) convertOperandVector(o bytecode.Operand[W1]) bytecode.Operand[W2] {
	if o.IsRegisterVector() {
		// Easy case
		return bytecode.NewRegisterVectorOperand[W2](o.AsRegisterVector())
	}
	//
	var (
		constants  = o.AsConstants()
		nconstants = make([]W2, len(constants))
	)
	//
	for i, c := range constants {
		nconstants[i] = p.convertConstant(c)
	}
	//
	return bytecode.NewConstantOperand(nconstants...)
}

func (p programToProgram[W1, W2]) convertConstant(c W1) W2 {
	var w2 W2
	//
	if !c.FitsWithin(p.bandwidth) {
		panic(fmt.Sprintf("constant 0x%s exceeds u%d bandwidth", c.Text(16), p.bandwidth))
	}
	//
	return w2.SetBigInt(c.BigInt())
}

func (p programToProgram[W1, W2]) checkRegisterWidth(r descriptor.Register[W1]) {
	if bw := r.Bitwidth(); bw.HasValue() && bw.Unwrap() > p.bandwidth {
		panic(fmt.Sprintf("\"%s\" exceeds max register width (u%d vs u%d)", r.Name(), bw.Unwrap(), p.bandwidth))
	}
}
