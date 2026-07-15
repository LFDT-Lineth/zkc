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

// RegisterId provides a convenient alias
type RegisterId = bytecode.RegisterId

// Operation provides a convenient alias
type Operation = bytecode.Operation

// Bytecode provides a convenient alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// ModuleId provides a convenient alias
type ModuleId = bytecode.ModuleId

// Address just provides a convenient alias to make the code more readable.
type Address = bytecode.Address

// Cond just provides a convenient alias to make the code more readable.
type Cond = bytecode.Condition

// RegisterVector just provides a convenient alias to make the code more readable.
type RegisterVector = bytecode.RegisterVector

// OPCODE_MASK is used to extract the actual opcode from the opcode byte.  The
// current format of an opcode byte is:
//
//	7   6   5                   0
//
// +---+---+---+---+---+---+---+---+
// | B | W |        OPCODE         |
// +---+---+---+---+---+---+---+---+
//
// Here, B is the Breakpoint flag, W is the Wide instruction flag, whilst the
// lower 6-bits form the opcode itself.  For reference, the breakpoint bit
// signals a breakpoint should be triggered immediately before the current
// instruction executes.
const OPCODE_MASK = 0x3f

// WIDE is a modifier bit (bit 6 of the opcode byte) marking the "wide" form of
// an instruction, whose register operands are u16 rather than u8.  Wide forms
// arise for functions with more than 256 registers.  By convention, a wide
// form keeps its non-register fields in the first instruction word exactly as
// the narrow form does (where they still fit), whilst its register operands
// move into subsequent words, packed two per word (least significant half
// first) in the order they appear in the narrow encoding.
const WIDE = 0x40

// BREAKPOINT is a modifier bit (bit 7 of the opcode byte) which, when set,
// signals that a breakpoint should be triggered immediately before the
// instruction executes.  Like WIDE, it lies above the opcode field, so dispatch
// (which masks with OPCODE_MASK) is unaffected and both flagged and unflagged
// instructions reach the same executor.
const BREAKPOINT = 0x80

// IsWideForm checks whether the instruction word at the given position has the
// WIDE modifier bit set (i.e. carries u16 register operands).
func IsWideForm(pc uint32, codes []uint32) bool {
	return codes[pc]&WIDE != 0
}

// IsWideRegisters checks whether any of the given registers requires the wide
// (u16) instruction form, i.e. does not fit within a single byte.
func IsWideRegisters(regs ...RegisterId) bool {
	for _, r := range regs {
		if r > math.MaxUint8 {
			return true
		}
	}
	//
	return false
}

// IsWideRegisterVectors checks whether any of the given register vectors
// requires the wide (u16) instruction form, i.e. has a base or length which
// does not fit within a single byte.
func IsWideRegisterVectors(vecs []RegisterVector) bool {
	for _, v := range vecs {
		if v.Base > math.MaxUint8 || v.Len > math.MaxUint8 {
			return true
		}
	}
	//
	return false
}

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
	// SKIP_M (skip table) instruction: dispatches on a source register against a
	// table of (value, target) pairs.
	SKIP_M
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
	// SEQ_rv (skip forward if equal)
	SEQ_rv
	// SNE_rv (skip forward if not equal)
	SNE_rv
	// SLT_rv (skip forward if less than)
	SLT_rv
	// SGT_rr (skip forward if greater than)
	SGT_rv
	// SLE_rv (skip forward if less than or equal)
	SLE_rv
	// SGE_rr (skip forward if greater than or equal)
	SGE_rv
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
	// RD_PRAM_nm instruction
	RD_PRAM_nm
	// WR_PRAM_nm instruction
	WR_PRAM_nm
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
	// INTRINSIC instruction (e.g. division hint, wide shift-left)
	INTRINSIC
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

// Encode encodes the given bytecode into its sequence of 32-bit instruction
// words.  Here, pc is the address at which the instruction will reside within
// the compiled sequence (needed to compute relative branch offsets), whilst env
// supplies the symbol information required to resolve branch targets, memories
// and formatted chunks.
func Encode[W word.Word[W]](b Bytecode[W], pc uint32, env Environment[W]) []uint32 {
	switch b := b.(type) {
	case *bytecode.Arith[W]:
		return Arith(*b, env)
	case *bytecode.Bitwise[W]:
		return Bitwise(b)
	case *bytecode.Call[W]:
		return Call(pc, b, env)
	case *bytecode.Cat[W]:
		return Cat(b)
	case *bytecode.CheckCast[W]:
		return CheckCast(b)
	case *bytecode.Debug[W]:
		return Debug(b, env)
	case *bytecode.DivRem[W]:
		return DivRem(b)
	case *bytecode.Intrinsic[W]:
		return Intrinsic(b)
	case *bytecode.Fail[W]:
		return Fail(b, env)
	case *bytecode.FieldArith[W]:
		return FieldArith(b)
	case *bytecode.Skip[W]:
		return Skip(pc, b, env)
	case *bytecode.SkipIf[W]:
		return SkipIf(pc, b, env)
	case *bytecode.Jmp[W]:
		return Jmp(pc, b, env)
	case *bytecode.ReadWrite[W]:
		return ReadWrite(b, env)
	case *bytecode.Ret[W]:
		return Ret(b, env)
	case *bytecode.Switch[W]:
		return Switch(b, env)
	default:
		panic(fmt.Sprintf("unknown instruction encountered (%s)", b.String(nil)))
	}
}

// MaxEncodedLength returns the maximum length (i.e. number of uint32 words) an
// encoding of the given bytecode can occupy.
func MaxEncodedLength[W word.Word[W]](b bytecode.Bytecode[W], env Environment[W]) uint {
	//
	switch b := b.(type) {
	case *bytecode.Call[W]:
		return MaxCallEncodedLength(b)
	case *bytecode.Skip[W], *bytecode.Jmp[W]:
		return 1
	case *bytecode.SkipIf[W]:
		// The wide form carries its base registers in an additional word.
		if IsWideRegisters(b.Left.Base, b.Right.Base) {
			return 3
		}
		//
		return 2
	default:
		// NOTE: the maximum length of the remaining bytecodes is not dependent
		// on this position within the encoded sequence.
		return uint(len(Encode(b, 0, env)))
	}
}

// GetBranchTarget resolves an absolute branch target from a base offset and a
// width-bit two's complement relative offset.
func GetBranchTarget(offset uint32, relOffset uint32, width uint) Address {
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

// GetRelativeOffset encodes the width-bit two's complement relative offset from
// pc to target, returning ok=false when the target is out of range.
func GetRelativeOffset(pc uint32, target Address, width uint) (roff uint32, ok bool) {
	var (
		sign = uint32(0x1) << (width - 1)
		max  = uint32(0x1) << width
		diff uint32
	)
	//
	if width >= 32 {
		// Should use absolute address here.
		panic("unsupported relative offset")
	}
	// NOTE: the offset is decoded (by getBranchTarget) as a width-bit two's
	// complement value; hence, forward branches must fit below the sign bit,
	// whilst backward branches must keep it set.
	if target > pc {
		diff = target - (pc + 1)
		//
		if diff >= sign {
			return 0, false
		}
	} else {
		diff = max + target - (pc + 1)
		//
		if diff < sign || diff >= max {
			return 0, false
		}
	}
	//
	return diff, true
}

// PackBytesIntoCodes packs a given array of bytes into an array of codes, such
// that the last code is padded with 0xff.
func PackBytesIntoCodes(bytes []byte) []uint32 {
	var (
		nBytes = uint32(len(bytes))
		ncodes = NumCodesPackedSmall(uint(nBytes))
		//
		codes = make([]uint32, ncodes)
	)
	//
	for i := range ncodes {
		var ith uint32
		//
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

// NumCodesPackedSmall returns the number of 32-bit codes required to pack n
// bytes, four bytes per code (rounding up).
func NumCodesPackedSmall(n uint) uint32 {
	var (
		// 4 bytes per code
		ncodes = uint32(n) / 4
	)
	// Round up if necessary
	if n%4 != 0 {
		ncodes++
	}
	//
	return ncodes
}

// PackShortsIntoCodes packs a given array of u16 operands into an array of
// codes (two per code, least significant half first), such that the last code
// is padded with 0xffff.
func PackShortsIntoCodes(shorts []uint16) []uint32 {
	var (
		nShorts = uint32(len(shorts))
		ncodes  = NumCodesPackedWide(uint(nShorts))
		//
		codes = make([]uint32, ncodes)
	)
	//
	for i := range ncodes {
		var ith uint32
		//
		for j := range uint32(2) {
			var jth uint32 = 0xffff
			//
			if k := (i * 2) + j; k < nShorts {
				jth = uint32(shorts[k])
			}
			//
			ith = ith | (jth << (j * 16))
		}
		//
		codes[i] = ith
	}
	//
	return codes
}

// NumCodesPackedWide returns the number of 32-bit codes required to pack n
// u16 operands, two per code (rounding up).
func NumCodesPackedWide(n uint) uint32 {
	var (
		// 2 shorts per code
		ncodes = uint32(n) / 2
	)
	// Round up if necessary
	if n%2 != 0 {
		ncodes++
	}
	//
	return ncodes
}

// RegsAsBytes packs an array of (small) registers into an array of bytes.  This
// will panic if any register is encountered which does not fit into a byte.
func RegsAsBytes(regs []RegisterId) []byte {
	var bytes = make([]byte, len(regs))
	// sanity checks
	bytecode.CheckSmallArgs(regs)
	//
	for i, r := range regs {
		bytes[i] = uint8(r & 0xff)
	}
	//
	return bytes
}

// RegisterVectorsAsBytes packs an array of (small) registers into an array of bytes.  This
// will panic if any register is encountered which does not fit into a byte.
func RegisterVectorsAsBytes(vecs []RegisterVector) []byte {
	var bytes = make([]byte, len(vecs)*2)
	//
	for i, r := range vecs {
		bytes[2*i] = util.Cast[uint8](r.Base)
		bytes[(2*i)+1] = util.Cast[uint8](r.Len)
	}
	//
	return bytes
}

// RegsAsShorts packs an array of registers into an array of u16 operands, as
// used by wide instruction forms.
func RegsAsShorts(regs []RegisterId) []uint16 {
	var shorts = make([]uint16, len(regs))
	//
	copy(shorts, regs)
	//
	return shorts
}

// RegisterVectorsAsShorts packs an array of register vectors into an array of
// u16 operands (as base / length pairs), as used by wide instruction forms.
func RegisterVectorsAsShorts(vecs []RegisterVector) []uint16 {
	var shorts = make([]uint16, len(vecs)*2)
	//
	for i, r := range vecs {
		shorts[2*i] = r.Base
		shorts[(2*i)+1] = r.Len
	}
	//
	return shorts
}

func init() {
	if MAX_BYTECODE > OPCODE_MASK {
		panic("overflowing opcodes")
	}
}
