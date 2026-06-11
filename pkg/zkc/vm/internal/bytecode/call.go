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
package bytecode

import (
	"fmt"
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Call invokes another function module.
type Call struct {
	// address of target function
	Target Address
	// FrameWidth of target function
	FrameWidth uint16
	// Arguments are caller-frame registers copied into callee inputs.
	Arguments []Reg
	// Returns are caller-frame registers receiving callee outputs.
	Returns []Reg
}

func (p *Call) String(mapping SystemMap) string {
	var builder strings.Builder
	//
	builder.WriteString("call ")
	//
	if len(p.Returns) > 0 {
		builder.WriteString(registersToString(p.Returns, mapping, ","))
		builder.WriteString(" = ")
	}
	//
	fmt.Fprintf(&builder, "(%s) 0x%08x", registersToString(p.Arguments, mapping, ","), p.Target)
	//
	return builder.String()
}

// Codes implementation for Bytecode interface.
func (p *Call) Codes(pc uint32) (codes []uint32) {
	// Encode enter
	codes = append(codes, encodeEnter_n(pc, p.Target, p.FrameWidth, p.Arguments)...)
	// Encode leave
	return append(codes, encodeLeave_n(p.Returns)...)
}

// Patch implementation for Bytecode interface
func (p *Call) Patch(labels []Address) {
	p.Target = labels[p.Target]
}

func decodeCall[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		// Decode ENTER
		width, target, argsIter, n = decodeEnter_n(pc, codes)
		// Decode LEAVE
		retsIter, m = decodeLeave_n(pc+n, codes)
		//
		args []Reg = OpIterToArray[uint16](argsIter)
		rets []Reg = OpIterToArray[uint16](retsIter)
	)
	// //
	return &Call{target, width, args, rets}, n + m
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

func encodeEnter_n(pc, target uint32, width uint16, args []Reg) []uint32 {
	if width > math.MaxUint8 || len(args) > math.MaxUint8 {
		panic("wide call instructions not supported")
	}
	//
	var (
		roff   = getRelativeOffset(pc, target, 16) << 16
		_width = uint32(width) << 8
		codes  = []uint32{roff | _width | ENTER_n}
		bytes  = []uint8{uint8(len(args))}
	)
	//
	bytes = append(bytes, regsAsBytes(args)...)
	//
	return append(codes, packRegsIntoCodes(bytes)...)
}

func decodeEnter_n(pc uint32, codes []uint32) (width uint16, target uint32, args Op8Iter, n uint32) {
	var nargs = uint(codes[pc+1] & 0xff)
	//
	width = uint16((codes[pc] >> 8) & 0xff)
	target = getBranchTarget(pc, codes[pc]>>16, 16)
	args = NewOp8Iter(1, nargs, codes[pc+1:])
	n = 1 + nCodesPackedSmall(nargs+1)
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

func encodeLeave_n(rets []Reg) []uint32 {
	if len(rets) > math.MaxUint16 {
		panic("wide call instructions not supported")
	}
	//
	var (
		nrets = uint32(len(rets)) << 8
		codes = []uint32{nrets | LEAVE_n}
		bytes = regsAsBytes(rets)
	)
	//
	return append(codes, packRegsIntoCodes(bytes)...)
}

func decodeLeave_n(pc uint32, codes []uint32) (rets Op8Iter, n uint32) {
	var (
		nrets = uint(codes[pc]>>8) & 0xffff
	)
	//
	rets = NewOp8Iter(0, nrets, codes[pc+1:])
	n = 1 + nCodesPackedSmall(nrets)
	//
	return
}
