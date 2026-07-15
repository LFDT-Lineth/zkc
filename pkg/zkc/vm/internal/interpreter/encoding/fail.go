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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Fail encodes a fail bytecode, interning its formatted chunks in the symbol
// table and referencing them by index.
func Fail[W word.Word[W]](p *bytecode.Fail[W], env Environment[W]) []uint32 {
	var index = env.ChunksIndex(p.Chunks...)
	//
	return encodeFail_n(util.Cast[uint16](index), p.Sources)
}

// DecodeFail decodes a fail instruction, returning the index of its formatted
// chunks, an iterator over its source register vectors and the instruction
// width.
func DecodeFail(pc uint32, codes []uint32) (index uint, sources Operands, n uint32) {
	return decodeFail_n(pc, codes)
}

// ============================================================================
// Fail instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nvecs |     index       | opcode |
// +--------+--------+--------+--------+
// |  len1  |  base1 |  len0  | base0  |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  len2  | base2  |
// +-----------------------------------+
//
// Here, nvecs determines the number of packed source register vectors, whilst
// index identifies the formatted chunks (i.e. the failure message template) to
// emit.  Each source vector is encoded as a (base, len) byte pair, packed two
// vectors per word.  The wide form retains the header but packs each vector as
// a (base, len) pair of u16 operands (i.e. one word per vector).
// ============================================================================
// encodeFail_n encodes a fail instruction referencing the formatted chunks at
// the given index, packing its source register vectors.
func encodeFail_n(index uint16, sources []RegisterVector) []uint32 {
	var nsources = uint32(util.Cast[uint8](uint(len(sources)))) << 24
	//
	if IsWideRegisterVectors(sources) {
		// nolint
		var codes = []uint32{nsources | uint32(index)<<8 | FAIL | WIDE}
		//
		return append(codes, PackShortsIntoCodes(RegisterVectorsAsShorts(sources))...)
	}
	//
	var (
		codes = []uint32{
			// nolint
			nsources | uint32(index)<<8 | FAIL,
		}
		bytes = RegisterVectorsAsBytes(sources)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// decodeFail_n decodes the operands of a fail instruction.
func decodeFail_n(pc uint32, codes []uint32) (index uint, sources Operands, n uint32) {
	var nops = 2 * uint(codes[pc]>>24)
	//
	index = uint(codes[pc]>>8) & 0xffff
	//
	if IsWideForm(pc, codes) {
		sources = NewWideOperands(0, nops, codes[pc+1:])
		n = 1 + NumCodesPackedWide(nops)
	} else {
		sources = NewOperands(0, nops, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(nops)
	}
	//
	return index, sources, n
}
