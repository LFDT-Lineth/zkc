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
//	7   6                       0
//
// +---+---+---+---+---+---+---+---+
// | B |          OPCODE           |
// +---+---+---+---+---+---+---+---+
//
// Here, B is the Breakpoint flag, whilst the lower 7-bits form the opcode
// itself.  For reference, the breakpoint bit signals a breakpoint should be
// triggered immediately before the current instruction executes.
const OPCODE_MASK = 0x7f

// BREAKPOINT is a modifier bit (bit 7 of the opcode byte) which, when set,
// signals that a breakpoint should be triggered immediately before the
// instruction executes.  It lies above the opcode field, so dispatch (which
// masks with OPCODE_MASK) is unaffected and both flagged and unflagged
// instructions reach the same executor.
const BREAKPOINT = 0x80

// WIDE is a distinguished escape opcode (the top of the 7-bit opcode space)
// signalling a "wide encoding".  For instructions dispatched this way, the
// second byte of the first word (bits 8-15, which every wide form reserves)
// holds the wide opcode itself (see the wide opcode enumeration below),
// giving wide instructions an 8-bit opcode space of their own.  NOTE: when
// the breakpoint flag is also set, the full first byte reads 0xFF.
const WIDE = 0x7f

// IsWideForm checks whether the instruction at the given position is a wide
// form (i.e. carries u16 register operands), which holds exactly when its
// opcode is the WIDE escape.
func IsWideForm(pc uint32, codes []uint32) bool {
	return codes[pc]&OPCODE_MASK == WIDE
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

// Every instruction occupies one or more 32-bit words, where the first byte
// of the first word holds the breakpoint flag (bit 7) above the 7-bit opcode
// (bits 0-6), as described for OPCODE_MASK above.
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
	// SKIP_B (skip bit-table) instruction: one-hot dispatch over a table of
	// (bit register, target) pairs.
	SKIP_B
	// SEQ_rr (skip forward if equal).  NOTE: each skip-if family carries only
	// four conditions (EQ, NEQ, LT, GE): GT and LE are normalised away at
	// encode time, either by swapping operands (a > b ⇔ b < a) or, for
	// constant operands, by adjusting the constant (x > c ⇔ x >= c+1).  This
	// keeps the opcode space compact.
	SEQ_rr
	// SNE_rr (skip forward if not equal)
	SNE_rr
	// SLT_rr (skip forward if less than)
	SLT_rr
	// SGE_rr (skip forward if greater than or equal)
	SGE_rr
	// SEQ_rv (skip forward if equal)
	SEQ_rv
	// SNE_rv (skip forward if not equal)
	SNE_rv
	// SLT_rv (skip forward if less than)
	SLT_rv
	// SGE_rv (skip forward if greater than or equal)
	SGE_rv
	// SEQ_rc (skip forward if equal to constant)
	SEQ_rc
	// SNE_rc (skip forward if not equal to constant)
	SNE_rc
	// SLT_rc (skip forward if less than constant)
	SLT_rc
	// SGE_rc (skip forward if greater than or equal to constant)
	SGE_rc
	// SEQ_rcv (skip forward if vector equal to constant vector)
	SEQ_rcv
	// SNE_rcv (skip forward if vector not equal to constant vector)
	SNE_rcv
	// SLT_rcv (skip forward if vector less than constant vector)
	SLT_rcv
	// SGE_rcv (skip forward if vector greater than or equal to constant vector)
	SGE_rcv
	// ENTER_n instruction
	ENTER_n
	// LEAVE_n instruction
	LEAVE_n
	// RET instruction
	RET
	// DONE instruction
	DONE
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
	// UINT_TO_FIELD instruction
	UINT_TO_FIELD
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
	// DIVMOD (combined division / remainder) instruction
	DIVMOD
	// DIVMODC (combined division / remainder by constant) instruction
	DIVMODC
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
	// FIELD_TO_UINT instruction
	FIELD_TO_UINT
	// CAT_2n1 instruction: dedicated (narrow-only) encoding of CAT for the
	// common two-target, one-source shape, distributing a single source
	// register across two targets without the general register-list
	// machinery.  Added at the end of the enum (rather than alongside CAT) so
	// existing opcode values are undisturbed.
	CAT_2n1
	// ENTER_2 instruction: dedicated (narrow-only) encoding of ENTER_n for the
	// common single-argument call, binding its one argument directly rather
	// than through the general register-list machinery.
	ENTER_2
	// LEAVE_2 instruction: dedicated (narrow-only) encoding of LEAVE_n for the
	// common single-return call, binding its one return directly rather than
	// through the general register-list machinery.
	LEAVE_2
	// CAT_1n instruction: dedicated (narrow-only) encoding of CAT for the
	// one-source, N-target shape, distributing a single source register
	// across N targets without the general register-list machinery.
	CAT_1n
	// CAT_n1 instruction: dedicated (narrow-only) encoding of CAT for the
	// N-source, one-target shape, combining N source registers into a
	// single target without the general register-list machinery.
	CAT_n1
	// ANDC (and with constant) instruction.  Added at the end of the enum
	// (rather than alongside AND) so existing opcode values are undisturbed.
	ANDC
	// ORC (or with constant) instruction [must follow ANDC]
	ORC
	// XORC (xor with constant) instruction [must follow ORC]
	XORC
	// TAILCALL_n instruction: identical to ENTER_n in every respect (same
	// payload layout, same execution), but distinguished as a call occurring
	// in tail position.
	TAILCALL_n
	// TAILCALL_2 instruction: dedicated (narrow-only) encoding of TAILCALL_n
	// for the common single-argument call, exactly as ENTER_2 is to ENTER_n.
	TAILCALL_2
	//
	MAX_BYTECODE
)

// The following enumerate the "wide opcodes", which occupy a separate 8-bit
// opcode space of their own: instructions encoded this way carry the WIDE
// escape opcode in the first byte of their first word, with the wide opcode
// itself in the second byte (bits 8-15) -- the byte which every wide form
// reserves.  Each WIDE_XX opcode is the wide form of the corresponding XX
// opcode, whose register operands are u16 rather than u8.  Wide forms arise
// for functions with more than 256 registers.  By convention, a wide form
// keeps its non-register fields in the first instruction word exactly as the
// narrow form does (where they still fit), whilst its register operands move
// into subsequent words, packed two per word (least significant half first)
// in the order they appear in the narrow encoding.  NOTE: family contiguity
// mirrors the narrow opcodes (e.g. WIDE_SUB_2n1 must follow WIDE_ADD_2n1),
// since encoders and decoders compute wide opcodes by offset from the family
// base.
const (
	// WIDE_FAIL instruction
	WIDE_FAIL uint32 = iota
	// WIDE_CHECKCAST instruction
	WIDE_CHECKCAST
	// WIDE_SEQ_rr (skip forward if equal)
	WIDE_SEQ_rr
	// WIDE_SNE_rr (skip forward if not equal)
	WIDE_SNE_rr
	// WIDE_SLT_rr (skip forward if less than)
	WIDE_SLT_rr
	// WIDE_SGE_rr (skip forward if greater than or equal)
	WIDE_SGE_rr
	// WIDE_SEQ_rv (skip forward if equal)
	WIDE_SEQ_rv
	// WIDE_SNE_rv (skip forward if not equal)
	WIDE_SNE_rv
	// WIDE_SLT_rv (skip forward if less than)
	WIDE_SLT_rv
	// WIDE_SGE_rv (skip forward if greater than or equal)
	WIDE_SGE_rv
	// WIDE_SEQ_rcv (skip forward if vector equal to constant vector)
	WIDE_SEQ_rcv
	// WIDE_SNE_rcv (skip forward if vector not equal to constant vector)
	WIDE_SNE_rcv
	// WIDE_SLT_rcv (skip forward if vector less than constant vector)
	WIDE_SLT_rcv
	// WIDE_SGE_rcv (skip forward if vector greater than or equal to constant
	// vector)
	WIDE_SGE_rcv
	// WIDE_ENTER_n instruction
	WIDE_ENTER_n
	// WIDE_LEAVE_n instruction
	WIDE_LEAVE_n
	// WIDE_RET instruction
	WIDE_RET
	// WIDE_RD_ROM_nm instruction
	WIDE_RD_ROM_nm
	// WIDE_RD_SROM_nm instruction
	WIDE_RD_SROM_nm
	// WIDE_WR_WOM_nm instruction
	WIDE_WR_WOM_nm
	// WIDE_RD_RAM_nm instruction
	WIDE_RD_RAM_nm
	// WIDE_WR_RAM_nm instruction
	WIDE_WR_RAM_nm
	// WIDE_RD_PRAM_nm instruction
	WIDE_RD_PRAM_nm
	// WIDE_WR_PRAM_nm instruction
	WIDE_WR_PRAM_nm
	// WIDE_MOVE instruction
	WIDE_MOVE
	// WIDE_LDC_w (load wide constant) instruction
	WIDE_LDC_w
	// WIDE_UINT_TO_FIELD instruction
	WIDE_UINT_TO_FIELD
	// WIDE_ADD_2n1 instruction
	WIDE_ADD_2n1
	// WIDE_SUB_2n1 instruction [must follow WIDE_ADD_2n1]
	WIDE_SUB_2n1
	// WIDE_MUL_2n1 instruction [must follow WIDE_SUB_2n1]
	WIDE_MUL_2n1
	// WIDE_ADDC (add with constant) instruction
	WIDE_ADDC
	// WIDE_SUBC (subtract with constant) instruction
	WIDE_SUBC
	// WIDE_MULC (multiply with constant) instruction
	WIDE_MULC
	// WIDE_ADD_nm (addition with vector target) instruction
	WIDE_ADD_nm
	// WIDE_SUB_nm (subtraction with vector target) instruction [must follow
	// WIDE_ADD_nm]
	WIDE_SUB_nm
	// WIDE_MUL_nm (multiplication with vector target) instruction [must
	// follow WIDE_SUB_nm]
	WIDE_MUL_nm
	// WIDE_DIVMOD (combined division / remainder) instruction
	WIDE_DIVMOD
	// WIDE_DIVMODC (combined division / remainder by pooled constant)
	// instruction
	WIDE_DIVMODC
	// WIDE_INTRINSIC instruction
	WIDE_INTRINSIC
	// WIDE_ADDMOD_P instruction
	WIDE_ADDMOD_P
	// WIDE_SUBMOD_P instruction
	WIDE_SUBMOD_P
	// WIDE_MULMOD_P instruction
	WIDE_MULMOD_P
	// WIDE_AND instruction
	WIDE_AND
	// WIDE_OR instruction
	WIDE_OR
	// WIDE_XOR instruction
	WIDE_XOR
	// WIDE_NOT instruction
	WIDE_NOT
	// WIDE_SHL instruction
	WIDE_SHL
	// WIDE_SHR instruction
	WIDE_SHR
	// WIDE_CAT instruction
	WIDE_CAT
	// WIDE_DEBUG instruction
	WIDE_DEBUG
	// WIDE_FIELD_TO_UINT instruction
	WIDE_FIELD_TO_UINT
	// WIDE_SKIP_M instruction
	WIDE_SKIP_M
	// WIDE_ANDC (and with constant) instruction
	WIDE_ANDC
	// WIDE_ORC (or with constant) instruction [must follow WIDE_ANDC]
	WIDE_ORC
	// WIDE_XORC (xor with constant) instruction [must follow WIDE_ORC]
	WIDE_XORC
	// WIDE_TAILCALL_n instruction: the wide form of TAILCALL_n, exactly as
	// WIDE_ENTER_n is to ENTER_n.  There is no wide form of TAILCALL_2, just
	// as there is none for ENTER_2: a frame width or argument register which
	// doesn't fit falls back to the general TAILCALL_n encoding instead.
	WIDE_TAILCALL_n
	//
	MAX_WIDE_BYTECODE
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
		return Bitwise(b, env)
	case *bytecode.Call[W]:
		return Call(pc, b, env)
	case *bytecode.Cat[W]:
		return Cat(b)
	case *bytecode.CheckCast[W]:
		return CheckCast(b)
	case *bytecode.Debug[W]:
		return Debug(b, env)
	case *bytecode.DivRem[W]:
		return DivRem(b, env)
	case *bytecode.Intrinsic[W]:
		return Intrinsic(b, env)
	case *bytecode.Fail[W]:
		return Fail(b, env)
	case *bytecode.FieldArith[W]:
		return FieldArith(b, env)
	case *bytecode.UintToField[W]:
		return UintToField(b)
	case *bytecode.FieldToUint[W]:
		return FieldToUint(b)
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
		return Switch(pc, b, env)
	case *bytecode.Dispatch[W]:
		return Dispatch(pc, b, env)
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
	case *bytecode.Dispatch[W]:
		// word 0 plus one (skip, bit) word per case.  NOTE: an explicit case
		// is required (rather than falling through to Encode below) because
		// the case skips are relative, and hence cannot be computed at a
		// placeholder position.
		return uint(1 + len(b.Cases))
	case *bytecode.Switch[W]:
		// word 0, plus word 1 for the wide form's source register / base pool
		// identifier, plus one u16 skip per case, packed two per word.  NOTE:
		// an explicit case is required (rather than falling through to Encode
		// below) because the case skips are relative, and hence cannot be
		// computed at a placeholder position.  The wide form's extra word is
		// always accounted for here, since the fixpoint iteration must start
		// from a conservative (maximal) size.
		return uint(2 + NumCodesPackedWide(uint(len(b.Cases))))
	case *bytecode.SkipIf[W]:
		if b.Right.IsRegisterVector() {
			// The wide form carries its base registers in an additional word.
			if IsWideRegisters(b.Left.Base, b.Right.AsRegisterVector().Base) {
				return 3
			}
			//
			return 2
		}
		// Constant form: bounded by the general (wide) constant-vector
		// fallback, which carries its register vector and constant vector in
		// words of their own.  The compact forms (single-word SXX_rc and
		// two-word SXX_rcv) are always smaller.
		return 3
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
	// NOTE: the WIDE escape occupies the top of the (7-bit) opcode space, so
	// regular opcodes must stay strictly below it.
	if MAX_BYTECODE > WIDE {
		panic("overflowing opcodes")
	}
	// The wide opcode space is a single byte.
	if MAX_WIDE_BYTECODE > 256 {
		panic("overflowing wide opcodes")
	}
}
