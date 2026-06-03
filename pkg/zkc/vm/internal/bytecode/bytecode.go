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
	// JMP (jump unconditional) instruction.  Format of this instruction is:
	//
	//	31                       8 7    5 4       0
	// +--------------------------+------+---------+
	// |        offset            | n/a  | opcode  |
	// +--------------------------+------+---------+
	//
	// Here, offset is a signed u16 relative offset, where the following
	// instruction is considered to be at offset 0.
	JMP = uint32(1)
	// JIF (jump conditional) instruction.  Format of this instruction is:
	//
	//	31                                0
	// +--------+--------+--------+------+--------+
	// | offset |  rs0   |  rs1   |  op  | opcode |
	// +--------+--------+--------+------+--------+
	//
	// Here, offset is a signed u8 relative offset, where the following
	// instruction is considered to be at offset 0.  Likewise, rs0 and rs1 are
	// u8 source registers, whilst op identifies the operation.
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
