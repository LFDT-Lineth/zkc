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

package split

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bitwise splits a bitwise AND, OR or XOR instruction into one (or more)
// bitwise instructions of the same kind.  Unlike for e.g. addition, this does
// not require adding any carry lines.  For example, consider splitting this
// instruction (where all registers are u16):
//
// > x = y & z
//
// Suppose now that x, y and z are split into u8 registers (i.e. x=x1::x0, etc).
// Then, we end up with the following instructions:
//
// > x0 = y0 & z0
// > x1 = y1 & z1
//
// The same limb-wise decomposition applies to OR and XOR.  Thus, we can see
// that splitting bitwise operations is relatively straightforward.
func Bitwise[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W], insn *bytecode.Bitwise[W],
) []Bytecode[W] {
	var (
		// LimbIds returns limbs least-significant first, which is exactly the
		// ordering we want since bitwise operations are limb-wise (i.e.
		// bit-position-aligned).
		targetLimbs = mapping.LimbIds(insn.Target)
		leftLimbs   = mapping.LimbIds(insn.Left)
		rightLimbs  = mapping.LimbIds(insn.Right)
	)
	// All operands of a bitwise instruction share the same bitwidth and,
	// therefore, produce the same number of limbs after splitting.
	if len(targetLimbs) != len(leftLimbs) || len(targetLimbs) != len(rightLimbs) {
		panic("inconsistent limb counts for bitwise operation")
	}
	//
	var insns []Bytecode[W]
	//
	for i := range targetLimbs {
		var (
			tgt = targetLimbs[i]
			lhs = leftLimbs[i]
			rhs = rightLimbs[i]
			// Determine the bitwidth of this limb from the target limb itself.
			bw = uint16(mapping.Limb(tgt).Bitwidth().Unwrap())
		)
		//
		insns = append(insns, bytecode.NewBitwise[W](insn.Op, tgt, lhs, rhs, bw))
	}
	//
	return insns
}
