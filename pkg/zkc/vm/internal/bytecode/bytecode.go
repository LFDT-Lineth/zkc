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

// Every instruction occupies 32 bits, where the first byte is as follows:
//
//	7   5 4       0
//
// +-----+---------+
// | : : | : : : : |
// +-----+---------+
//
//	(n)   (opcode)
//
// Currently, n is instruction specific.
const (
	// FAIL instruction
	FAIL = uint32(0)
	// JMP instruction
	JMP = uint32(1)
	// JIF instruction
	JIF = uint32(2)
	// CALL instruction
	CALL = uint32(3)
	// RET instruction
	RET = uint32(4)
	// LOAD instruction
	LOAD = uint32(5)
	// STORE instruction
	STORE = uint32(6)
	// PUSH instruction
	PUSH = uint32(7)
	// POP instruction
	POP = uint32(8)
	// MOVE instruction.  Format of this instruction is:
	//
	//  31                                       0
	// +--------+--------+--------+------+--------+
	// |   n/a  |   rs   |   rd   | n/a  | opcode |
	// +--------+--------+--------+------+--------+
	//
	// Here, rs is a u8 source register whilst rd is a u8 destination register.
	MOVE = uint32(9)
	// DESTRUCT instruction
	DESTRUCT = uint32(10)
	// CAST instruction
	CAST = uint32(11)
	// ADD instruction.  Format of this instruction is:
	//
	//  31                                       0
	// +--------+--------+--------+------+--------+
	// |  rs0   |  rs1   |   rd   | n/a  | opcode |
	// +--------+--------+--------+------+--------+
	//
	// Here, rs0 and rs1 are u8 source registers, whilst rd is a u8 destination
	// register.
	ADD = uint32(12)
	// ADDC (add with constant) instruction
	ADDC = uint32(13)
	// SUB instruction
	SUB = uint32(14)
	// SUBC (subtract with constant) instruction
	SUBC = uint32(15)
	// CSUB (subtract from constant) instruction
	CSUB = uint32(16)
	// MUL instruction
	MUL = uint32(17)
	// MULC (multiply with constant) instruction
	MULC = uint32(18)
	// DIV instruction
	DIV = uint32(19)
	// ADDMOD_P instruction
	ADDMOD_P = uint32(20)
	// SUBMOD_P instruction
	SUBMOD_P = uint32(21)
	// MULMOD_P instruction
	MULMOD_P = uint32(22)
	// AND instruction
	AND = uint32(23)
	// OR instruction
	OR = uint32(24)
	// XOR instruction
	XOR = uint32(25)
	// NOT instruction
	NOT = uint32(26)
	// SHL instruction
	SHL = uint32(27)
	// SHR instruction
	SHR = uint32(28)
	// CAT instruction
	CAT = uint32(29)
	//
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode interface {
	String() string
	Codes(uint) []uint32
	Patch(labels []uint)
}

// Fail instruction
type Fail struct{}

func (p *Fail) String() string {
	return "fail"
}

// Codes implementation for Bytecode interface
func (p *Fail) Codes(_ uint) []uint32 {
	return []uint32{FAIL}
}

// Patch implementation for Bytecode interface
func (p *Fail) Patch(_ []uint) {
	// do nothing
}

// ============================================================================
// Ret
// ============================================================================

// Ret (return from function call) instruction.
type Ret struct{}

func (p *Ret) String() string {
	return "ret"
}

// Codes implementation for Bytecode interface
func (p *Ret) Codes(_ uint) []uint32 {
	return []uint32{RET}
}

// Patch implementation for Bytecode interface
func (p *Ret) Patch(_ []uint) {
	// do nothing
}

// Jmp (jump unconditional) instruction.  Format of this instruction is:
//
// +--------------------------+------+---------+
// |        offset            | n/a  | opcode  |
// +--------------------------+------+---------+
//
//	31                       8 7    5 4       0
//
// Here, offset is a signed u16 relative offset, where the following
// instruction is considered to be at offset 0.
type Jmp struct{ Target uint }

func (p *Jmp) String() string {
	//
	return fmt.Sprintf("jmp 0x%04x", p.Target)
}

// Codes implementation for Bytecode interface
func (p *Jmp) Codes(offset uint) []uint32 {
	var roff = getRelativeOffset(offset, p.Target, 24) << 8
	//
	return []uint32{
		roff | JMP,
	}
}

// Patch implementation for Bytecode interface
func (p *Jmp) Patch(labels []uint) {
	p.Target = labels[p.Target]
}

// ============================================================================
// Jif
// ============================================================================

// Jif (jump conditional) instruction.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+------+--------+
// | offset |  rs0   |  rs1   |  op  | opcode |
// +--------+--------+--------+------+--------+
//
// Here, offset is a signed u8 relative offset, where the following
// instruction is considered to be at offset 0.  Likewise, rs0 and rs1 are
// u8 source registers, whilst op identifies the operation.
type Jif struct {
	Op     opcode.Condition
	Src1   uint
	Src0   uint
	Target uint
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
func (p *Jif) Codes(offset uint) []uint32 {
	var (
		op   = uint32(p.Op) << 5
		r    = uint32(p.Src1) << 8
		l    = uint32(p.Src0) << 16
		roff = getRelativeOffset(offset, p.Target, 8) << 24
	)
	//
	return []uint32{
		roff | l | r | op | JIF,
	}
}

// Patch implementation for Bytecode interface
func (p *Jif) Patch(labels []uint) {
	p.Target = labels[p.Target]
}

// ============================================================================
// Helpers
// ============================================================================

func getBranchTarget(offset uint, relOffset uint, width uint) uint {
	var (
		sign = uint(0x1) << (width - 1)
		max  = uint(0x1) << width
	)
	//
	if relOffset < sign {
		return offset + 1 + relOffset
	}
	//
	return offset + 1 - max + relOffset
}

func getRelativeOffset(offset uint, target uint, width uint) uint32 {
	var sign_bit = uint32(0x1) << width
	//
	if target > offset {
		return uint32(target - offset - 1)
	}
	//
	roff := uint32(1 + offset - target)
	//
	if roff >= sign_bit {
		panic("branch target overflow")
	}
	//
	return sign_bit - roff
}
