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
package bytecode

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

type Encoder[W word.Word[W], T comparable] struct {
	bytecodes []Bytecode
	//
	labels map[T]uint32
	//
	marks []Address
	//
	symbols map[uint32]string
}

// Encode returns the final encoded bytecode sequence.  Observe that, once this
// is done, the internal bytecode sequence is reset.
func (p *Encoder[W, T]) Encode() Program {
	var symbols = p.symbols
	// patch branch targets
	patchBranchTargets(p.bytecodes, p.marks)
	// encode bytecodes
	bytecodes := encode(p.bytecodes)
	// Reset internal buffers
	p.bytecodes = nil
	p.labels = nil
	p.symbols = nil
	p.marks = nil
	// Done
	return Program{bytecodes, symbols}
}

// MarkLabel current position with given label.
func (p *Encoder[W, T]) MarkLabel(label T) {
	var index = p.getLabelIndex(label)
	//
	p.marks[index] = uint32(len(p.bytecodes))
}

// MarkSymbol marks a given symbol at the current position.
func (p *Encoder[W, T]) MarkSymbol(symbol string) {
	var offset = uint32(len(p.bytecodes))
	//
	if p.symbols == nil {
		p.symbols = make(map[uint32]string)
	}
	//
	p.symbols[offset] = symbol
}

// Len returns the length of the bytecode sequence encoded thus far.
func (p *Encoder[W, T]) Len() uint {
	return uint(len(p.bytecodes))
}

// Add encodes an integer addition instruction.
func (p *Encoder[W, T]) Add(src []Reg, dst []Reg) {
	var zero W
	p.AddConst(zero, src, dst)
}

// AddConst encodes an integer addition instruction involving a constant and
// register.
func (p *Encoder[W, T]) AddConst(constant W, src []Reg, dst []Reg) {
	p.bytecodes = append(p.bytecodes, &Add[W]{Constant: constant, Source: src, Target: dst})
}

// Call encodes a CALL instruction to a given label.
func (p *Encoder[W, T]) Call(label uint, inputs uint) {
	panic("todo")
}

// Fail encodes a FAIL instruction to a given label.
func (p *Encoder[W, T]) Fail() {
	p.bytecodes = append(p.bytecodes, &Fail{})
}

// LoadConst encodes an LDC instruction.
func (p *Encoder[W, T]) LoadConst(constant W, rd Reg) {
	p.AddConst(constant, nil, []Reg{rd})
}

// Jmp encodes a JMP instruction to a given label.
func (p *Encoder[W, T]) Jmp(label T) {
	var index = p.getLabelIndex(label)
	//
	p.bytecodes = append(p.bytecodes, &Jmp{index})
}

// JmpIf encodes a JIF instruction to a given label.
func (p *Encoder[W, T]) JmpIf(label T, op Cond, rs0, rs1 Reg) {
	var (
		index uint32 = p.getLabelIndex(label)
	)
	// sanity checks
	checkRegisterId(rs0, "rs0")
	checkRegisterId(rs1, "rs1")
	//
	p.bytecodes = append(p.bytecodes, &Jif{Target: index, Src0: rs0, Src1: rs1, Op: op})
}

// ReadRam encodes a LDRAM instruction from a given function call.
func (p *Encoder[W, T]) ReadRam(mid uint16, rs Reg, slot uint8, rd Reg) {
	panic("todo")
}

// ReadRom encodes a LDROM instruction from a given function call.
func (p *Encoder[W, T]) ReadRom(mid uint16, rs Reg, slot uint8, rd Reg) {
	panic("todo")
}

// Ret encodes a RET instruction from a given function call.
func (p *Encoder[W, T]) Ret(ninputs, width uint) {
	p.bytecodes = append(p.bytecodes, &Ret{})
}

func (p *Encoder[W, T]) getLabelIndex(label T) uint32 {
	var (
		index uint32
		ok    bool
	)
	// Initialise labels (if not done already)
	if p.labels == nil {
		p.labels = make(map[T]Address)
	}
	// Check whether label encountered already
	if index, ok = p.labels[label]; !ok {
		// No, first time so allocate new label
		index = uint32(len(p.labels))
		// Ensure space for the mark
		p.marks = append(p.marks, 0)
		// Record index to avoid reallocation
		p.labels[label] = index
	}
	//
	return index
}

func checkRegisterId(id Reg, name string) {
	// sanity checks
	if id >= 256 {
		panic(fmt.Sprintf("%s register out of bounds", name))
	}
}

func patchBranchTargets(bytecodes []Bytecode, labels []Address) {
	for _, b := range bytecodes {
		if b, ok := b.(Patchable); ok {
			b.Patch(labels)
		}
	}
}

func encode(bytecodes []Bytecode) (codes []uint32) {
	var offset uint32
	//
	for _, bytecode := range bytecodes {
		var cs = bytecode.Codes(offset)
		//
		codes = append(codes, cs...)
		offset += uint32(len(cs))
	}
	//
	return codes
}
