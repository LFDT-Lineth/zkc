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

// Encoder is used for encoding bytecode sequences into compiled bytecode programs.
type Encoder[W word.Word[W], T comparable] struct {
	modules []Module
	// map memory module identifiers to their memory-specific identifies (e.g.
	// for ROMs or WOMs, etc).  For example, if we had modules [fn0,rom0,rom1],
	// then the memmap would map [0=>0,1=>0,2=>1].
	memmap []MemoryId
	//
	bytecodes []Bytecode[W]
	// Labels maps a given label to a bytecode offset.  These used by the
	// encoded to construct branch targets.  Specifically, when creating a
	// branch instruction we first create a symbol label to represents its
	// target.  When the actuall target address is known, we patch the bytecode
	// instruction with the final target by looking it up in this map.
	labels map[T]uint
	//
	marks []Address
	// Symbols maps bytecode offsets (i.e. addresses) to their corresponding
	// entry in the modules array.  This mechanism allows us to associate a
	// position in the bytecode sequence with a given module (i.e. a function).
	symbols map[Address]uint
}

// NewEncoder constructs a new bytecode encoder.
func NewEncoder[W word.Word[W], T comparable](modules ...Module) *Encoder[W, T] {
	// Construct the memory map
	var memmap = buildMemoryMap[W](modules...)
	//
	return &Encoder[W, T]{modules, memmap, nil, nil, nil, nil}
}

// Encode returns the final encoded bytecode sequence.  Observe that, once this
// is done, the internal bytecode sequence is reset.
func (p *Encoder[W, T]) Encode() Program[W] {
	// encode bytecodes
	bytecodes, symbols := p.compile()
	// Reset internal buffers
	p.bytecodes = nil
	p.labels = nil
	p.symbols = nil
	p.marks = nil
	// Done
	return NewProgram[W](p.modules, bytecodes, nil, symbols)
}

// MarkLabel current position with given label.
func (p *Encoder[W, T]) MarkLabel(label T) {
	var index = p.getLabelIndex(label)
	//
	p.marks[index] = uint32(len(p.bytecodes))
}

// MarkModule marks a given symbol at the current position.
func (p *Encoder[W, T]) MarkModule(mid uint) {
	var offset = uint32(len(p.bytecodes))
	//
	if p.symbols == nil {
		p.symbols = make(map[uint32]uint)
	}
	//
	p.symbols[offset] = mid
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
func (p *Encoder[W, T]) Add(bc Bytecode[W]) {
	p.bytecodes = append(p.bytecodes, bc)
}

func (p *Encoder[W, T]) getLabelIndex(label T) uint32 {
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
	return uint32(index)
}

func (p *Encoder[W, T]) compile() (codes []uint32, symbols map[uint32]uint) {
	var (
		offset  uint32
		mapping []uint32 = determineBytecodeMapping(p.bytecodes)
	)
	// patch branch targets
	patchBranchingBytecodes(mapping, p.bytecodes, p.marks)
	// patch symbols
	symbols = patchSymbols(mapping, p.symbols)
	// TODO: patch functions
	// patch memories
	patchIoBytecodes(p.memmap, p.bytecodes)
	// compile bytecodes into raw words
	for _, bytecode := range p.bytecodes {
		var cs = bytecode.Codes(offset)
		//
		codes = append(codes, cs...)
		offset += uint32(len(cs))
	}
	//
	return codes, symbols
}

// Determine the mapping from bytecode indices to actual bytecode offsets in the
// final compiled sequence.
func determineBytecodeMapping[W word.Word[W]](bytecodes []Bytecode[W]) []uint32 {
	var (
		mapping = make([]Address, len(bytecodes))
		offset  uint32
	)
	// determine true bytecode offsets
	for i, b := range bytecodes {
		mapping[i] = offset
		offset += uint32(len(b.Codes(offset)))
	}
	//
	return mapping
}

// Patch branch instructions to target instruction offsets, rather than labels.
// That is, the target of a branch instruction on entry is an index into the
// label array.  The corresponding label identifies the (bytecode) offset of the
// target instruction.  Observe that the bytecode offset must be converted into
// a true offset.
func patchBranchingBytecodes[W word.Word[W]](mapping []uint32, bytecodes []Bytecode[W], labels []Address) {
	// update labels accordingly
	for i, l := range labels {
		labels[i] = mapping[l]
	}
	// patch instructions
	for _, b := range bytecodes {
		if b, ok := b.(Patchable[W]); ok {
			b.Patch(labels)
		}
	}
}

// Patch memory identifies used in I/O bytecodes to ensure they are on a
// class-by-class basis, rather than a true module identifier.  Specifically,
// bytecodes are created initially with *module* identifiers (i.e. indices into
// the modules array).  However, for encoding, the bytecodes must use their
// class-based identifier.  For example, consider setup where we have the
// following symbols [f,ram,rom0,rom1].  For a read/write bytecode which targets
// rom1, its initial identifier would "3".  However, after patching, its
// identifier is "1" (i.e. the index in [rom0,rom1]).
func patchIoBytecodes[W word.Word[W]](memmap []MemoryId, bytecodes []Bytecode[W]) {
	// patch instructions
	for _, b := range bytecodes {
		if b, ok := b.(*ReadWrite); ok {
			b.Id = memmap[b.Id].index
		}
	}
}

// Patch symbols simply updates the recorded offsets for each symbol to be in
// terms of the final code sequence, rather than the abstract bytecode sequence.
// If every bytecode was to generate one uint32 code in the final sequence,
func patchSymbols(mapping []uint32, symbols map[Address]uint) map[Address]uint {
	var nsyms = make(map[uint32]uint)
	//
	for a, s := range symbols {
		nsyms[mapping[a]] = s
	}
	//
	return nsyms
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
