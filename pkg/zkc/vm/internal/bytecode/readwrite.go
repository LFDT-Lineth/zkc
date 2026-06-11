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
	"math"

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

// RwMode determines whether what kind of memory is being operated on (e.g. ROM
// or RAM, etc) and what operation is being performed (i.e. READ or WRITE).
type RwMode struct {
	tag uint8
}

// Kind returns the memory kind associated with this read-write mode.
func (p RwMode) Kind() MemoryKind {
	switch p {
	case SROM_READ:
		return STATIC_READONLY_MEMORY
	case ROM_READ:
		return READONLY_MEMORY
	case WOM_WRITE:
		return WRITEONCE_MEMORY
	case SRAM_READ, SRAM_WRITE:
		return READWRITE_MEMORY
	case PRAM_READ, PRAM_WRITE:
		return PAGED_READWRITE_MEMORY
	default:
		panic("invalid read/write mode")
	}
}

// ReadWrite instruction captures memory read/writes.
type ReadWrite struct {
	// RwMode determines whether this is a read or write operation and,
	// furthermore, what kind of memory is being accessed.
	Mode RwMode
	// Identifies the memory being read or written.
	Id uint16
	// Address lines used to determine which data row to read.
	Address []Reg
	// Data lines identify where the data row is written.
	Data []Reg
}

func (p *ReadWrite) String(mapping SystemMap) string {
	var (
		name    = mapping.Module(uint(p.Id)).Name()
		address = registersToString(p.Address, mapping, ",")
		data    = registersToString(p.Data, mapping, ",")
	)
	//
	switch p.Mode {
	case SROM_READ, ROM_READ, SRAM_READ, PRAM_READ:
		return fmt.Sprintf("%s = %s[%s]", data, name, address)
	case WOM_WRITE, SRAM_WRITE, PRAM_WRITE:
		return fmt.Sprintf("%s[%s] = %s", name, address, data)
	default:
		panic("unknown read/write mode")
	}
}

// Codes implementation for Bytecode interface
func (p *ReadWrite) Codes(_ uint32) []uint32 {
	//
	return encodeReadWrite_sn(p.Mode, p.Id, p.Address, p.Data)
}

func decodeReadWrite[W word.Word[W]](pc uint32, codes []uint32, rmap map[MemoryId]uint16) (Bytecode[W], uint32) {
	var (
		// determine read/write mode
		m = RwMode{tag: uint8(codes[pc] - RD_ROM_nm)}
		// decode remainder
		id, addrIter, dataIter, n = decodeReadWrite_sn(pc, codes)
		// flattern iterators
		addr = OpIterToArray[uint16](addrIter)
		data = OpIterToArray[uint16](dataIter)
		//
		mid = MemoryId{m.Kind(), id}
	)
	// remap memory identifier to be module-specific
	id = rmap[mid]
	// done
	return &ReadWrite{m, id, addr, data}, n
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
// registers.
// ============================================================================

func encodeReadWrite_sn(m RwMode, id uint16, addr []Reg, data []Reg) []uint32 {
	var (
		opcode = RD_ROM_nm + uint32(m.tag)
		_id    = uint32(id) << 8
		naddr  = uint32(len(addr)) << 16
		ndata  = uint32(len(data)) << 24
		codes  = []uint32{
			ndata | naddr | _id | opcode,
		}
	)
	// construct register bytes
	bytes := append(regsAsBytes(addr), regsAsBytes(data)...)
	// pack bytes into bytecodes
	return append(codes, packRegsIntoCodes(bytes)...)
}

func decodeReadWrite_sn(pc uint32, codes []uint32) (id uint16, addr, data Op8Iter, n uint32) {
	naddr := uint((codes[pc] >> 16) & 0xff)
	ndata := uint((codes[pc] >> 24) & 0xff)
	ns := nCodesPackedSmall(naddr + ndata)
	id = uint16((codes[pc] >> 8) & 0xff)
	addr = NewOp8Iter(0, naddr, codes[pc+1:])
	data = NewOp8Iter(naddr, ndata, codes[pc+1:])
	n = 1 + ns
	//
	return
}

// ============================================================================
// Helpers
// ============================================================================

func checkSmallArgs(args []Reg) {
	//
	if len(args) > math.MaxUint8 {
		panic("too many arguments")
	}
	//
	for _, r := range args {
		if r > math.MaxUint8 {
			panic("support wide read/write instructions")
		}
	}
}
