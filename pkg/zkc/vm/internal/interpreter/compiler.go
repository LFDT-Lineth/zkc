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
package interpreter

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Symbol provides a convenient alias
type Symbol = encoding.Symbol

// Label provides a convenient alias
type Label = encoding.Label

// Address provides a convenient alias
type Address = encoding.Address

// Function provides a convenient alias
type Function[W word.Word[W]] = descriptor.Function[W]

// SymbolTable provides a convenient alias
type SymbolTable[W word.Word[W]] = encoding.SymbolTable[W]

// Environment provides a convenient alias
type Environment[W word.Word[W]] = encoding.Environment[W]

// RegisterId provides a useful alias
type RegisterId = bytecode.RegisterId

// ModuleId provides a useful alias
type ModuleId = bytecode.ModuleId

// CompileProgram compiles a program descriptor into an executable (i.e.
// compiled) bytecode program.  If tracing is enabled then a break point is set
// for the terminal instruction(s) of each bytecode vector.
func CompileProgram[W word.Word[W]](program descriptor.Program[W], tracing bool) encoding.Binary[W] {
	var (
		symtab             = initialiseSymbolTable(program)
		bytecodes, changed = encodeBytecodes(program, &symtab, tracing)
	)
	// Encode the bytecodes
	for changed {
		// continue until we reach a fixed point
		bytecodes, changed = encodeBytecodes(program, &symtab, tracing)
	}
	// Flag every instruction at which a breakpoint has been registered by setting
	// the BREAKPOINT modifier bit on its (resolved) first word.
	for _, bp := range program.BreakPoints() {
		var (
			lab = Label{ModuleId: bp.Function, Point: bp.ProgramCounter}
		)
		//
		bytecodes[symtab.SymbolAt(lab).Offset] |= encoding.BREAKPOINT
	}
	// Done
	return encoding.NewBinary(symtab, bytecodes)
}

// Determine a conservative mapping from bytecode labels to bytecode offsets,
// sizing every (unresolved) branch at its maximal width.  This provides the
// starting point for the fixpoint iteration in compile: branch targets cannot
// be consulted yet (they still hold label indices), and starting from maximal
// sizes ensures the iteration only ever shrinks.
func initialiseSymbolTable[W word.Word[W]](program descriptor.Program[W]) SymbolTable[W] {
	var (
		offset Address
		env    = encoding.NewSymbolTable(program.Modules()...)
		// memory counters
		nsroms, nroms, nwoms, nrams, nprams uint32
	)
	//
	// First pass: insert a symbol for every memory module.  This must happen
	// before any function is sized below, because sizing a function encodes its
	// bytecodes -- including memory reads/writes, whose opcode is derived from
	// the target memory's symbol kind (see encoding.ReadWrite).  A function may
	// reference a memory declared later in module order, so all memory symbols
	// must already be present.
	for i, m := range program.Modules() {
		var mid = util.Cast[uint16](uint(i))
		//
		if m, ok := m.(*descriptor.Memory[W]); ok {
			var (
				symbol encoding.Symbol
				lab    = Label{ModuleId: mid}
			)
			//
			switch {
			case m.IsStatic():
				symbol = encoding.NewSymbol(encoding.STATIC_MEMORY, nsroms)
				nsroms++
			case m.IsReadOnly():
				symbol = encoding.NewSymbol(encoding.READONLY_MEMORY, nroms)
				nroms++
			case m.IsWriteOnly():
				symbol = encoding.NewSymbol(encoding.WRITEONCE_MEMORY, nwoms)
				nwoms++
			case m.IsPaged():
				symbol = encoding.NewSymbol(encoding.PAGED_READWRITE_MEMORY, nprams)
				nprams++
			case m.IsReadWrite():
				symbol = encoding.NewSymbol(encoding.READWRITE_MEMORY, nrams)
				nrams++
			default:
				panic("unknown memory encountered")
			}
			// Mark this label at the given offset
			env.Insert(lab, symbol)
		}
	}
	// Second pass: size each function's program points, now that every memory
	// symbol is available for resolution.
	for i, m := range program.Modules() {
		var mid = util.Cast[uint16](uint(i))
		//
		if m, ok := m.(*descriptor.Function[W]); ok {
			offset = initFunctionPoints(offset, mid, *m, &env)
		}
	}
	// Sanity checks
	checkMemoryCount(nroms, "read only")
	checkMemoryCount(nsroms, "static read-only")
	checkMemoryCount(nwoms, "write once")
	checkMemoryCount(nrams, "(unpaged) random access")
	checkMemoryCount(nprams, "(paged) random access")
	//
	return env
}

// Insert a symbol table entry for every distinct "program point" in the given
// function.  Here, a program point represents the start of an encoded bytecode
// in the compiled sequence.
func initFunctionPoints[W word.Word[W]](offset Address, fid uint16, fn Function[W], env *SymbolTable[W]) Address {
	//
	for pc, vec := range fn.Vectors() {
		for cc, b := range vec.Bytecodes {
			var (
				pp = descriptor.ProgramPoint{Macro: uint(pc), Micro: uint(cc)}
				// construct environment for this point
				fenv = env.EnvironmentFor(fid, pp)
				// Construct label for this program point
				lab    = Label{ModuleId: fid, Point: pp}
				symbol = Symbol{Kind: encoding.FUNCTION_SYMBOL, Offset: offset}
			)
			// Mark this label at the given offset
			env.Insert(lab, symbol)
			//
			// Update mapping with maximum width of bytecode (for now).
			offset += Address(encoding.MaxEncodedLength(b, fenv))
		}
	}
	//
	return offset
}

func encodeBytecodes[W word.Word[W]](prog descriptor.Program[W], symtab *SymbolTable[W], tracing bool,
) (codes []uint32, changed bool) {
	var (
		offset Address
		words  []uint32
	)
	//
	changed = false
	//
	for i, m := range prog.Modules() {
		var (
			mid    = ModuleId(i)
			vwords []uint32
			c      bool
		)
		//
		if f, ok := m.(*descriptor.Function[W]); ok {
			vwords, offset, c = encodeVectors(mid, f.Vectors(), offset, symtab, tracing)
			changed = changed || c
			//
			words = append(words, vwords...)
		}
	}
	//
	return words, changed
}

func encodeVectors[W word.Word[W]](fid ModuleId, vectors []bytecode.Vector[W], offset Address,
	symtab *SymbolTable[W], tracing bool) ([]uint32, Address, bool) {
	//
	var (
		words   []uint32
		changed = false
	)
	//
	for pc, vec := range vectors {
		for cc, b := range vec.Bytecodes {
			var (
				pp = descriptor.ProgramPoint{Macro: uint(pc), Micro: uint(cc)}
				// construct environment for this bytecode
				env = symtab.EnvironmentFor(fid, pp)
				// Construct label representing this program point
				lab = Label{ModuleId: fid, Point: pp}
				// Old symbol config
				symbol = symtab.SymbolAt(lab)
				// Encode given bytecode
				codes = encoding.Encode(b, offset, env)
			)
			// Insert breakpoint if tracing
			if tracing && isVectorTerminal[W](b) {
				codes[0] |= encoding.BREAKPOINT
			}
			//
			words = append(words, codes...)
			// Check whether offset in encoded sequence of this instruction has
			// changed.  If so, we need to update the symbol table and ensure
			// that we recalculate all encodings appropriately.
			if offset != symbol.Offset {
				// Update offset for this instruction
				symtab.Insert(lab, encoding.NewSymbol(symbol.Kind, offset))
				// Force re-encoding of all bytecodes
				changed = true
			}
			// Continue
			offset += uint32(len(codes))
		}
	}
	//
	return words, offset, changed
}

func checkMemoryCount(count uint32, name string) {
	// NOTE: in reality, this should never be trigged.  But, it is included just
	// in case.
	if count > math.MaxUint16 {
		panic(fmt.Sprintf("too many %s memory modules (%d)", name, count))
	}
}

// check whether the next bytecode to execute will terminate the enclosing
// function, or not.
func isVectorTerminal[W word.Word[W]](b bytecode.Bytecode[W]) bool {
	switch b.(type) {
	case *bytecode.Fail[W]:
		return true
	case *bytecode.Jmp[W]:
		return true
	case *bytecode.Ret[W]:
		return true
	default:
		return false
	}
}
