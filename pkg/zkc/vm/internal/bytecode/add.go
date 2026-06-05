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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Add instruction.
type Add[W word.Word[W]] struct {
	Source   []Reg
	Target   []Reg
	Constant W
}

func (p *Add[W]) String() string {
	return fmt.Sprintf("add ???")
}

// Codes implementation for Bytecode interface
func (p *Add[W]) Codes(_ uint32) []uint32 {
	var (
		n = len(p.Source)
		m = len(p.Target)
		z = p.Constant.Cmp64(0) == 0
	)
	switch {
	case n == 0 && m == 1 && !z:
		return encodeLdc_1(p.Constant, p.Target[0])
	case n == 1 && m == 1 && z:
		return encodeMove_1s1(p.Source[0], p.Target[0])
	case n == 2 && m == 1 && z:
		return encodeAdd_2s1(p.Source[0], p.Source[1], p.Target[1])
	default:
		panic("unsupported instruction form")
	}
}

// ============================================================================
// Encoding
// ============================================================================

// Add_2s1 instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  rs0   |  rs1   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rs0 and rs1 are u8 source registers, whilst rd is a u8 destination
// register.
func encodeAdd_2s1(rs0, rs1, rd uint16) []uint32 {
	var (
		_rd  = uint32(rd) << 8
		_rs1 = uint32(rs1) << 16
		_rs0 = uint32(rs0) << 24
	)
	//
	if rs0 >= 256 || rs1 >= 256 || rd >= 256 {
		// NOTE: this corresponds to a WIDE instruction, but these are not
		// supported at this time.
		panic("wide instructions not supported")
	}
	//
	return []uint32{
		_rs0 | _rs1 | _rd | ADD,
	}
}

// Ldc_1 instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |      imm16      |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rs0 and rs1 are u8 source registers, whilst rd is a u8 destination
// register.
func encodeLdc_1[W word.Word[W]](constant W, rd uint16) []uint32 {
	panic("got here")
}

// Move instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   n/a  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rs is a u8 source register whilst rd is a u8 destination register.
func encodeMove_1s1(rs, rd uint16) []uint32 {
	var (
		_rd = uint32(rd) << 8
		_rs = uint32(rs) << 16
	)
	//
	if rs >= 256 || rd >= 256 {
		// NOTE: this corresponds to a WIDE instruction, but these are not
		// supported at this time.
		panic("wide instructions not supported")
	}
	//
	return []uint32{
		_rs | _rd | MOVE,
	}
}

// ============================================================================
// Decoding
// ============================================================================

func decodeAdd(codes []uint32) (Bytecode, uint32) {
	panic("todo")
}

func decodeAdd_2s1(codes []uint32) (rs0, rs1, rd uint16, n uint32) {
	rd = Reg((codes[0] >> 8) & 0xff)
	rs1 = Reg((codes[0] >> 16) & 0xff)
	rs0 = Reg((codes[0] >> 24) & 0xff)
	//
	return rs0, rs1, rd, 1
}
