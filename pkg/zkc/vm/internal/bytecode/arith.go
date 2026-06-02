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
	case n == 2 && m == 1 && cz:
		return encodeArith_2n1(p.Op, p.Source[0], p.Source[1], p.Target[0])
	default:
		panic(fmt.Sprintf("unsupported add instruction form (%d, %d, %t)", n, m, cz))
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
	case ADD_2n1:
		return arithop_ADD
	case SUB_2n1:
		return arithop_SUB
	case MUL_2n1:
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

var (
	arithop_ADD = arithOp{0}
	arithop_SUB = arithOp{1}
	arithop_MUL = arithOp{2}
)
