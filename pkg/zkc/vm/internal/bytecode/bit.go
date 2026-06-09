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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Not computes a bitwise complement within the given bit width.
type Not struct {
	// Bitwidth bounds the complement mask.
	Bitwidth uint8
	// Target receives the complemented value.
	Target Reg
	// Source provides the value to complement.
	Source Reg
}

func (p *Not) String(mapping SystemMap) string {
	var (
		target = registerToString(p.Target, mapping)
		source = registerToString(p.Source, mapping)
	)
	//
	return fmt.Sprintf("%s = ~%s [u%d]", target, source, p.Bitwidth)
}

// Codes implementation for Bytecode interface.
func (p *Not) Codes(_ uint32) []uint32 {
	return encodeNot(p.Target, p.Source, p.Bitwidth)
}

func decodeNot[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		target, source, bitwidth, n = decodeNot_1n1(pc, codes)
	)
	//
	return &Not{bitwidth, target, source}, n
}

// ============================================================================
// NOT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | width  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// This intentionally stays small; wide bit widths can be added when needed.
// ============================================================================

func encodeNot(rd, rs Reg, bitwidth uint8) []uint32 {
	if rd >= 256 || rs >= 256 {
		panic("wide not instructions not supported")
	}
	//
	return []uint32{uint32(bitwidth)<<24 | uint32(rs)<<16 | uint32(rd)<<8 | NOT}
}

func decodeNot_1n1(pc uint32, codes []uint32) (rd, rs Reg, bitwidth uint8, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs = Reg((codes[pc] >> 16) & 0xff)
	bitwidth = uint8((codes[pc] >> 24) & 0xff)
	//
	return rd, rs, bitwidth, 1
}

// NewNot constructs a bitwise-not bytecode.
func NewNot(target, source register.Id, bitwidth uint) *Not {
	if bitwidth >= 256 {
		panic("wide not instructions not supported")
	}
	//
	return &Not{uint8(bitwidth), asReg(target), asReg(source)}
}

// Bitwise computes a binary bitwise operation between two registers.  The
// operation is identified by Opcode, which is one of AND, OR or XOR.
type Bitwise struct {
	// Opcode selects the operation (AND, OR or XOR).
	Opcode uint32
	// Target receives the result.
	Target Reg
	// Left and Right are the operand registers.
	Left, Right Reg
}

func (p *Bitwise) String(mapping SystemMap) string {
	var (
		target = registerToString(p.Target, mapping)
		left   = registerToString(p.Left, mapping)
		right  = registerToString(p.Right, mapping)
	)
	//
	return fmt.Sprintf("%s = %s %s %s", target, left, bitwiseSymbol(p.Opcode), right)
}

// Codes implementation for Bytecode interface.
func (p *Bitwise) Codes(_ uint32) []uint32 {
	return encodeBitwise(p.Opcode, p.Target, p.Left, p.Right)
}

func decodeBitwise[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	op, rd, lhs, rhs, n := decodeBitwise_2n1(pc, codes)
	//
	return &Bitwise{op, rd, lhs, rhs}, n
}

// ============================================================================
// AND / OR / XOR instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | right  |  left  |   rd   | opcode |
// +--------+--------+--------+--------+
//
// The opcode itself distinguishes the three operations, so no width is needed.
// ============================================================================

func encodeBitwise(op uint32, rd, lhs, rhs Reg) []uint32 {
	if rd >= 256 || lhs >= 256 || rhs >= 256 {
		panic("wide bitwise instructions not supported")
	}
	//
	return []uint32{uint32(rhs)<<24 | uint32(lhs)<<16 | uint32(rd)<<8 | op}
}

func decodeBitwise_2n1(pc uint32, codes []uint32) (op uint32, rd, lhs, rhs Reg, n uint32) {
	op = codes[pc] & OPCODE_MASK
	rd = Reg((codes[pc] >> 8) & 0xff)
	lhs = Reg((codes[pc] >> 16) & 0xff)
	rhs = Reg((codes[pc] >> 24) & 0xff)
	//
	return op, rd, lhs, rhs, 1
}

func bitwiseSymbol(op uint32) string {
	switch op {
	case AND:
		return "&"
	case OR:
		return "|"
	case XOR:
		return "^"
	default:
		panic("unknown bitwise operation")
	}
}

// NewBitwise constructs a binary bitwise bytecode for op (AND, OR or XOR).
func NewBitwise(op uint32, target, left, right register.Id) *Bitwise {
	return &Bitwise{op, asReg(target), asReg(left), asReg(right)}
}

// Shift shifts the value in Source by the amount held in Amount.  Left shifts
// (SHL) mask the result to Bitwidth bits; right shifts (SHR) ignore Bitwidth.
type Shift struct {
	// Opcode selects the direction (SHL or SHR).
	Opcode uint32
	// Bitwidth bounds a left-shifted result.
	Bitwidth uint8
	// Target receives the result.
	Target Reg
	// Source is the value being shifted; Amount is the shift distance.
	Source, Amount Reg
}

func (p *Shift) String(mapping SystemMap) string {
	var (
		target = registerToString(p.Target, mapping)
		source = registerToString(p.Source, mapping)
		amount = registerToString(p.Amount, mapping)
		symbol = "<<"
	)
	//
	if p.Opcode == SHR {
		symbol = ">>"
	}
	//
	return fmt.Sprintf("%s = %s %s %s [u%d]", target, source, symbol, amount, p.Bitwidth)
}

// Codes implementation for Bytecode interface.
func (p *Shift) Codes(_ uint32) []uint32 {
	return encodeShift(p.Opcode, p.Target, p.Source, p.Amount, p.Bitwidth)
}

func decodeShift[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op                       = codes[pc] & OPCODE_MASK
		rd, rs, amt, bitwidth, n = decodeShift_2n1(pc, codes)
	)
	//
	return &Shift{op, bitwidth, rd, rs, amt}, n
}

// ============================================================================
// SHL / SHR instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | amount | source |   rd   | opcode |
// +--------+--------+--------+--------+
// |              bitwidth             |
// +--------+--------+--------+--------+
//
// A second word carries the (left-shift) mask width, which does not fit
// alongside the three register operands.
// ============================================================================

func encodeShift(op uint32, rd, rs, amt Reg, bitwidth uint8) []uint32 {
	if rd >= 256 || rs >= 256 || amt >= 256 {
		panic("wide shift instructions not supported")
	}
	//
	return []uint32{
		uint32(amt)<<24 | uint32(rs)<<16 | uint32(rd)<<8 | op,
		uint32(bitwidth),
	}
}

func decodeShift_2n1(pc uint32, codes []uint32) (rd, rs, amt Reg, bitwidth uint8, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs = Reg((codes[pc] >> 16) & 0xff)
	amt = Reg((codes[pc] >> 24) & 0xff)
	bitwidth = uint8(codes[pc+1])
	//
	return rd, rs, amt, bitwidth, 2
}

// NewShift constructs a shift bytecode for op (SHL or SHR).
func NewShift(op uint32, target, source, amount register.Id, bitwidth uint) *Shift {
	if bitwidth >= 256 {
		panic("wide shift instructions not supported")
	}
	//
	return &Shift{op, uint8(bitwidth), asReg(target), asReg(source), asReg(amount)}
}
