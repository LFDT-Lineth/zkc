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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
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

// NewJif constructs a new conditional branch instruction.
func NewJif(op Cond, target Address, left, right register.Id) *Jif {
	return &Jif{
		target,
		NewRegVec(asReg(left)),
		NewRegVec(asReg(right)),
		op,
	}
}

// NewJifVec constructs a new conditional branch instruction.
func NewJifVec(op Cond, target Address, left, right register.Vector) *Jif {
	return &Jif{
		target,
		NewRegVec(asRegs(left.Registers()...)...),
		NewRegVec(asRegs(right.Registers()...)...),
		op,
	}
}

func (p *Jif) String() string {
	var ops string
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
	return fmt.Sprintf("if %s %s %s goto 0x%08x", p.Src0, ops, p.Src1, p.Target)
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
	default:
		panic("unsupported instruction form")
	}
}

// Patch implementation for Bytecode interface
func (p *Jif) Patch(labels []Address) {
	p.Target = labels[p.Target]
}

func decodeJif[W word.Word[W]](offset uint32, codes []uint32) (bc Bytecode[W], n uint32) {
	var (
		code   = codes[0]
		target Address
		op     Cond
		src0   RegVec
		src1   RegVec
	)
	//
	switch code & 0x1f {
	case JIF:
		var rs0, rs1 Reg
		//
		target, rs0, rs1, op = decodeJif_rr(offset, code)
		src0, src1 = RegVec{rs0, 1}, RegVec{rs1, 1}
		n = 1
		//
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
// jif_rr (jump conditional) instruction with (small) reg-reg operands.  Format is:
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

func encodeJif_rr(offset uint32, target Address, rs0, rs1 Reg, op Cond) []uint32 {
	var (
		_op   = uint32(op) << 5
		_rs1  = uint32(rs1) << 8
		_rs0  = uint32(rs0) << 16
		_roff = getRelativeOffset(offset, target, 8) << 24
	)
	//
	return []uint32{
		_roff | _rs0 | _rs1 | _op | JIF,
	}
}

func decodeJif_rr(offset uint32, code uint32) (target Address, rs0, rs1 Reg, op Cond) {
	op = Cond((code >> 5) & 0x7)
	rs1 = Reg((code >> 8) & 0xff)
	rs0 = Reg((code >> 16) & 0xff)
	target = getBranchTarget(offset, code>>24, 8)
	//
	return target, rs0, rs1, op
}
