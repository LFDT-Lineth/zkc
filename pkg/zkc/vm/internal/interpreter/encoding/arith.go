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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Arith encodes an arithmetic bytecode, selecting the most compact instruction
// form supported by its operands (e.g. load-constant, move, register-register,
// register-constant, or the general vectored form).
func Arith[W word.Word[W]](p bytecode.Arith[W], env Environment[W]) []uint32 {
	var (
		n             = len(p.Source)
		m             = len(p.Target)
		cz            = bytecode.IsUnusedConstant(p.Op, p.Constant)
		constIsUint24 = p.Constant.Cmp64(0x100_0000) < 0
	)
	//
	switch {
	case n == 0 && m == 1 && constIsUint24:
		return encodeLdc_1(p.Constant, p.Target[0])
	case n == 0 && m == 1:
		return encodeLdc_w(p.Constant, p.Target[0])
	case n == 1 && m == 1 && cz:
		return encodeMove_1s1(p.Source[0], p.Target[0])
	case n == 1 && m == 1 && constIsUint24:
		return encodeArith_1n1c(p.Op, p.Source[0], p.Target[0], p.Constant)
	case n == 2 && m == 1 && cz:
		return encodeArith_2n1(p.Op, p.Source[0], p.Source[1], p.Target[0])
	case n == 2 && m == 1 && constIsUint24 && p.Op != bytecode.OP_SUB:
		// SUB is excluded: the two-step pairing wraps each step separately,
		// which is not equivalent to the single wrap (at CalculateSubBitwidth
		// width) that the vectored form performs when the first step
		// underflows.  See encodeArith_2n1c.
		return encodeArith_2n1c(p.Op, p.Source[0], p.Source[1], p.Target[0], p.Constant)
	default:
		return encodeArith_vec(p.Op, p.Target, p.Source, p.Constant, env)
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
// register.  The wide form carries the (now u16) destination register in the
// first word, with both source registers in a second:
//
// +--------+--------+--------+--------+
// |        rd       |  n/a   | opcode |
// +--------+--------+--------+--------+
// |       rs0       |       rs1       |
// +-----------------+-----------------+
// ============================================================================

// encodeArith_2n1 encodes a two-source, one-target arithmetic instruction,
// where aop selects the operation (ADD/SUB/MUL).
func encodeArith_2n1(aop bytecode.Operation, rs0, rs1, rd uint16) []uint32 {
	var opcode = ADD_2n1 + uint32(aop-bytecode.OP_ADD)
	//
	if IsWideRegisters(rs0, rs1, rd) {
		return []uint32{
			uint32(rd)<<16 | opcode | WIDE,
			uint32(rs1) | uint32(rs0)<<16,
		}
	}
	//
	return []uint32{
		uint32(rs0)<<24 | uint32(rs1)<<16 | uint32(rd)<<8 | opcode,
	}
}

// DecodeArith_2n1 decodes a two-source, one-target arithmetic instruction,
// returning the source and destination registers and the instruction width.
func DecodeArith_2n1(pc uint32, codes []uint32) (rs0, rs1, rd uint16, n uint32) {
	if IsWideForm(pc, codes) {
		rd = RegisterId(codes[pc] >> 16)
		rs1 = RegisterId(codes[pc+1] & 0xffff)
		rs0 = RegisterId(codes[pc+1] >> 16)
		//
		return rs0, rs1, rd, 2
	}
	//
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
// the small constant operand and opcode is ADDC, SUBC or MULC.  The wide form
// extends the constant operand to imm24, moving the (now u16) registers into a
// subsequent word:
//
// +--------+--------+--------+--------+
// |           imm24          | opcode |
// +--------+--------+--------+--------+
// |       rs        |        rd       |
// +-----------------+-----------------+
//
// The wide form is also selected when the registers are small but the constant
// exceeds a byte, being more compact than the general vectored form.
// ============================================================================

// encodeArith_2n1c encodes a two-source, one-target arithmetic instruction with
// a constant operand.  This form applies only to ADD and MUL: those are exact
// (a fold in two steps computes the same value as one), whereas a subtraction
// wraps per-instruction on underflow, so splitting it would wrap at the wrong
// width — SUB must use the vectored form instead (see Arith).
func encodeArith_2n1c[W word.Word[W]](aop bytecode.Operation, rs0, rs1, rd uint16, constant W) []uint32 {
	// Sanity check
	if aop == bytecode.OP_SUB {
		panic("two-step encoding unsound for subtraction")
	}
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
	if constant.Cmp64(0x100_0000) >= 0 {
		panic("constant exceeds arithmetic-with-constant form")
	}
	//
	var (
		imm    = uint32(constant.Uint64())
		opcode = ADDC + uint32(aop-bytecode.OP_ADD)
	)
	//
	if IsWideRegisters(rs, rd) || imm > 0xff {
		return []uint32{
			imm<<8 | opcode | WIDE,
			uint32(rd) | uint32(rs)<<16,
		}
	}
	//
	return []uint32{
		imm<<24 | uint32(rs)<<16 | uint32(rd)<<8 | opcode,
	}
}

// DecodeArith_1n1c decodes a one-source-plus-constant arithmetic instruction,
// returning the source and destination registers, constant and instruction width.
func DecodeArith_1n1c[W word.Word[W]](pc uint32, codes []uint32) (rs, rd uint16, constant W, n uint32) {
	if IsWideForm(pc, codes) {
		constant = constant.SetUint64(uint64(codes[pc] >> 8))
		rd = RegisterId(codes[pc+1] & 0xffff)
		rs = RegisterId(codes[pc+1] >> 16)
		//
		return rs, rd, constant, 2
	}
	//
	constant = constant.SetUint64(uint64((codes[pc] >> 24) & 0xff))
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	rs = RegisterId((codes[pc] >> 16) & 0xff)
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
// Here, rd is a u8 destination register and imm16 is the constant operand.
// The wide form extends the constant operand to imm24, moving the (now u16)
// destination register into a subsequent word:
//
// +--------+--------+--------+--------+
// |           imm24          | opcode |
// +--------+--------+--------+--------+
// |       n/a       |        rd       |
// +-----------------+-----------------+
//
// The wide form is also selected when the register is small but the constant
// exceeds 16 bits, being no larger than the general Ldc_w form.
// ============================================================================

// encodeLdc_1 encodes a load-constant instruction carrying a small (u16 or,
// in the wide form, u24) constant into the target register.
func encodeLdc_1[W word.Word[W]](constant W, rd uint16) []uint32 {
	// Sanity checks.  Constants which do not fit within 24 bits must use the
	// general form (see encodeLdc_w).
	if constant.Cmp64(0x100_0000) >= 0 {
		panic("constant exceeds short load form")
	}
	// Encoding
	c := uint32(constant.Uint64())
	//
	if IsWideRegisters(rd) || c > 0xffff {
		return []uint32{
			c<<8 | LDC | WIDE,
			uint32(rd),
		}
	}
	//
	return []uint32{
		c<<16 | uint32(rd)<<8 | LDC,
	}
}

// DecodeLdc_1 decodes a load-constant instruction carrying a small (u16 or,
// in the wide form, u24) constant, returning the constant, destination
// register and instruction width.
func DecodeLdc_1[W word.Word[W]](pc uint32, codes []uint32) (constant W, rd uint16, n uint32) {
	var c W
	//
	if IsWideForm(pc, codes) {
		c = c.SetUint64(uint64(codes[pc] >> 8))
		rd = RegisterId(codes[pc+1] & 0xffff)
		//
		return c, rd, 2
	}
	//
	c = c.SetUint64(uint64(codes[pc] >> 16))
	rd = RegisterId((codes[pc] >> 8) & 0xff)
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
// to the short LDC form which carries a single 16-bit immediate.  The wide
// form carries the (now u16) destination register in the first word, shrinking
// the limb count to a u8 (i.e. wide-form constants are capped at 255 limbs, or
// 8160 bits — far beyond anything computable, since vectored arithmetic is
// itself capped at 255 bits):
//
// +--------+--------+--------+--------+
// |        rd       | n (u8) | opcode |
// +--------+--------+--------+--------+
// |     limb 0 (least significant)    |
// +-----------------------------------+
// |                ...                |
// +-----------------------------------+
// ============================================================================

// encodeLdc_w encodes a wide load-constant instruction, carrying the constant
// inline as a sequence of 32-bit limbs (least significant first).
func encodeLdc_w[W word.Word[W]](constant W, rd uint16) []uint32 {
	var (
		// NOTE: big-endian byte ordering
		bytes  = constant.BigInt().Bytes()
		nlimbs = max(1, (len(bytes)+3)/4)
		wide   = IsWideRegisters(rd)
		codes  = make([]uint32, nlimbs+1)
	)
	//
	if wide {
		// The wide form carries only a u8 limb count.
		if nlimbs > 0xff {
			panic("constant exceeds wide load form")
		}
		//
		codes[0] = uint32(rd)<<16 | uint32(nlimbs)<<8 | LDC_w | WIDE
	} else {
		codes[0] = uint32(nlimbs)<<16 | uint32(rd)<<8 | LDC_w
	}
	// Pack bytes into limbs, least significant limb first.
	for i, b := range bytes {
		var k = uint(len(bytes) - 1 - i)
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
		nlimbs uint32
	)
	//
	if IsWideForm(pc, codes) {
		nlimbs = (codes[pc] >> 8) & 0xff
		rd = RegisterId(codes[pc] >> 16)
	} else {
		nlimbs = (codes[pc] >> 16) & 0xffff
		rd = RegisterId((codes[pc] >> 8) & 0xff)
	}
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
// The wide form moves the (now u16) registers into a subsequent word:
//
// +--------+--------+--------+--------+
// |           n/a            | opcode |
// +--------+--------+--------+--------+
// |       rs        |        rd       |
// +-----------------+-----------------+
// ============================================================================

// encodeMove_1s1 encodes a register-to-register move instruction.
func encodeMove_1s1(rs, rd uint16) []uint32 {
	if IsWideRegisters(rs, rd) {
		return []uint32{
			MOVE | WIDE,
			uint32(rd) | uint32(rs)<<16,
		}
	}
	//
	return []uint32{
		uint32(rs)<<16 | uint32(rd)<<8 | MOVE,
	}
}

// DecodeMove_1s1 decodes a register-to-register move instruction, returning the
// source and destination registers and the instruction width.
func DecodeMove_1s1(pc uint32, codes []uint32) (rs, rd uint16, n uint32) {
	if IsWideForm(pc, codes) {
		rd = RegisterId(codes[pc+1] & 0xffff)
		rs = RegisterId(codes[pc+1] >> 16)
		//
		return rs, rd, 2
	}
	//
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
// |   bw   |  nsrc  | ntgt   | opcode |
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
// StoreAcross writes the low limbs first.  The wide form retains the header
// and constant words but packs the (now u16) target and source registers two
// per word.
// ============================================================================

// encodeArith_vec encodes the general (multi-target, multi-source) vectored form
// of an arithmetic instruction (ADD_nm/SUB_nm/MUL_nm).
func encodeArith_vec[W word.Word[W]](aop bytecode.Operation, targets []RegisterId, sources []RegisterId,
	constant W, env Environment[W]) []uint32 {
	var (
		// determine total bitwidth of lhs
		lhs_bitwidth = descriptor.BitwidthOf(env.RegisterMap(), targets...).Unwrap()
		// determine bitwidth required for rhs
		rhs_bitwidth = calculateArithBitwidth(aop, sources, constant, env.RegisterMap())
		// determine overall bitwidth
		bitwidth = max(lhs_bitwidth, rhs_bitwidth)
	)
	//
	if len(targets) == 0 {
		panic("targetless arithmetic instructions not supported")
	} else if len(targets) >= 256 || len(sources) >= 256 {
		panic("vector arithmetic operand counts not supported")
	} else if constant.Cmp64(^uint64(0)) > 0 {
		panic("wide vector arithmetic constants not supported")
	} else if bitwidth > 255 {
		panic("wide vector arithmetic not supported")
	}
	//
	var (
		opcode = ADD_nm + uint32(aop-bytecode.OP_ADD)
		bw     = uint32(bitwidth) << 24
		nsrc   = uint32(len(sources)) << 16
		ntgt   = uint32(len(targets)) << 8
		c      = constant.Uint64()
		regs   = append(RegsAsShorts(targets), RegsAsShorts(sources)...)
	)
	//
	if IsWideRegisters(regs...) {
		codes := []uint32{bw | nsrc | ntgt | opcode | WIDE, uint32(c), uint32(c >> 32)}
		//
		return append(codes, PackShortsIntoCodes(regs)...)
	}
	//
	var (
		codes    = []uint32{bw | nsrc | ntgt | opcode, uint32(c), uint32(c >> 32)}
		regBytes = append(RegsAsBytes(targets), RegsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(regBytes)...)
}

// DecodeArith_nm decodes a vectored (multi-target, multi-source) arithmetic
// instruction, returning iterators over its target and source registers, the
// constant operand and the instruction width.
func DecodeArith_nm[W word.Word[W]](pc uint32, codes []uint32) (
	targets, sources Operands, constant W, bitwidth uint, n uint32) {
	//
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
		bw       = uint((codes[pc] >> 24) & 0xff)
		c        = uint64(codes[pc+1]) | (uint64(codes[pc+2]) << 32)
	)
	//
	if IsWideForm(pc, codes) {
		targets = NewWideOperands(0, ntargets, codes[pc+3:])
		sources = NewWideOperands(ntargets, nsources, codes[pc+3:])
		n = 3 + NumCodesPackedWide(ntargets+nsources)
	} else {
		targets = NewOperands(0, ntargets, codes[pc+3:])
		sources = NewOperands(ntargets, nsources, codes[pc+3:])
		n = 3 + NumCodesPackedSmall(ntargets+nsources)
	}
	//
	constant = constant.SetUint64(c)
	//
	return targets, sources, constant, bw, n
}

func calculateArithBitwidth[W word.Word[W]](aop bytecode.Operation, sources []RegisterId, constant W,
	env descriptor.RegisterMap[W]) uint {
	//
	var bitwidth util.Option[uint]
	//
	switch aop {
	case bytecode.OP_ADD:
		bitwidth = descriptor.CalculateAddBitwidth(sources, constant, env)
	case bytecode.OP_SUB:
		bitwidth = descriptor.CalculateSubBitwidth(sources, constant, env)
	case bytecode.OP_MUL:
		bitwidth = descriptor.CalculateMulBitwidth(sources, constant, env)
	default:
		panic("unknown operation encountered")
	}
	//
	return bitwidth.Unwrap()
}
