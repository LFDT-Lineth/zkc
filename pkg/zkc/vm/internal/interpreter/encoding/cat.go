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

// Cat encodes a concatenation bytecode.
func Cat[W word.Word[W]](p *bytecode.Cat[W]) []uint32 {
	return encodeCat(p.Targets, p.Sources)
}

// DecodeCat decodes a concatenation instruction at the given program counter.
func DecodeCat[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		tIter, sIter, n = DecodeCatOperands(pc, codes)
		targets         = OpIterToArray[uint16](tIter)
		sources         = OpIterToArray[uint16](sIter)
	)
	//
	return &bytecode.Cat[W]{Targets: targets, Sources: sources}, n
}

// ============================================================================
// CAT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   n/a  |  nsrc  | ntgt   | opcode |
// +--------+--------+--------+--------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// The first source and target are the least-significant limbs.  The wide form
// retains the header but packs the (now u16) target and source registers two
// per word.
// ============================================================================

// encodeCat encodes a concatenation instruction, packing its target and source
// registers (least-significant limb first).
func encodeCat(targets []RegisterId, sources []RegisterId) []uint32 {
	if len(targets) == 0 {
		panic("cat requires at least one target")
	} else if len(targets) == 0 || len(sources) == 0 {
		panic("cat requires at least one source")
	} else if len(targets) >= 256 || len(sources) >= 256 {
		panic("cat has too many operands")
	}
	//
	var (
		nsrc = uint32(len(sources)) << 16
		ntgt = uint32(len(targets)) << 8
		regs = append(RegsAsShorts(targets), RegsAsShorts(sources)...)
	)
	//
	if IsWideRegisters(regs...) {
		var codes = []uint32{nsrc | ntgt | CAT | WIDE}
		//
		return append(codes, PackShortsIntoCodes(regs)...)
	}
	//
	var (
		codes = []uint32{nsrc | ntgt | CAT}
		bytes = append(RegsAsBytes(targets), RegsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeCatOperands decodes the target and source operands of a concatenation instruction.
func DecodeCatOperands(pc uint32, codes []uint32) (targets, sources Operands, n uint32) {
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
	)
	//
	if IsWideForm(pc, codes) {
		targets = NewWideOperands(0, ntargets, codes[pc+1:])
		sources = NewWideOperands(ntargets, nsources, codes[pc+1:])
		n = 1 + NumCodesPackedWide(ntargets+nsources)
	} else {
		targets = NewOperands(0, ntargets, codes[pc+1:])
		sources = NewOperands(ntargets, nsources, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(ntargets+nsources)
	}
	//
	return
}
