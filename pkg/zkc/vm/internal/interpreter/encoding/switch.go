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
// |  case 0: skip   |  case 0: cid    |
// +-----------------+-----------------+
// |                ...                |
// +-----------------+-----------------+
// |  count-1: skip  |  count-1: cid   |
// +-----------------+-----------------+
//
// Here, source identifies the register being switched upon and count gives the
// number of cases which follow, one word each.  Each case is encoded as a u16
// constant pool identifier for its value together with an unsigned u16
// relative skip offset (where the following instruction is considered to be
// at offset 0, as for the other skip forms).  Control skips forward to the
// target of the first case whose value matches the source register.  There is
// no encoding for a skip exceeding u16 (encoding panics instead).
// ============================================================================

// Switch encodes a multiway-skip (switch) bytecode, which dispatches on a source
// register against a table of (value, skip) pairs.
func Switch[W word.Word[W]](pc Address, p *bytecode.Switch[W], env Environment[W]) []uint32 {
	// count occupies a single byte (bits 24..31) of word 0.
	if len(p.Cases) > math.MaxUint8 {
		panic("too many cases in multiway skip")
	}
	// word 0: count | source | opcode
	var codes = []uint32{uint32(len(p.Cases))<<24 | uint32(p.Source)<<8 | SKIP_M}
	// Append one (skip, cid) word per case.
	for _, c := range p.Cases {
		var (
			// Resolve this case's target program point, ...
			target = env.Point().Skip(uint(c.Skip))
			// ... then its relative offset from the following instruction.
			skip = env.OffsetFor(env.enclosing, target) - (pc + 1)
			//
			cid = env.ConstantIndex(c.Value)
		)
		//
		if skip > math.MaxUint16 {
			panic("skip exceeds multiway skip form")
		}
		//
		codes = append(codes, skip<<16|uint32(cid))
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
	// 	var word = codes[pc+1+i]
	// 	//
	// 	cases[i] = bytecode.SwitchCase{
	// 		Value: pool[word&0xffff],
	// 		Skip:  word >> 16,
	// 	}
	// }
	// //
	// return &bytecode.Switch{Source: source, Cases: cases}, 1 + count
	panic("not efficient")
}
