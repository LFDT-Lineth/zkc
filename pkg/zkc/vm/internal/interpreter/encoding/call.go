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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Call encodes a call bytecode as an ENTER/LEAVE instruction pair: ENTER
// allocates the callee's frame, binds its arguments and pushes a stack-frame
// record, whilst LEAVE binds the returns to their destination registers.
func Call[W word.Word[W]](pc uint32, p *bytecode.Call[W], env Environment[W]) (codes []uint32) {
	var (
		zero   ProgramPoint
		offset = env.OffsetFor(p.Target, zero)
		// Extract frame width
		width = uint16(env.Module(p.Target).Width())
	)
	// Check for never call
	if p.Never {
		// Encode enter
		return append(codes, encodeTailCall(pc, offset, width, p.Arguments)...)
	}
	// Encode enter
	codes = append(codes, encodeEnter(pc, offset, width, p.Arguments)...)
	// Encode leave
	return append(codes, encodeLeave_n(substituteDiscardedRegisters(p.Returns))...)
}

// substituteDiscardedRegisters rewrites a binding list which may contain discarded (DISCARD)
// entries into an equivalent dense list. Trailing discarded slots are removed.
// For example, _, y = f(x) becomes y, y = f(x)
func substituteDiscardedRegisters(regs []RegisterId) []RegisterId {
	if !slices.Contains(regs, bytecode.DISCARD) {
		return regs
	}
	//
	var (
		dense = make([]RegisterId, len(regs))
		next  = bytecode.DISCARD
		n     = 0
	)
	// Bind each discarded slot to the register of the next bound slot, working
	// backwards so that register is already known when the slot is reached.
	for i := len(regs) - 1; i >= 0; i-- {
		if regs[i] != bytecode.DISCARD {
			next = regs[i]
			// Record where the trailing discarded slots (if any) begin.
			n = max(n, i+1)
		}
		//
		dense[i] = next
	}
	// Drop trailing discarded slots.
	return dense[:n]
}

// MaxCallEncodedLength returns the maximum length (in u32 words) which an
// encoding of the given call bytecode can occupy, i.e. the size of its wide
// ENTER/LEAVE pair (the wide form is never smaller than the narrow form).
func MaxCallEncodedLength[W word.Word[W]](p *bytecode.Call[W]) uint {
	var (
		enter = 2 + NumCodesPackedWide(uint(len(p.Arguments))+1)
		leave = 1 + NumCodesPackedWide(uint(len(p.Returns)))
	)
	//
	return uint(enter + leave)
}

func encodeEnter(pc, target uint32, width uint16, args []RegisterId) []uint32 {
	if len(args) > math.MaxUint8 {
		panic("too many call arguments")
	}
	//
	if len(args) == 1 && width <= math.MaxUint8 {
		return encodeEnter_2(pc, target, width, args[0], ENTER_2)
	}
	//
	return encodeEnter_n(pc, target, width, args, ENTER_n, WIDE_ENTER_n)
}

func encodeTailCall(pc, target uint32, width uint16, args []RegisterId) []uint32 {
	if len(args) > math.MaxUint8 {
		panic("too many call arguments")
	}
	//
	if len(args) == 1 && width <= math.MaxUint8 {
		return encodeEnter_2(pc, target, width, args[0], TAILCALL_2)
	}
	//
	return encodeEnter_n(pc, target, width, args, TAILCALL_n, WIDE_TAILCALL_n)
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
// offset to the target.  The wide form carries a u16 frame width (leaving
// bits 8-15 clear, as for all wide forms) and an absolute u32 target address,
// with the argument count and the (now u16) argument registers packed two per
// word — mirroring the narrow form, where nargs likewise occupies the first
// packed slot:
//
// +--------+--------+--------+--------+
// |      width      |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// | ............ target ............. |
// +--------+--------+--------+--------+
// |       arg0      |      nargs      |
// +-----------------+-----------------+
// |       arg2      |       arg1      |
// +-----------------+-----------------+
// ============================================================================

// encodeEnter_n encodes the ENTER (function entry) instruction, computing the
// relative branch offset to the target.  The single-argument case is
// dispatched to the dedicated ENTER_2 form, which covers the full RegisterId
// range and any branch distance -- so only the frame width can force a
// fallback to the general encoding below.
func encodeEnter_n(pc, target uint32, width uint16, args []RegisterId, opcode, wopcode uint32) []uint32 {
	var (
		roff, ok = GetRelativeOffset(pc, target, 16)
	)
	// The wide form is required whenever the frame width or an argument
	// register overflows a byte, and also rescues relative branch targets which
	// overflow the narrow form's 16-bit offset (the wide form's target is
	// absolute).
	if width > math.MaxUint8 || !ok || IsWideRegisters(args...) {
		codes := []uint32{
			uint32(width)<<16 | wopcode<<8 | WIDE,
			target,
		}
		// nargs occupies the first packed slot, followed by the arguments.
		shorts := append([]uint16{uint16(len(args))}, RegsAsShorts(args)...)
		//
		return append(codes, PackShortsIntoCodes(shorts)...)
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
func DecodeEnter_n(pc uint32, codes []uint32) (width uint16, target uint32, args Operands, n uint32) {
	if IsWideForm(pc, codes) {
		var nargs = uint(codes[pc+2] & 0xffff)
		//
		width = uint16(codes[pc] >> 16)
		target = codes[pc+1]
		args = NewWideOperands(1, nargs, codes[pc+2:])
		n = 2 + NumCodesPackedWide(nargs+1)
		//
		return
	}
	//
	var nargs = uint(codes[pc+1] & 0xff)
	//
	width = uint16((codes[pc] >> 8) & 0xff)
	target = GetBranchTarget(pc, codes[pc]>>16, 16)
	args = NewOperands(1, nargs, codes[pc+1:])
	n = 1 + NumCodesPackedSmall(nargs+1)
	//
	return
}

// ============================================================================
// ENTER_2 instruction: the dedicated single-argument form of ENTER_n. Format
// is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |      arg0       | width  | opcode |
// +--------+--------+--------+--------+
// | ................ roff ............ |
// +--------+--------+--------+--------+
//
// Here, width matches the general narrow form (a u8 frame width), whilst
// arg0 is a full u16 argument register -- the entire RegisterId range, so
// (unlike the general narrow form) an argument register can never force a
// fallback.  roff is a full u32 relative branch offset (target - (pc+1)),
// computed and reconstructed with plain wrapping arithmetic rather than the
// signed, bit-width-limited scheme GetRelativeOffset uses elsewhere, so it
// likewise never overflows.  There is no wide form: only a frame width
// exceeding u8 falls back to the general ENTER_n encoding instead.
// ============================================================================

// encodeEnter_2 encodes a single-argument ENTER instruction.
func encodeEnter_2(pc, target uint32, width uint16, arg RegisterId, op uint32) []uint32 {
	return []uint32{
		uint32(arg)<<16 | uint32(width)<<8 | op,
		target - (pc + 1),
	}
}

// DecodeEnter_2 decodes the operands of a single-argument enter (function
// entry) instruction.
func DecodeEnter_2(pc uint32, codes []uint32) (width uint16, target uint32, arg RegisterId, n uint32) {
	width = uint16((codes[pc] >> 8) & 0xff)
	arg = RegisterId(codes[pc] >> 16)
	target = (pc + 1) + codes[pc+1]
	n = 2

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
// moves nrets up a byte (leaving bits 8-15 clear, as for all wide forms) and
// packs the (now u16) return registers two per word:
//
// +-----------------+--------+--------+
// |      nrets      |  wop   |  WIDE  |
// +-----------------+--------+--------+
// |       ret1      |       ret0      |
// +-----------------+-----------------+
// ============================================================================

// encodeLeave_n encodes the LEAVE (function exit) instruction, packing the
// return registers.  The single-return case is dispatched to the dedicated
// LEAVE_2 form when it fits narrow; there is no wide form for LEAVE_2 --
// anything which would otherwise need the wide form falls through to the
// general encoding below instead.
func encodeLeave_n(rets []RegisterId) []uint32 {
	if len(rets) > math.MaxUint16 {
		panic("too many call returns")
	}
	//
	if len(rets) == 1 && !IsWideRegisters(rets[0]) {
		return encodeLeave_2(rets[0])
	}
	//
	var nrets = uint32(len(rets))
	//
	if IsWideRegisters(rets...) {
		var codes = []uint32{nrets<<16 | WIDE_LEAVE_n<<8 | WIDE}
		//
		return append(codes, PackShortsIntoCodes(RegsAsShorts(rets))...)
	}
	//
	var (
		codes = []uint32{nrets<<8 | LEAVE_n}
		bytes = RegsAsBytes(rets)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeLeave_n decodes the operands of a leave (function exit) instruction.
func DecodeLeave_n(pc uint32, codes []uint32) (rets Operands, n uint32) {
	if IsWideForm(pc, codes) {
		var nrets = uint(codes[pc] >> 16)
		//
		rets = NewWideOperands(0, nrets, codes[pc+1:])
		n = 1 + NumCodesPackedWide(nrets)
	} else {
		var nrets = uint(codes[pc]>>8) & 0xffff
		//
		rets = NewOperands(0, nrets, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(nrets)
	}
	//
	return
}

// ============================================================================
// LEAVE_2 instruction: the dedicated single-return form of LEAVE_n. Format
// is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  ret0  |       n/a       | opcode |
// +--------+--------+--------+--------+
//
// Here, ret0 is the single u8 return register, packed directly into the
// header word's otherwise-unused top byte -- so, unlike the general form,
// LEAVE_2 needs no second (packed) word at all.  There is no wide form: a
// return register which doesn't fit falls back to the general LEAVE_n
// encoding instead.
// ============================================================================

// encodeLeave_2 encodes a single-return LEAVE instruction.
func encodeLeave_2(ret RegisterId) []uint32 {
	return []uint32{uint32(ret)<<24 | LEAVE_2}
}

// DecodeLeave_2 decodes the operand of a single-return leave (function exit)
// instruction.
func DecodeLeave_2(pc uint32, codes []uint32) (ret RegisterId, n uint32) {
	ret = RegisterId(codes[pc] >> 24)
	n = 1

	return
}
