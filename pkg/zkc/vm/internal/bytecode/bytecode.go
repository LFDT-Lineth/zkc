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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Cond provides a convenient alias to make the code more readable.
type Cond = opcode.Condition

// Reg just provides a convenient alias to make the code more readable.
type Reg = uint16

// Address just provides a convenient alias to make the code more readable.
type Address = uint32

// OPCODE_MASK determines how many bits of the opcode byte are used for the
// opcode itself.
const OPCODE_MASK = 0x3f

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
	FAIL uint32 = iota
	// JMP instruction
	JMP
	// JEQ_RR (jump if equal)
	JEQ_RR
	// JNE_RR (jump if not equal)
	JNE_RR
	// JLT_RR (jump if less than)
	JLT_RR
	// JLE_RR (jump if less than or equal)
	JGT_RR
	// JGE_RR (jump if greater than or equal)
	JLE_RR
	// JGT_RR (jump if greater than)
	JGE_RR
	// CALL instruction
	CALL
	// RET instruction
	RET
	// RD_ROM_N_M instruction
	RD_ROM_N_M
	// WR_WOM_N_M instruction
	WR_WOM_N_M
	// WR_SRAM instruction
	RD_RAM_N_M
	// WR_RAM_N_M instruction
	WR_RAM_N_M
	// WR_BRAM instruction
	RD_BRAM_N_M
	// WR_BRAM_N_M instruction
	WR_BRAM_N_M
	// PUSH instruction
	PUSH
	// POP instruction
	POP
	// MOVE instruction
	MOVE
	// LDC (load constant) instruction
	LDC
	// DESTRUCT instruction
	DESTRUCT
	// CAST instruction
	CAST
	// ADD instruction
	ADD
	// ADDC (add with constant) instruction
	ADDC
	// SUB instruction
	SUB
	// SUBC (subtract with constant) instruction
	SUBC
	// CSUB (subtract from constant) instruction
	CSUB
	// MUL instruction
	MUL
	// MULC (multiply with constant) instruction
	MULC
	// DIV instruction
	DIV
	// ADDMOD_P instruction
	ADDMOD_P
	// SUBMOD_P instruction
	SUBMOD_P
	// MULMOD_P instruction
	MULMOD_P
	// AND instruction
	AND
	// OR instruction
	OR
	// XOR instruction
	XOR
	// NOT instruction
	NOT
	// SHL instruction
	SHL
	// SHR instruction
	SHR
	// CAT instruction
	CAT
	//
	MAX_BYTECODE
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W word.Word[W]] interface {
	String() string
	Codes(uint32) []uint32
}

// Patchable bytecodes support the patch method.
type Patchable[W word.Word[W]] interface {
	Bytecode[W]
	Patch(labels []Address)
}

// ============================================================================
// Fail
// ============================================================================

// Fail instruction
type Fail struct{}

// NewFail constructs a new fail instruction.
func NewFail() *Fail {
	return &Fail{}
}

func (p *Fail) String() string {
	return "fail"
}

// Codes implementation for Bytecode interface
func (p *Fail) Codes(_ uint32) []uint32 {
	return []uint32{FAIL}
}

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

// Unpackage a given array of codes packed as n (small) registers.
func unpackCodesToSmallRegs(n uint32, codes []uint32) ([]Reg, uint32) {
	var (
		regs   = make([]Reg, n)
		ncodes = nCodesPackedSmall(n)
		code   uint32
	)
	//
	for i := range n {
		if i%4 == 0 {
			code = codes[i/4]
		}
		//
		regs[i] = uint16(code & 0xff)
		code = code >> 8
	}
	//
	return regs, ncodes
}

// Pack a given array of bytes into an array of codes, such that the last code
// is padded with 0xff.
func packRegsIntoCodes(bytes []byte) []uint32 {
	var (
		nBytes = uint32(len(bytes))
		ncodes = nCodesPackedSmall(nBytes)
		//
		codes = make([]uint32, ncodes)
	)
	//
	for i := range ncodes {
		var ith uint32
		for j := range uint32(4) {
			var jth uint32 = 0xff
			//
			if k := (i * 4) + j; k < nBytes {
				jth = uint32(bytes[k])
			}
			//
			ith = ith | (jth << (j * 8))
		}
		//
		codes[i] = ith
	}
	//
	return codes
}

func nCodesPackedSmall(n uint32) uint32 {
	var (
		// 4 bytes per code
		ncodes = n / 4
	)
	// Round up if necessary
	if n%4 != 0 {
		ncodes++
	}
	//
	return ncodes
}

func init() {
	if MAX_BYTECODE > OPCODE_MASK {
		panic("overflowing opcodes")
	}
}
