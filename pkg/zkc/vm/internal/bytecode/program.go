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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Module provides a convenient alias
type Module = base.Module

// SystemMap provides a convenient alias
type SystemMap = base.SystemMap

// Program represents a self-contained bytecode program with a given entry
// point.
type Program[W word.Word[W]] struct {
	// Modules declaredThe by the program
	modules []Module
	// The bytecode sequence itself.
	bytecodes []uint32
	// A constant pool
	constants []W
	// Symbols associated with bytecode offsets
	symbols map[uint32]uint
}

// NewProgram constructs a new bytecode program with a given entry point.
func NewProgram[W word.Word[W]](modules []Module, bytecodes []uint32, constants []W, symbols map[uint32]uint,
) Program[W] {
	//
	return Program[W]{
		modules,
		bytecodes,
		constants,
		symbols,
	}
}

// Bytecodes decodes this program into a more human-friendly representation.
// Observe that this decoding procedure is non-trivial, and may allocate memory.
// As such, it should not be used when performance is critical.  In such
// situations, direct decoding should be preferred.
func (p Program[W]) Bytecodes() (bytecodes []Bytecode[W]) {
	var (
		rmap   = buildReverseMemoryMap[W](p.modules...)
		ncodes = uint32(len(p.bytecodes))
		offset uint32
	)
	//
	for offset < ncodes {
		bc, n := decodeBytecode[W](offset, p.bytecodes, rmap)
		bytecodes = append(bytecodes, bc)
		offset += n
	}
	//
	return bytecodes
}

// HasModule returns the identifier for the module with the given name, or returns
// false if no such module exists.
func (p Program[W]) HasModule(name string) (uint, bool) {
	var mid = array.FindMatching(p.modules, func(m Module) bool {
		return m.Name() == name
	})
	//
	return mid, mid != math.MaxUint
}

// Module returns the module with the given identifier.
func (p Program[W]) Module(mid uint) Module {
	return p.modules[mid]
}

// AddressOf determines the address of a given (function) symbol, or returns an
// error if no such symbol exists.
func (p Program[W]) AddressOf(mid uint) (uint32, bool) {
	// Find the symbols address
	for addr, id := range p.symbols {
		if id == mid {
			return addr, true
		}
	}
	// failed
	return 0, false
}

// SymbolAt determines whether or not there is a symbol associated with a given
// instruction address.
func (p Program[W]) SymbolAt(address Address) util.Option[uint] {
	if idx, ok := p.symbols[address]; ok {
		return util.Some(idx)
	}
	//
	return util.None[uint]()
}

// Modules returns information about the modules declared within this program.
func (p Program[W]) Modules() []Module {
	return p.modules
}

func decodeBytecode[W word.Word[W]](pc uint32, codes []uint32, rmap map[MemoryId]uint16) (Bytecode[W], uint32) {
	var (
		code = codes[pc]
	)
	//
	switch code & OPCODE_MASK {
	case ADD_2n1, ADDC, ADD_nm, SUB_nm, MUL_nm:
		return decodeArith[W](pc, codes)
	case ENTER_n:
		return decodeCall[W](pc, codes)
	case CAT:
		return decodeCat[W](pc, codes)
	case CHECKCAST:
		rd, width, n := decodeCheckCast(pc, codes)
		return &CheckCast{width, rd}, n
	case DEBUG:
		return &Debug{}, 1
	case FAIL:
		return &Fail{}, 1
	case JMP:
		target, n := decodeJmp1(pc, codes)
		return &Jmp{Target: target}, n
	case SKIP:
		// SKIP is simply the forward-branch encoding of an unconditional jump,
		// hence it decodes back to a Jmp.
		target, n := decodeSkip1(pc, codes)
		return &Jmp{Target: target}, n
	case JEQ_rr, JNE_rr, JLT_rr, JLE_rr, JGT_rr, JGE_rr:
		return decodeJif[W](pc, codes)
	case SEQ_rr, SNE_rr, SLT_rr, SLE_rr, SGT_rr, SGE_rr:
		return decodeJif[W](pc, codes)
	case JEQ_rv, JNE_rv, JLT_rv, JLE_rv, JGT_rv, JGE_rv:
		return decodeJif[W](pc, codes)
	case LDC, LDC_w, MOVE:
		return decodeArith[W](pc, codes)
	case MUL_2n1, MULC:
		return decodeArith[W](pc, codes)
	case NOT:
		return decodeNot[W](pc, codes)
	case AND, OR, XOR:
		return decodeBitwise[W](pc, codes)
	case DIV, REM:
		return decodeDivRem[W](pc, codes)
	case DIVHINT:
		return decodeDivHint[W](pc, codes)
	case SHL, SHR:
		return decodeShift[W](pc, codes)
	case RD_SROM_nm, RD_ROM_nm, WR_WOM_nm, RD_RAM_nm, WR_RAM_nm, RD_PRAM_nm, WR_PRAM_nm:
		return decodeReadWrite[W](pc, codes, rmap)
	case RET:
		return decodeRet[W](pc, codes)
	case SUB_2n1, SUBC:
		return decodeArith[W](pc, codes)
	default:
		panic("unknown bytecode encountered")
	}
}
