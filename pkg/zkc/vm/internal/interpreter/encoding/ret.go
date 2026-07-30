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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Ret encodes a return bytecode, emitting the enclosing function's frame width
// and the offset of its return registers within the frame.
func Ret[W word.Word[W]](p *bytecode.Ret[W], env Environment[W]) []uint32 {
	var (
		module = env.Module(env.enclosing)
		// Extract frame width
		width = util.Cast[uint16](module.Width())
		//
		offset = module.NumInputs()
	)
	//
	return encodeRet1(width, uint32(offset))
}

// ============================================================================
// RET.  Format of these instruction is:
//
//	31                                0
//
// +--------+-----------------+--------+
// | offset |   frame width   | opcode |
// +--------+-----------------+--------+
//
// Here, offset is the u8 offset of the return registers within the frame.  The
// wide form moves the frame width up a byte (leaving bits 8-15 clear, as for
// all wide forms) and the (now wider) offset into a subsequent word:
//
// +-----------------+--------+--------+
// |   frame width   |  n/a   | opcode |
// +-----------------+--------+--------+
// | ............ offset ............. |
// +-----------------------------------+
// ============================================================================

// DecodeRet1 decodes the operands of a return instruction.
func DecodeRet1(pc uint32, codes []uint32) (width uint16, roffset uint32, n uint32) {
	if IsWideForm(pc, codes) {
		width = uint16(codes[pc] >> 16)
		roffset = codes[pc+1]
		//
		return width, roffset, 2
	}
	//
	width = uint16((codes[pc] >> 8) & 0xffff)
	roffset = codes[pc] >> 24
	//
	return width, roffset, 1
}

// encodeRet1 encodes a return instruction with the given frame width and return
// offset.
func encodeRet1(width uint16, roffset uint32) []uint32 {
	var _width = uint32(width)
	//
	if roffset > math.MaxUint8 {
		return []uint32{
			_width<<16 | RET | WIDE,
			roffset,
		}
	}
	//
	return []uint32{
		roffset<<24 | _width<<8 | RET,
	}
}
