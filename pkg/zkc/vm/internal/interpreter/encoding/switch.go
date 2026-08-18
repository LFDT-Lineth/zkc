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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ============================================================================
// SKIP_M (skip table / switch) instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  count |  cid   | source | opcode |
// +--------+--------+--------+--------+
// |  case 1: skip   |  case 0: skip   |
// +-----------------+-----------------+
// |                ...                |
// +-----------------+-----------------+
//
// Here, source is the u8 register being switched upon and count gives the
// number of cases which follow.  Each case's dispatch value is held in the
// constant pool as a consecutive run of count entries starting at the u8
// base pool identifier cid (i.e. case i's value resides at pool index
// cid+i), interned as a constant vector via ConstantVectorIndex, in
// ascending order (allowing the interpreter to dispatch via binary search
// rather than a linear scan).  Each case also carries an
// unsigned u16 relative skip offset (where the following instruction is
// considered to be at offset 0, as for the other skip forms), packed two
// per word (least significant half first, in the same (sorted) case
// order as the constant vector).  Control skips forward to the target of
// the case whose pooled value matches the source register; if none
// match, control falls through past the whole instruction.  There is no
// wide encoding yet for a source register or base pool identifier
// exceeding u8, or a skip exceeding u16 (encoding panics instead).
// ============================================================================

// Switch encodes a multiway-skip (switch) bytecode, which dispatches on a source
// register against a table of (value, skip) pairs.
func Switch[W word.Word[W]](pc Address, p *bytecode.Switch[W], env Environment[W]) []uint32 {
	// count occupies a single byte (bits 24..31) of word 0.
	if len(p.Cases) > math.MaxUint8 || IsWideRegisters(p.Source) {
		panic("support wide multiway skip")
	}
	// Sort a copy of the cases by dispatch value, so that the encoded
	// constant vector is in ascending order and the interpreter can binary
	// search it.  NOTE: dispatch values are guaranteed unique (see
	// Switch.Validate), so this order is well-defined and stable across the
	// repeated encoding passes taken to reach a layout fixpoint.  A copy is
	// sorted (rather than p.Cases in place) since the same bytecode value is
	// re-encoded across those passes.
	var cases = slices.Clone(p.Cases)
	slices.SortFunc(cases, func(a, b bytecode.SwitchCase[W]) int {
		return a.Value.Cmp(b.Value)
	})
	//
	var (
		skips     = make([]uint16, len(cases))
		constants = make([]W, len(cases))
	)
	//
	for i, c := range cases {
		var (
			// Resolve this case's target program point, ...
			target = env.Point().Skip(uint(c.Skip))
			// ... then its relative offset from the following instruction.
			skip = env.OffsetFor(env.enclosing, target) - (pc + 1)
		)
		//
		if skip > math.MaxUint16 {
			panic("support wide multiway skip")
		}
		//
		skips[i] = uint16(skip)
		constants[i] = c.Value
	}
	// Intern the (sorted) case values as a single consecutive run in the
	// constant pool.
	var cid = env.ConstantVectorIndex(constants)
	// NOTE: no wide form yet for a base pool identifier exceeding u8.
	if cid > math.MaxUint8 {
		panic("support wide multiway skip")
	}
	// word 0: count | cid | source | opcode
	var codes = []uint32{uint32(len(cases))<<24 | uint32(cid)<<16 | uint32(p.Source)<<8 | SKIP_M}
	//
	return append(codes, PackShortsIntoCodes(skips)...)
}

// DecodeSkipTable decodes an SMW instruction at the given program counter.
func DecodeSkipTable[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	panic("not efficient")
}
