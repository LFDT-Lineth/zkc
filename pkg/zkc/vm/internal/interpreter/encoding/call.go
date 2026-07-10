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

// Call encodes a call bytecode as an ENTER/LEAVE instruction pair: ENTER
// allocates the callee's frame, binds its arguments and pushes a stack-frame
// record, whilst LEAVE binds the returns to their destination registers.
func Call[W word.Word[W]](pc uint32, p *bytecode.Call, env Environment[W]) (codes []uint32) {
	var (
		zero   = ProgramPoint{0, 0}
		offset = env.OffsetFor(p.Target, zero)
		// Extract frame width
		width = uint16(env.Module(p.Target).Width())
	)
	// Encode enter
	codes = append(codes, encodeEnter_n(pc, offset, width, p.Arguments)...)
	// Encode leave
	return append(codes, encodeLeave_n(p.Returns)...)
}

// MaxCallEncodedLength returns the maximum length (in u32 words) which an
// encoding of the given call bytecode can occupy, i.e. the size of its wide
// ENTER/LEAVE pair (the wide form is never smaller than the narrow form).
func MaxCallEncodedLength(p *bytecode.Call) uint {
	var (
		enter = 2 + NumCodesPackedWide(uint(len(p.Arguments)))
		leave = 1 + NumCodesPackedWide(uint(len(p.Returns)))
	)
	//
	return uint(enter + leave)
}

// NOTE: a call bytecode compiles down into a pair of instructions, ENTER/LEAVE.
// The ENTER instruction prepares for the call by allocating the frame,
// assigning the arguments and pushing a stackframe record.  The LEAVE
// instruction handles the assignment of returns to their destination registers.
//
// ============================================================================
// ENTER_n instruction. Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |     offset      | width  | opcode |
// +--------+--------+--------+--------+
// |  arg2  |  arg1  |  arg0  | nargs  |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  arg5  |  arg4  |
// +-----------------------------------+
//
// Here, nargs determines the number of packed argument registers, whilst width
// determines the frame width to allocate and offset determines (relative)
// offset to the target.  The wide form carries a u16 frame width, an absolute
// u32 target address and (now u16) argument registers packed two per word:
//
// +--------+--------+--------+--------+
// |      width      | nargs  | opcode |
// +--------+--------+--------+--------+
// | ............ target ............. |
// +--------+--------+--------+--------+
// |  arg1  |  arg0   (u16 pairs) ...  |
// +-----------------+-----------------+
// ============================================================================

// encodeEnter_n encodes the ENTER (function entry) instruction, computing the
// relative branch offset to the target.
func encodeEnter_n(pc, target uint32, width uint16, args []RegisterId) []uint32 {
	if len(args) > math.MaxUint8 {
		panic("too many call arguments")
	}
	//
	var (
		roff, ok = GetRelativeOffset(pc, target, 16)
		opcode   = ENTER_n
	)
	// The wide form is required whenever the frame width or an argument
	// register overflows a byte, and also rescues relative branch targets which
	// overflow the narrow form's 16-bit offset (the wide form's target is
	// absolute).
	if width > math.MaxUint8 || !ok || IsWideRegisters(args...) {
		codes := []uint32{
			uint32(width)<<16 | uint32(len(args))<<8 | opcode | WIDE,
			target,
		}
		//
		return append(codes, PackShortsIntoCodes(RegsAsShorts(args))...)
	}
	//
	var (
		codes = []uint32{roff<<16 | uint32(width)<<8 | opcode}
		bytes = []uint8{uint8(len(args))}
	)
	//
	bytes = append(bytes, RegsAsBytes(args)...)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeEnter_n decodes the operands of an enter (function entry) instruction.
func DecodeEnter_n(pc uint32, codes []uint32) (width uint16, target uint32, args OpIter, n uint32) {
	if IsWideForm(pc, codes) {
		var nargs = uint((codes[pc] >> 8) & 0xff)
		//
		width = uint16(codes[pc] >> 16)
		target = codes[pc+1]
		args = NewOp16Iter(0, nargs, codes[pc+2:])
		n = 2 + NumCodesPackedWide(nargs)
		//
		return
	}
	//
	var nargs = uint(codes[pc+1] & 0xff)
	//
	width = uint16((codes[pc] >> 8) & 0xff)
	target = GetBranchTarget(pc, codes[pc]>>16, 16)
	args = NewOp8Iter(1, nargs, codes[pc+1:])
	n = 1 + NumCodesPackedSmall(nargs+1)
	//
	return
}

// ============================================================================
// LEAVE_n instruction. Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  n/a   |     nrets       | opcode |
// +--------+--------+--------+--------+
// |  ...   |  ...   |  ret1  |  ret0  |
// +-----------------------------------+
//
// Here, nrets determines the number of packed return registers.  The wide form
// retains the header but packs the (now u16) return registers two per word.
//
// ============================================================================

// encodeLeave_n encodes the LEAVE (function exit) instruction, packing the
// return registers.
func encodeLeave_n(rets []RegisterId) []uint32 {
	if len(rets) > math.MaxUint16 {
		panic("too many call returns")
	}
	//
	var nrets = uint32(len(rets)) << 8
	//
	if IsWideRegisters(rets...) {
		var codes = []uint32{nrets | LEAVE_n | WIDE}
		//
		return append(codes, PackShortsIntoCodes(RegsAsShorts(rets))...)
	}
	//
	var (
		codes = []uint32{nrets | LEAVE_n}
		bytes = RegsAsBytes(rets)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeLeave_n decodes the operands of a leave (function exit) instruction.
func DecodeLeave_n(pc uint32, codes []uint32) (rets OpIter, n uint32) {
	var (
		nrets = uint(codes[pc]>>8) & 0xffff
	)
	//
	if IsWideForm(pc, codes) {
		rets = NewOp16Iter(0, nrets, codes[pc+1:])
		n = 1 + NumCodesPackedWide(nrets)
	} else {
		rets = NewOp8Iter(0, nrets, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(nrets)
	}
	//
	return
}
