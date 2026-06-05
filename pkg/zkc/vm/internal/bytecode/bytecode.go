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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// Cond provides a convenient alias to make the code more readable.
type Cond = opcode.Condition

// Reg just provides a convenient alias to make the code more readable.
type Reg = uint16

// Address just provides a convenient alias to make the code more readable.
type Address = uint32

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
	// READ instruction
	READ = uint32(5)
	// WRITE instruction
	WRITE = uint32(6)
	// PUSH instruction
	PUSH = uint32(7)
	// POP instruction
	POP = uint32(8)
	// MOVE instruction
	MOVE = uint32(9)
	// LDC (load constant) instruction
	LDC = uint32(9)
	// DESTRUCT instruction
	DESTRUCT = uint32(10)
	// CAST instruction
	CAST = uint32(11)
	// ADD instruction
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
	Codes(uint32) []uint32
}

// Patchable bytecodes support the patch method.
type Patchable interface {
	Bytecode
	Patch(labels []Address)
}

// ============================================================================
// Fail
// ============================================================================

// Fail instruction
type Fail struct{}

func (p *Fail) String() string {
	return "fail"
}

// Codes implementation for Bytecode interface
func (p *Fail) Codes(_ uint32) []uint32 {
	return []uint32{FAIL}
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
func (p *Ret) Codes(_ uint32) []uint32 {
	return []uint32{RET}
}

// Patch implementation for Bytecode interface
func (p *Ret) Patch(_ []Address) {
	// do nothing
}

// ============================================================================
// Jif
// ============================================================================

// ============================================================================
// Helpers
// ============================================================================

func getBranchTarget(offset uint32, relOffset uint32, width uint) Address {
	var (
		sign = uint32(0x1) << (width - 1)
		max  = uint32(0x1) << width
	)
	//
	if relOffset < sign {
		return offset + 1 + relOffset
	}
	//
	return offset + 1 - max + relOffset
}

func getRelativeOffset(offset uint32, target Address, width uint) uint32 {
	var sign_bit = uint32(0x1) << width
	//
	if target > offset {
		return target - offset - 1
	}
	//
	roff := 1 + offset - target
	//
	if roff >= sign_bit {
		panic("branch target overflow")
	}
	//
	return sign_bit - roff
}
