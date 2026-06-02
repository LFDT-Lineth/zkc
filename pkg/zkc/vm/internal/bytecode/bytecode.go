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
	FAIL = uint8(0)
	// JMP (jump unconditional) instruction
	JMP = uint8(1)
	// JIF (jump conditional) instruction
	JIF = uint8(2)
	// CALL instruction
	CALL = uint8(3)
	// RET instruction
	RET = uint8(4)
	// LOAD instruction
	LOAD = uint8(5)
	// STORE instruction
	STORE = uint8(6)
	// PUSH instruction
	PUSH = uint8(7)
	// POP instruction
	POP = uint8(8)
	// MOVE instruction
	MOVE = uint8(9)
	// DESTRUCT instruction
	DESTRUCT = uint8(10)
	// CAST instruction
	CAST = uint8(11)
	// ADD instruction
	ADD = uint8(12)
	// ADDC (add with constant) instruction
	ADDC = uint8(13)
	// SUB instruction
	SUB = uint8(14)
	// SUBC (subtract with constant) instruction
	SUBC = uint8(15)
	// CSUB (subtract from constant) instruction
	CSUB = uint8(16)
	// MUL instruction
	MUL = uint8(17)
	// MULC (multiply with constant) instruction
	MULC = uint8(18)
	// DIV instruction
	DIV = uint8(19)
	// ADDMOD_P instruction
	ADDMOD_P = uint8(20)
	// SUBMOD_P instruction
	SUBMOD_P = uint8(21)
	// MULMOD_P instruction
	MULMOD_P = uint8(22)
	// AND instruction
	AND = uint8(23)
	// OR instruction
	OR = uint8(24)
	// XOR instruction
	XOR = uint8(25)
	// NOT instruction
	NOT = uint8(26)
	// SHL instruction
	SHL = uint8(27)
	// SHR instruction
	SHR = uint8(28)
	// CAT instruction
	CAT = uint8(29)
	//
)

// Condition represents the set of permission comparitors for a Jc
// instruction.
type Condition uint

const (
	// EQ indicates an equality condition
	EQ Condition = 0
	// NEQ indicates a non-equality condition
	NEQ Condition = 1
	// LT indicates a less-than condition
	LT Condition = 2
	// GT indicates a greater-than condition
	GT Condition = 3
	// LTEQ indicates a less-than-or-equals condition
	LTEQ Condition = 4
	// GTEQ indicates a greater-than-or-equals condition
	GTEQ Condition = 5
)
