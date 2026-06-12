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

// DivRem computes the (truncated) integer quotient or remainder of two
// registers.  The operation is identified by Opcode, which is one of DIV or
// REM.  A zero divisor aborts execution with a division-by-zero error.
type DivRem struct {
	// Opcode selects the operation (DIV or REM).
	Opcode uint32
	// Target receives the result.
	Target Reg
	// Dividend and Divisor are the operand registers.
	Dividend, Divisor Reg
}

func (p *DivRem) String(mapping SystemMap) string {
	var (
		target   = registerToString(p.Target, mapping)
		dividend = registerToString(p.Dividend, mapping)
		divisor  = registerToString(p.Divisor, mapping)
		symbol   = "/"
	)
	//
	if p.Opcode == REM {
		symbol = "%"
	}
	//
	return fmt.Sprintf("%s = %s %s %s", target, dividend, symbol, divisor)
}

// Codes implementation for Bytecode interface.
func (p *DivRem) Codes(_ uint32) []uint32 {
	return encodeDivRem(p.Opcode, p.Target, p.Dividend, p.Divisor)
}

func decodeDivRem[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op              = codes[pc] & OPCODE_MASK
		rd, lhs, rhs, n = decodeDivRem_2n1(pc, codes)
	)
	//
	return &DivRem{op, rd, lhs, rhs}, n
}

// ============================================================================
// DIV / REM instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | divisor|dividend|   rd   | opcode |
// +--------+--------+--------+--------+
//
// The opcode itself distinguishes the two operations, so no width is needed.
// ============================================================================

func encodeDivRem(op uint32, rd, dividend, divisor Reg) []uint32 {
	if rd >= 256 || dividend >= 256 || divisor >= 256 {
		panic("wide division instructions not supported")
	}
	//
	return []uint32{uint32(divisor)<<24 | uint32(dividend)<<16 | uint32(rd)<<8 | op}
}

func decodeDivRem_2n1(pc uint32, codes []uint32) (rd, dividend, divisor Reg, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	dividend = Reg((codes[pc] >> 16) & 0xff)
	divisor = Reg((codes[pc] >> 24) & 0xff)
	//
	return rd, dividend, divisor, 1
}

// NewDivRem constructs a division/remainder bytecode for op (DIV or REM).
func NewDivRem(op uint32, target, dividend, divisor register.Id) *DivRem {
	return &DivRem{op, asReg(target), asReg(dividend), asReg(divisor)}
}

// DivHint computes quotient, remainder and range witness for a division hint
// (i.e. as produced by the LowerDivisions transform).  Specifically, Quotient =
// Dividend / Divisor, Remainder = Dividend % Divisor and Witness = Divisor -
// Remainder - 1, with correctness validated by subsequent arithmetic checks.  A
// zero divisor aborts execution with a division-by-zero error.
type DivHint struct {
	// Quotient, Remainder and Witness receive the results.
	Quotient, Remainder, Witness Reg
	// Dividend and Divisor are the operand registers.
	Dividend, Divisor Reg
}

func (p *DivHint) String(mapping SystemMap) string {
	var (
		quotient  = registerToString(p.Quotient, mapping)
		remainder = registerToString(p.Remainder, mapping)
		witness   = registerToString(p.Witness, mapping)
		dividend  = registerToString(p.Dividend, mapping)
		divisor   = registerToString(p.Divisor, mapping)
	)
	//
	return fmt.Sprintf("%s::%s::%s = hint(%s, %s)", quotient, remainder, witness, dividend, divisor)
}

// Codes implementation for Bytecode interface.
func (p *DivHint) Codes(_ uint32) []uint32 {
	return encodeDivHint(p.Quotient, p.Remainder, p.Witness, p.Dividend, p.Divisor)
}

func decodeDivHint[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var rq, rr, rw, rx, ry, n = decodeDivHint_2n3(pc, codes)
	//
	return &DivHint{rq, rr, rw, rx, ry}, n
}

// ============================================================================
// DIVHINT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   rw   |   rr   |   rq   | opcode |
// +--------+--------+--------+--------+
// |   n/a  |   n/a  |   ry   |   rx   |
// +--------+--------+--------+--------+
//
// Here, rx and ry are the dividend and divisor source registers, whilst rq, rr
// and rw are the quotient, remainder and witness destination registers.
// ============================================================================

func encodeDivHint(rq, rr, rw, rx, ry Reg) []uint32 {
	if rq >= 256 || rr >= 256 || rw >= 256 || rx >= 256 || ry >= 256 {
		panic("wide division hint instructions not supported")
	}
	//
	return []uint32{
		uint32(rw)<<24 | uint32(rr)<<16 | uint32(rq)<<8 | DIVHINT,
		uint32(ry)<<8 | uint32(rx),
	}
}

func decodeDivHint_2n3(pc uint32, codes []uint32) (rq, rr, rw, rx, ry Reg, n uint32) {
	rq = Reg((codes[pc] >> 8) & 0xff)
	rr = Reg((codes[pc] >> 16) & 0xff)
	rw = Reg((codes[pc] >> 24) & 0xff)
	rx = Reg(codes[pc+1] & 0xff)
	ry = Reg((codes[pc+1] >> 8) & 0xff)
	//
	return rq, rr, rw, rx, ry, 2
}

// NewDivHint constructs a division hint bytecode.
func NewDivHint(quotient, remainder, witness, dividend, divisor register.Id) *DivHint {
	return &DivHint{asReg(quotient), asReg(remainder), asReg(witness), asReg(dividend), asReg(divisor)}
}
