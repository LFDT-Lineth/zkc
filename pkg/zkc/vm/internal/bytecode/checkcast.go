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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// CheckCast instruction.
type CheckCast struct {
	Bitwidth uint16
	Target   Reg
}

// NewCheckCast constructs a new check cast instruction.
func NewCheckCast(target register.Id, bitwidth uint) *CheckCast {
	//
	if bitwidth > math.MaxUint16 {
		panic("register too large")
	}
	//
	return &CheckCast{
		uint16(bitwidth),
		asReg(target),
	}
}

func (p *CheckCast) String(mapping SystemMap) string {
	return fmt.Sprintf("check %s:u%d", registerToString(p.Target, mapping), p.Bitwidth)
}

// Codes implementation for Bytecode interface
func (p *CheckCast) Codes(_ uint32) []uint32 {
	var (
		rd       = uint32(p.Target) << 8
		bitwidth = uint32(p.Bitwidth) << 16
	)
	//
	return []uint32{
		bitwidth | rd | CHECKCAST,
	}
}

func decodeCheckCast(pc uint32, codes []uint32) (rd uint16, bitwidth uint16, n uint32) {
	rd = uint16((codes[pc] >> 8) & 0xff)
	bitwidth = uint16(codes[pc] >> 16)
	//
	return rd, bitwidth, 1
}
