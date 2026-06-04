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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

type Encoder[T comparable] struct {
	bytecodes []uint32
	//
	labels map[T]uint
	//
	marks []uint
}

// Encode returns the final encoded bytecode sequence.  Observe that, once this
// is done, the internal bytecode sequence is reset.
func (p *Encoder[T]) Encode() []uint32 {
	var bytecodes = p.bytecodes
	// TODO: patch labels
	// Reset internal buffers
	p.bytecodes = nil
	p.labels = nil
	// Done
	return bytecodes
}

// Mark current position with given label.
func (p *Encoder[T]) Mark(label T) {
	var (
		index uint = p.getLabelIndex(label)
	)
	//
	p.marks[index] = uint(len(p.bytecodes))
}

// Len returns the length of the bytecode sequence encoded thus far.
func (p *Encoder[T]) Len() uint {
	return uint(len(p.bytecodes))
}

// Fail encodes a FAIL instruction to a given label.
func (p *Encoder[T]) Fail() {
	p.bytecodes = append(p.bytecodes, FAIL)
}

// Jmp encodes a JMP instruction to a given label.
func (p *Encoder[T]) Jmp(label T) {
	var (
		insn  uint32
		index uint = p.getLabelIndex(label)
	)
	//
	insn = (uint32(index) << 5) | JMP
	//
	p.bytecodes = append(p.bytecodes, insn)
}

// JmpIf encodes a JIF instruction to a given label.
func (p *Encoder[T]) JmpIf(label T, condition opcode.Condition, left, right register.Id) {
	var (
		insn, opcode uint32
		l                 = uint32(left.Unwrap())
		r                 = uint32(right.Unwrap())
		index        uint = p.getLabelIndex(label)
	)
	// sanity checks
	if l >= 256 {
		panic("left register out of bounds")
	} else if r >= 256 {
		panic("right register out of bounds")
	}
	//
	opcode = (uint32(condition) << 5) | JIF
	insn = (uint32(index) << 24) | (l << 16) | (r << 8) | opcode
	//
	p.bytecodes = append(p.bytecodes, insn)
}

// Call encodes a CALL instruction to a given label.
func (p *Encoder[T]) Call(label uint, inputs uint) {
	panic("todo")
}

// Ret encodes a RET instruction from a given function call.
func (p *Encoder[T]) Ret(ninputs, width uint) {
	panic("todo")
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
