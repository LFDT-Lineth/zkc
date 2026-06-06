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

// NewJmp creates a new jump target
func NewJmp(target Address) *Jmp {
	return &Jmp{target}
}

func (p *Jmp) String() string {
	return fmt.Sprintf("jmp 0x%08x", p.Target)
}

// Codes implementation for Bytecode interface
func (p *Jmp) Codes(offset uint32) []uint32 {
	var roff = getRelativeOffset(offset, p.Target, 24) << 8
	//
	return []uint32{
		roff | JMP,
	}
}

// Patch implementation for Bytecode interface
func (p *Jmp) Patch(labels []Address) {
	p.Target = labels[p.Target]
}

// Jmp (jump unconditional) instruction.  Format of this instruction is:
//
// +--------------------------+---------+
// |        offset            | opcode  |
// +--------------------------+---------+
//
// Here, offset is a signed u16 relative offset, where the following
// instruction is considered to be at offset 0.
func decodeJmp1(offset uint32, codes []uint32) (Jmp, uint32) {
	var target = getBranchTarget(offset, codes[0]>>8, 24)
	return Jmp{target}, 1
}
