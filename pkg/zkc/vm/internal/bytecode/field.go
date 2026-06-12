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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FieldArith encodes a modular field-arithmetic operation, computing
// "target = sources[0] op ... op sources[n-1] op constant" reduced modulo the
// surrounding machine's prime characteristic.  The operation is identified by
// Op, which is one of ADDMOD_P, SUBMOD_P or MULMOD_P.  Unlike the integer Arith
// instruction the result always fits within a single (native) target register,
// so no cast check is ever required.
type FieldArith[W word.Word[W]] struct {
	// Op selects the operation (ADDMOD_P, SUBMOD_P or MULMOD_P).
	Op uint32
	// Target receives the result.
	Target Reg
	// Sources are the operand registers, with Sources[0] the leftmost operand.
	Sources []Reg
	// Constant is folded into the operation (the identity element when unused:
	// zero for ADDMOD_P / SUBMOD_P, one for MULMOD_P).
	Constant W
}

func (p *FieldArith[W]) String(mapping SystemMap) string {
	var (
		builder strings.Builder
		symbol  = fieldArithSymbol(p.Op)
		cz      = fieldArithUnusedConstant(p.Op, p.Constant)
	)
	//
	builder.WriteString(fieldArithPrefix(p.Op))
	builder.WriteString(" ")
	builder.WriteString(registerToString(p.Target, mapping))
	builder.WriteString(" = ")
	builder.WriteString(registersToString(p.Sources, mapping, symbol))
	// Append the constant operand unless it is the (elided) identity element.
	if !cz {
		if len(p.Sources) > 0 {
			builder.WriteString(symbol)
		}
		//
		fmt.Fprintf(&builder, "0x%s", p.Constant.Text(16))
	}
	//
	return builder.String()
}

// Codes implementation for Bytecode interface.
func (p *FieldArith[W]) Codes(_ uint32) []uint32 {
	return encodeFieldArith(p.Op, p.Target, p.Sources, p.Constant)
}

func decodeFieldArith[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		op                       = codes[pc] & OPCODE_MASK
		rd, sources, constant, n = decodeFieldArithOperands[W](pc, codes)
		srcs                     = OpIterToArray[uint16](sources)
	)
	//
	return &FieldArith[W]{op, rd, srcs, constant}, n
}

// decodeFieldArithOperands extracts the raw operands (target register, source
// register iterator, constant and instruction width) of a field-arithmetic
// instruction.  It is shared by the disassembler (decodeFieldArith) and the
// interpreter's executor.
func decodeFieldArithOperands[W word.Word[W]](pc uint32, codes []uint32) (
	rd Reg, sources Op8Iter, constant W, n uint32) {
	//
	var (
		nlimbs = (codes[pc] >> 16) & 0xff
		nsrc   = uint((codes[pc] >> 24) & 0xff)
		limb   W
	)
	//
	rd = Reg((codes[pc] >> 8) & 0xff)
	// Reconstruct the constant from its 32-bit limbs, most significant limb
	// first: each limb is shifted into the low bits of the accumulator in turn.
	for i := nlimbs; i > 0; i-- {
		limb = limb.SetUint64(uint64(codes[pc+i]))
		constant = constant.Shl64(32).Or(limb)
	}
	// Source registers follow the constant limbs.
	sources = NewOp8Iter(0, nsrc, codes[pc+1+nlimbs:])
	n = 1 + nlimbs + nCodesPackedSmall(nsrc)
	//
	return rd, sources, constant, n
}

// ============================================================================
// ADDMOD_P / SUBMOD_P / MULMOD_P instruction. Format of these instructions is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nsrc  |  ncon  |   rd   | opcode |
// +--------+--------+--------+--------+
// | constant limb 0 (least significant)|
// +------------------------------------+
// |                ...                 |
// +------------------------------------+
// | constant limb ncon-1 (most sig.)   |
// +------------------------------------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// Here, rd is a u8 destination register, ncon is the number of 32-bit constant
// limbs (carried inline, least significant first, as for LDC_w) and nsrc is the
// number of source registers (packed four-per-code after the constant).  The
// operation itself (add, subtract or multiply, modulo the prime) is identified
// by the opcode.  A field-sized constant can be wider than the 64 bits carried
// by the integer vector forms, hence the inline limb encoding.
// ============================================================================

func encodeFieldArith[W word.Word[W]](op uint32, rd Reg, sources []Reg, constant W) []uint32 {
	if rd >= 256 || len(sources) >= 256 {
		panic("wide field instructions not supported")
	}
	//
	var (
		// NOTE: big-endian byte ordering
		bytes  = constant.BigInt().Bytes()
		nlimbs = (len(bytes) + 3) / 4
	)
	//
	if nlimbs >= 256 {
		panic("wide field constants not supported")
	}
	//
	var (
		header = uint32(len(sources))<<24 | uint32(nlimbs)<<16 | uint32(rd)<<8 | op
		codes  = make([]uint32, nlimbs+1)
	)
	//
	codes[0] = header
	// Pack constant bytes into limbs, least significant limb first.
	for i, b := range bytes {
		var k = len(bytes) - 1 - i
		//
		codes[1+(k/4)] |= uint32(b) << (8 * (k % 4))
	}
	// Append packed source registers.
	return append(codes, packRegsIntoCodes(regsAsBytes(sources))...)
}

// fieldArithSymbol returns the infix operator used when disassembling a given
// field-arithmetic operation.
func fieldArithSymbol(op uint32) string {
	switch op {
	case ADDMOD_P:
		return " ⊕ "
	case SUBMOD_P:
		return " ⊖ "
	case MULMOD_P:
		return " ⊗ "
	default:
		panic("unknown field arithmetic operation")
	}
}

// fieldArithPrefix returns the mnemonic prefix used when disassembling a given
// field-arithmetic operation.
func fieldArithPrefix(op uint32) string {
	switch op {
	case ADDMOD_P:
		return "addmodp"
	case SUBMOD_P:
		return "submodp"
	case MULMOD_P:
		return "mulmodp"
	default:
		panic("unknown field arithmetic operation")
	}
}

// fieldArithUnusedConstant checks whether a given constant is the "identity
// element" for the operation, in which case it can be elided when
// disassembling.  For addition and subtraction this is zero, whilst for
// multiplication it is one.
func fieldArithUnusedConstant[W word.Word[W]](op uint32, constant W) bool {
	if op == MULMOD_P {
		return constant.Cmp64(1) == 0
	}
	//
	return constant.Cmp64(0) == 0
}

// NewFieldArith constructs a modular field-arithmetic bytecode for op (one of
// ADDMOD_P, SUBMOD_P or MULMOD_P), computing "target = sources[0] op ... op
// constant" modulo the machine's prime characteristic.
func NewFieldArith[W word.Word[W]](op uint32, target register.Id, sources []register.Id, constant W) *FieldArith[W] {
	return &FieldArith[W]{op, asReg(target), asRegs(sources...), constant}
}
