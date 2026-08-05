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

// DivRem encodes a division/remainder bytecode; the opcode held within the
// bytecode selects between quotient (DIV) and remainder (REM).
func DivRem[W word.Word[W]](p *bytecode.DivRem[W]) []uint32 {
	return encodeDivRem(p.Opcode, p.Target, p.Dividend, p.Divisor)
}

// Intrinsic encodes an intrinsic bytecode (e.g. DIV_HINT, which supplies the
// prover with the quotient, remainder and witness for a division, or WIDE_SHL).
func Intrinsic[W word.Word[W]](p *bytecode.Intrinsic[W]) []uint32 {
	return encodeIntrinsic(p.Op, p.Targets, p.Sources)
}

// DecodeIntrinsic decodes an intrinsic instruction at the given program counter.
func DecodeIntrinsic[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op, tIter, sIter, n = DecodeIntrinsicOperands(pc, codes)
		targets             = registerVectorsFromIter(tIter)
		sources             = registerVectorsFromIter(sIter)
	)
	//
	return &bytecode.Intrinsic[W]{Op: op, Targets: targets, Sources: sources}, n
}

// registerVectorsFromIter reconstructs the register vectors packed as (base, len)
// pairs within the given iterator.
func registerVectorsFromIter(iter Operands) []RegisterVector {
	var vecs []RegisterVector
	//
	for iter.HasNext() {
		var (
			base = iter.Next()
			len  = iter.Next()
		)
		//
		vecs = append(vecs, RegisterVector{Base: base, Len: len})
	}
	//
	return vecs
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
// The wide form carries the (now u16) destination register in the first word,
// with both source registers in a second:
//
// +--------+--------+--------+--------+
// |        rd       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |     divisor     |    dividend     |
// +-----------------+-----------------+
// ============================================================================

// encodeDivRem encodes a division/remainder instruction, where op distinguishes
// the two operations.
func encodeDivRem(op uint32, rd, dividend, divisor RegisterId) []uint32 {
	if IsWideRegisters(rd, dividend, divisor) {
		return []uint32{
			uint32(rd)<<16 | (WIDE_DIV+(op-DIV))<<8 | WIDE,
			uint32(dividend) | uint32(divisor)<<16,
		}
	}
	//
	return []uint32{uint32(divisor)<<24 | uint32(dividend)<<16 | uint32(rd)<<8 | op}
}

// DecodeDivRem_2n1 decodes the operands of a division/remainder instruction.
func DecodeDivRem_2n1(pc uint32, codes []uint32) (rd, dividend, divisor RegisterId, n uint32) {
	if IsWideForm(pc, codes) {
		rd = RegisterId(codes[pc] >> 16)
		dividend = RegisterId(codes[pc+1] & 0xffff)
		divisor = RegisterId(codes[pc+1] >> 16)
		//
		return rd, dividend, divisor, 2
	}
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	dividend = RegisterId((codes[pc] >> 16) & 0xff)
	divisor = RegisterId((codes[pc] >> 24) & 0xff)
	//
	return rd, dividend, divisor, 1
}

// ============================================================================
// INTRINSIC instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  |  ntgt  | opcode |
// +--------+--------+--------+--------+
// | ... packed tgt/src reg vectors .. |
// +-----------------------------------+
//
// Here, op selects the hint operation (e.g. DIV_HINT), whilst ntgt and nsrc give
// the number of target (return) and source (argument) register vectors
// respectively.  Each vector is packed as a (base, len) byte pair, targets
// first.  The wide form moves the target count into the first packed slot
// (leaving bits 8-15 of the first word clear, as for all wide forms), with
// each vector following as a (base, len) pair of u16 operands:
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |    tgt0 base    |      ntgt       |
// +-----------------+-----------------+
// |    tgt1 base    |    tgt0 len     |
// +-----------------+-----------------+
// | ... packed tgt/src reg vectors .. |
// +-----------------------------------+
// ============================================================================

// encodeIntrinsic encodes a hint instruction, where op selects the operation and the
// target (return) and source (argument) register vectors are packed as (base,
// len) byte pairs, targets first.
func encodeIntrinsic(op Operation, targets, sources []RegisterVector) []uint32 {
	if len(targets) == 0 || len(sources) == 0 || len(targets) >= 256 || len(sources) >= 256 {
		panic("hint instruction operand counts not supported")
	}
	//
	var (
		nop  = uint32(op) << 24
		nsrc = uint32(len(sources)) << 16
		ntgt = uint32(len(targets)) << 8
	)
	//
	if IsWideRegisterVectors(targets) || IsWideRegisterVectors(sources) {
		var (
			codes = []uint32{nop | nsrc | WIDE_INTRINSIC<<8 | WIDE}
			// ntgt occupies the first packed slot, followed by the vectors.
			shorts = make([]uint16, 0, 1+2*(len(targets)+len(sources)))
		)
		//
		shorts = append(shorts, uint16(len(targets)))
		shorts = append(shorts, RegisterVectorsAsShorts(targets)...)
		shorts = append(shorts, RegisterVectorsAsShorts(sources)...)
		//
		return append(codes, PackShortsIntoCodes(shorts)...)
	}
	//
	var (
		codes = []uint32{nop | nsrc | ntgt | INTRINSIC}
		bytes = append(RegisterVectorsAsBytes(targets), RegisterVectorsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeIntrinsicOperands decodes the operation selector along with the target and
// source operands of a hint instruction.  Each vector is packed as a (base,
// len) byte pair, hence the iterators range over twice the vector counts.
func DecodeIntrinsicOperands(pc uint32, codes []uint32) (op Operation, targets, sources Operands, n uint32) {
	var nsources = uint((codes[pc] >> 16) & 0xff)
	//
	op = Operation((codes[pc] >> 24) & 0xff)
	//
	if IsWideForm(pc, codes) {
		var ntargets = uint(codes[pc+1] & 0xffff)
		//
		targets = NewWideOperands(1, 2*ntargets, codes[pc+1:])
		sources = NewWideOperands(1+(2*ntargets), 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedWide(1+2*(ntargets+nsources))
	} else {
		var ntargets = uint((codes[pc] >> 8) & 0xff)
		//
		targets = NewOperands(0, 2*ntargets, codes[pc+1:])
		sources = NewOperands(2*ntargets, 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(2*(ntargets+nsources))
	}
	//
	return
}
