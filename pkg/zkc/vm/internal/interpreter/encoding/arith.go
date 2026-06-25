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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Arith encodes an arithmetic bytecode, selecting the most compact instruction
// form supported by its operands (e.g. load-constant, move, register-register,
// register-constant, or the general vectored form).
func Arith[W word.Word[W]](p bytecode.Arith[W]) []uint32 {
	var (
		n             = len(p.Source)
		m             = len(p.Target)
		cz            = bytecode.IsUnusedConstant(p.Op, p.Constant)
		constIsUint8  = p.Constant.Cmp64(256) < 0
		constIsUint16 = p.Constant.Cmp64(65536) < 0
	)
	//
	switch {
	case n == 0 && m == 1 && constIsUint16:
		return encodeLdc_1(p.Constant, p.Target[0])
	case n == 0 && m == 1:
		return encodeLdc_w(p.Constant, p.Target[0])
	case n == 1 && m == 1 && cz:
		return encodeMove_1s1(p.Source[0], p.Target[0])
	case n == 1 && m == 1 && constIsUint8:
		return encodeArith_1n1c(p.Op, p.Source[0], p.Target[0], p.Constant)
	case n == 2 && m == 1 && cz:
		return encodeArith_2n1(p.Op, p.Source[0], p.Source[1], p.Target[0])
	case n == 2 && m == 1 && constIsUint8:
		return encodeArith_2n1c(p.Op, p.Source[0], p.Source[1], p.Target[0], p.Constant)
	default:
		return encodeArith_vec(p.Op, p.Target, p.Source, p.Constant)
	}
}

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
// ============================================================================

// encodeArith_2n1 encodes a two-source, one-target arithmetic instruction,
// where aop selects the operation (ADD/SUB/MUL).
func encodeArith_2n1(aop bytecode.Operation, rs0, rs1, rd uint16) []uint32 {
	var (
		_rd    = uint32(rd) << 8
		_rs1   = uint32(rs1) << 16
		_rs0   = uint32(rs0) << 24
		opcode = ADD_2n1 + uint32(aop-bytecode.OP_ADD)
	)
	//
	if rs0 >= 256 || rs1 >= 256 || rd >= 256 {
		// NOTE: this corresponds to a WIDE instruction, but these are not
		// supported at this time.
		panic("wide instructions not supported")
	}
	//
	return []uint32{
		_rs0 | _rs1 | _rd | opcode,
	}
}

// DecodeArith_2n1 decodes a two-source, one-target arithmetic instruction,
// returning the source and destination registers and the instruction width.
func DecodeArith_2n1(pc uint32, codes []uint32) (rs0, rs1, rd uint16, n uint32) {
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	rs1 = RegisterId((codes[pc] >> 16) & 0xff)
	rs0 = RegisterId((codes[pc] >> 24) & 0xff)
	//
	return rs0, rs1, rd, 1
}

// ============================================================================
// Arithmetic-with-constant instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  imm8  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rs is a u8 source register, rd is a u8 destination register, imm8 is
// the small constant operand and opcode is ADDC, SUBC or MULC.
// ============================================================================

// encodeArith_2n1c encodes a two-source, one-target arithmetic instruction with
// a constant operand.
func encodeArith_2n1c[W word.Word[W]](aop bytecode.Operation, rs0, rs1, rd uint16, constant W) []uint32 {
	// There is no 2-source-plus-constant instruction form, so compute
	// "x op y" first, then fold in the constant (when used) with a second
	// one-source instruction operating in place on the target.
	codes := encodeArith_2n1(aop, rs0, rs1, rd)
	//
	return append(codes, encodeArith_1n1c(aop, rd, rd, constant)...)
}

// encodeArith_1n1c encodes a one-source, one-target arithmetic-with-constant
// instruction (ADDC/SUBC/MULC).
func encodeArith_1n1c[W word.Word[W]](aop bytecode.Operation, rs, rd uint16, constant W) []uint32 {
	if rs >= 256 || rd >= 256 || constant.Cmp64(256) >= 0 {
		// NOTE: this corresponds to a WIDE instruction, but these are not
		// supported at this time.
		panic("wide instructions not supported")
	}
	//
	var (
		_rd    = uint32(rd) << 8
		_rs    = uint32(rs) << 16
		_imm   = uint32(constant.Uint64()) << 24
		opcode = ADDC + uint32(aop-bytecode.OP_ADD)
	)
	//
	return []uint32{
		_imm | _rs | _rd | opcode,
	}
}

// DecodeArith_1n1c decodes a one-source-plus-constant arithmetic instruction,
// returning the source and destination registers, constant and instruction width.
func DecodeArith_1n1c[W word.Word[W]](pc uint32, codes []uint32) (rs, rd uint16, constant W, n uint32) {
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	rs = RegisterId((codes[pc] >> 16) & 0xff)
	constant = constant.SetUint64(uint64((codes[pc] >> 24) & 0xff))
	//
	return rs, rd, constant, 1
}

// ============================================================================
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
// ============================================================================

// encodeLdc_1 encodes a load-constant instruction carrying a small (u16)
// constant into the target register.
func encodeLdc_1[W word.Word[W]](constant W, rd uint16) []uint32 {
	// Sanity checks.  Constants which do not fit within 16 bits must use the
	// wide form (see encodeLdc_w).
	if rd >= 256 || constant.Cmp64(65536) >= 0 {
		panic("constant exceeds short load form")
	}
	// Encoding
	_rd := uint32(rd) << 8
	c := uint32(constant.Uint64()) << 16
	//
	return []uint32{
		c | _rd | LDC,
	}
}

// DecodeLdc_1 decodes a load-constant instruction carrying a small (u16)
// constant, returning the constant, destination register and instruction width.
func DecodeLdc_1[W word.Word[W]](pc uint32, codes []uint32) (constant W, rd uint16, n uint32) {
	var c W
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	c = c.SetUint64(uint64(codes[pc] >> 16))
	//
	return c, rd, 1
}

// ============================================================================
// Ldc_w (wide load constant) instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |      n          |   rd   | opcode |
// +--------+--------+--------+--------+
// |     limb 0 (least significant)    |
// +-----------------------------------+
// |                ...                |
// +-----------------------------------+
// |     limb n-1 (most significant)   |
// +-----------------------------------+
//
// Here, rd is a u8 destination register and the constant is carried inline as
// n 32-bit limbs (least significant first).  This form supports constants of
// arbitrary width (e.g. field-sized constants on a Uint machine), in contrast
// to the short LDC form which carries a single 16-bit immediate.
// ============================================================================

// encodeLdc_w encodes a wide load-constant instruction, carrying the constant
// inline as a sequence of 32-bit limbs (least significant first).
func encodeLdc_w[W word.Word[W]](constant W, rd uint16) []uint32 {
	// Sanity checks
	if rd >= 256 {
		panic("wide instructions not supported")
	}
	//
	var (
		// NOTE: big-endian byte ordering
		bytes  = constant.BigInt().Bytes()
		nlimbs = max(1, (len(bytes)+3)/4)
		codes  = make([]uint32, nlimbs+1)
	)
	//
	codes[0] = uint32(nlimbs)<<16 | uint32(rd)<<8 | LDC_w
	// Pack bytes into limbs, least significant limb first.
	for i, b := range bytes {
		var k = len(bytes) - 1 - i
		//
		codes[1+(k/4)] |= uint32(b) << (8 * (k % 4))
	}
	//
	return codes
}

// DecodeLdc_w decodes a load-constant instruction carrying a wide constant,
// returning the constant, destination register and instruction width.
func DecodeLdc_w[W word.Word[W]](pc uint32, codes []uint32) (constant W, rd uint16, n uint32) {
	var (
		c      big.Int
		limb   big.Int
		nlimbs = (codes[pc] >> 16) & 0xffff
	)
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	// Unpack limbs, most significant limb first.
	for i := nlimbs; i > 0; i-- {
		limb.SetUint64(uint64(codes[pc+i]))
		c.Lsh(&c, 32)
		c.Or(&c, &limb)
	}
	//
	return constant.SetBigInt(&c), rd, nlimbs + 1
}

// ============================================================================
// Move instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   n/a  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rs is a u8 source register whilst rd is a u8 destination register.
// ============================================================================

// encodeMove_1s1 encodes a register-to-register move instruction.
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

// DecodeMove_1s1 decodes a register-to-register move instruction, returning the
// source and destination registers and the instruction width.
func DecodeMove_1s1(pc uint32, codes []uint32) (rs, rd uint16, n uint32) {
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	rs = RegisterId((codes[pc] >> 16) & 0xff)
	//
	return rs, rd, 1
}

// ============================================================================
// Vector-target arithmetic instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   n/a  |  nsrc  | ntgt   | opcode |
// +--------+--------+--------+--------+
// |        constant low 32 bits        |
// +------------------------------------+
// |        constant high 32 bits       |
// +------------------------------------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// The arithmetic operation (add, subtract or multiply) is identified by the
// opcode itself (ADD_nm, SUB_nm or MUL_nm).  Targets are packed first because
// StoreAcross writes the low limbs first.
// ============================================================================

// encodeArith_vec encodes the general (multi-target, multi-source) vectored form
// of an arithmetic instruction (ADD_nm/SUB_nm/MUL_nm).
func encodeArith_vec[W word.Word[W]](aop bytecode.Operation, targets []RegisterId, sources []RegisterId,
	constant W) []uint32 {
	//
	if len(targets) == 0 {
		panic("targetless arithmetic instructions not supported")
	} else if len(targets) >= 256 || len(sources) >= 256 {
		panic("wide vector arithmetic instructions not supported")
	} else if constant.Cmp64(^uint64(0)) > 0 {
		panic("wide vector arithmetic constants not supported")
	}
	//
	var (
		opcode   = ADD_nm + uint32(aop-bytecode.OP_ADD)
		nsrc     = uint32(len(sources)) << 16
		ntgt     = uint32(len(targets)) << 8
		c        = constant.Uint64()
		codes    = []uint32{nsrc | ntgt | opcode, uint32(c), uint32(c >> 32)}
		regBytes = append(RegsAsBytes(targets), RegsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(regBytes)...)
}

// DecodeArith_nm decodes a vectored (multi-target, multi-source) arithmetic
// instruction, returning iterators over its target and source registers, the
// constant operand and the instruction width.
func DecodeArith_nm[W word.Word[W]](pc uint32, codes []uint32) (
	targets, sources Op8Iter, constant W, n uint32) {
	//
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
		c        = uint64(codes[pc+1]) | (uint64(codes[pc+2]) << 32)
	)
	//
	targets = NewOp8Iter(0, ntargets, codes[pc+3:])
	sources = NewOp8Iter(ntargets, nsources, codes[pc+3:])
	//
	constant = constant.SetUint64(c)
	//
	return targets, sources, constant,
		3 + NumCodesPackedSmall(ntargets+nsources)
}
