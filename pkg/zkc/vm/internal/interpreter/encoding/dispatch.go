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
// |          case 0: bit              |
// +-----------------------------------+
// |          case 0: target           |
// +-----------------------------------+
// |                ...                |
// +-----------------------------------+
// |       case count-1: bit           |
// +-----------------------------------+
// |       case count-1: target        |
// +-----------------------------------+
//
// Here, default identifies the (1-bit) register holding the sum of the case
// bits, and count gives the number of (bit, target) cases which follow.  Each
// case is encoded as a bit register identifier followed by an absolute u32
// target address.  Control transfers to the target of the first case whose bit
// register is set; when no bit is set, control falls through past the whole
// instruction.
// ============================================================================

// Dispatch encodes a one-hot dispatch bytecode, which dispatches over a table
// of (bit register, target) pairs.
func Dispatch[W word.Word[W]](p *bytecode.Dispatch[W], env Environment[W]) []uint32 {
	// count occupies a single byte (bits 24..31) of word 0.
	if len(p.Cases) > math.MaxUint8 {
		panic("too many cases in one-hot dispatch")
	}
	// word 0: count | default | opcode
	var codes = []uint32{uint32(len(p.Cases))<<24 | uint32(p.Default)<<8 | SKIP_B}
	// Append one (bit, target) pair per case.
	for _, c := range p.Cases {
		var (
			// Resolve this case's target program point, ...
			target = env.Point().Skip(uint(c.Skip))
			// ... then its absolute offset in the encoded sequence (SKIP_B
			// targets are absolute, unlike the relative SKIP/JMP forms).
			offset = env.OffsetFor(env.enclosing, target)
		)
		//
		codes = append(codes, uint32(c.Bit), offset)
	}
	//
	return codes
}
