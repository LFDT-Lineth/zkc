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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
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
	// CHECKCAST instruction
	CHECKCAST
	// JMP instruction
	JMP
	// SKIP (unconditional forward branch) instruction
	SKIP
	// JEQ_rr (jump if equal)
	JEQ_rr
	// JNE_rr (jump if not equal)
	JNE_rr
	// JLT_rr (jump if less than)
	JLT_rr
	// JLE_RR (jump if less than or equal)
	JGT_rr
	// JGE_RR (jump if greater than or equal)
	JLE_rr
	// JGT_RR (jump if greater than)
	JGE_rr
	// JEQ_rv (vectored jump if equal)
	JEQ_rv
	// JNE_rv (vectored jump if not equal)
	JNE_rv
	// JLT_rv (vectored jump if less than)
	JLT_rv
	// JLE_nm (vectored jump if less than or equal)
	JGT_rv
	// JGE_nm (vectored jump if greater than or equal)
	JLE_rv
	// JGT_nm (vectored jump if greater than)
	JGE_rv
	// SEQ_rr (skip forward if equal)
	SEQ_rr
	// SNE_rr (skip forward if not equal)
	SNE_rr
	// SLT_rr (skip forward if less than)
	SLT_rr
	// SGT_rr (skip forward if greater than)
	SGT_rr
	// SLE_rr (skip forward if less than or equal)
	SLE_rr
	// SGE_rr (skip forward if greater than or equal)
	SGE_rr
	// ENTER_n instruction
	ENTER_n
	// LEAVE_n instruction
	LEAVE_n
	// RET instruction
	RET
	// RD_ROM_nm instruction
	RD_ROM_nm
	// RD_SROM_nm instruction
	RD_SROM_nm
	// WR_WOM_nm instruction
	WR_WOM_nm
	// WR_SRAM instruction
	RD_RAM_nm
	// WR_RAM_nm instruction
	WR_RAM_nm
	// WR_BRAM instruction
	RD_BRAM_nm
	// WR_BRAM_nm instruction
	WR_BRAM_nm
	// PUSH instruction
	PUSH
	// POP instruction
	POP
	// MOVE instruction
	MOVE
	// LDC (load constant) instruction
	LDC
	// LDC_w (load wide constant) instruction
	LDC_w
	// DESTRUCT instruction
	DESTRUCT
	// CAST instruction
	CAST
	// ADD_2n1 instruction
	ADD_2n1
	// SUB_2n1 instruction [must follow ADD_2n1]
	SUB_2n1
	// MUL_2n1 instruction [must follow SUB_2n1]
	MUL_2n1
	// ADDC (add with constant) instruction
	ADDC
	// SUBC (subtract with constant) instruction
	SUBC
	// MULC (multiply with constant) instruction
	MULC
	// ADD_nm (addition with vector target) instruction
	ADD_nm
	// SUB_nm (subtraction with vector target) instruction [must follow ADD_nm]
	SUB_nm
	// MUL_nm (multiplication with vector target) instruction [must follow SUB_nm]
	MUL_nm
	// CSUB (subtract from constant) instruction
	CSUB
	// DIV instruction
	DIV
	// REM instruction
	REM
	// DIVHINT (division hint) instruction
	DIVHINT
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
	// DEBUG instruction
	DEBUG
	//
	MAX_BYTECODE
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W word.Word[W]] interface {
	String(SystemMap) string
	Codes(uint32) []uint32
}

// Patchable bytecodes contain a branch target which must be resolved during
// encoding.  Until resolved, their encoded width is unknown (it can depend on
// the target), hence MaxWidth provides a conservative bound.
type Patchable[W word.Word[W]] interface {
	Bytecode[W]
	// Patch returns a copy of this bytecode with its target resolved against
	// the given label addresses.  The receiver is left untouched.
	Patch(labels []Address) Patched
	// MaxWidth returns the largest number of code words this bytecode can
	// occupy, regardless of where its target resolves.
	MaxWidth() uint32
}

// Patched is a bytecode whose branch target has been resolved (i.e. the result
// of Patchable.Patch).  Its method set matches Bytecode, which is independent
// of the word type; hence patched bytecodes convert directly into Bytecode[W].
type Patched interface {
	String(SystemMap) string
	Codes(uint32) []uint32
}

// ============================================================================
// Constructors
// ============================================================================
//
// The constructors below provide a more readable way to build bytecode
// instructions than instantiating the underlying instruction structs directly.
// Several of them are thin wrappers around the general-purpose Arith
// instruction, which computes "target = source[0] op source[1] op ... op
// constant" for some arithmetic operation op (add, subtract or multiply).  The
// "Vec" variants accept a slice of target registers, allowing a single logical
// value to be spread across multiple register limbs (e.g. when a value is wider
// than the underlying word type W).

// AddConst constructs an addition instruction computing
// "target = sum(sources) + constant" into a single target register.
func AddConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_ADD, asRegs(target), asRegs(sources...), constant)
}

// AddVec constructs a vectored addition instruction computing
// "targets = sum(sources)" (i.e. with no constant addend), where targets is a
// multi-limb register vector.
func AddVec[W word.Word[W]](targets []register.Id, sources []register.Id) *Arith[W] {
	var zero W
	return newArith(arithop_ADD, asRegs(targets...), asRegs(sources...), zero)
}

// AddVecConst constructs a vectored addition instruction computing
// "targets = sum(sources) + constant", where targets is a multi-limb register
// vector.
func AddVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_ADD, asRegs(targets...), asRegs(sources...), constant)
}

// CallFun constructs a function-call bytecode.
func CallFun(target Address, width uint16, args []register.Id, returns []register.Id) *Call {
	return &Call{target, width, asRegs(args...), asRegs(returns...)}
}

// NewFail constructs a fail instruction, which causes the machine to panic when
// executed.
func NewFail() *Fail {
	return &Fail{}
}

// Jump creates an unconditional jump instruction transferring control to the
// given target address.
func Jump(target Address) *Jmp {
	return &Jmp{target}
}

// JumpIf constructs a conditional branch instruction which jumps to the target
// address when "left op right" holds, comparing single registers.
func JumpIf(op Cond, target Address, left, right register.Id) *Jif {
	return &Jif{target, NewRegVec(asReg(left)), NewRegVec(asReg(right)), op}
}

// JumpIfVec constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing multi-limb register
// vectors.
func JumpIfVec(op Cond, target Address, left, right register.Vector) *Jif {
	return &Jif{target, NewRegVec(asRegs(left.Registers()...)...), NewRegVec(asRegs(right.Registers()...)...), op}
}

// LoadConst constructs a load-constant (LDC) instruction which assigns the
// given constant to the target register.
func LoadConst[W word.Word[W]](target register.Id, constant W) *Arith[W] {
	return newArith(arithop_ADD, asRegs(target), nil, constant)
}

// Move constructs a move instruction which copies the source register into the
// target register.
func Move[W word.Word[W]](target register.Id, source register.Id) *Arith[W] {
	var zero W
	return newArith(arithop_ADD, asRegs(target), asRegs(source), zero)
}

// MulConst constructs a multiplication instruction computing
// "target = product(sources) * constant" into a single target register.
func MulConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_MUL, asRegs(target), asRegs(sources...), constant)
}

// MulVecConst constructs a vectored multiplication instruction computing
// "targets = product(sources) * constant", where targets is a multi-limb
// register vector.
func MulVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_MUL, asRegs(targets...), asRegs(sources...), constant)
}

// ReadRom constructs a read instruction for a (non-static) read-only memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadRom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{ROM_READ, id, asRegs(address...), asRegs(data...)}
}

// ReadStaticRom constructs a read instruction for a static read-only memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadStaticRom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{SROM_READ, id, asRegs(address...), asRegs(data...)}
}

// ReadRam constructs a read instruction for a (small) random-access memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{SRAM_READ, id, asRegs(address...), asRegs(data...)}
}

// ReadBigRam constructs a read instruction for a (large) bipartite
// random-access memory.  The data registers receive the row located at the
// address given by the address registers, in the memory identified by id.
func ReadBigRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{BRAM_READ, id, asRegs(address...), asRegs(data...)}
}

// SubConst constructs a subtraction instruction computing
// "target = sources[0] - ... - constant" into a single target register.
func SubConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_SUB, asRegs(target), asRegs(sources...), constant)
}

// SubVecConst constructs a vectored subtraction instruction computing
// "targets = sources[0] - ... - constant", where targets is a multi-limb
// register vector.
func SubVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return newArith(arithop_SUB, asRegs(targets...), asRegs(sources...), constant)
}

// WriteWom constructs a write instruction for a write-once memory.  The data
// registers are written to the row located at the address given by the address
// registers, in the memory identified by id.
func WriteWom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{WOM_WRITE, id, asRegs(address...), asRegs(data...)}
}

// WriteRam constructs a write instruction for a (small) random-access memory.
// The data registers are written to the row located at the address given by the
// address registers, in the memory identified by id.
func WriteRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{SRAM_WRITE, id, asRegs(address...), asRegs(data...)}
}

// WriteBigRam constructs a write instruction for a (large) bipartite
// random-access memory.  The data registers are written to the row located at
// the address given by the address registers, in the memory identified by id.
func WriteBigRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{BRAM_WRITE, id, asRegs(address...), asRegs(data...)}
}

func init() {
	if MAX_BYTECODE > OPCODE_MASK {
		panic("overflowing opcodes")
	}
}
