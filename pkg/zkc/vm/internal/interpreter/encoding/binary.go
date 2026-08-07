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
package encoding

import (
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Binary represents a compiled program: the symbol table describing its modules
// together with the encoded bytecode sequence ready for execution.
type Binary[W word.Word[W]] struct {
	env SymbolTable[W]
	// Compiled program being executed
	bytecodes []uint32
	// Constant pool referenced (by index) from instructions whose constant
	// operand is not carried inline.
	constants []W
	// Mapping from addresses in the encoded binary program back to their
	// relevant labels.  This can be used to determine the function at a given
	// address and/or the appropriate PC value.
	rmap []Label
}

// NewBinary returns a new Binary[W] initialized with the given parameters.
func NewBinary[W word.Word[W]](env SymbolTable[W], bytecodes []uint32) Binary[W] {
	var rmap = make([]Label, len(bytecodes))
	// NOTE: building the reverse map
	for l, s := range env.mapping {
		if s.Kind == FUNCTION_SYMBOL {
			rmap[s.Offset] = l
		}
	}
	//
	return Binary[W]{env, bytecodes, env.Constants(), rmap}
}

// Bytecodes returns the raw bytecode sequence
func (p Binary[W]) Bytecodes() []uint32 {
	return p.bytecodes
}

// Constants returns the constant pool.
func (p Binary[W]) Constants() []W {
	return p.constants
}

// Chunks returns formatted chunks needed for I/O instructions.
func (p Binary[W]) Chunks(index uint) []bytecode.FormattedChunk {
	return p.env.chunks[index]
}

// HasModule returns the identifier for the module with the given name, or returns
// false if no such module exists.
func (p Binary[W]) HasModule(name string) (uint16, bool) {
	var mid = array.FindMatching(p.env.Modules(), func(m descriptor.Module[W]) bool {
		return m.Name() == name
	})
	//
	if mid > math.MaxUint16 {
		return math.MaxUint16, false
	}
	//
	return uint16(mid), mid != math.MaxUint
}

// Module returns the module with the given identifier.
func (p Binary[W]) Module(mid uint16) descriptor.Module[W] {
	return p.env.Module(mid)
}

// AddressOf determines the address of a given (function) symbol, or returns an
// error if no such symbol exists.
func (p Binary[W]) AddressOf(mid uint16) (Symbol, bool) {
	var lab = Label{mid, ProgramPoint{}}
	//
	if p.env.HasSymbol(lab) {
		return p.env.SymbolAt(lab), true
	}
	//
	return Symbol{}, false
}

// FunctionAt determines whether or not there is a symbol associated with a given
// instruction address.
func (p Binary[W]) FunctionAt(address Address) Label {
	return p.rmap[address]
}

// Modules returns information about the modules declared within this program.
func (p Binary[W]) Modules() []descriptor.Module[W] {
	return p.env.Modules()
}

// Encoding returns the binary encoding of the instruction at the given program
// point within the given module.
func (p Binary[W]) Encoding() (encoded [][]uint32) {
	var (
		marks bit.Set
		last  uint
	)
	// Mark all instruction boundaries
	for _, s := range p.env.mapping {
		// NOTE: only mark function symbols.
		if s.Kind == FUNCTION_SYMBOL && s.Offset != 0 {
			marks.Insert(uint(s.Offset))
		}
	}
	// Mark last offset
	marks.Insert(uint(len(p.bytecodes)))
	// Extract encodings
	for i := marks.Iter(); i.HasNext(); {
		mark := i.Next()
		// Skip first mark as this is always at 0
		encoded = append(encoded, p.bytecodes[last:mark])
		//
		last = mark
	}
	//
	return encoded
}
