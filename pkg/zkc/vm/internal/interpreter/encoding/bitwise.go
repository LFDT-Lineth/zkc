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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bitwise encodes a bitwise bytecode (AND/OR/XOR/NOT/SHL/SHR), selecting the
// constant form (ANDC/ORC/XORC) when the right operand is a constant.
func Bitwise[W word.Word[W]](p *bytecode.Bitwise[W], env Environment[W]) []uint32 {
	if p.Right.IsConstant() {
		return encodeBitwiseConst(p.Op, p.Target, p.Left, p.Right.AsConstant(), p.Bitwidth, env)
	}
	//
	return encodeBitwise(p.Op, p.Target, p.Left, p.Right.AsRegister(), p.Bitwidth)
}

// ============================================================================
// AND / OR / XOR / NOT / SHL / SHR instruction. Format of these instructions is:
//
//	31                                0
// +--------+--------+--------+--------+
// | amount | source |   rd   | opcode |
// +--------+--------+--------+--------+
// |       n/a       |     bitwidth    |
// +-----------------+-----------------+
//
// The opcode itself distinguishes the operations, so no width is needed.  The
// wide form carries the (now u16) destination register in the first word, with
// both source registers in a third:
//
// +--------+--------+--------+--------+
// |        rd       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       n/a       |     bitwidth    |
// +-----------------+-----------------+
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
			uint32(rd)<<16 | (WIDE_AND+uint32(op-bytecode.OP_AND))<<8 | WIDE,
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

// ============================================================================
// Bitwise-with-constant instruction (ANDC/ORC/XORC).  Format of these
// instructions mirrors ADDC/SUBC/MULC, extended with the bitwidth word all
// bitwise forms carry:
//
//	31                                0
// +--------+--------+--------+--------+
// |  imm8  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
// |       n/a       |     bitwidth    |
// +-----------------+-----------------+
//
// Here, rs is a u8 source register, rd is a u8 destination register and imm8
// is the small constant operand.  The wide form replaces the constant operand
// with a u16 constant pool identifier, moving the (now u16) registers into a
// third word:
//
// +--------+--------+--------+--------+
// |       cid       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       n/a       |     bitwidth    |
// +-----------------+-----------------+
// |        rs       |        rd       |
// +-----------------+-----------------+
//
// The wide form is also selected when the registers are small but the
// constant exceeds a byte, being more compact than materialising the constant
// into a register.
// ============================================================================

// encodeBitwiseConst encodes a bitwise-with-constant instruction
// (ANDC/ORC/XORC), where op selects the operation and bitwidth gives the
// operand width.
func encodeBitwiseConst[W word.Word[W]](op bytecode.Operation, rd, lhs RegisterId, constant W,
	bitwidth uint16, env Environment[W]) []uint32 {
	//
	var opcode = ANDC + uint32(op-bytecode.OP_AND)
	// The wide (pooled) form carries a constant of arbitrary width; the narrow
	// form only a u8 immediate.
	if IsWideRegisters(rd, lhs) || constant.Cmp64(0xff) > 0 {
		return []uint32{
			uint32(env.ConstantIndex(constant))<<16 | (WIDE_ANDC+uint32(op-bytecode.OP_AND))<<8 | WIDE,
			uint32(bitwidth),
			uint32(rd) | uint32(lhs)<<16,
		}
	}
	//
	return []uint32{
		uint32(constant.Uint64())<<24 | uint32(lhs)<<16 | uint32(rd)<<8 | opcode,
		uint32(bitwidth),
	}
}

// DecodeBitwise_1n1c decodes a one-source-plus-constant bitwise instruction,
// returning the destination and source registers, constant, bitwidth and
// instruction width.
func DecodeBitwise_1n1c[W word.Word[W]](pc uint32, codes []uint32, pool []W) (rd, lhs RegisterId, constant W,
	bitwidth uint16, n uint32) {
	//
	bitwidth = uint16(codes[pc+1])
	//
	if IsWideForm(pc, codes) {
		constant = pool[codes[pc]>>16]
		rd = RegisterId(codes[pc+2] & 0xffff)
		lhs = RegisterId(codes[pc+2] >> 16)
		//
		return rd, lhs, constant, bitwidth, 3
	}
	//
	constant = constant.SetUint64(uint64((codes[pc] >> 24) & 0xff))
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	lhs = RegisterId((codes[pc] >> 16) & 0xff)
	//
	return rd, lhs, constant, bitwidth, 2
}
