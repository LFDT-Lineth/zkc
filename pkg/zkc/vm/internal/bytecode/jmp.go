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

import "fmt"

// Jmp (unconditional branch) instruction
type Jmp struct{ Target Address }

func (p *Jmp) String(_ SystemMap) string {
	return fmt.Sprintf("jmp 0x%08x", p.Target)
}

// Codes implementation for Bytecode interface
func (p *Jmp) Codes(pc uint32) []uint32 {
	// Forward branches are preferred as SKIP instructions, whose offset is
	// unsigned and hence offers a greater forward range.
	if p.Target > pc {
		return []uint32{encodeSkip1(pc, p.Target)}
	} else if roffset, ok := getRelativeOffset(pc, p.Target, 24); ok {
		return []uint32{
			roffset<<8 | JMP,
		}
	}
	//
	panic("branch target overflow")
}

// Patch implementation for Patchable interface
func (p *Jmp) Patch(labels []Address) Patched {
	return &Jmp{labels[p.Target]}
}

// MaxWidth implementation for Patchable interface: a jump always occupies a
// single code word.
func (p *Jmp) MaxWidth() uint32 {
	return 1
}

// Jmp (jump unconditional) instruction.  Format of this instruction is:
//
// +--------------------------+---------+
// |        offset            | opcode  |
// +--------------------------+---------+
//
// Here, offset is a signed u16 relative offset, where the following
// instruction is considered to be at offset 0.
func decodeJmp1(pc uint32, codes []uint32) (uint32, uint32) {
	var target = getBranchTarget(pc, codes[pc]>>8, 24)
	return target, 1
}

// Skip (unconditional forward branch) instruction.  Format of this instruction
// is:
//
// +--------------------------+---------+
// |        offset            | opcode  |
// +--------------------------+---------+
//
// Here, offset is an unsigned u24 relative offset, where the following
// instruction is considered to be at offset 0 (i.e. skip 0 transfers control
// to the next instruction).
func encodeSkip1(pc uint32, target Address) uint32 {
	var offset = target - (pc + 1)
	//
	if offset >= (1 << 24) {
		panic("branch target overflow")
	}
	//
	return offset<<8 | SKIP
}

func decodeSkip1(pc uint32, codes []uint32) (uint32, uint32) {
	var target = pc + 1 + (codes[pc] >> 8)
	return target, 1
}
