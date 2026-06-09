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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

func newArith[W word.Word[W]](op arithOp, targets []Reg, sources []Reg, constant W) *Arith[W] {
	return &Arith[W]{op, constant, sources, targets}
}

// ============================================================================

// Arith (arithmetic) instruction encodes a wide range of related arithmetic
// operations (e.g. +,-,*) including various bitwise operations.
type Arith[W word.Word[W]] struct {
	Op       arithOp
	Constant W
	Source   []Reg
	Target   []Reg
}

func (p *Arith[W]) String(mapping SystemMap) string {
	var (
		builder strings.Builder
		cz      = IsUnusedConstant(p.Op, p.Constant)
		cstr    = fmt.Sprintf("0x%s", p.Constant.Text(16))
	)
	//
	builder.WriteString(registersToString(array.Reverse(p.Target), mapping, "::"))
	builder.WriteString(" = ")
	builder.WriteString(registersToString(p.Source, mapping, p.Op.String()))
	//
	if len(p.Source) == 0 {
		builder.WriteString(cstr)
	} else if !cz {
		builder.WriteString(p.Op.String())
		builder.WriteString(cstr)
	}
	//
	return builder.String()
}

// Codes implementation for Bytecode interface
func (p *Arith[W]) Codes(_ uint32) []uint32 {
	var (
		n  = len(p.Source)
		m  = len(p.Target)
		cz = IsUnusedConstant(p.Op, p.Constant)
	)
	//
	switch {
	case n == 0 && m == 1:
		return encodeLdc_1(p.Constant, p.Target[0])
	case n == 1 && m == 1 && cz:
		return encodeMove_1s1(p.Source[0], p.Target[0])
	case n == 1 && m == 1:
		return encodeArith_1n1c(p.Op, p.Source[0], p.Target[0], p.Constant)
	case n == 2 && m == 1:
		// x*y*0 is always 0: fold to a constant load so the intermediate
		// product x*y cannot raise a spurious overflow error.
		if p.Op == arithop_MUL && p.Constant.Cmp64(0) == 0 {
			return encodeLdc_1(p.Constant, p.Target[0])
		}
		// There is no 2-source-plus-constant instruction form, so compute
		// "x op y" first, then fold in the constant (when used) with a second
		// one-source instruction operating in place on the target.
		codes := encodeArith_2n1(p.Op, p.Source[0], p.Source[1], p.Target[0])
		//
		if !cz {
			codes = append(codes, encodeArith_1n1c(p.Op, p.Target[0], p.Target[0], p.Constant)...)
		}
		//
		return codes
	case m > 0:
		return encodeArith_vec(p.Op, p.Target, p.Source, p.Constant)
	default:
		panic(fmt.Sprintf("unsupported arithmetic instruction form (%d, %d, %t)", n, m, cz))
	}
}

func decodeArith[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		rs0, rs1, rd Reg
		opcode       = codes[pc] & OPCODE_MASK
		constant     W
		sources      []Reg
		targets      []Reg
		n            uint32
		op           arithOp
	)

	switch opcode {
	case ADD_2n1, SUB_2n1:
		rs0, rs1, rd, n = decodeArith_2n1(pc, codes)
		sources = []Reg{rs0, rs1}
		targets = []Reg{rd}
		op = opcodeToArithOp(opcode)
	case MUL_2n1:
		rs0, rs1, rd, n = decodeArith_2n1(pc, codes)
		sources = []Reg{rs0, rs1}
		targets = []Reg{rd}
		constant = word.Const64[W](1)
		op = opcodeToArithOp(opcode)
	case ADDC, SUBC, MULC:
		rs0, rd, constant, n = decodeArith_1n1c[W](pc, codes)
		sources = []Reg{rs0}
		targets = []Reg{rd}
		op = opcodeToArithOp(opcode)
	case ARITHV:
		op, targets, sources, constant, n = decodeArith_vec[W](pc, codes)
	case MOVE:
		rs0, rd, n = decodeMove_1s1(pc, codes)
		sources = []Reg{rs0}
		targets = []Reg{rd}
		op = arithop_ADD
	case LDC:
		constant, rd, n = decodeLdc_1[W](pc, codes)
		targets = []Reg{rd}
		op = arithop_ADD
	default:
		panic("unsupported instruction form")
	}
	//
	return &Arith[W]{Op: op, Constant: constant, Source: sources, Target: targets}, n
}

func opcodeToArithOp(opcode uint32) arithOp {
	switch opcode {
	case ADD_2n1, ADDC:
		return arithop_ADD
	case SUB_2n1, SUBC:
		return arithop_SUB
	case MUL_2n1, MULC:
		return arithop_MUL
	default:
		panic("unknown arithmetic operation")
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

func encodeArith_2n1(aop arithOp, rs0, rs1, rd uint16) []uint32 {
	var (
		_rd    = uint32(rd) << 8
		_rs1   = uint32(rs1) << 16
		_rs0   = uint32(rs0) << 24
		opcode = ADD_2n1 + uint32(aop.tag)
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

func decodeArith_2n1(pc uint32, codes []uint32) (rs0, rs1, rd uint16, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs1 = Reg((codes[pc] >> 16) & 0xff)
	rs0 = Reg((codes[pc] >> 24) & 0xff)
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

func encodeArith_1n1c[W word.Word[W]](aop arithOp, rs, rd uint16, constant W) []uint32 {
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
		opcode = ADDC + uint32(aop.tag)
	)
	//
	return []uint32{
		_imm | _rs | _rd | opcode,
	}
}

func decodeArith_1n1c[W word.Word[W]](pc uint32, codes []uint32) (rs, rd uint16, constant W, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs = Reg((codes[pc] >> 16) & 0xff)
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

func encodeLdc_1[W word.Word[W]](constant W, rd uint16) []uint32 {
	// Sanity checks
	if rd >= 256 || constant.Cmp64(65536) > 1 {
		// NOTE: this corresponds to a WIDE instruction, but these are not
		// supported at this time.
		panic("wide instructions not supported")
	}
	// Encoding
	_rd := uint32(rd) << 8
	c := uint32(constant.Uint64()) << 16
	//
	return []uint32{
		c | _rd | LDC,
	}
}

func decodeLdc_1[W word.Word[W]](pc uint32, codes []uint32) (constant W, rd uint16, n uint32) {
	var c W
	//
	rd = Reg((codes[pc] >> 8) & 0xff)
	c = c.SetUint64(uint64(codes[pc] >> 16))
	//
	return c, rd, 1
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

func decodeMove_1s1(pc uint32, codes []uint32) (rs, rd uint16, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs = Reg((codes[pc] >> 16) & 0xff)
	//
	return rs, rd, 1
}

// ============================================================================
// Vector-target arithmetic instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  | ntgt   | opcode |
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
// Targets are packed first because StoreAcross writes the low limbs first.
// ============================================================================

func encodeArith_vec[W word.Word[W]](aop arithOp, targets []Reg, sources []Reg, constant W) []uint32 {
	if len(targets) == 0 || len(targets) >= 256 || len(sources) >= 256 {
		panic("wide vector arithmetic instructions not supported")
	} else if constant.Cmp64(^uint64(0)) > 0 {
		panic("wide vector arithmetic constants not supported")
	}
	//
	var (
		opcode   = uint32(aop.tag) << 24
		nsrc     = uint32(len(sources)) << 16
		ntgt     = uint32(len(targets)) << 8
		c        = constant.Uint64()
		codes    = []uint32{opcode | nsrc | ntgt | ARITHV, uint32(c), uint32(c >> 32)}
		regBytes = append(regsAsBytes(targets), regsAsBytes(sources)...)
	)
	//
	return append(codes, packRegsIntoCodes(regBytes)...)
}

func decodeArith_vec[W word.Word[W]](pc uint32, codes []uint32) (
	op arithOp, targets, sources []Reg, constant W, n uint32) {
	//
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
		tag      = uint8((codes[pc] >> 24) & 0xff)
		c        = uint64(codes[pc+1]) | (uint64(codes[pc+2]) << 32)
		iter     = NewOp8Iter(0, codes[pc+3:])
		regs     = OpIterToArray[uint16](ntargets+nsources, iter)
	)
	//
	op = arithOpFromTag(tag)
	constant = constant.SetUint64(c)
	//
	return op, regs[:ntargets], regs[ntargets:], constant,
		3 + nCodesPackedSmall(uint32(ntargets+nsources))
}

// ============================================================================

type arithOp struct{ tag uint8 }

func (p arithOp) String() string {
	switch p {
	case arithop_ADD:
		return " + "
	case arithop_SUB:
		return " - "
	case arithop_MUL:
		return " * "
	default:
		panic("unknown arithmetic operation")
	}
}

// IsUnusedConstant checks whether a given constant is the "identity element".
// This depends on the arithmetic operation in question.  For example, for
// addition and subtraction, this is zero.  But, for multiplication it is one.
func IsUnusedConstant[W word.Word[W]](op arithOp, constant W) bool {
	switch op {
	case arithop_ADD:
		return constant.Cmp64(0) == 0
	case arithop_SUB:
		return constant.Cmp64(0) == 0
	case arithop_MUL:
		return constant.Cmp64(1) == 0
	default:
		panic("unknown arithmetic operation")
	}
}

func arithOpFromTag(tag uint8) arithOp {
	switch tag {
	case arithop_ADD.tag:
		return arithop_ADD
	case arithop_SUB.tag:
		return arithop_SUB
	case arithop_MUL.tag:
		return arithop_MUL
	default:
		panic("unknown arithmetic operation")
	}
}

var (
	arithop_ADD = arithOp{0}
	arithop_SUB = arithOp{1}
	arithop_MUL = arithOp{2}
)
