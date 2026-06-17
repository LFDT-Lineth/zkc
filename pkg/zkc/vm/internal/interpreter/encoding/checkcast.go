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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
)

// CheckCast encodes a check-cast bytecode, which checks that the value held in
// the target register fits within the given bit width.
func CheckCast(p *bytecode.CheckCast) []uint32 {
	var (
		rd       = uint32(util.Cast[uint8](p.Target)) << 8
		bitwidth = uint32(p.Bitwidth) << 16
	)
	//
	return []uint32{
		bitwidth | rd | CHECKCAST,
	}
}

// DecodeCheckCast decodes a check-cast instruction, returning its register, bit width and instruction width.
func DecodeCheckCast(pc uint32, codes []uint32) (rd uint16, bitwidth uint16, n uint32) {
	rd = uint16((codes[pc] >> 8) & 0xff)
	bitwidth = uint16(codes[pc] >> 16)
	//
	return rd, bitwidth, 1
}
