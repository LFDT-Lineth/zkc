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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ROM_READ representing reading from a read-only memory.
var ROM_READ = RwMode{0}

// WOM_WRITE representing writing to a write-once memory.
var WOM_WRITE = RwMode{1}

// SRAM_READ representing reading from a (small) random-access memory.
var SRAM_READ = RwMode{2}

// SRAM_WRITE representing write to a (small) random-access memory.
var SRAM_WRITE = RwMode{3}

// RwMode determines whether what kind of memory is being operated on (e.g. ROM
// or RAM, etc) and what operation is being performed (i.e. READ or WRITE).
type RwMode struct {
	tag uint8
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

// NewReadWrite constructs a new memory read (or write) instruction.
func NewReadWrite(mode RwMode, id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{
		mode,
		id,
		asRegs(address...),
		asRegs(data...),
	}
}

func (p *ReadWrite) String() string {
	switch p.Mode {
	case ROM_READ:
		return rwReadStr("rom", p.Id, p.Address, p.Data)
	case WOM_WRITE:
		return rwWriteStr("wom", p.Id, p.Address, p.Data)
	case SRAM_READ:
		return rwReadStr("ram", p.Id, p.Address, p.Data)
	case SRAM_WRITE:
		return rwWriteStr("ram", p.Id, p.Address, p.Data)
	default:
		panic("unknown read/write mode")
	}
}

// Codes implementation for Bytecode interface
func (p *ReadWrite) Codes(_ uint32) []uint32 {
	//
	return encodeReadWrite_sn(p.Mode, p.Id, p.Address, p.Data)
}

func decodeReadWrite[W word.Word[W]](codes []uint32) (Bytecode[W], uint32) {
	m, id, addr, data, n := decodeReadWrite_sn(codes)
	//
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
		opcode = RD_ROM_N_M + uint32(m.tag)
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

func decodeReadWrite_sn(codes []uint32) (m RwMode, id uint16, addr []Reg, data []Reg, n uint32) {
	var (
		naddr    = (codes[0] >> 16) & 0xff
		ndata    = (codes[0] >> 24) & 0xff
		regs, ns = unpackCodesToSmallRegs(naddr+ndata, codes[1:])
	)
	//
	m = RwMode{tag: uint8(codes[0] - RD_ROM_N_M)}
	id = uint16((codes[0] >> 8) & 0xff)
	addr = regs[:naddr]
	data = regs[naddr:]
	//
	return m, id, addr, data, 1 + ns
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

func rwReadStr(prefix string, id uint16, address []Reg, data []Reg) string {
	var (
		alines = rwArguments(address)
		dlines = rwArguments(data)
	)
	//
	return fmt.Sprintf("%s = %s%d[%s]", dlines, prefix, id, alines)
}

func rwWriteStr(prefix string, id uint16, address []Reg, data []Reg) string {
	var (
		alines = rwArguments(address)
		dlines = rwArguments(data)
	)
	//
	return fmt.Sprintf("%s%d[%s] = %s", prefix, id, alines, dlines)
}

func rwArguments(args []Reg) string {
	var builder strings.Builder
	//
	for i, r := range args {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(fmt.Sprintf("r%d", r))
	}
	//
	return builder.String()
}
