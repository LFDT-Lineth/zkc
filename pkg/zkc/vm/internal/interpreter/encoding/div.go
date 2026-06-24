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

// DivRem encodes a division/remainder bytecode; the opcode held within the
// bytecode selects between quotient (DIV) and remainder (REM).
func DivRem(p *bytecode.DivRem) []uint32 {
	return encodeDivRem(p.Opcode, p.Target, p.Dividend, p.Divisor)
}

// DivHint encodes a division-hint bytecode, which supplies the prover with the
// quotient, remainder and witness for a division.
func DivHint(p *bytecode.DivHint) []uint32 {
	return encodeDivHint(p.Quotient, p.Remainder, p.Witness, p.Dividend, p.Divisor)
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

// encodeDivRem encodes a division/remainder instruction, where op distinguishes
// the two operations.
func encodeDivRem(op uint32, rd, dividend, divisor RegisterId) []uint32 {
	if rd >= 256 || dividend >= 256 || divisor >= 256 {
		panic("wide division instructions not supported")
	}
	//
	return []uint32{uint32(divisor)<<24 | uint32(dividend)<<16 | uint32(rd)<<8 | op}
}

// DecodeDivRem_2n1 decodes the operands of a division/remainder instruction.
func DecodeDivRem_2n1(pc uint32, codes []uint32) (rd, dividend, divisor RegisterId, n uint32) {
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	dividend = RegisterId((codes[pc] >> 16) & 0xff)
	divisor = RegisterId((codes[pc] >> 24) & 0xff)
	//
	return rd, dividend, divisor, 1
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

// encodeDivHint encodes a division-hint instruction, where rx and ry are the
// dividend and divisor and rq, rr and rw are the quotient, remainder and witness
// destination registers.
func encodeDivHint(rq, rr, rw, rx, ry RegisterId) []uint32 {
	if rq >= 256 || rr >= 256 || rw >= 256 || rx >= 256 || ry >= 256 {
		panic("wide division hint instructions not supported")
	}
	//
	return []uint32{
		uint32(rw)<<24 | uint32(rr)<<16 | uint32(rq)<<8 | DIVHINT,
		uint32(ry)<<8 | uint32(rx),
	}
}

// DecodeDivHint_2n3 decodes the operands of a division-hint instruction.
func DecodeDivHint_2n3(pc uint32, codes []uint32) (rq, rr, rw, rx, ry RegisterId, n uint32) {
	rq = RegisterId((codes[pc] >> 8) & 0xff)
	rr = RegisterId((codes[pc] >> 16) & 0xff)
	rw = RegisterId((codes[pc] >> 24) & 0xff)
	rx = RegisterId(codes[pc+1] & 0xff)
	ry = RegisterId((codes[pc+1] >> 8) & 0xff)
	//
	return rq, rr, rw, rx, ry, 2
}
