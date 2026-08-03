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
	"fmt"
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// These constants enumerate the possible kinds of Symbol stored in a
// SymbolTable.  The kind determines how a symbol's Offset should be
// interpreted: for FUNCTION_SYMBOL it is a bytecode address, whilst for the
// memory kinds it is a memory-specific identifier (i.e. an index amongst all
// memories of that kind).
const (
	// FUNCTION_SYMBOL identifies a program point within a function; its Offset
	// is the address of the corresponding instruction in the compiled bytecode.
	FUNCTION_SYMBOL uint8 = iota
	// STATIC_MEMORY identifies a static (compile-time initialised) read-only
	// memory.
	STATIC_MEMORY
	// READONLY_MEMORY identifies a read-only memory.
	READONLY_MEMORY
	// WRITEONCE_MEMORY identifies a write-once memory.
	WRITEONCE_MEMORY
	// READWRITE_MEMORY identifies a (random access) read-write memory.
	READWRITE_MEMORY
	// PAGED_READWRITE_MEMORY identifies a paged (random access) read-write
	// memory.
	PAGED_READWRITE_MEMORY
)

// ProgramPoint provides a convenient alias.
type ProgramPoint = descriptor.ProgramPoint

// Symbol describes the resolved location of a labelled entity (a function
// program point or a memory) within the compiled program.
type Symbol struct {
	// Kind distinguishes the entity this symbol refers to (e.g.
	// FUNCTION_SYMBOL, READONLY_MEMORY, etc.), and hence how Offset should be
	// interpreted.
	Kind uint8
	// Offset is the resolved value of this symbol.  For FUNCTION_SYMBOL it is a
	// bytecode address; for memory kinds it is a memory-specific identifier.
	Offset Address
}

// NewSymbol constructs a Symbol of the given kind with the given offset.
func NewSymbol(kind uint8, offset Address) Symbol {
	return Symbol{kind, offset}
}

// Label uniquely identifies a labelled entity within the program, and serves
// as the key under which its resolved Symbol is stored in a SymbolTable.
type Label struct {
	// ModuleId identifies the enclosing module (function or memory) of this
	// label.
	ModuleId uint16
	// Point identifies the program point for this label.  For memory modules,
	// this is always (0,0).
	Point ProgramPoint
}

// String returns a human-readable representation of this label of the form
// "module:(macro,micro)".
func (p Label) String() string {
	return fmt.Sprintf("%d:(%d,%d)", p.ModuleId, p.Point.Macro, p.Point.Micro)
}

// SymbolTable records the information needed to encode (and later interpret) a
// compiled program.  Specifically, it holds the program's modules, the
// formatted chunks referenced by certain instructions, and the mapping from
// labels to their resolved symbols.  It is populated incrementally during
// compilation as branch targets and memory identifiers are resolved.
type SymbolTable[W word.Word[W]] struct {
	// modules are the functions and memories making up the program, indexed by
	// ModuleId.
	modules []descriptor.Module[W]
	// chunks holds formatted chunks referenced (by index) from certain
	// instructions, such as failures and debug statements.
	chunks [][]bytecode.FormattedChunk
	// constants is the constant pool: words referenced (by index) from
	// instructions whose constant operand does not fit inline, in place of the
	// constant itself.
	constants []W
	// cindex indexes the constant pool for interning, keyed on the constant's
	// (big-endian) byte representation.
	cindex map[string]uint16
	// cvindex indexes consecutive runs of pool entries for interning constant
	// vectors, keyed on the (length-prefixed) byte representations of their
	// elements and mapping to the pool index of the first element.
	cvindex map[string]uint16
	// mapping maps either: (i) labels to their bytecode addresses; (ii)
	// memories to their memory-specific identifiers.  This allows one, for
	// example, to determine the starting address of a function.
	mapping map[Label]Symbol
}

// NewSymbolTable constructs an (initially empty) symbol table for the given
// modules.
func NewSymbolTable[W word.Word[W]](modules ...descriptor.Module[W]) SymbolTable[W] {
	return SymbolTable[W]{
		modules, nil, nil,
		make(map[string]uint16), make(map[string]uint16),
		make(map[Label]Symbol),
	}
}

// ChunksIndex returns the index identifying the given sequence of formatted
// chunks, interning them on first use.  That is, if an identical sequence has
// been seen before its existing index is returned; otherwise, the sequence is
// stored and a fresh index allocated.
func (p *SymbolTable[W]) ChunksIndex(chunks ...bytecode.FormattedChunk) uint {
	var n = uint(len(p.chunks))
	//
	for i, cs := range p.chunks {
		if array.Compare(cs, chunks) == 0 {
			return uint(i)
		}
	}
	// Allocate new chunks
	p.chunks = append(p.chunks, chunks)
	//
	return n
}

// ConstantIndex returns the constant pool index identifying the given
// constant, interning it on first use.  That is, if an identical constant has
// been seen before its existing index is returned; otherwise, the constant is
// stored and a fresh index allocated.  Interning is idempotent, which matters
// because encoding runs several times whilst the layout reaches a fixpoint.
func (p *SymbolTable[W]) ConstantIndex(constant W) uint16 {
	var key = string(constant.BigInt().Bytes())
	//
	if index, ok := p.cindex[key]; ok {
		return index
	}
	// Sanity check pool capacity, since indices are encoded as u16.
	if len(p.constants) > math.MaxUint16 {
		panic(fmt.Sprintf("too many constant pool entries (%d)", len(p.constants)))
	}
	// Allocate new constant
	var index = uint16(len(p.constants))
	//
	p.constants = append(p.constants, constant)
	p.cindex[key] = index
	//
	return index
}

// ConstantVectorIndex returns the constant pool index of the first element of
// the given constant vector, interning the whole vector as a consecutive run
// of pool entries on first use.  That is, element i of the vector resides at
// pool index ConstantVectorIndex(v)+i.  As for ConstantIndex, interning is
// idempotent.
func (p *SymbolTable[W]) ConstantVectorIndex(constants []W) uint16 {
	var sb strings.Builder
	// NOTE: elements are length-prefixed to keep the key unambiguous.
	for _, c := range constants {
		var bytes = c.BigInt().Bytes()
		//
		sb.WriteByte(byte(len(bytes) >> 8))
		sb.WriteByte(byte(len(bytes)))
		sb.Write(bytes)
	}
	//
	var key = sb.String()
	//
	if index, ok := p.cvindex[key]; ok {
		return index
	}
	// Sanity check pool capacity, since indices are encoded as u16.
	if len(p.constants)+len(constants) > math.MaxUint16+1 {
		panic(fmt.Sprintf("too many constant pool entries (%d)", len(p.constants)+len(constants)))
	}
	// Allocate new (consecutive) run of constants.
	var index = uint16(len(p.constants))
	//
	p.constants = append(p.constants, constants...)
	p.cvindex[key] = index
	// Record any element not already interned individually, allowing
	// subsequent singleton lookups to reuse this run's entries.
	for i, c := range constants {
		var ckey = string(c.BigInt().Bytes())
		//
		if _, ok := p.cindex[ckey]; !ok {
			p.cindex[ckey] = index + uint16(i)
		}
	}
	//
	return index
}

// Constants returns the constant pool.
func (p *SymbolTable[W]) Constants() []W {
	return p.constants
}

// EnvironmentFor returns an Environment which views this symbol table from the
// perspective of the module with the given identifier (i.e. as the enclosing
// function during encoding).
func (p *SymbolTable[W]) EnvironmentFor(id ModuleId, point ProgramPoint) Environment[W] {
	return Environment[W]{p, id, point}
}

// Insert records (or overwrites) the symbol associated with the given label.
func (p *SymbolTable[W]) Insert(lab Label, symbol Symbol) {
	p.mapping[lab] = symbol
}

// HasSymbol determines whether a symbol has been recorded for the given label.
func (p *SymbolTable[W]) HasSymbol(lab Label) bool {
	_, ok := p.mapping[lab]
	//
	return ok
}

// Module returns the module with the given identifier.
func (p *SymbolTable[W]) Module(id ModuleId) descriptor.Module[W] {
	return p.modules[id]
}

// Modules returns all modules (functions and memories) making up the program.
func (p *SymbolTable[W]) Modules() []descriptor.Module[W] {
	return p.modules
}

// SymbolAt returns the symbol recorded for the given label.  The zero Symbol is
// returned if no such label has been recorded.
func (p *SymbolTable[W]) SymbolAt(lab Label) Symbol {
	return p.mapping[lab]
}

// Environment provides a view of a SymbolTable from within a specific
// (enclosing) module, supplying the context required to encode that module's
// instructions.  In particular, it allows branch targets within the enclosing
// function to be resolved without having to thread the module identifier
// through explicitly.
type Environment[W word.Word[W]] struct {
	*SymbolTable[W]
	// enclosing identifies the function within which encoding is currently
	// taking place.
	enclosing ModuleId
	// point identifies the program point of the current instruction.
	point ProgramPoint
}

// Point returns the program point for the current bytecode
func (p *Environment[W]) Point() ProgramPoint {
	return p.point
}

// OffsetFor determines the encoded offset for the given program point in the
// enclosing function.
func (p *Environment[W]) OffsetFor(id ModuleId, pp ProgramPoint) uint32 {
	var lab = Label{id, pp}
	return p.mapping[lab].Offset
}

// RegisterMap returns a register map for the enclosing module
func (p *Environment[W]) RegisterMap() descriptor.RegisterMap[W] {
	return p.modules[p.enclosing]
}
