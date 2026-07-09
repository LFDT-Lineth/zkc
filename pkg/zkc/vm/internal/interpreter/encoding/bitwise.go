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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
)

// Bitwise encodes a bitwise bytecode (AND/OR/XOR/NOT/SHL/SHR).
func Bitwise(p *bytecode.Bitwise) []uint32 {
	return encodeBitwise(p.Op, p.Target, p.Left, p.Right, p.Bitwidth)
}

// ============================================================================
// AND / OR / XOR / NOT / SHL / SHR instruction. Format of these instructions is:
//
//	31                                0
// +--------+--------+--------+--------+
// | amount | source |   rd   | opcode |
// +--------+--------+--------+--------+
// |                 |     bitwidth    |
// +--------+--------+--------+--------+
//
// The opcode itself distinguishes the operations, so no width is needed.  The
// wide form carries the (now u16) destination register in the first word, with
// both source registers in a third:
//
// +--------+--------+--------+--------+
// |        rd       |  n/a   | opcode |
// +--------+--------+--------+--------+
// |                 |     bitwidth    |
// +--------+--------+--------+--------+
// |     amount      |      source     |
// +-----------------+-----------------+
// ============================================================================

// encodeBitwise encodes a bitwise instruction, where op selects the operation
// and bitwidth gives the operand width.
func encodeBitwise(op bytecode.Operation, rd, lhs, rhs RegisterId, bitwidth uint16) []uint32 {
	var opcode = AND + uint32(op-bytecode.OP_AND)
	//
	if IsWideRegisters(rd, lhs, rhs) {
		return []uint32{
			uint32(rd)<<16 | opcode | WIDE,
			uint32(bitwidth),
			uint32(lhs) | uint32(rhs)<<16,
		}
	}
	//
	return []uint32{
		uint32(rhs)<<24 | uint32(lhs)<<16 | uint32(rd)<<8 | opcode,
		uint32(bitwidth),
	}
}

// DecodeBitwise_2n1 decodes the operands of a two-source bitwise instruction.
func DecodeBitwise_2n1(pc uint32, codes []uint32) (rd, lhs, rhs RegisterId, bitwidth uint16, n uint32) {
	bitwidth = uint16(codes[pc+1])
	//
	if IsWideForm(pc, codes) {
		rd = RegisterId(codes[pc] >> 16)
		lhs = RegisterId(codes[pc+2] & 0xffff)
		rhs = RegisterId(codes[pc+2] >> 16)
		//
		return rd, lhs, rhs, bitwidth, 3
	}
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	lhs = RegisterId((codes[pc] >> 16) & 0xff)
	rhs = RegisterId((codes[pc] >> 24) & 0xff)
	//
	return rd, lhs, rhs, bitwidth, 2
}
