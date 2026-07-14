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
	"github.com/LFDT-Lineth/zkc/pkg/util"
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
		zero      W
		bytecodes []Bytecode[W]
		// LimbIds returns limbs least-significant first, which is exactly the
		// ordering we want since bitwise operations are limb-wise (i.e.
		// bit-position-aligned).
		targets = mapping.LimbIds(insn.Target)
		left    = mapping.LimbIds(insn.Left)
		right   = mapping.LimbIds(insn.Right)
		//
		n = max(len(targets), len(left), len(right))
	)
	//
	for i := range n {
		var (
			target RegisterId
			code   Bytecode[W]
		)
		//
		if i >= len(targets) {
			target = alloc.ZeroRegister()
		} else {
			target = targets[i]
		}
		//
		if i < len(left) && i < len(right) {
			code = newBitwiseBytecode(insn.Op, mapping, target, left[i], right[i])
		} else if i < len(left) {
			code = newBitwiseBytecode(insn.Op, mapping, target, left[i])
		} else if i < len(right) {
			code = newBitwiseBytecode(insn.Op, mapping, target, right[i])
		} else if i < len(targets) {
			code = bytecode.LoadConst(target, zero)
		} else {
			panic("unreachable")
		}
		//
		bytecodes = append(bytecodes, code)
	}
	//
	return bytecodes
}

func newBitwiseBytecode[W word.Word[W]](op bytecode.Operation, mapping descriptor.LimbsMap[W], target RegisterId,
	sources ...RegisterId) Bytecode[W] {
	//
	var bitwidth uint
	// Determine operand bitwidth
	for _, src := range sources {
		bitwidth = max(bitwidth, mapping.Limb(src).Bitwidth().Unwrap())
	}
	//
	switch op {
	case bytecode.OP_AND:
		return newAndBytecode[W](bitwidth, target, sources...)
	case bytecode.OP_OR:
		return newOrBytecode[W](bitwidth, target, sources...)
	case bytecode.OP_XOR:
		return newXorBytecode[W](bitwidth, target, sources...)
	case bytecode.OP_NOT:
		return newNotBytecode[W](bitwidth, target, sources[0])
	default:
		panic("unknown bitwise operator")
	}
}

func newAndBytecode[W word.Word[W]](bitwidth uint, target RegisterId, sources ...RegisterId) Bytecode[W] {
	var zero W
	//
	if len(sources) != 2 {
		return bytecode.LoadConst(target, zero)
	}
	//
	return bytecode.NewBitwise[W](bytecode.OP_AND, target, sources[0], sources[1], util.Cast[uint16](bitwidth))
}

func newOrBytecode[W word.Word[W]](bitwidth uint, target RegisterId, sources ...RegisterId) Bytecode[W] {
	if len(sources) != 2 {
		return bytecode.Assign[W](target, sources[0])
	}
	//
	return bytecode.NewBitwise[W](bytecode.OP_OR, target, sources[0], sources[1], util.Cast[uint16](bitwidth))
}

func newXorBytecode[W word.Word[W]](bitwidth uint, target RegisterId, sources ...RegisterId) Bytecode[W] {
	if len(sources) != 2 {
		return bytecode.NewBitwise[W](bytecode.OP_NOT, target, sources[0], sources[0], util.Cast[uint16](bitwidth))
	}
	//
	return bytecode.NewBitwise[W](bytecode.OP_XOR, target, sources[0], sources[1], util.Cast[uint16](bitwidth))
}

func newNotBytecode[W word.Word[W]](bitwidth uint, target RegisterId, source RegisterId) Bytecode[W] {
	//
	return bytecode.NewBitwise[W](bytecode.OP_NOT, target, source, source, util.Cast[uint16](bitwidth))
}
