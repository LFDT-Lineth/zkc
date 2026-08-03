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
	return encodeRegisterLists(CAT, p.Targets, p.Sources)
}

// DecodeCat decodes a concatenation instruction at the given program counter.
func DecodeCat[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		tIter, sIter, n = DecodeRegisterLists(pc, codes)
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
// |  nsrc  |  ntgt  |  n/a   | opcode |
// +--------+--------+--------+--------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...   |
// +-----------------------------------+
//
// The first source and target are the least-significant limbs.  The wide form
// retains the header but packs the (now u16) target and source registers two
// per word:
//
// +--------+--------+--------+--------+
// |  nsrc  |  ntgt  |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       tgt1      |       tgt0      |
// +-----------------+-----------------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// This layout is shared by the UINT_TO_FIELD and FIELD_TO_UINT instructions
// (see field_cast.go), which also encode via encodeRegisterLists.
// ============================================================================

// encodeRegisterLists encodes target and source register lists.
func encodeRegisterLists(opcode uint32, targets []RegisterId, sources []RegisterId) []uint32 {
	if len(targets) == 0 {
		panic("instruction requires at least one target")
	} else if len(sources) == 0 {
		panic("instruction requires at least one source")
	} else if len(targets) >= 256 || len(sources) >= 256 {
		panic("instruction has too many operands")
	}
	//
	var (
		nsrc = uint32(len(sources)) << 24
		ntgt = uint32(len(targets)) << 16
		regs = append(RegsAsShorts(targets), RegsAsShorts(sources)...)
	)
	//
	if IsWideRegisters(regs...) {
		var codes = []uint32{nsrc | ntgt | wideRegisterListOpcode(opcode)<<8 | WIDE}
		//
		return append(codes, PackShortsIntoCodes(regs)...)
	}
	//
	var (
		codes = []uint32{nsrc | ntgt | opcode}
		bytes = append(RegsAsBytes(targets), RegsAsBytes(sources)...)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// wideRegisterListOpcode maps a register-list opcode (CAT, UINT_TO_FIELD or
// FIELD_TO_UINT) to its wide counterpart.
func wideRegisterListOpcode(opcode uint32) uint32 {
	switch opcode {
	case CAT:
		return WIDE_CAT
	case UINT_TO_FIELD:
		return WIDE_UINT_TO_FIELD
	case FIELD_TO_UINT:
		return WIDE_FIELD_TO_UINT
	default:
		panic("unknown register-list opcode")
	}
}

// DecodeRegisterLists decodes target and source register lists.
func DecodeRegisterLists(pc uint32, codes []uint32) (targets, sources Operands, n uint32) {
	var (
		ntargets = uint((codes[pc] >> 16) & 0xff)
		nsources = uint((codes[pc] >> 24) & 0xff)
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
