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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Jif (jump conditional) instruction.
type Jif struct {
	Target Address
	Src0   RegVec
	Src1   RegVec
	Op     Cond
}

func (p *Jif) String(mapping SystemMap) string {
	var (
		ops  string
		src0 = registerVectorToString(p.Src0, mapping)
		src1 = registerVectorToString(p.Src1, mapping)
	)
	//
	switch p.Op {
	case opcode.EQ:
		ops = "=="
	case opcode.NEQ:
		ops = "!="
	case opcode.LT:
		ops = "<"
	case opcode.LTEQ:
		ops = "<="
	case opcode.GT:
		ops = ">"
	case opcode.GTEQ:
		ops = ">="
	default:
		ops = "??"
	}
	//
	return fmt.Sprintf("jif %s %s %s 0x%08x", src0, ops, src1, p.Target)
}

// Codes implementation for Bytecode interface
func (p *Jif) Codes(offset uint32) []uint32 {
	var (
		n = p.Src0.Len
		m = p.Src1.Len
	)
	//
	switch {
	case n == 1 && m == 1:
		return encodeJif_rr(offset, p.Target, p.Src0.Base, p.Src1.Base, p.Op)
	case n == m:
		return encodeJif_rv(offset, p.Target, p.Src0, p.Src1, p.Op)
	default:
		panic("unsupported instruction form")
	}
}

// Patch implementation for Patchable interface
func (p *Jif) Patch(labels []Address) Patched {
	return &Jif{Target: labels[p.Target], Src0: p.Src0, Src1: p.Src1, Op: p.Op}
}

// MaxWidth implementation for Patchable interface: the vectored form always
// occupies two code words.
func (p *Jif) MaxWidth() uint32 {
	return 2
}

func decodeJif[W word.Word[W]](pc uint32, codes []uint32) (bc Bytecode[W], n uint32) {
	var (
		code   = codes[pc]
		target Address
		op     Cond
		src0   RegVec
		src1   RegVec
	)
	//
	switch code & OPCODE_MASK {
	case JEQ_rr, JNE_rr, JLT_rr, JLE_rr, JGT_rr, JGE_rr:
		var rs0, rs1 Reg
		//
		target, rs0, rs1, op, n = decodeJif_rr(pc, codes)
		src0, src1 = RegVec{rs0, 1}, RegVec{rs1, 1}
		//
	case SEQ_rr, SNE_rr, SLT_rr, SLE_rr, SGT_rr, SGE_rr:
		var rs0, rs1 Reg
		//
		target, rs0, rs1, op, n = decodeSkipIf_rr(pc, codes)
		src0, src1 = RegVec{rs0, 1}, RegVec{rs1, 1}
		//
	case JEQ_rv, JNE_rv, JLT_rv, JLE_rv, JGT_rv, JGE_rv:
		target, src0, src1, op, n = decodeJif_rv(pc, codes)
	default:
		panic("unsupported instruction")
	}
	//
	return &Jif{
		Target: target,
		Src0:   src0,
		Src1:   src1,
		Op:     op}, n
}

// ============================================================================
// jeq/jneq/jlt,jleq,jgt,jgeq (jump conditional) instruction with (small)
// reg-reg operands.  Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | offset |  rs0   |  rs1   | opcode |
// +--------+--------+--------+--------+
//
// Here, offset is a signed u8 relative offset, where the following
// instruction is considered to be at offset 0.  Likewise, rs0 and rs1 are
// u8 source registers, whilst op identifies the operation.
// ============================================================================

func encodeJif_rr(pc uint32, target Address, rs0, rs1 Reg, op Cond) []uint32 {
	var (
		_rs1 = uint32(rs1) << 8
		_rs0 = uint32(rs0) << 16
	)
	// Forward branches are preferred as SKIP_IF instructions, whose offset is
	// unsigned and hence offers a greater forward range.
	if target > pc {
		if offset := target - (pc + 1); offset <= 0xff {
			return []uint32{
				offset<<24 | _rs0 | _rs1 | (SEQ_rr + uint32(op)),
			}
		}
	} else if offset, ok := getRelativeOffset(pc, target, 8); ok {
		return []uint32{
			offset<<24 | _rs0 | _rs1 | (JEQ_rr + uint32(op)),
		}
	}
	// fall back to vectored form which offers longer range.
	return encodeJif_rv(pc, target, RegVec{rs0, 1}, RegVec{rs1, 1}, op)
}

func decodeJif_rr(pc uint32, codes []uint32) (target Address, rs0, rs1 Reg, op Cond, n uint32) {
	op = Cond((codes[pc] & OPCODE_MASK) - JEQ_rr)
	rs1 = Reg((codes[pc] >> 8) & 0xff)
	rs0 = Reg((codes[pc] >> 16) & 0xff)
	target = getBranchTarget(pc, codes[pc]>>24, 8)
	n = 1
	//
	return
}

// ============================================================================
// seq/sne/slt,sgt,sle,sge (skip conditional) instruction with (small) reg-reg
// operands.  This is the forward-branch encoding of a conditional jump, whose
// format matches the reg-reg form above except that offset is an unsigned u8
// relative offset, where the following instruction is considered to be at
// offset 0 (i.e. skip 0 transfers control to the next instruction).
// ============================================================================

func decodeSkipIf_rr(pc uint32, codes []uint32) (target Address, rs0, rs1 Reg, op Cond, n uint32) {
	op = Cond((codes[pc] & OPCODE_MASK) - SEQ_rr)
	rs1 = Reg((codes[pc] >> 8) & 0xff)
	rs0 = Reg((codes[pc] >> 16) & 0xff)
	target = pc + 1 + (codes[pc] >> 24)
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
func encodeJif_rv(_ uint32, target Address, rs0, rs1 RegVec, op Cond) []uint32 {
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
		nv | rs0b | rs1b | (JEQ_rv + uint32(op)),
		target,
	}
}

func decodeJif_rv(pc uint32, codes []uint32) (target Address, rs0, rs1 RegVec, op Cond, n uint32) {
	var (
		rs1b = Reg((codes[pc] >> 8) & 0xff)
		rs0b = Reg((codes[pc] >> 16) & 0xff)
		nv   = Reg((codes[pc] >> 24) & 0xff)
	)
	//
	op = Cond((codes[pc] & OPCODE_MASK) - JEQ_rv)
	target = codes[pc+1]
	rs0 = RegVec{rs0b, nv}
	rs1 = RegVec{rs1b, nv}
	//
	return target, rs0, rs1, op, 2
}
