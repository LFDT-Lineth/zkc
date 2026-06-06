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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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

// Label returns a suitable index for representing the given label within a
// branching instruction.
func (p *Encoder[W, T]) Label(label T) uint32 {
	return p.getLabelIndex(label)
}

// Len returns the length of the bytecode sequence encoded thus far.
func (p *Encoder[W, T]) Len() uint {
	return uint(len(p.bytecodes))
}

// Add encodes an integer addition instruction.
func (p *Encoder[W, T]) Add(bc Bytecode) {
	p.bytecodes = append(p.bytecodes, bc)
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

func asReg(rid register.Id) Reg {
	if rid.Unwrap() > math.MaxUint16 {
		panic("invalid register id")
	}
	//
	return uint16(rid.Unwrap())
}

func asRegs(rids ...register.Id) []Reg {
	return array.Map(rids, func(_ uint, r register.Id) Reg {
		return asReg(r)
	})
}
