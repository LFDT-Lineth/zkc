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

import "github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"

func decode(codes []uint32) (bytecodes []Bytecode) {
	var (
		ncodes = uint(len(codes))
		offset uint
	)
	//
	for offset < ncodes {
		bc, n := decodeBytecode(offset, codes[offset:])
		bytecodes = append(bytecodes, bc)
		offset += n
	}
	//
	return bytecodes
}

func decodeBytecode(offset uint, codes []uint32) (Bytecode, uint) {
	var (
		code = codes[0]
	)
	//
	switch code & 0x1f {
	case FAIL:
		return &Fail{}, 1
	case JMP:
		return decodeJmp(offset, codes)
	case JIF:
		return decodeJif(offset, codes)
	case RET:
		return &Ret{}, 1
	default:
		panic("unknown bytecode encountered")
	}
}

func decodeJmp(offset uint, codes []uint32) (Bytecode, uint) {
	var target = getBranchTarget(offset, uint(codes[0]>>8), 24)
	return &Jmp{target}, 1
}

func decodeJif(offset uint, codes []uint32) (Bytecode, uint) {
	var (
		op     = opcode.Condition((codes[0] >> 5) & 0x3)
		rs1    = uint((codes[0] >> 8) & 0xff)
		rs0    = uint((codes[0] >> 16) & 0xff)
		target = getBranchTarget(offset, uint(codes[0]>>24), 8)
	)
	//
	return &Jif{op, rs1, rs0, target}, 1
}
