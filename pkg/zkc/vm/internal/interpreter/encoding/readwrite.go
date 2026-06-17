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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ReadWrite encodes a memory read/write bytecode, resolving the target memory to
// its memory-specific identifier via the symbol table.
func ReadWrite[W word.Word[W]](p *bytecode.ReadWrite, env Environment[W]) []uint32 {
	var (
		lab = Label{p.Id, ProgramPoint{}}
		id  = util.Cast[uint8](env.SymbolAt(lab).Offset)
	)
	//
	return encodeReadWrite_sn(p.Mode, id, p.Address, p.Data)
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

// encodeReadWrite_sn encodes a memory read/write instruction, packing its
// address and data registers.  The opcode is determined by the read/write mode.
func encodeReadWrite_sn(m bytecode.RwMode, id uint8, addr []RegisterId, data []RegisterId) []uint32 {
	var (
		opcode = RD_ROM_nm + uint32(m.Tag())
		_id    = uint32(id) << 8
		naddr  = uint32(util.Cast[uint8](uint(len(addr)))) << 16
		ndata  = uint32(util.Cast[uint8](uint(len(data)))) << 24
		codes  = []uint32{
			ndata | naddr | _id | opcode,
		}
	)
	// construct register bytes
	bytes := append(RegsAsBytes(addr), RegsAsBytes(data)...)
	// pack bytes into bytecodes
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeReadWrite_sn decodes the operands of a memory read/write instruction.
func DecodeReadWrite_sn(pc uint32, codes []uint32) (id uint16, addr, data Op8Iter, n uint32) {
	naddr := uint((codes[pc] >> 16) & 0xff)
	ndata := uint((codes[pc] >> 24) & 0xff)
	ns := NumCodesPackedSmall(naddr + ndata)
	id = uint16((codes[pc] >> 8) & 0xff)
	addr = NewOp8Iter(0, naddr, codes[pc+1:])
	data = NewOp8Iter(naddr, ndata, codes[pc+1:])
	n = 1 + ns
	//
	return
}
