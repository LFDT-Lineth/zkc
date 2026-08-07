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
// SKIP_B (skip bit-table / one-hot dispatch) instruction.  Format of this
// instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  count |    default      | opcode |
// +--------+--------+--------+--------+
// |  case 0: skip   |  case 0: bit    |
// +-----------------+-----------------+
// |                ...                |
// +-----------------+-----------------+
// |  count-1: skip  |  count-1: bit   |
// +-----------------+-----------------+
//
// Here, default identifies the (1-bit) register holding the sum of the case
// bits, and count gives the number of cases which follow, one word each.  Each
// case is encoded as a u16 bit register identifier together with an unsigned
// u16 relative skip offset (where the following instruction is considered to
// be at offset 0, as for the other skip forms).  Control skips forward to the
// target of the first case whose bit register is set; when no bit is set,
// control falls through past the whole instruction.  There is no encoding for
// a skip exceeding u16 (encoding panics instead).
// ============================================================================

// Dispatch encodes a one-hot dispatch bytecode, which dispatches over a table
// of (bit register, skip) pairs.
func Dispatch[W word.Word[W]](pc Address, p *bytecode.Dispatch[W], env Environment[W]) []uint32 {
	// count occupies a single byte (bits 24..31) of word 0.
	if len(p.Cases) > math.MaxUint8 {
		panic("too many cases in one-hot dispatch")
	}
	// word 0: count | default | opcode
	var codes = []uint32{uint32(len(p.Cases))<<24 | uint32(p.Default)<<8 | SKIP_B}
	// Append one (skip, bit) word per case.
	for _, c := range p.Cases {
		var (
			// Resolve this case's target program point, ...
			target = env.Point().Skip(uint(c.Skip))
			// ... then its relative offset from the following instruction.
			skip = env.OffsetFor(env.enclosing, target) - (pc + 1)
		)
		//
		if skip > math.MaxUint16 {
			panic("skip exceeds one-hot dispatch form")
		}
		//
		codes = append(codes, skip<<16|uint32(c.Bit))
	}
	//
	return codes
}
