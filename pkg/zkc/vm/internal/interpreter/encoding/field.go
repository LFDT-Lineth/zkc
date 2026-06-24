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

// FieldArith encodes a field-arithmetic bytecode (ADDMOD_P/SUBMOD_P/MULMOD_P).
func FieldArith[W word.Word[W]](p *bytecode.FieldArith[W]) []uint32 {
	return encodeFieldArith(p.Op, p.Target, p.Sources, p.Constant)
}

// DecodeFieldArith decodes a field arithmetic instruction at the given program counter.
func DecodeFieldArith[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op                       = codes[pc] & OPCODE_MASK
		rd, sources, constant, n = DecodeFieldArithOperands[W](pc, codes)
		srcs                     = OpIterToArray[uint16](sources)
	)
	//
	return &bytecode.FieldArith[W]{Op: op, Target: rd, Sources: srcs, Constant: constant}, n
}

// DecodeFieldArithOperands extracts the raw operands (target register, source
// register iterator, constant and instruction width) of a field-arithmetic
// instruction.  It is shared by the disassembler (DecodeFieldArith) and the
// interpreter's executor.
func DecodeFieldArithOperands[W word.Word[W]](pc uint32, codes []uint32) (
	rd RegisterId, sources Op8Iter, constant W, n uint32) {
	//
	var (
		nlimbs = (codes[pc] >> 16) & 0xff
		nsrc   = uint((codes[pc] >> 24) & 0xff)
		limb   W
	)
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	// Reconstruct the constant from its 32-bit limbs, most significant limb
	// first: each limb is shifted into the low bits of the accumulator in turn.
	for i := nlimbs; i > 0; i-- {
		limb = limb.SetUint64(uint64(codes[pc+i]))
		_, constant = constant.Shl64(32)
		constant = constant.Or(limb)
	}
	// Source registers follow the constant limbs.
	sources = NewOp8Iter(0, nsrc, codes[pc+1+nlimbs:])
	n = 1 + nlimbs + NumCodesPackedSmall(nsrc)
	//
	return rd, sources, constant, n
}

// ============================================================================
// ADDMOD_P / SUBMOD_P / MULMOD_P instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nsrc  |  ncon  |   rd   | opcode |
// +--------+--------+--------+--------+
// | constant limb 0 (least significant)|
// +------------------------------------+
// |                ...                 |
// +------------------------------------+
// | constant limb ncon-1 (most sig.)   |
// +------------------------------------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// Here, rd is a u8 destination register, ncon is the number of 32-bit constant
// limbs (carried inline, least significant first, as for LDC_w) and nsrc is the
// number of source registers (packed four-per-code after the constant).  The
// operation itself (add, subtract or multiply, modulo the prime) is identified
// by the opcode.  A field-sized constant can be wider than the 64 bits carried
// by the integer vector forms, hence the inline limb encoding.
// ============================================================================

// encodeFieldArith encodes a field-arithmetic instruction, carrying the
// (possibly field-sized) constant inline as a sequence of 32-bit limbs followed
// by the packed source registers.
func encodeFieldArith[W word.Word[W]](op uint32, rd RegisterId, sources []RegisterId, constant W) []uint32 {
	if rd >= 256 || len(sources) >= 256 {
		panic("wide field instructions not supported")
	}
	//
	var (
		// NOTE: big-endian byte ordering
		bytes  = constant.BigInt().Bytes()
		nlimbs = (len(bytes) + 3) / 4
	)
	//
	if nlimbs >= 256 {
		panic("wide field constants not supported")
	}
	//
	var (
		header = uint32(len(sources))<<24 | uint32(nlimbs)<<16 | uint32(rd)<<8 | op
		codes  = make([]uint32, nlimbs+1)
	)
	//
	codes[0] = header
	// Pack constant bytes into limbs, least significant limb first.
	for i, b := range bytes {
		var k = len(bytes) - 1 - i
		//
		codes[1+(k/4)] |= uint32(b) << (8 * (k % 4))
	}
	// Append packed source registers.
	return append(codes, PackBytesIntoCodes(RegsAsBytes(sources))...)
}
