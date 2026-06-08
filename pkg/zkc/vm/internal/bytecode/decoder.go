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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Decode an encoded bytecode program into a sequence of bytecodes.
func Decode[W word.Word[W]](p Program) (bytecodes []Bytecode[W]) {
	var (
		ncodes = uint32(len(p.bytecodes))
		offset uint32
	)
	//
	for offset < ncodes {
		bc, n := decodeBytecode[W](offset, p.bytecodes)
		bytecodes = append(bytecodes, bc)
		offset += n
	}
	//
	return bytecodes
}

func decodeBytecode[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		code = codes[pc]
	)
	//
	switch code & OPCODE_MASK {
	case ADD, LDC, MOVE:
		return decodeAdd[W](pc, codes)
	case FAIL:
		return &Fail{}, 1
	case JMP:
		target, n := decodeJmp1(pc, codes)
		return &Jmp{target}, n
	case JEQ_RR, JNE_RR, JLT_RR, JLE_RR, JGT_RR, JGE_RR:
		return decodeJif[W](pc, codes)
	case RD_ROM_N_M, WR_WOM_N_M, RD_RAM_N_M, WR_RAM_N_M:
		return decodeReadWrite[W](pc, codes)
	case RET:
		return decodeRet[W](pc, codes)
	default:
		panic("unknown bytecode encountered")
	}
}
