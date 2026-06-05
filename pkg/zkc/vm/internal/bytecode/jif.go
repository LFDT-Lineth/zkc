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
)

// Jif (jump conditional) instruction.  Format of this instruction is:
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
type Jif struct {
	Target Address
	Src0   Reg
	Src1   Reg
	Op     Cond
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
	return fmt.Sprintf("jif r%d %s r%d 0x%04x", p.Src0, ops, p.Src1, p.Target)
}

// Codes implementation for Bytecode interface
func (p *Jif) Codes(offset uint32) []uint32 {
	var (
		op   = uint32(p.Op) << 5
		rs1  = uint32(p.Src1) << 8
		rs0  = uint32(p.Src0) << 16
		roff = getRelativeOffset(offset, p.Target, 8) << 24
	)
	//
	return []uint32{
		roff | rs0 | rs1 | op | JIF,
	}
}

// Patch implementation for Bytecode interface
func (p *Jif) Patch(labels []Address) {
	p.Target = labels[p.Target]
}

func decodeJif(offset uint32, codes []uint32) (Jif, uint32) {
	var (
		op     = Cond((codes[0] >> 5) & 0x7)
		rs1    = Reg((codes[0] >> 8) & 0xff)
		rs0    = Reg((codes[0] >> 16) & 0xff)
		target = getBranchTarget(offset, codes[0]>>24, 8)
	)
	//
	return Jif{
		Op:     op,
		Src1:   rs1,
		Src0:   rs0,
		Target: target,
	}, 1
}
