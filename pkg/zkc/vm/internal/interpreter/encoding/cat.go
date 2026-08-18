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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Cat encodes a concatenation bytecode, selecting the most specific dedicated
// form applicable -- two-target/one-source (CAT_2n1), one-source/N-target
// (CAT_1n), or N-source/one-target (CAT_n1) -- and falling back to the
// general register-list form otherwise.
func Cat[W word.Word[W]](p *bytecode.Cat[W]) []uint32 {
	// NOTE: do not remove this assertion, as it is there to prevent expensive
	// cat operations being used when they don't need to be.
	util.Assert(len(p.Sources) > 1 || len(p.Targets) > 1, "invalid cat bytecode")
	//
	switch {
	case len(p.Targets) == 2 && len(p.Sources) == 1 &&
		!IsWideRegisters(p.Targets[0], p.Targets[1], p.Sources[0]):
		return encodeCat_2n1(p.Sources[0], p.Targets[0], p.Targets[1])
	case len(p.Sources) == 1 && len(p.Targets) <= math.MaxUint8 &&
		!IsWideRegisters(p.Sources[0]) && !IsWideRegisters(p.Targets...):
		return encodeCat_1n(p.Sources[0], p.Targets)
	case len(p.Targets) == 1 && len(p.Sources) <= math.MaxUint8 &&
		!IsWideRegisters(p.Targets[0]) && !IsWideRegisters(p.Sources...):
		return encodeCat_n1(p.Targets[0], p.Sources)
	default:
		return encodeRegisterLists(CAT, p.Targets, p.Sources)
	}
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
// CAT_2n1 instruction: the dedicated two-target, one-source form of CAT.
// Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   rs   |  tgt1  |  tgt0  | opcode |
// +--------+--------+--------+--------+
//
// Here, rs is the u8 source register, whilst tgt0 and tgt1 are u8 target
// registers, tgt0 being the least-significant limb (matching the general CAT
// form's target ordering).  There is no wide form: a register exceeding u8
// falls back to the general CAT encoding instead.
// ============================================================================

// encodeCat_2n1 encodes a two-target, one-source concatenation instruction.
func encodeCat_2n1(rs, t0, t1 uint16) []uint32 {
	return []uint32{
		uint32(rs)<<24 | uint32(t1)<<16 | uint32(t0)<<8 | CAT_2n1,
	}
}

// DecodeCat_2n1 decodes a two-target, one-source concatenation instruction.
func DecodeCat_2n1(pc uint32, codes []uint32) (rs, t0, t1 uint16, n uint32) {
	rs = RegisterId(codes[pc] >> 24)
	t1 = RegisterId((codes[pc] >> 16) & 0xff)
	t0 = RegisterId((codes[pc] >> 8) & 0xff)

	return rs, t0, t1, 1
}

// ============================================================================
// CAT_1n instruction: the dedicated one-source, N-target form of CAT. Format
// is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  n/a   |  ntgt  |   rs   | opcode |
// +--------+--------+--------+--------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  ...   |  ...   |
// +-----------------------------------+
//
// Here, rs is the u8 source register and ntgt gives the number of (u8)
// target registers which follow, packed four per word (tgt0 being the
// least-significant limb, matching the general CAT form's target ordering).
// Unlike the general form, no source count is needed (it is always one), and
// no target vector list is separately maintained -- the source is read once
// and distributed directly.  There is no wide form: a register exceeding u8,
// or more than 255 targets, falls back to the general CAT encoding instead.
// ============================================================================

// encodeCat_1n encodes a one-source, N-target concatenation instruction.
func encodeCat_1n(rs RegisterId, targets []RegisterId) []uint32 {
	var (
		codes = []uint32{uint32(len(targets))<<16 | uint32(rs)<<8 | CAT_1n}
		bytes = RegsAsBytes(targets)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeCat_1n decodes a one-source, N-target concatenation instruction.
func DecodeCat_1n(pc uint32, codes []uint32) (rs RegisterId, targets Operands, n uint32) {
	var ntgt = uint((codes[pc] >> 16) & 0xff)
	//
	rs = RegisterId((codes[pc] >> 8) & 0xff)
	targets = NewOperands(0, ntgt, codes[pc+1:])
	n = 1 + NumCodesPackedSmall(ntgt)
	//
	return
}

// ============================================================================
// CAT_n1 instruction: the dedicated N-source, one-target form of CAT. Format
// is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nsrc  |  n/a   |   rd   | opcode |
// +--------+--------+--------+--------+
// | src3   | src2   | src1   | src0   |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  ...   |  ...   |
// +-----------------------------------+
//
// Here, rd is the u8 target register and nsrc gives the number of (u8)
// source registers which follow, packed four per word (src0 being the
// least-significant limb, matching the general CAT form's source ordering).
// Unlike the general form, no target count is needed (it is always one), and
// the combined source value is written directly to the one target rather
// than distributed via the general storeAcross loop.  There is no wide form:
// a register exceeding u8, or more than 255 sources, falls back to the
// general CAT encoding instead.
// ============================================================================

// encodeCat_n1 encodes an N-source, one-target concatenation instruction.
func encodeCat_n1(rd RegisterId, sources []RegisterId) []uint32 {
	var (
		codes = []uint32{uint32(len(sources))<<24 | uint32(rd)<<8 | CAT_n1}
		bytes = RegsAsBytes(sources)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeCat_n1 decodes an N-source, one-target concatenation instruction.
func DecodeCat_n1(pc uint32, codes []uint32) (rd RegisterId, sources Operands, n uint32) {
	var nsrc = uint((codes[pc] >> 24) & 0xff)
	//
	rd = RegisterId((codes[pc] >> 8) & 0xff)
	sources = NewOperands(0, nsrc, codes[pc+1:])
	n = 1 + NumCodesPackedSmall(nsrc)
	//
	return
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
