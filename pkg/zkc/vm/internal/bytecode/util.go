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
)

func registersToString(registers []Reg, mapping SystemMap, separator string) string {
	var builder strings.Builder
	//
	for i, r := range registers {
		if i != 0 {
			builder.WriteString(separator)
		}
		//
		builder.WriteString(registerToString(r, mapping))
	}
	//
	return builder.String()
}

func registerVectorToString(reg RegVec, mapping SystemMap) string {
	var (
		first = registerToString(reg.Base, mapping)
	)
	switch reg.Len {
	case 1:
		return first
	case 2:
		var second = registerToString(reg.Base+1, mapping)
		return fmt.Sprintf("%s;%s", first, second)
	default:
		var last = registerToString(reg.Base+reg.Len-1, mapping)
		return fmt.Sprintf("%s;,,;%s", first, last)
	}
}

func registerToString(reg Reg, mapping SystemMap) string {
	if mapping == nil {
		return fmt.Sprintf("?%d", reg)
	}
	//
	return mapping.Register(register.NewId(uint(reg))).Name()
}

func getBranchTarget(offset uint32, relOffset uint32, width uint) Address {
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

func getRelativeOffset(pc uint32, target Address, width uint) (roff uint32, ok bool) {
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

// Pack a given array of bytes into an array of codes, such that the last code
// is padded with 0xff.
func packRegsIntoCodes(bytes []byte) []uint32 {
	var (
		nBytes = uint32(len(bytes))
		ncodes = nCodesPackedSmall(uint(nBytes))
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

func nCodesPackedSmall(n uint) uint32 {
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
