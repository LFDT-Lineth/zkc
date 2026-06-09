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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Not computes a bitwise complement within the given bit width.
type Not struct {
	// Bitwidth bounds the complement mask.
	Bitwidth uint8
	// Target receives the complemented value.
	Target Reg
	// Source provides the value to complement.
	Source Reg
}

func (p *Not) String(mapping SystemMap) string {
	var (
		target = registerToString(p.Target, mapping)
		source = registerToString(p.Source, mapping)
	)
	//
	return fmt.Sprintf("%s = ~%s [u%d]", target, source, p.Bitwidth)
}

// Codes implementation for Bytecode interface.
func (p *Not) Codes(_ uint32) []uint32 {
	return encodeNot(p.Target, p.Source, p.Bitwidth)
}

func decodeNot[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		target, source, bitwidth, n = decodeNot_1n1(pc, codes)
	)
	//
	return &Not{bitwidth, target, source}, n
}

// ============================================================================
// NOT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | width  |   rs   |   rd   | opcode |
// +--------+--------+--------+--------+
//
// This intentionally stays small; wide bit widths can be added when needed.
// ============================================================================

func encodeNot(rd, rs Reg, bitwidth uint8) []uint32 {
	if rd >= 256 || rs >= 256 {
		panic("wide not instructions not supported")
	}
	//
	return []uint32{uint32(bitwidth)<<24 | uint32(rs)<<16 | uint32(rd)<<8 | NOT}
}

func decodeNot_1n1(pc uint32, codes []uint32) (rd, rs Reg, bitwidth uint8, n uint32) {
	rd = Reg((codes[pc] >> 8) & 0xff)
	rs = Reg((codes[pc] >> 16) & 0xff)
	bitwidth = uint8((codes[pc] >> 24) & 0xff)
	//
	return rd, rs, bitwidth, 1
}

// NewNot constructs a bitwise-not bytecode.
func NewNot(target, source register.Id, bitwidth uint) *Not {
	if bitwidth >= 256 {
		panic("wide not instructions not supported")
	}
	//
	return &Not{uint8(bitwidth), asReg(target), asReg(source)}
}
