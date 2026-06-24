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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// SkipIf encodes a conditional forward-branch bytecode, selecting the
// register-register or register-vector form according to its operands.
func SkipIf[W word.Word[W]](pc Address, b *bytecode.SkipIf, env Environment[W]) []uint32 {
	var (
		target = env.Point().Skip(uint(b.Skip))
		offset = env.OffsetFor(env.enclosing, target)
		skip   = offset - (pc + 1)
		//
		n = b.Left.Len
		m = b.Right.Len
	)
	//
	switch {
	case n == 1 && m == 1 && skip <= math.MaxUint8:
		return encodeSkipIf_rr(util.Cast[uint8](skip), b.Left.Base, b.Right.Base, b.Op)
	case n == m:
		return encodeSkipIf_rv(skip, b.Left, b.Right, b.Op)
	default:
		panic("unsupported instruction form")
	}
}

// ============================================================================
// seq/sneq/slt,sleq,sgt,sgeq (skip conditional) instruction with (small)
// reg-reg operands.  Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  skip  |  rs0   |  rs1   | opcode |
// +--------+--------+--------+--------+
//
// Here, skip is a u8 identifying the number of instructions to skip, where the
// following instruction is considered to be at offset 0.  Likewise, rs0 and rs1
// are u8 source registers, whilst op identifies the operation.
// ============================================================================

// encodeSkipIf_rr encodes a register-register conditional skip instruction,
// where op identifies the comparison.
func encodeSkipIf_rr(skip uint8, rs0, rs1 RegisterId, op Cond) []uint32 {
	var (
		_rs1 = uint32(rs1) << 8
		_rs0 = uint32(rs0) << 16
	)
	// Forward branches are preferred as SKIP_IF instructions, whose offset is
	// unsigned and hence offers a greater forward range.
	return []uint32{
		uint32(skip)<<24 | _rs0 | _rs1 | (SEQ_rr + uint32(op)),
	}
}

// ============================================================================
// seq/sne/slt,sgt,sle,sge (skip conditional) instruction with (small) reg-reg
// operands.  This is the forward-branch encoding of a conditional jump, whose
// format matches the reg-reg form above except that offset is an unsigned u8
// relative offset, where the following instruction is considered to be at
// offset 0 (i.e. skip 0 transfers control to the next instruction).
// ============================================================================

// DecodeSkipIf_rr decodes the operands of a register-register conditional skip.
func DecodeSkipIf_rr(pc uint32, codes []uint32) (skip uint32, rs0, rs1 RegisterId, op Cond, n uint32) {
	op = Cond((codes[pc] & OPCODE_MASK) - SEQ_rr)
	rs1 = RegisterId((codes[pc] >> 8) & 0xff)
	rs0 = RegisterId((codes[pc] >> 16) & 0xff)
	skip = codes[pc] >> 24
	n = 1
	//
	return
}

// ============================================================================
// jeq/jneq/jlt,jleq,jgt,jgeq (jump conditional) instruction with vectored
// operands. Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   nv   |  rs0   |  rs1   | opcode |
// +--------+--------+--------+--------+
// | ............ target ............. |
// +--------+--------+--------+--------+
//
// Here, rs0 and rs1 are the base registers for the left and right vectors
// whilst nv is the vector length (which assumes both vectors have the same
// length).  Likewise, target is an absolute u32 target address.
// ============================================================================
// encodeSkipIf_rv encodes a register-vector conditional skip instruction.
func encodeSkipIf_rv(skip uint32, rs0, rs1 RegVec, op Cond) []uint32 {
	var (
		rs1b = uint32(rs1.Base) << 8
		rs0b = uint32(rs0.Base) << 16
		nv   = uint32(rs1.Len) << 24
	)
	// check core invariant
	if rs0.Len != rs1.Len {
		panic(fmt.Sprintf("mismatched length for source vectors (%d vs %d)", rs0.Len, rs1.Len))
	}
	//
	return []uint32{
		nv | rs0b | rs1b | (SEQ_rv + uint32(op)),
		skip,
	}
}

// DecodeSkipIf_rv decodes the operands of a register-vector conditional branch.
func DecodeSkipIf_rv(pc uint32, codes []uint32) (skip uint32, rs0, rs1 RegVec, op Cond, n uint32) {
	var (
		rs1b = RegisterId((codes[pc] >> 8) & 0xff)
		rs0b = RegisterId((codes[pc] >> 16) & 0xff)
		nv   = RegisterId((codes[pc] >> 24) & 0xff)
	)
	//
	op = Cond((codes[pc] & OPCODE_MASK) - SEQ_rv)
	skip = codes[pc+1]
	rs0 = RegVec{Base: rs0b, Len: nv}
	rs1 = RegVec{Base: rs1b, Len: nv}
	//
	return skip, rs0, rs1, op, 2
}
