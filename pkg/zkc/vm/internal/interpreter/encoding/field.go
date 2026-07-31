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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FieldArith encodes a field-arithmetic bytecode (ADDMOD_P/SUBMOD_P/MULMOD_P).
func FieldArith[W word.Word[W]](p *bytecode.FieldArith[W], env Environment[W]) []uint32 {
	return encodeFieldArith(p.Op, p.Target, p.Sources, p.Constant, env)
}

// DecodeFieldArithOperands extracts the raw operands (target register, source
// register iterator, constant and instruction width) of a field-arithmetic
// instruction.  It is shared by the disassembler (DecodeFieldArith) and the
// interpreter's executor.
func DecodeFieldArithOperands[W word.Word[W]](pc uint32, codes []uint32, pool []W) (
	rd RegisterId, sources Operands, constant W, n uint32) {
	//
	var nsrc = uint((codes[pc] >> 24) & 0xff)
	//
	if IsWideForm(pc, codes) {
		rd = RegisterId(codes[pc+1] & 0xffff)
		constant = pool[codes[pc+1]>>16]
		sources = NewWideOperands(0, nsrc, codes[pc+2:])
		n = 2 + NumCodesPackedWide(nsrc)
	} else {
		rd = RegisterId((codes[pc] >> 8) & 0xff)
		constant = pool[(codes[pc]>>16)&0xff]
		sources = NewOperands(0, nsrc, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(nsrc)
	}
	//
	return rd, sources, constant, n
}

// ============================================================================
// ADDMOD_P / SUBMOD_P / MULMOD_P instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nsrc  |  cid   |   rd   | opcode |
// +--------+--------+--------+--------+
// | ... packed source registers ...   |
// +-----------------------------------+
//
// Here, rd is a u8 destination register, cid is a u8 constant pool identifier
// for the constant operand and nsrc is the number of source registers (packed
// four-per-code).  The operation itself (add, subtract or multiply, modulo
// the prime) is identified by the opcode.  A field-sized constant can be
// wider than the 64 bits carried by the integer vector forms, which the pool
// accommodates naturally.  The wide form moves the (now u16) destination
// register and the (now u16) pool identifier into a subsequent word, and
// packs the source registers two per word:
//
// +--------+--------+--------+--------+
// |  nsrc  |  n/a   |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       cid       |        rd       |
// +-----------------+-----------------+
// | ... packed source registers ...   |
// +-----------------------------------+
//
// The wide form is also selected when the registers are small but the pool
// identifier exceeds a byte.
// ============================================================================

// encodeFieldArith encodes a field-arithmetic instruction, carrying its
// (possibly field-sized) constant as a constant pool identifier followed by
// the packed source registers.
func encodeFieldArith[W word.Word[W]](op bytecode.Operation, rd RegisterId, sources []RegisterId, constant W,
	env Environment[W]) []uint32 {
	//
	if len(sources) >= 256 {
		panic("field instruction operand counts not supported")
	}
	//
	var (
		opcode = ADDMOD_P + uint32(op-bytecode.OP_ADDMOD_P)
		wide   = IsWideRegisters(rd) || IsWideRegisters(sources...)
		header = uint32(len(sources)) << 24
		// NOTE: the pool identifier is stable across encoding passes
		// (ConstantIndex interns), so the choice of form below cannot
		// oscillate whilst the layout reaches a fixpoint.
		cid = uint32(env.ConstantIndex(constant))
	)
	//
	if !wide && cid <= math.MaxUint8 {
		var codes = []uint32{header | cid<<16 | uint32(rd)<<8 | opcode}
		//
		return append(codes, PackBytesIntoCodes(RegsAsBytes(sources))...)
	}
	// Wide form: also the fallback when the pool identifier exceeds a byte.
	var codes = []uint32{
		header | (WIDE_ADDMOD_P+uint32(op-bytecode.OP_ADDMOD_P))<<8 | WIDE,
		cid<<16 | uint32(rd),
	}
	//
	return append(codes, PackShortsIntoCodes(RegsAsShorts(sources))...)
}
