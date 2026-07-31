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

// SkipIf encodes a conditional forward-branch bytecode, selecting the
// register-register, register-vector or register-constant form according to
// its operands.
func SkipIf[W word.Word[W]](pc Address, b *bytecode.SkipIf[W], env Environment[W]) []uint32 {
	var (
		target = env.Point().Skip(uint(b.Skip))
		offset = env.OffsetFor(env.enclosing, target)
		skip   = offset - (pc + 1)
	)
	//
	if b.Right.IsConstant() {
		return encodeSkipIfConstant(skip, b, env)
	}
	//
	var (
		left  = b.Left
		right = b.Right.AsRegisterVector()
		op    = b.Op
	)
	// The instruction set carries only EQ/NEQ/LT/GE conditions: normalise
	// GT/LE by swapping operands (a > b ⇔ b < a, a <= b ⇔ b >= a).
	switch op {
	case bytecode.CONDITION_GT:
		left, right, op = right, left, bytecode.CONDITION_LT
	case bytecode.CONDITION_LTEQ:
		left, right, op = right, left, bytecode.CONDITION_GTEQ
	}
	//
	var (
		n = left.Len
		m = right.Len
	)
	//
	switch {
	case n == 1 && m == 1 && skip <= math.MaxUint16:
		return encodeSkipIf_rr(util.Cast[uint16](skip), left.Base, right.Base, op)
	case n == m:
		return encodeSkipIf_rv(skip, left, right, op)
	default:
		panic(fmt.Sprintf("unsupported instruction form (%d v %d limbs)", m, n))
	}
}

// encodeSkipIfConstant encodes a conditional skip whose right operand is a
// constant, selecting the single-constant (rc) or constant-vector (rcv) form
// according to the number of comparison elements.  Since operands cannot be
// swapped here, GT/LE are instead normalised by adjusting the constant
// (x > c ⇔ x >= c+1, x <= c ⇔ x < c+1); when c+1 overflows, the condition is
// statically decided and encoded as a comparison against zero which is
// trivially false (x < 0) or trivially true (x >= 0).
func encodeSkipIfConstant[W word.Word[W]](skip uint32, b *bytecode.SkipIf[W], env Environment[W]) []uint32 {
	var (
		nv = uint(b.Left.Len)
		op = b.Op
		// NOTE: the constants are copied (zero-extended at the front to one
		// element per register, since a short or empty vector denotes zero)
		// both because the normalisation below mutates them, and because
		// encoding runs several times whilst the layout reaches a fixpoint.
		constants = make([]W, nv)
	)
	//
	if uint(len(b.Right.AsConstants())) > nv {
		panic(fmt.Sprintf("unsupported instruction form (%d limbs v %d constants)", nv, len(b.Right.AsConstants())))
	}
	//
	copy(constants[nv-uint(len(b.Right.AsConstants())):], b.Right.AsConstants())
	// Normalise GT/LE by incrementing the constant.  On overflow the
	// constants are left holding zero, giving exactly the trivially false
	// (x < 0) or trivially true (x >= 0) encodings required.
	switch op {
	case bytecode.CONDITION_GT:
		if incrementConstants(constants) {
			op = bytecode.CONDITION_GTEQ
		} else {
			// x > max is trivially false
			op = bytecode.CONDITION_LT
		}
	case bytecode.CONDITION_LTEQ:
		if incrementConstants(constants) {
			op = bytecode.CONDITION_LT
		} else {
			// x <= max is trivially true
			op = bytecode.CONDITION_GTEQ
		}
	}
	//
	// The compact single-constant form carries its skip, register and constant
	// pool identifier as u8 fields in a single word; anything larger falls
	// back to the general constant-vector form with a single element.  NOTE:
	// the pool identifier is stable across encoding passes (ConstantIndex
	// interns), so this choice cannot oscillate whilst the layout reaches a
	// fixpoint.
	if nv == 1 && skip <= math.MaxUint8 && !IsWideRegisters(b.Left.Base) {
		if cid := env.ConstantIndex(constants[0]); cid <= math.MaxUint8 {
			return encodeSkipIf_rc(util.Cast[uint8](skip), b.Left.Base, uint8(cid), op)
		}
	}
	//
	return encodeSkipIf_rcv(skip, b.Left, constants, op, env)
}

// incrementConstants increments a constant vector (most significant element
// first) by one, propagating any carry between elements.  It returns false
// when the increment overflows the vector as a whole (i.e. every element
// wraps), in which case the vector is left holding zero.  Carries between
// elements arise at the word bandwidth, which is sound because the register
// values compared against are themselves held at word bandwidth.
func incrementConstants[W word.Word[W]](constants []W) bool {
	for i := len(constants) - 1; i >= 0; i-- {
		var overflow bool
		//
		if constants[i], overflow = constants[i].Add64(1); !overflow {
			return true
		}
		// NOTE: ensure the wrapped element reads as zero, since Add64 makes
		// no guarantee about its result on overflow.
		constants[i] = constants[i].SetUint64(0)
	}
	//
	return false
}

// condOffset maps a (normalised) condition to its offset within a skip-if
// opcode family (SEQ, SNE, SLT, SGE).  GT/LE have no encoding — SkipIf
// normalises them away by swapping operands (or adjusting the constant) — so
// they panic here.
func condOffset(op Cond) uint32 {
	switch op {
	case bytecode.CONDITION_EQ:
		return 0
	case bytecode.CONDITION_NEQ:
		return 1
	case bytecode.CONDITION_LT:
		return 2
	case bytecode.CONDITION_GTEQ:
		return 3
	default:
		panic(fmt.Sprintf("condition %d has no skip-if encoding", op))
	}
}

// condFromOffset inverts condOffset, recovering the condition from an opcode's
// offset within its skip-if family.
func condFromOffset(offset uint32) Cond {
	switch offset {
	case 0:
		return bytecode.CONDITION_EQ
	case 1:
		return bytecode.CONDITION_NEQ
	case 2:
		return bytecode.CONDITION_LT
	case 3:
		return bytecode.CONDITION_GTEQ
	default:
		panic(fmt.Sprintf("invalid skip-if opcode offset %d", offset))
	}
}

// ============================================================================
// seq/sneq/slt,sleq,sgt,sgeq (skip conditional) instruction with (small)
// reg-reg operands.  Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  skip  |  rs0   |  rs1   | opcode |
// +--------+--------+--------+--------+
//
// Here, skip is a u8 identifying the number of instructions to skip, where the
// following instruction is considered to be at offset 0.  Likewise, rs0 and rs1
// are u8 source registers, whilst op identifies the operation.  The wide form
// extends the skip to a u16 in the upper half of the first word, moving the
// (now u16) registers into a subsequent word:
//
// +--------+--------+--------+--------+
// |      skip       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       rs0       |       rs1       |
// +-----------------+-----------------+
//
// The wide form is also selected when the registers are small but the skip
// exceeds a byte; a skip exceeding u16 falls back to the register-vector form
// (SXX_rv), which carries a u32 skip.
// ============================================================================

// encodeSkipIf_rr encodes a register-register conditional skip instruction,
// where op identifies the comparison.
func encodeSkipIf_rr(skip uint16, rs0, rs1 RegisterId, op Cond) []uint32 {
	// Forward branches are preferred as SKIP_IF instructions, whose offset is
	// unsigned and hence offers a greater forward range.
	if IsWideRegisters(rs0, rs1) || skip > math.MaxUint8 {
		return []uint32{
			uint32(skip)<<16 | (WIDE_SEQ_rr+condOffset(op))<<8 | WIDE,
			uint32(rs1) | uint32(rs0)<<16,
		}
	}
	//
	return []uint32{
		uint32(skip)<<24 | uint32(rs0)<<16 | uint32(rs1)<<8 | (SEQ_rr + condOffset(op)),
	}
}

// ============================================================================
// seq/sne/slt,sgt,sle,sge (skip conditional) instruction with (small) reg-reg
// operands.  This is the forward-branch encoding of a conditional jump, whose
// format matches the reg-reg form above except that offset is an unsigned u8
// relative offset, where the following instruction is considered to be at
// offset 0 (i.e. skip 0 transfers control to the next instruction).
// ============================================================================

// DecodeSkipIf_rr decodes the operands of a register-register conditional skip.
func DecodeSkipIf_rr(pc uint32, codes []uint32) (skip uint32, rs0, rs1 RegisterId, op Cond, n uint32) {
	if IsWideForm(pc, codes) {
		op = condFromOffset(((codes[pc] >> 8) & 0xff) - WIDE_SEQ_rr)
		skip = codes[pc] >> 16
		rs1 = RegisterId(codes[pc+1] & 0xffff)
		rs0 = RegisterId(codes[pc+1] >> 16)
		n = 2
	} else {
		op = condFromOffset((codes[pc] & OPCODE_MASK) - SEQ_rr)
		skip = codes[pc] >> 24
		rs1 = RegisterId((codes[pc] >> 8) & 0xff)
		rs0 = RegisterId((codes[pc] >> 16) & 0xff)
		n = 1
	}
	//
	return
}

// ============================================================================
// jeq/jneq/jlt,jleq,jgt,jgeq (jump conditional) instruction with vectored
// operands. Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   nv   |  rs0   |  rs1   | opcode |
// +--------+--------+--------+--------+
// | ............ target ............. |
// +--------+--------+--------+--------+
//
// Here, rs0 and rs1 are the base registers for the left and right vectors
// whilst nv is the vector length (which assumes both vectors have the same
// length).  Likewise, target is an absolute u32 target address.  The wide form
// keeps nv and target in place, moving the (now u16) base registers into a
// third word:
//
// +--------+--------+--------+--------+
// |   nv   |  n/a   |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// | ............ target ............. |
// +--------+--------+--------+--------+
// |       rs0       |       rs1       |
// +-----------------+-----------------+
// ============================================================================
// encodeSkipIf_rv encodes a register-vector conditional skip instruction.
func encodeSkipIf_rv(skip uint32, rs0, rs1 RegisterVector, op Cond) []uint32 {
	var nv = uint32(util.Cast[uint8](rs1.Len)) << 24
	// check core invariant
	if rs0.Len != rs1.Len {
		panic(fmt.Sprintf("mismatched length for source vectors (%d vs %d)", rs0.Len, rs1.Len))
	}
	//
	if IsWideRegisters(rs0.Base, rs1.Base) {
		return []uint32{
			nv | (WIDE_SEQ_rv+condOffset(op))<<8 | WIDE,
			skip,
			uint32(rs1.Base) | uint32(rs0.Base)<<16,
		}
	}
	//
	return []uint32{
		nv | uint32(rs0.Base)<<16 | uint32(rs1.Base)<<8 | (SEQ_rv + condOffset(op)),
		skip,
	}
}

// DecodeSkipIf_rv decodes the operands of a register-vector conditional branch.
func DecodeSkipIf_rv(pc uint32, codes []uint32) (skip uint32, rs0, rs1 RegisterVector, op Cond, n uint32) {
	var (
		rs0b, rs1b RegisterId
		nv         = RegisterId((codes[pc] >> 24) & 0xff)
	)
	//
	skip = codes[pc+1]
	//
	if IsWideForm(pc, codes) {
		op = condFromOffset(((codes[pc] >> 8) & 0xff) - WIDE_SEQ_rv)
		rs1b = RegisterId(codes[pc+2] & 0xffff)
		rs0b = RegisterId(codes[pc+2] >> 16)
		n = 3
	} else {
		op = condFromOffset((codes[pc] & OPCODE_MASK) - SEQ_rv)
		rs1b = RegisterId((codes[pc] >> 8) & 0xff)
		rs0b = RegisterId((codes[pc] >> 16) & 0xff)
		n = 2
	}
	//
	rs0 = RegisterVector{Base: rs0b, Len: nv}
	rs1 = RegisterVector{Base: rs1b, Len: nv}
	//
	return skip, rs0, rs1, op, n
}

// ============================================================================
// seq/sne/slt,sge (skip conditional) instruction with a single register and a
// single constant operand.  Format is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  skip  |  rs0   |  cid   | opcode |
// +--------+--------+--------+--------+
//
// Here, skip is a u8 identifying the number of instructions to skip (where the
// following instruction is considered to be at offset 0), rs0 is the u8 source
// register and cid is a u8 constant pool identifier for the constant operand.
// There is no wide form: a wide (u16) register, a skip exceeding u8, or a pool
// identifier exceeding u8 falls back to the general constant-vector form
// (SXX_rcv) with a single element.
// ============================================================================

// encodeSkipIf_rc encodes a register-constant conditional skip instruction,
// where op identifies the (normalised) comparison.
func encodeSkipIf_rc(skip uint8, rs0 RegisterId, cid uint8, op Cond) []uint32 {
	return []uint32{
		uint32(skip)<<24 | uint32(rs0)<<16 | uint32(cid)<<8 | (SEQ_rc + condOffset(op)),
	}
}

// DecodeSkipIf_rc decodes the operands of a register-constant conditional
// skip.
func DecodeSkipIf_rc[W word.Word[W]](pc uint32, codes []uint32, pool []W) (skip uint32, rs0 RegisterId,
	constant W, op Cond, n uint32) {
	//
	op = condFromOffset((codes[pc] & OPCODE_MASK) - SEQ_rc)
	skip = codes[pc] >> 24
	rs0 = RegisterId((codes[pc] >> 16) & 0xff)
	constant = pool[(codes[pc]>>8)&0xff]
	//
	return skip, rs0, constant, op, 1
}

// ============================================================================
// seq/sne/slt,sge (skip conditional) instruction with a register vector and a
// constant vector operand.  Format of the compact form is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   nv   |  rs0   |  n/a   | opcode |
// +--------+--------+--------+--------+
// |      skip       |  cid   |   cn   |
// +-----------------+--------+--------+
//
// Here, rs0 is the u8 base of the register vector, nv its length and skip an
// unsigned u16 relative offset (where the following instruction is considered
// to be at offset 0).  The constant vector is described by cid and cn: a u8
// base constant pool identifier and u8 length, such that element i of the
// vector (most significant first, mirroring the register vector layout)
// resides at pool index cid+i.  The general (wide) form serves as the
// fallback whenever the compact fields do not fit — a wide (u16) base
// register, or a base pool identifier exceeding u8.  It keeps the u16 skip in
// the first word, carrying the register vector (u16 base and length) in the
// second word and the constant vector (u16 base pool identifier and length)
// in the third:
//
// +--------+--------+--------+--------+
// |      skip       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       nv        |       rs0       |
// +-----------------+-----------------+
// |       cn        |       cid       |
// +-----------------+-----------------+
//
// There is no encoding for a skip exceeding u16 (as for SKIP, whose offset is
// bounded by its instruction format, encoding panics instead).
// ============================================================================

// encodeSkipIf_rcv encodes a register vector versus constant vector
// conditional skip instruction, where op identifies the (normalised)
// comparison.  The constants must carry exactly one element per register (see
// encodeSkipIfConstant, which zero-extends them).
func encodeSkipIf_rcv[W word.Word[W]](skip uint32, rs0 RegisterVector, constants []W, op Cond,
	env Environment[W]) []uint32 {
	//
	var (
		nv     = uint(util.Cast[uint8](rs0.Len))
		opcode = SEQ_rcv + condOffset(op)
	)
	// check core invariant
	if uint(len(constants)) != nv {
		panic(fmt.Sprintf("mismatched length for constant vector (%d vs %d)", len(constants), nv))
	}
	// Neither form can encode a skip exceeding u16.
	if skip > math.MaxUint16 {
		panic("skip exceeds conditional skip form")
	}
	// The compact form carries a u8 base pool identifier and u8 length
	// describing the constant vector as a consecutive run of pool entries;
	// anything larger falls back to the general (wide) form.  NOTE: pool
	// identifiers are stable across encoding passes (interning), so this
	// choice cannot oscillate whilst the layout reaches a fixpoint.
	if !IsWideRegisters(rs0.Base) {
		if cid := env.ConstantVectorIndex(constants); cid <= math.MaxUint8 {
			return []uint32{
				uint32(nv)<<24 | uint32(rs0.Base)<<16 | opcode,
				skip<<16 | uint32(cid)<<8 | uint32(nv),
			}
		}
	}
	// General form
	var cid = env.ConstantVectorIndex(constants)
	//
	return []uint32{
		skip<<16 | (WIDE_SEQ_rcv+condOffset(op))<<8 | WIDE,
		uint32(nv)<<16 | uint32(rs0.Base),
		uint32(nv)<<16 | uint32(cid),
	}
}

// DecodeSkipIf_rcv decodes the operands of a register vector versus constant
// vector conditional skip.
func DecodeSkipIf_rcv[W word.Word[W]](pc uint32, codes []uint32, pool []W) (skip uint32, rs0 RegisterVector,
	constants []W, op Cond, n uint32) {
	//
	// NOTE: in both forms, the constant vector is a consecutive run of pool
	// entries.
	if IsWideForm(pc, codes) {
		var (
			cid = codes[pc+2] & 0xffff
			cn  = codes[pc+2] >> 16
		)
		//
		op = condFromOffset(((codes[pc] >> 8) & 0xff) - WIDE_SEQ_rcv)
		skip = codes[pc] >> 16
		constants = pool[cid : cid+cn]
		rs0 = RegisterVector{
			Base: RegisterId(codes[pc+1] & 0xffff),
			Len:  RegisterId(codes[pc+1] >> 16),
		}
		//
		return skip, rs0, constants, op, 3
	}
	//
	var (
		nv    = codes[pc] >> 24
		word1 = codes[pc+1]
		cid   = (word1 >> 8) & 0xff
		cn    = word1 & 0xff
	)
	//
	op = condFromOffset((codes[pc] & OPCODE_MASK) - SEQ_rcv)
	skip = word1 >> 16
	constants = pool[cid : cid+cn]
	rs0 = RegisterVector{Base: RegisterId((codes[pc] >> 16) & 0xff), Len: util.Cast[uint16](uint(nv))}
	//
	return skip, rs0, constants, op, 2
}
