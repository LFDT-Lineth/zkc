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
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// SwitchCase is a single (value, target) entry of a multiway-skip dispatch table:
// when the source register holds Value, control transfers to Target.
type SwitchCase struct {
	// Value compared against the source register.
	Value uint64
	// Target branch address (a label index until resolved by Smw.Patch).
	Target Address
}

// Switch (skip multiway) dispatches on the value of a source register: it is
// compared, in order, against each case's value and, on the first match,
// control transfers to that case's target.  If no case matches, control falls
// through to the following instruction.
//
// Encoding (1 + 3*len(Cases) words):
//
//	31      24 23       8 7      0
//	+---------+----------+--------+
//	|  count  |  source  | opcode |   word 0
//	+---------+----------+--------+
//	| .......... value_lo .......|   word 1   (case 0)
//	| .......... value_hi .......|   word 2
//	| .......... target .........|   word 3
//	|             ...            |            (further cases)
//
// Here count is the number of cases (<= 255), source is the dispatch register,
// and each case occupies three words: a 64-bit value (low word then high word)
// followed by an absolute target address.
type Switch struct {
	Source Reg
	Cases  []SwitchCase
}

func (p *Switch) String(_ SystemMap) string {
	var b strings.Builder
	//
	fmt.Fprintf(&b, "switch r%d [", p.Source)
	//
	for i, c := range p.Cases {
		if i != 0 {
			b.WriteString(", ")
		}
		//
		fmt.Fprintf(&b, "%d->0x%08x", c.Value, c.Target)
	}
	//
	b.WriteString("]")
	//
	return b.String()
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *Switch) Clone() Patched {
	return &Switch{p.Source, slices.Clone(p.Cases)}
}

// Codes implementation for Bytecode interface.
func (p *Switch) Codes(_ uint32) []uint32 {
	if len(p.Cases) > 0xff {
		panic("too many cases in multiway skip")
	}
	//
	var codes = make([]uint32, 0, p.MaxWidth())
	// word 0: count | source | opcode
	codes = append(codes, uint32(len(p.Cases))<<24|uint32(p.Source)<<8|SKIP_M)
	// one (value_lo, value_hi, target) triple per case
	for _, c := range p.Cases {
		codes = append(codes, uint32(c.Value), uint32(c.Value>>32), c.Target)
	}
	//
	return codes
}

// Patch implementation for Patchable interface: resolve each case's target from
// a label index into a concrete address.
func (p *Switch) Patch(labels []Address) Patched {
	var ncases = make([]SwitchCase, len(p.Cases))
	//
	for i, c := range p.Cases {
		ncases[i] = SwitchCase{Value: c.Value, Target: labels[c.Target]}
	}
	//
	return &Switch{Source: p.Source, Cases: ncases}
}

// MaxWidth implementation for Patchable interface.  The width is fixed by the
// number of cases (it does not depend on where the targets resolve).
func (p *Switch) MaxWidth() uint32 {
	return 1 + 3*uint32(len(p.Cases))
}

// decodeSkipTable decodes an SMW instruction at the given program counter.
func decodeSkipTable[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		word0  = codes[pc]
		count  = (word0 >> 24) & 0xff
		source = Reg((word0 >> 8) & 0xffff)
		cases  = make([]SwitchCase, count)
	)
	//
	for i := range count {
		var base = pc + 1 + (i * 3)
		//
		cases[i] = SwitchCase{
			Value:  uint64(codes[base]) | (uint64(codes[base+1]) << 32),
			Target: codes[base+2],
		}
	}
	//
	return &Switch{Source: source, Cases: cases}, 1 + (3 * count)
}

// executeSkipTable implements the SMW dispatch: the source register is
// compared against each case value and, on the first match, control transfers
// to that case's (absolute) target; otherwise control falls through past the
// whole instruction to the following one.
func executeSkipTable[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		word0  = codes[pc]
		count  = (word0 >> 24) & 0xff
		source = (word0 >> 8) & 0xffff
		val    = stack[source]
	)
	//
	for i := range count {
		var (
			base  = pc + 1 + (i * 3)
			value = uint64(codes[base]) | (uint64(codes[base+1]) << 32)
		)
		//
		if val.Cmp64(value) == 0 {
			return codes[base+2]
		}
	}
	// no match: fall through past the whole instruction
	return pc + 1 + (3 * count)
}
