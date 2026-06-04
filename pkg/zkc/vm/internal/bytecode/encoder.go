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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

type Encoder[T comparable] struct {
	bytecodes []Bytecode
	//
	labels map[T]uint
	//
	marks []uint
	//
	symbols map[uint]string
}

// Encode returns the final encoded bytecode sequence.  Observe that, once this
// is done, the internal bytecode sequence is reset.
func (p *Encoder[T]) Encode() Program {
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
func (p *Encoder[T]) MarkLabel(label T) {
	var index uint = p.getLabelIndex(label)
	//
	p.marks[index] = uint(len(p.bytecodes))
}

// MarkSymbol marks a given symbol at the current position.
func (p *Encoder[T]) MarkSymbol(symbol string) {
	var offset = uint(len(p.bytecodes))
	//
	if p.symbols == nil {
		p.symbols = make(map[uint]string)
	}
	//
	p.symbols[offset] = symbol
}

// Len returns the length of the bytecode sequence encoded thus far.
func (p *Encoder[T]) Len() uint {
	return uint(len(p.bytecodes))
}

// Add encodes an integer addition instruction.
func (p *Encoder[T]) Add(rs0, rs1, rd register.Id) {
	panic("todo")
}

// Call encodes a CALL instruction to a given label.
func (p *Encoder[T]) Call(label uint, inputs uint) {
	panic("todo")
}

// Fail encodes a FAIL instruction to a given label.
func (p *Encoder[T]) Fail() {
	p.bytecodes = append(p.bytecodes, &Fail{})
}

// Jmp encodes a JMP instruction to a given label.
func (p *Encoder[T]) Jmp(label T) {
	var index uint = p.getLabelIndex(label)
	//
	p.bytecodes = append(p.bytecodes, &Jmp{index})
}

// JmpIf encodes a JIF instruction to a given label.
func (p *Encoder[T]) JmpIf(label T, op opcode.Condition, rs0, rs1 register.Id) {
	var (
		index uint = p.getLabelIndex(label)
	)
	// sanity checks
	checkRegisterId(rs0, "rs0")
	checkRegisterId(rs1, "rs1")
	//
	p.bytecodes = append(p.bytecodes, &Jif{op, rs0, rs1, index})
}

// Move encodes an register move instruction.
func (p *Encoder[T]) Move(rs, rd register.Id) {
	// sanity checks
	checkRegisterId(rs, "rs")
	checkRegisterId(rd, "rd")
	//
	p.bytecodes = append(p.bytecodes, &Move{rs, rd})
}

// Ret encodes a RET instruction from a given function call.
func (p *Encoder[T]) Ret(ninputs, width uint) {
	p.bytecodes = append(p.bytecodes, &Ret{})
}

func (p *Encoder[T]) getLabelIndex(label T) uint {
	var (
		index uint
		ok    bool
	)
	// Initialise labels (if not done already)
	if p.labels == nil {
		p.labels = make(map[T]uint)
	}
	// Check whether label encountered already
	if index, ok = p.labels[label]; !ok {
		// No, first time so allocate new label
		index = uint(len(p.labels))
		// Ensure space for the mark
		p.marks = append(p.marks, 0)
		// Record index to avoid reallocation
		p.labels[label] = index
	}
	//
	return index
}

func checkRegisterId(id register.Id, name string) {
	// sanity checks
	if id.Unwrap() >= 256 {
		panic(fmt.Sprintf("%s register out of bounds", name))
	}
}

func patchBranchTargets(bytecodes []Bytecode, labels []uint) {
	for i := range bytecodes {
		bytecodes[i].Patch(labels)
	}
}

func encode(bytecodes []Bytecode) (codes []uint32) {
	var offset uint
	//
	for _, bytecode := range bytecodes {
		var cs = bytecode.Codes(offset)
		//
		codes = append(codes, cs...)
		offset += uint(len(cs))
	}
	//
	return codes
}
