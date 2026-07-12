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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Jmp encodes an unconditional jump bytecode.  A forward branch is preferred as
// a SKIP instruction (whose unsigned offset offers greater forward range),
// falling back to a JMP carrying a signed relative offset otherwise.
func Jmp[W word.Word[W]](pc uint32, b *bytecode.Jmp[W], env Environment[W]) []uint32 {
	var (
		offset = env.OffsetFor(env.enclosing, ProgramPoint{uint(b.Target), 0})
	)
	// Forward branches are preferred as SKIP instructions, whose offset is
	// unsigned and hence offers a greater forward range.
	if offset > pc {
		return []uint32{encodeSkip1(pc, offset)}
	} else if roffset, ok := GetRelativeOffset(pc, offset, 24); ok {
		return []uint32{
			roffset<<8 | JMP,
		}
	}
	//
	panic("branch target overflow")
}

// Skip encodes an unconditional forward-branch (SKIP) bytecode.
func Skip[W word.Word[W]](pc uint32, b *bytecode.Skip[W], env Environment[W]) []uint32 {
	var (
		target = env.Point().Skip(uint(b.Skip))
		offset = env.OffsetFor(env.enclosing, target)
	)
	//
	return []uint32{encodeSkip1(pc, offset)}
}

// Jmp (jump unconditional) instruction.  Format of this instruction is:
//
// +--------------------------+---------+
// |        offset            | opcode  |
// +--------------------------+---------+
//
// Here, offset is a signed u16 relative offset, where the following
// instruction is considered to be at offset 0.

// DecodeJmp1 decodes the target of an unconditional jump instruction.
func DecodeJmp1(pc uint32, codes []uint32) (uint32, uint32) {
	var target = GetBranchTarget(pc, codes[pc]>>8, 24)
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

// DecodeSkip1 decodes the target of a forward-skip instruction.
func DecodeSkip1(pc uint32, codes []uint32) (uint32, uint32) {
	var target = pc + 1 + (codes[pc] >> 8)
	return target, 1
}
