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
		constIsUint16 = p.Constant.Cmp64(0x1_0000) < 0
	)
	//
	switch {
	case n == 0 && m == 1 && constIsUint16 && !IsWideRegisters(p.Target[0]):
		return encodeLdc_1(p.Constant, p.Target[0])
	case n == 0 && m == 1:
		return encodeLdc_w(p.Constant, p.Target[0], env)
	case n == 1 && m == 1 && cz:
		return encodeMove_1s1(p.Source[0], p.Target[0])
	case n == 1 && m == 1:
		return encodeArith_1n1c(p.Op, p.Source[0], p.Target[0], p.Constant, env)
	case n == 2 && m == 1 && cz:
		return encodeArith_2n1(p.Op, p.Source[0], p.Source[1], p.Target[0])
	case n == 2 && m == 1 && p.Op != bytecode.OP_SUB:
		// SUB is excluded: the two-step pairing wraps each step separately,
		// which is not equivalent to the single wrap (at CalculateSubBitwidth
		// width) that the vectored form performs when the first step
		// underflows.  See encodeArith_2n1c.
		return encodeArith_2n1c(p.Op, p.Source[0], p.Source[1], p.Target[0], p.Constant, env)
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
// replaces the constant operand with a u16 constant pool identifier, moving
// the (now u16) registers into a subsequent word:
//
// +--------+--------+--------+--------+
// |       cid       |  n/a   | opcode |
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
func encodeArith_2n1c[W word.Word[W]](aop bytecode.Operation, rs0, rs1, rd uint16, constant W,
	env Environment[W]) []uint32 {
	// Sanity check
	if aop == bytecode.OP_SUB {
		panic("two-step encoding unsound for subtraction")
	}
	// There is no 2-source-plus-constant instruction form, so compute
	// "x op y" first, then fold in the constant (when used) with a second
	// one-source instruction operating in place on the target.
	codes := encodeArith_2n1(aop, rs0, rs1, rd)
	//
	return append(codes, encodeArith_1n1c(aop, rd, rd, constant, env)...)
}

// encodeArith_1n1c encodes a one-source, one-target arithmetic-with-constant
// instruction (ADDC/SUBC/MULC).
func encodeArith_1n1c[W word.Word[W]](aop bytecode.Operation, rs, rd uint16, constant W,
	env Environment[W]) []uint32 {
	//
	var opcode = ADDC + uint32(aop-bytecode.OP_ADD)
	// The wide (pooled) form carries a constant of arbitrary width; the narrow
	// form only a u8 immediate.
	if IsWideRegisters(rs, rd) || constant.Cmp64(0xff) > 0 {
		return []uint32{
			uint32(env.ConstantIndex(constant))<<16 | opcode | WIDE,
			uint32(rd) | uint32(rs)<<16,
		}
	}
	//
	return []uint32{
		uint32(constant.Uint64())<<24 | uint32(rs)<<16 | uint32(rd)<<8 | opcode,
	}
}

// DecodeArith_1n1c decodes a one-source-plus-constant arithmetic instruction,
// returning the source and destination registers, constant and instruction width.
func DecodeArith_1n1c[W word.Word[W]](pc uint32, codes []uint32, pool []W) (rs, rd uint16, constant W, n uint32) {
	if IsWideForm(pc, codes) {
		constant = pool[codes[pc]>>16]
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
// There is no wide form: a wide (u16) register, or a constant exceeding 16
// bits, uses the pooled Ldc_w form instead (which is no larger).
// ============================================================================

// encodeLdc_1 encodes a load-constant instruction carrying a small (u16)
// constant into the target register.
func encodeLdc_1[W word.Word[W]](constant W, rd uint16) []uint32 {
	// Sanity checks.  Constants which do not fit within 16 bits, or wide
	// registers, must use the pooled form (see encodeLdc_w).
	if constant.Cmp64(0x1_0000) >= 0 || IsWideRegisters(rd) {
		panic("constant exceeds short load form")
	}
	//
	return []uint32{
		uint32(constant.Uint64())<<16 | uint32(rd)<<8 | LDC,
	}
}

// DecodeLdc_1 decodes a load-constant instruction carrying a small (u16)
// constant, returning the constant, destination register and instruction
// width.
func DecodeLdc_1[W word.Word[W]](pc uint32, codes []uint32) (constant W, rd uint16, n uint32) {
	var c W
	//
	c = c.SetUint64(uint64(codes[pc] >> 16))
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	//
	return c, rd, 1
}

// ============================================================================
// Ldc_w (pooled load constant) instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |       cid       |   rd   | opcode |
// +--------+--------+--------+--------+
//
// Here, rd is a u8 destination register and cid is a u16 constant pool
// identifier for the constant operand.  This form supports constants of
// arbitrary width (e.g. field-sized constants on a Uint machine), in contrast
// to the short LDC form which carries a single 16-bit immediate.  The wide
// form keeps the pool identifier in place, moving the (now u16) destination
// register into a subsequent word:
//
// +--------+--------+--------+--------+
// |       cid       |  n/a   | opcode |
// +--------+--------+--------+--------+
// |       n/a       |        rd       |
// +-----------------+-----------------+
// ============================================================================

// encodeLdc_w encodes a pooled load-constant instruction, carrying its
// constant operand as a u16 constant pool identifier.
func encodeLdc_w[W word.Word[W]](constant W, rd uint16, env Environment[W]) []uint32 {
	var cid = uint32(env.ConstantIndex(constant))
	//
	if IsWideRegisters(rd) {
		return []uint32{
			cid<<16 | LDC_w | WIDE,
			uint32(rd),
		}
	}
	//
	return []uint32{
		cid<<16 | uint32(rd)<<8 | LDC_w,
	}
}

// DecodeLdc_w decodes a pooled load-constant instruction, returning the
// constant, destination register and instruction width.
func DecodeLdc_w[W word.Word[W]](pc uint32, codes []uint32, pool []W) (constant W, rd uint16, n uint32) {
	var cid = codes[pc] >> 16
	//
	if IsWideForm(pc, codes) {
		return pool[cid], RegisterId(codes[pc+1] & 0xffff), 2
	}
	//
	return pool[cid], RegisterId((codes[pc] >> 8) & 0xff), 1
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
// |       cid (constant pool id)       |
// +------------------------------------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// The arithmetic operation (add, subtract or multiply) is identified by the
// opcode itself (ADD_nm, SUB_nm or MUL_nm), whilst the constant operand is
// carried as a (u16) constant pool identifier.  Targets are packed first
// because StoreAcross writes the low limbs first.  The wide form moves the
// target count into the upper half of the constant pool word (leaving bits
// 8-15 of the first word clear, as for all wide forms) and packs the (now
// u16) target and source registers two per word:
//
// +--------+--------+--------+--------+
// |   bw   |  nsrc  |  n/a   | opcode |
// +--------+--------+--------+--------+
// |      ntgt       |       cid       |
// +-----------------+-----------------+
// |       tgt1      |       tgt0      |
// +-----------------+-----------------+
// | ... packed source registers ...    |
// +------------------------------------+
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
	} else if bitwidth > 255 {
		panic("wide vector arithmetic not supported")
	}
	//
	var (
		opcode = ADD_nm + uint32(aop-bytecode.OP_ADD)
		bw     = uint32(bitwidth) << 24
		nsrc   = uint32(len(sources)) << 16
		ntgt   = uint32(len(targets)) << 8
		cid    = uint32(env.ConstantIndex(constant))
		regs   = append(RegsAsShorts(targets), RegsAsShorts(sources)...)
	)
	//
	if IsWideRegisters(regs...) {
		codes := []uint32{bw | nsrc | opcode | WIDE, uint32(len(targets))<<16 | cid}
		//
		return append(codes, PackShortsIntoCodes(regs)...)
	}
	//
	var (
		codes    = []uint32{bw | nsrc | ntgt | opcode, cid}
		regBytes = append(RegsAsBytes(targets), RegsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(regBytes)...)
}

// DecodeArith_nm decodes a vectored (multi-target, multi-source) arithmetic
// instruction, returning iterators over its target and source registers, the
// constant operand and the instruction width.
func DecodeArith_nm[W word.Word[W]](pc uint32, codes []uint32, pool []W) (
	targets, sources Operands, constant W, bitwidth uint, n uint32) {
	//
	var (
		nsources = uint((codes[pc] >> 16) & 0xff)
		bw       = uint((codes[pc] >> 24) & 0xff)
	)
	//
	if IsWideForm(pc, codes) {
		var ntargets = uint(codes[pc+1] >> 16)
		//
		constant = pool[codes[pc+1]&0xffff]
		targets = NewWideOperands(0, ntargets, codes[pc+2:])
		sources = NewWideOperands(ntargets, nsources, codes[pc+2:])
		n = 2 + NumCodesPackedWide(ntargets+nsources)
	} else {
		var ntargets = uint((codes[pc] >> 8) & 0xff)
		//
		constant = pool[codes[pc+1]]
		targets = NewOperands(0, ntargets, codes[pc+2:])
		sources = NewOperands(ntargets, nsources, codes[pc+2:])
		n = 2 + NumCodesPackedSmall(ntargets+nsources)
	}
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
