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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ROM_READ representing reading from a read-only memory.
var ROM_READ = RwMode{0}

// SROM_READ representing reading from a (static) read-only memory.
var SROM_READ = RwMode{1}

// WOM_WRITE representing writing to a write-once memory.
var WOM_WRITE = RwMode{2}

// SRAM_READ representing reading from a (small) random-access memory.
var SRAM_READ = RwMode{3}

// SRAM_WRITE representing write to a (small) random-access memory.
var SRAM_WRITE = RwMode{4}

// PRAM_READ representing reading from a (paged) random-access memory.
var PRAM_READ = RwMode{5}

// PRAM_WRITE representing write to a (paged) random-access memory.
var PRAM_WRITE = RwMode{6}

// RwMode determines what kind of memory is being operated on (e.g. ROM or RAM,
// etc) and what operation is being performed (i.e. READ or WRITE).  Its tag
// selects the corresponding opcode (RD_ROM_nm + tag).
type RwMode struct {
	tag uint8
}

// Tag gets the underlying tag for this enum.
func (p RwMode) Tag() uint8 {
	return p.tag
}

// ReadWrite encodes a memory read/write bytecode, resolving the target memory to
// its memory-specific identifier via the symbol table.  The bytecode records
// only whether the access is a read or a write; the precise mode (and hence
// opcode) is recovered here by combining that with the memory's kind, as
// recorded in the symbol table.
func ReadWrite[W word.Word[W]](p *bytecode.ReadWrite[W], env Environment[W]) []uint32 {
	var (
		lab  = Label{p.Id, ProgramPoint{}}
		sym  = env.SymbolAt(lab)
		id   = util.Cast[uint16](sym.Offset)
		mode = rwModeOf(sym.Kind, p.Write)
	)
	// A (static) read may discard some of its data lines; densify them so the
	// positional executor binds every remaining line correctly.
	data := p.Data
	//
	if !p.Write {
		data = denseBindings(data)
	}
	//
	return encodeReadWrite_sn(mode, id, p.Address, data)
}

// rwModeOf determines the read/write mode from a memory's symbol kind and
// whether the access is a write.  The kind alone fixes the mode for read-only
// and write-once memories; for read-write memories the write flag selects
// between the read and write modes.
func rwModeOf(kind uint8, write bool) RwMode {
	switch kind {
	case STATIC_MEMORY:
		return SROM_READ
	case READONLY_MEMORY:
		return ROM_READ
	case WRITEONCE_MEMORY:
		return WOM_WRITE
	case READWRITE_MEMORY:
		if write {
			return SRAM_WRITE
		}
		//
		return SRAM_READ
	case PAGED_READWRITE_MEMORY:
		if write {
			return PRAM_WRITE
		}
		//
		return PRAM_READ
	default:
		panic("invalid memory kind for read/write")
	}
}

// ============================================================================
// Encoders / Decoders
// ============================================================================

// ============================================================================
// RDS_n and WRS_n instruction.  Format of these instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  ndata |  naddr |   id   | opcode |
// +--------+--------+--------+--------+
// |  ra3   |  ra2   |  ra1   |  ra0   |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  ...   |  ...   |
// +--------+--------+--------+--------+
// |  rd2   |  rd1   |  rd0   |  ...   |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  ...   |  ...   |
// +--------+--------+--------+--------+
//
//
// Here, ra0...raN are u8 address registers, whilst rd0..rdN are u8 data
// registers.  The wide form moves the (now u16) memory identifier into the
// first packed slot (leaving bits 8-15 of the first word clear, as for all
// wide forms), with the (now u16) address and data registers following two
// per word:
//
// +--------+--------+--------+--------+
// |  ndata |  naddr |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       ra0       |       id        |
// +-----------------+-----------------+
// |       ra2       |       ra1       |
// +-----------------+-----------------+
// |   ... packed data registers ...   |
// +-----------------------------------+
//
// The wide form is also selected when the registers are small but the memory
// identifier exceeds a byte.
// ============================================================================

// encodeReadWrite_sn encodes a memory read/write instruction, packing its
// address and data registers.  The opcode is determined by the read/write mode.
func encodeReadWrite_sn(m RwMode, id uint16, addr []RegisterId, data []RegisterId) []uint32 {
	var (
		opcode = RD_ROM_nm + uint32(m.Tag())
		naddr  = uint32(util.Cast[uint8](uint(len(addr)))) << 16
		ndata  = uint32(util.Cast[uint8](uint(len(data)))) << 24
		regs   = append(RegsAsShorts(addr), RegsAsShorts(data)...)
	)
	//
	if IsWideRegisters(regs...) || id > math.MaxUint8 {
		var (
			codes = []uint32{ndata | naddr | (WIDE_RD_ROM_nm+uint32(m.Tag()))<<8 | WIDE}
			// The identifier occupies the first packed slot, followed by the
			// address and data registers.
			shorts = make([]uint16, 0, 1+len(regs))
		)
		//
		shorts = append(shorts, id)
		shorts = append(shorts, regs...)
		//
		return append(codes, PackShortsIntoCodes(shorts)...)
	}
	//
	var codes = []uint32{ndata | naddr | uint32(id)<<8 | opcode}
	// construct register bytes
	bytes := append(RegsAsBytes(addr), RegsAsBytes(data)...)
	// pack bytes into bytecodes
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeReadWrite_sn decodes the operands of a memory read/write instruction.
func DecodeReadWrite_sn(pc uint32, codes []uint32) (id uint16, addr, data Operands, n uint32) {
	naddr := uint((codes[pc] >> 16) & 0xff)
	ndata := uint((codes[pc] >> 24) & 0xff)
	//
	if IsWideForm(pc, codes) {
		id = uint16(codes[pc+1] & 0xffff)
		addr = NewWideOperands(1, naddr, codes[pc+1:])
		data = NewWideOperands(1+naddr, ndata, codes[pc+1:])
		n = 1 + NumCodesPackedWide(1+naddr+ndata)
	} else {
		id = uint16((codes[pc] >> 8) & 0xff)
		addr = NewOperands(0, naddr, codes[pc+1:])
		data = NewOperands(naddr, ndata, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(naddr+ndata)
	}
	//
	return
}
