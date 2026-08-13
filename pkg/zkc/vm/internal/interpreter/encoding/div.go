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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DivRem encodes a combined division/remainder bytecode, writing both the
// quotient and the remainder.  A constant divisor selects the constant form
// (DIVMODC) instead.
func DivRem[W word.Word[W]](p *bytecode.DivRem[W], env Environment[W]) []uint32 {
	if p.Divisor.IsConstant() {
		return encodeDivMod_C(p.Quotient, p.Remainder, p.Dividend, p.Divisor.AsConstant(), env)
	}
	//
	return encodeDivMod(p.Quotient, p.Remainder, p.Dividend, p.Divisor.AsRegister())
}

// Intrinsic encodes an intrinsic bytecode (e.g. DIV_HINT, which supplies the
// prover with the quotient, remainder and witness for a division, or WIDE_SHL).
func Intrinsic[W word.Word[W]](p *bytecode.Intrinsic[W], env Environment[W]) []uint32 {
	return encodeIntrinsic(p.Op, p.Targets, p.Sources, env)
}

// ============================================================================
// DIVMOD instruction, computing both the quotient (rq) and remainder (rr) of
// dividend / divisor.  Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | divisor|dividend|   rq   | opcode |
// +--------+--------+--------+--------+
// |                          |   rr   |
// +--------------------------+--------+
//
// The wide form carries the (now u16) quotient register in the first word,
// with the remaining registers in a second and third:
//
// +--------+--------+--------+--------+
// |        rq       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |     divisor     |    dividend     |
// +-----------------+-----------------+
// |                 |       rr        |
// +-----------------+-----------------+
// ============================================================================

// encodeDivMod encodes a combined division/remainder instruction with a
// register divisor.
func encodeDivMod(rq, rr, dividend, divisor RegisterId) []uint32 {
	if IsWideRegisters(rq, rr, dividend, divisor) {
		return []uint32{
			uint32(rq)<<16 | WIDE_DIVMOD<<8 | WIDE,
			uint32(dividend) | uint32(divisor)<<16,
			uint32(rr),
		}
	}
	//
	return []uint32{
		uint32(divisor)<<24 | uint32(dividend)<<16 | uint32(rq)<<8 | DIVMOD,
		uint32(rr),
	}
}

// DecodeDivMod decodes the operands of a combined division/remainder
// instruction with a register divisor.
func DecodeDivMod(pc uint32, codes []uint32) (rq, rr, dividend, divisor RegisterId, n uint32) {
	if IsWideForm(pc, codes) {
		rq = RegisterId(codes[pc] >> 16)
		dividend = RegisterId(codes[pc+1] & 0xffff)
		divisor = RegisterId(codes[pc+1] >> 16)
		rr = RegisterId(codes[pc+2] & 0xffff)
		//
		return rq, rr, dividend, divisor, 3
	}
	//
	rq = RegisterId((codes[pc] >> 8) & 0xff)
	dividend = RegisterId((codes[pc] >> 16) & 0xff)
	divisor = RegisterId((codes[pc] >> 24) & 0xff)
	rr = RegisterId(codes[pc+1] & 0xff)
	//
	return rq, rr, dividend, divisor, 2
}

// ============================================================================
// DIVMODC (constant divisor) instruction.  The first word mirrors the
// arithmetic-with-constant family (see encodeArith_1n1c), with rs holding the
// dividend and imm8 the small constant divisor; the remainder register
// follows in a second word:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  imm8  |   rs   |   rq   | opcode |
// +--------+--------+--------+--------+
// |                          |   rr   |
// +--------------------------+--------+
//
// The wide form replaces the constant operand with a u16 constant pool
// identifier, moving the (now u16) registers into subsequent words:
//
// +--------+--------+--------+--------+
// |       cid       |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |       rs        |        rq       |
// +-----------------+-----------------+
// |                 |       rr        |
// +-----------------+-----------------+
//
// The wide form is also selected when the registers are small but the
// constant exceeds a byte.
// ============================================================================

// encodeDivMod_C encodes a combined division/remainder instruction with a
// constant divisor.
func encodeDivMod_C[W word.Word[W]](rq, rr, dividend RegisterId, constant W,
	env Environment[W]) []uint32 {
	//
	if IsWideRegisters(rq, rr, dividend) || constant.Cmp64(0xff) > 0 {
		return []uint32{
			uint32(env.ConstantIndex(constant))<<16 | WIDE_DIVMODC<<8 | WIDE,
			uint32(rq) | uint32(dividend)<<16,
			uint32(rr),
		}
	}
	//
	return []uint32{
		uint32(constant.Uint64())<<24 | uint32(dividend)<<16 | uint32(rq)<<8 | DIVMODC,
		uint32(rr),
	}
}

// DecodeDivMod_C decodes the operands of a combined division/remainder
// instruction with a constant divisor, resolving the constant against the
// given pool.
func DecodeDivMod_C[W word.Word[W]](pc uint32, codes []uint32, pool []W,
) (rq, rr, dividend RegisterId, constant W, n uint32) {
	//
	if IsWideForm(pc, codes) {
		rq = RegisterId(codes[pc+1] & 0xffff)
		dividend = RegisterId(codes[pc+1] >> 16)
		rr = RegisterId(codes[pc+2] & 0xffff)
		constant = pool[codes[pc]>>16]
		//
		return rq, rr, dividend, constant, 3
	}
	//
	rq = RegisterId((codes[pc] >> 8) & 0xff)
	dividend = RegisterId((codes[pc] >> 16) & 0xff)
	rr = RegisterId(codes[pc+1] & 0xff)
	constant = constant.SetUint64(uint64((codes[pc] >> 24) & 0xff))
	//
	return rq, rr, dividend, constant, 2
}

// ============================================================================
// INTRINSIC instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  |  ntgt  | opcode |
// +--------+--------+--------+--------+
// | ... packed tgt/src reg vectors .. |
// +-----------------------------------+
//
// Here, op selects the hint operation (e.g. DIV_HINT), whilst ntgt and nsrc give
// the number of target (return) and source (argument) register vectors
// respectively.  Each vector is packed as a (base, len) byte pair, targets
// first.  The wide form moves the target count into the first packed slot
// (leaving bits 8-15 of the first word clear, as for all wide forms), with
// each vector following as a (base, len) pair of u16 operands:
//
// +--------+--------+--------+--------+
// |   op   |  nsrc  |  wop   |  WIDE  |
// +--------+--------+--------+--------+
// |    tgt0 base    |      ntgt       |
// +-----------------+-----------------+
// |    tgt1 base    |    tgt0 len     |
// +-----------------+-----------------+
// | ... packed tgt/src reg vectors .. |
// +-----------------------------------+
// ============================================================================

// encodeIntrinsic encodes a hint instruction, where op selects the operation
// and the target (return) register vectors and source (argument) operands are
// packed as (base, len) pairs, targets first.  A constant source is packed as
// a flagged pair whose base is a constant pool index and whose length is the
// number of pooled limbs (see Operands.NextOperand); the flag occupies the top
// bit of the length, so the wide form is forced whenever any length (or a
// pool index) would collide with the narrow (u8) representation.
func encodeIntrinsic[W word.Word[W]](op Operation, targets []RegisterVector,
	sources []bytecode.Operand[W], env Environment[W]) []uint32 {
	//
	if len(targets) == 0 || len(sources) == 0 || len(targets) >= 256 || len(sources) >= 256 {
		panic("hint instruction operand counts not supported")
	}
	//
	var (
		// Sources resolved into (base, len) pairs, with constants interned in
		// the constant pool.  Note that the constant flag only applies to
		// source lengths: targets are decoded as plain register vectors, so
		// they need no flag headroom.
		bases  = make([]uint16, len(sources))
		lens   = make([]uint16, len(sources))
		consts = make([]bool, len(sources))
		wide   = IsWideRegisterVectors(targets)
	)
	//
	for i, s := range sources {
		if s.IsConstant() {
			var limbs = s.AsConstants()
			// Constant operands are a single value by construction (see
			// split.Operand): limb widths are not recorded, so a multi-limb
			// run could not be reconstructed by the interpreter.
			// Intrinsic.Validate rejects offenders up front.
			if len(limbs) != 1 {
				panic("multi-limb constant operand")
			}
			//
			bases[i] = env.ConstantIndex(limbs[0])
			lens[i] = 1
			consts[i] = true
		} else {
			var v = s.AsRegisterVector()
			//
			bases[i] = v.Base
			lens[i] = v.Len
			// Lengths carry the constant flag in their top bit, so they must
			// stay strictly below it in either form.
			if lens[i] >= 0x8000 {
				panic("hint operand length collides with constant flag")
			}
		}
		//
		wide = wide || bases[i] > 0xff || lens[i] >= 0x80
	}
	//
	var (
		nop  = uint32(op) << 24
		nsrc = uint32(len(sources)) << 16
		ntgt = uint32(len(targets)) << 8
	)
	//
	if wide {
		var (
			codes = []uint32{nop | nsrc | WIDE_INTRINSIC<<8 | WIDE}
			// ntgt occupies the first packed slot, followed by the vectors.
			shorts = make([]uint16, 0, 1+2*(len(targets)+len(sources)))
		)
		//
		shorts = append(shorts, uint16(len(targets)))
		shorts = append(shorts, RegisterVectorsAsShorts(targets)...)
		//
		for i := range sources {
			var l = lens[i]
			//
			if consts[i] {
				l |= 0x8000
			}
			//
			shorts = append(shorts, bases[i], l)
		}
		//
		return append(codes, PackShortsIntoCodes(shorts)...)
	}
	//
	var (
		codes = []uint32{nop | nsrc | ntgt | INTRINSIC}
		bytes = RegisterVectorsAsBytes(targets)
	)
	//
	for i := range sources {
		var l = uint8(lens[i])
		//
		if consts[i] {
			l |= 0x80
		}
		//
		bytes = append(bytes, uint8(bases[i]), l)
	}
	//
	return append(codes, PackBytesIntoCodes(bytes)...)
}

// DecodeIntrinsicOperands decodes the operation selector along with the target and
// source operands of a hint instruction.  Each vector is packed as a (base,
// len) byte pair, hence the iterators range over twice the vector counts.
func DecodeIntrinsicOperands(pc uint32, codes []uint32) (op Operation, targets, sources Operands, n uint32) {
	var nsources = uint((codes[pc] >> 16) & 0xff)
	//
	op = Operation((codes[pc] >> 24) & 0xff)
	//
	if IsWideForm(pc, codes) {
		var ntargets = uint(codes[pc+1] & 0xffff)
		//
		targets = NewWideOperands(1, 2*ntargets, codes[pc+1:])
		sources = NewWideOperands(1+(2*ntargets), 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedWide(1+2*(ntargets+nsources))
	} else {
		var ntargets = uint((codes[pc] >> 8) & 0xff)
		//
		targets = NewOperands(0, 2*ntargets, codes[pc+1:])
		sources = NewOperands(2*ntargets, 2*nsources, codes[pc+1:])
		n = 1 + NumCodesPackedSmall(2*(ntargets+nsources))
	}
	//
	return
}
