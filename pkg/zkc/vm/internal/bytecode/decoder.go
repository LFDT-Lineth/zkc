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

import "github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"

func decode[W word.Word[W]](codes []uint32) (bytecodes []Bytecode) {
	var (
		ncodes = uint32(len(codes))
		offset uint32
	)
	//
	for offset < ncodes {
		bc, n := decodeBytecode[W](offset, codes[offset:])
		bytecodes = append(bytecodes, bc)
		offset += n
	}
	//
	return bytecodes
}

func decodeBytecode[W word.Word[W]](offset uint32, codes []uint32) (Bytecode, uint32) {
	var (
		code = codes[0]
	)
	//
	switch code & 0x1f {
	case ADD:
		return decodeAdd[W](codes)
	case FAIL:
		return &Fail{}, 1
	case JMP:
		c, n := decodeJmp1(offset, codes)
		return &c, n
	case JIF:
		return decodeJif(offset, codes)
	case RET:
		return decodeRet(codes)
	default:
		panic("unknown bytecode encountered")
	}
}
