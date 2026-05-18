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
package vm

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/poly"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	finsn "github.com/consensys/go-corset/pkg/zkc/vm/instruction/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Monomial is a useful alias
type Monomial = finsn.Monomial

// Polynomial is a useful alias
type Polynomial = finsn.Polynomial

// SystemMap is a useful alias
type SystemMap = instruction.SystemMap

// LowerWordMachine translates a machine over integer words into a machine over
// field elements.  In order to do this, it must "compile out" various
// high-level word operations (e.g. bitwise operations, division, etc) which
// have no direct correspondance within a field machine.
func LowerWordMachine[W word.Word[W], F field.Element[F]](cfg field.Config, wm *WordMachine[W]) (fm *FieldMachine[F]) {
	var (
		modules  = make([]Module, len(wm.Modules()))
		lowering = wordToField[W, F]{cfg}
	)
	//
	for i, m := range wm.Modules() {
		var (
			name = trace.ModuleName{Name: m.Name(), Multiplier: 1}
			//
			regMap = register.ArrayMap(name, m.Registers()...)
			// system map is useful for deubugging
			sysMap = instruction.NewSystemMap(regMap, wm.Modules())
		)
		//
		modules[i] = lowering.lowerWordModule(m, sysMap)
	}
	//
	return machine.NewField[F](modules...)
}

type wordToField[W word.Word[W], F field.Element[F]] struct {
	field field.Config
}

func (p wordToField[W, F]) lowerWordModule(wm Module, mapping SystemMap) (fm Module) {
	switch wm := wm.(type) {
	case *WordFunction:
		return p.lowerWordFunction(wm, mapping)
	case memory.Memory[W]:
		return p.lowerWordMemory(wm)
	default:
		panic(fmt.Sprintf("unknown word module \"%s\" encountered", wm.Name()))
	}
}

func (p wordToField[W, F]) lowerWordMemory(wf memory.Memory[W]) (ff memory.Memory[F]) {
	var (
		regs = slices.Clone(wf.Registers())
	)
	// Lower registers
	checkRegisterWidths(p.field.RegisterWidth, regs...)
	//
	switch wf := wf.(type) {
	case *memory.ReadOnly[W]:
		// Lower contents
		var contents = p.lowerMemoryContents(wf.Contents())
		// Done
		return NewInputMemory(wf.Name(), wf.IsPublic(), regs, contents...)
	case *memory.StaticReadOnly[W]:
		// Lower contents
		var contents = p.lowerMemoryContents(wf.Contents())
		// Done
		return NewStaticMemory(wf.Name(), wf.IsPublic(), regs, contents...)
	case *memory.WriteOnce[W]:
		return NewOutputMemory[F](wf.Name(), wf.IsPublic(), regs)
	case *memory.RandomAccess[W]:
		return NewReadWriteMemory[F](wf.Name(), regs)
	case *memory.BiPartiteRandomAccess[W]:
		return NewLargeReadWriteMemory[F](wf.Name(), regs)
	default:
		panic(fmt.Sprintf("unknown word memory %s", wf.Name()))
	}
}

func (p wordToField[W, F]) lowerMemoryContents(contents []W) []F {
	var ncontents = make([]F, len(contents))
	//
	for i, w := range contents {
		var f F
		// Copy over value
		f = f.SetBytes(w.BigInt().Bytes())
		// Write back
		ncontents[i] = f
	}
	//
	return ncontents
}

func (p wordToField[W, F]) lowerWordFunction(wf *WordFunction, mapping SystemMap) *FieldFunction {
	var (
		regs  = slices.Clone(wf.Registers())
		insns = make([]Vector[FieldInstruction], len(wf.Code()))
	)
	// Lower registers
	checkRegisterWidths(p.field.RegisterWidth, regs...)
	// Lower instructions
	for i, insn := range wf.Code() {
		insns[i] = p.lowerWordVector(insn, mapping)
	}
	//
	return NewFunction(wf.Name(), wf.IsNative(), regs, insns)
}

func (p wordToField[W, F]) lowerWordVector(wi Vector[WordInstruction], mapping SystemMap) Vector[FieldInstruction] {
	var (
		insns = make([]FieldInstruction, len(wi.Codes))
	)
	//
	for i, insn := range wi.Codes {
		insns[i] = p.lowerWordInstruction(insn, mapping)
	}
	//
	return instruction.NewVector(insns...)
}

func (p wordToField[W, F]) lowerWordInstruction(wi WordInstruction, mapping SystemMap) FieldInstruction {
	switch wi.OpCode() {
	// Base instructions translate directly as is.
	case opcode.CALL:
		return wi.(*instruction.Call)
	case opcode.DEBUG:
		return wi.(*instruction.Debug)
	case opcode.FAIL:
		return wi.(*instruction.Fail)
	case opcode.JUMP:
		return wi.(*instruction.Jump)
	case opcode.MEMORY_READ:
		return wi.(*instruction.MemRead)
	case opcode.MEMORY_WRITE:
		return wi.(*instruction.MemWrite)
	case opcode.RETURN:
		return wi.(*instruction.Return)
	case opcode.SKIP:
		return wi.(*instruction.Skip)
	case opcode.SKIP_IF:
		// NOTE: vector.BranchTable will panic if the skip condition is
		// something other than EQ/NEQ.
		var insn = wi.(*instruction.SkipIf)
		// Done
		return insn
	case opcode.BIT_CONCAT:
		var insn = wi.(*instruction.BitConcat[W])
		return p.lowerBitwiseConcatenation(insn.Target, insn.Sources, mapping)
	case opcode.HINT_DIVISION:
		return wi.(*instruction.FieldHint)
	case opcode.INT_CAST:
		var insn = wi.(*instruction.Cast)
		//
		return p.lowerCastInstruction(insn.Target, insn.Source)
	case opcode.INT_ADD:
		var insn = wi.(*instruction.IntAdd[W])
		return p.lowerArithInstruction(insn.Target, insn.Sources, insn.Constant, sum)
	case opcode.INT_SUB:
		var insn = wi.(*instruction.IntSub[W])
		return p.lowerArithInstruction(insn.Target, insn.Sources, insn.Constant, subtract)
	case opcode.INT_MUL:
		var insn = wi.(*instruction.IntMul[W])
		return p.lowerArithInstruction(insn.Target, insn.Sources, insn.Constant, product)
	case opcode.INT_ADDMOD_P:
		var insn = wi.(*instruction.IntAddModP[W])
		return p.lowerFieldInstruction(insn.Target, insn.Sources, insn.Constant, sum)
	case opcode.INT_SUBMOD_P:
		var insn = wi.(*instruction.IntSubModP[W])
		return p.lowerFieldInstruction(insn.Target, insn.Sources, insn.Constant, subtract)
	case opcode.INT_MULMOD_P:
		var insn = wi.(*instruction.IntMulModP[W])
		return p.lowerFieldInstruction(insn.Target, insn.Sources, insn.Constant, product)
	default:
		panic(fmt.Sprintf("unknown instruction encountered (%s)", wi.String(mapping)))
	}
}

type airConstructor[F field.Element[F]] func(...Monomial) Polynomial

func (p wordToField[W, F]) lowerArithInstruction(lhs register.Id, rhs []register.Id, c W,
	f airConstructor[F]) (fi FieldInstruction) {
	//
	var (
		one   = big.NewInt(1)
		zero  W
		terms = make([]Monomial, len(rhs))
	)
	// Construct register accesses as necessary
	for i, r := range rhs {
		terms[i] = poly.NewMonomial(*one, r)
	}
	// Add constant (if applicable)
	if n := c.BigInt(); n.BitLen() > int(p.field.RegisterWidth) {
		panic(fmt.Sprintf("constant exceeds max register width (u%d vs u%d)", n.BitLen(), p.field.RegisterWidth))
	} else if c.Cmp(zero) != 0 {
		// var c F
		// // Convert from word value to field element
		// c = c.SetBytes(n.Bytes())
		// Append field constant
		terms = append(terms, poly.NewMonomial[register.Id](*n))
	}
	// Done
	return instruction.NewFieldAssign[F](lhs, f(terms...))
}

func (p wordToField[W, F]) lowerFieldInstruction(lhs register.Id, rhs []register.Id, c W,
	f airConstructor[F]) (fi FieldInstruction) {
	//
	var (
		one   = big.NewInt(1)
		mod   F
		zero  W
		terms = make([]Monomial, len(rhs))
	)
	// Construct register accesses as necessary
	for i, r := range rhs {
		terms[i] = poly.NewMonomial(*one, r)
	}
	// Add constant (if applicable)
	if n := c.BigInt(); n.Cmp(mod.Modulus()) >= 0 {
		panic(fmt.Sprintf("constant exceeds field prime (0x%s vs 0x%s)", n.Text(16), mod.Modulus().Text(16)))
	} else if c.Cmp(zero) != 0 {
		//var c F
		// Convert from word value to field element
		//c = c.SetBytes(n.Bytes())
		// Append field constant
		terms = append(terms, poly.NewMonomial[register.Id](*n))
	}
	// Done
	return instruction.NewFieldAssign[F](lhs, f(terms...))
}

func (p wordToField[W, F]) lowerBitwiseConcatenation(lhs register.Id, rhs []register.Id, mapping instruction.SystemMap,
) (fi FieldInstruction) {
	var (
		terms = make([]Monomial, len(rhs))
		acc   = big.NewInt(1)
	)
	//
	for i := range len(rhs) {
		var (
			ith   = rhs[i]
			width = mapping.Register(ith).Width()
			coeff big.Int
		)
		//
		coeff.Set(acc)
		//
		terms[i] = poly.NewMonomial(coeff, ith)
		// Shift left accumulate by bitwidth
		acc = acc.Lsh(acc, width)
	}
	//
	return instruction.NewFieldAssign[F](lhs, sum(terms...))
}

func (p wordToField[W, F]) lowerCastInstruction(lhs register.Id, rhs register.Id) (fi FieldInstruction) {
	var (
		one = big.NewInt(1)
		e   Polynomial
	)
	//
	return instruction.NewFieldAssign[F](lhs, e.Set(poly.NewMonomial(*one, rhs)))
}

func checkRegisterWidths(registerWidth uint, regs ...register.Register) {
	// Lower registers
	for _, reg := range regs {
		// sanity check register width
		if !reg.IsNative() && reg.Width() > registerWidth {
			panic(fmt.Sprintf("\"%s\" exceeds max register width (u%d vs u%d)",
				reg.Name(), reg.Width(), registerWidth))
		}
	}
}

func sum(terms ...Monomial) Polynomial {
	var p Polynomial
	// Initialise polynomial
	return p.Set(terms...)
}

func subtract(terms ...Monomial) Polynomial {
	panic("todo")
}

func product(terms ...Monomial) Polynomial {
	var (
		p Polynomial
		m Monomial
	)
	//
	for i, t := range terms {
		if i == 0 {
			m = t
		} else {
			m = m.Mul(t)
		}
	}
	//
	return p.Set(m)
}
