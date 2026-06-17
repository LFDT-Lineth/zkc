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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ============================================================================
// SKIP_M (skip table / switch) instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  count |     source      | opcode |
// +--------+--------+--------+--------+
// |      case 0: value (low 32)       |
// +-----------------------------------+
// |      case 0: value (high 32)      |
// +-----------------------------------+
// |          case 0: target           |
// +-----------------------------------+
// |                ...                |
// +-----------------------------------+
// |   case count-1: value (low 32)    |
// +-----------------------------------+
// |   case count-1: value (high 32)   |
// +-----------------------------------+
// |       case count-1: target        |
// +-----------------------------------+
//
// Here, source identifies the register being switched upon and count gives the
// number of (value, target) cases which follow.  Each case is encoded as a
// 64-bit value (least-significant limb first) followed by an absolute u32 target
// address.  Control transfers to the target of the first case whose value
// matches the source register.
// ============================================================================

// Switch encodes a multiway-skip (switch) bytecode, which dispatches on a source
// register against a table of (value, target) pairs.
func Switch[W word.Word[W]](p *bytecode.Switch[W], env Environment[W]) []uint32 {
	// count occupies a single byte (bits 24..31) of word 0.
	if len(p.Cases) > math.MaxUint8 {
		panic("too many cases in multiway skip")
	}
	// word 0: count | source | opcode
	var codes = []uint32{uint32(len(p.Cases))<<24 | uint32(p.Source)<<8 | SKIP_M}
	// Append one (value_lo, value_hi, target) triple per case.
	for _, c := range p.Cases {
		var (
			// Resolve this case's target program point, ...
			target = env.Point().Skip(uint(c.Skip))
			// ... then its absolute offset in the encoded sequence (SKIP_M
			// targets are absolute, unlike the relative SKIP/JMP forms).
			offset = env.OffsetFor(env.enclosing, target)
		)
		// sanity check (for now)
		if c.Value.Cmp64(^uint64(0)) > 0 {
			panic("wide switch constants not supported")
		}
		// Convert into uint64
		value := c.Value.Uint64()
		//
		codes = append(codes, uint32(value), uint32(value>>32), offset)
	}
	//
	return codes
}

// DecodeSkipTable decodes an SMW instruction at the given program counter.
func DecodeSkipTable[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	// var (
	// 	word0  = codes[pc]
	// 	count  = (word0 >> 24) & 0xff
	// 	source = Reg((word0 >> 8) & 0xffff)
	// 	cases  = make([]bytecode.SwitchCase, count)
	// )
	// //
	// for i := range count {
	// 	var base = pc + 1 + (i * 3)
	// 	//
	// 	cases[i] = bytecode.SwitchCase{
	// 		Value:  uint64(codes[base]) | (uint64(codes[base+1]) << 32),
	// 		Target: codes[base+2],
	// 	}
	// }
	// //
	// return &bytecode.Switch{Source: source, Cases: cases}, 1 + (3 * count)
	panic("not efficient")
}
