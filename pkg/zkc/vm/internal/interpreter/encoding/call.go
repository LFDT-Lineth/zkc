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
	codes = append(codes, encodeEnter_n(pc, offset, p.Flags.CheckPoint, width, p.Arguments)...)
	// Encode leave
	return append(codes, encodeLeave_n(p.Returns)...)
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
// offset to the target.
// ============================================================================

// encodeEnter_n encodes the ENTER (function entry) instruction, computing the
// relative branch offset to the target.  When checkpoint is set, the
// checkpointing variant (ENTERCP_n) is emitted instead.
func encodeEnter_n(pc, target uint32, checkpoint bool, width uint16, args []RegisterId) []uint32 {
	if width > math.MaxUint8 || len(args) > math.MaxUint8 {
		panic("wide call instructions not supported")
	}
	//
	var (
		roff, ok = GetRelativeOffset(pc, target, 16)
		_width   = uint32(width) << 8
		bytes    = []uint8{uint8(len(args))}
		opcode   = ENTER_n
	)
	// sanity check
	if !ok {
		panic("branch target overflow")
	} else if checkpoint {
		opcode = ENTERCP_n
	}
	// Determine full opcode
	codes := []uint32{roff<<16 | _width | opcode}
	//
	bytes = append(bytes, RegsAsBytes(args)...)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeEnter_n decodes the operands of an enter (function entry) instruction.
func DecodeEnter_n(pc uint32, codes []uint32) (width uint16, target uint32, args Op8Iter, n uint32) {
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
// Here, nrets determines the number of packed return registers.
//
// ============================================================================

// encodeLeave_n encodes the LEAVE (function exit) instruction, packing the
// return registers.
func encodeLeave_n(rets []RegisterId) []uint32 {
	if len(rets) > math.MaxUint16 {
		panic("wide call instructions not supported")
	}
	//
	var (
		nrets = uint32(len(rets)) << 8
		codes = []uint32{nrets | LEAVE_n}
		bytes = RegsAsBytes(rets)
	)
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeLeave_n decodes the operands of a leave (function exit) instruction.
func DecodeLeave_n(pc uint32, codes []uint32) (rets Op8Iter, n uint32) {
	var (
		nrets = uint(codes[pc]>>8) & 0xffff
	)
	//
	rets = NewOp8Iter(0, nrets, codes[pc+1:])
	n = 1 + NumCodesPackedSmall(nrets)
	//
	return
}
