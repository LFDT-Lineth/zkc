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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Ret (return from function call) instruction.
type Ret struct {
	// FrameWidth determines the number of registers in the corresponding
	// function's frame.  This many registers are popped from the stack when
	// this instruction executes.
	FrameWidth uint16
}

// NewRet constructs a new return instruction for a given frame width.
func NewRet(width uint) *Ret {
	if width > math.MaxUint16 {
		panic("invalid frame width")
	}
	//
	return &Ret{uint16(width)}
}

func (p *Ret) String(_ SystemMap) string {
	return fmt.Sprintf("ret %d", p.FrameWidth)
}

// Codes implementation for Bytecode interface
func (p *Ret) Codes(_ uint32) []uint32 {
	return encodeRet1(p.FrameWidth)
}

// Patch implementation for Bytecode interface
func (p *Ret) Patch(_ []Address) {
	// do nothing
}

func decodeRet[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	width := decodeRet1(codes[pc])
	//
	return &Ret{width}, 1
}

func decodeRet1(code uint32) (width uint16) {
	// RET stores frame width in bits 8..23.
	return uint16((code >> 8) & math.MaxUint16)
}

func encodeRet1(width uint16) []uint32 {
	var _width = uint32(width)

	return []uint32{
		_width<<8 | RET,
	}
}
