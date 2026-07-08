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
func DivRem(p *bytecode.DivRem) []uint32 {
	return encodeDivRem(p.Opcode, p.Target, p.Dividend, p.Divisor)
}

// Hint encodes a hint bytecode.  Currently the only supported operation is
// DIV_HINT, which supplies the prover with the quotient, remainder and witness
// for a division.
func Hint(p *bytecode.Hint) []uint32 {
	return encodeHint(p.Op, p.Targets, p.Sources)
}

// DecodeHint decodes a hint instruction at the given program counter.
func DecodeHint[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op, tIter, sIter, n = DecodeHintOperands(pc, codes)
		targets             = registerVectorsFromIter(tIter)
		sources             = registerVectorsFromIter(sIter)
	)
	//
	return &bytecode.Hint{Op: op, Targets: targets, Sources: sources}, n
}

// registerVectorsFromIter reconstructs the register vectors packed as (base, len)
// pairs within the given iterator.
func registerVectorsFromIter(iter OpIter) []RegisterVector {
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
// |        rd       |  n/a   | opcode |
// +--------+--------+--------+--------+
// |     divisor     |    dividend     |
// +-----------------+-----------------+
// ============================================================================

// encodeDivRem encodes a division/remainder instruction, where op distinguishes
// the two operations.
func encodeDivRem(op uint32, rd, dividend, divisor RegisterId) []uint32 {
	if HasWideRegister(rd, dividend, divisor) {
		return []uint32{
			uint32(rd)<<16 | op | WIDE,
			uint32(dividend) | uint32(divisor)<<16,
		}
	}
	//
	return []uint32{uint32(divisor)<<24 | uint32(dividend)<<16 | uint32(rd)<<8 | op}
}

// DecodeDivRem_2n1 decodes the operands of a division/remainder instruction.
func DecodeDivRem_2n1(pc uint32, codes []uint32) (rd, dividend, divisor RegisterId, n uint32) {
	if IsWideInstruction(pc, codes) {
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
// HINT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  |  ntgt  | opcode |
// +--------+--------+--------+--------+
// | ... packed target then source register vectors ... |
// +-----------------------------------------------------+
//
// Here, op selects the hint operation (e.g. DIV_HINT), whilst ntgt and nsrc give
// the number of target (return) and source (argument) register vectors
// respectively.  Each vector is packed as a (base, len) byte pair, targets
// first.  The wide form retains the header but packs each vector as a (base,
// len) pair of u16 operands (i.e. one word per vector).
// ============================================================================

// encodeHint encodes a hint instruction, where op selects the operation and the
// target (return) and source (argument) register vectors are packed as (base,
// len) pairs, targets first.
func encodeHint(op Operation, targets, sources []RegisterVector) []uint32 {
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
	if HasWideRegisterVecs(targets) || HasWideRegisterVecs(sources) {
		var (
			codes  = []uint32{nop | nsrc | ntgt | HINT | WIDE}
			shorts = append(RegisterVectorsAsShorts(targets), RegisterVectorsAsShorts(sources)...)
		)
		//
		return append(codes, PackShortsIntoCodes(shorts)...)
	}
	//
	var (
		codes = []uint32{nop | nsrc | ntgt | HINT}
		bytes = append(RegisterVectorsAsBytes(targets), RegisterVectorsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeHintOperands decodes the operation selector along with the target and
// source operands of a hint instruction.  Each vector is packed as a (base,
// len) pair, hence the iterators range over twice the vector counts.
func DecodeHintOperands(pc uint32, codes []uint32) (op Operation, targets, sources OpIter, n uint32) {
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
	)
	//
	op = Operation((codes[pc] >> 24) & 0xff)
	//
	if IsWideInstruction(pc, codes) {
		targets = NewOp16Iter(0, 2*ntargets, codes[pc+1:])
		sources = NewOp16Iter(2*ntargets, 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedWide(2*(ntargets+nsources))
	} else {
		targets = NewOp8Iter(0, 2*ntargets, codes[pc+1:])
		sources = NewOp8Iter(2*ntargets, 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(2*(ntargets+nsources))
	}
	//
	return
}
