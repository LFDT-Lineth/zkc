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
package transform

import (
	"math/bits"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// rotation describes a recognised rotation idiom "(x << s) | (x >> t)" where
// s + t == width: an OR of two opposite shifts of the same source register.
// When both amounts are compile-time constants the rotation is realised
// inline as a single Destruct + Concat (see inlineRotlByConst); otherwise
// `amount` names the dynamic amount register and `op` selects the direction
// (OP_SHL: rotate left by amount; OP_SHR: rotate right by amount) — the
// other amount register is structurally "width - amount" and is discarded.
type rotation struct {
	// source is the register being rotated.
	source bytecode.RegisterId
	// constAmount is the rotate-left amount, valid iff isConst.  It lies in
	// [0, width]; both bounds denote the identity rotation.
	constAmount uint
	isConst     bool
	// amount is the dynamic amount register (unused when isConst).
	amount bytecode.RegisterId
	// op is OP_SHL for rotate-left, OP_SHR for rotate-right (unused when
	// isConst).
	op bytecode.Operation
}

// scanRotations identifies rotation idioms in a function: OR bytecodes whose
// two operands are single-use, single-writer results of a SHL and a SHR of
// the same (single-writer) source, with amounts summing to the operand
// width.  Amounts are constant when both appear in consts; a dynamic pair
// qualifies when one amount register is structurally defined as "width -
// other" (a single-writer two-source SUB whose first source is the constant
// width) — the shape produced by the usual "(x << n) | (x >> (w - n))"
// source-level rotation.  Dynamic recognition is limited to power-of-two
// widths, where the amount register (of width <= log2(width)) always holds
// an in-range rotation.
//
// The result maps each such OR to its rotation, alongside the set of shift
// bytecodes it obsoletes (which the caller must drop).
func scanRotations[W word.Word[W]](fn *descriptor.Function[W],
	consts map[bytecode.RegisterId]W,
) (map[*bytecode.Bitwise[W]]rotation, map[*bytecode.Bitwise[W]]bool) {
	var (
		regs   = fn.Registers()
		writes = make(map[bytecode.RegisterId]uint)
		uses   = make(map[bytecode.RegisterId]uint)
		defOf  = make(map[bytecode.RegisterId]Bytecode[W])
		rots   = make(map[*bytecode.Bitwise[W]]rotation)
		dead   = make(map[*bytecode.Bitwise[W]]bool)
	)
	//
	for _, vec := range fn.Vectors() {
		for _, insn := range vec.Bytecodes {
			for _, target := range insn.Definitions() {
				writes[target]++
				defOf[target] = insn
			}
			//
			for _, src := range insn.Uses() {
				uses[src]++
			}
		}
	}
	//
	for _, vec := range fn.Vectors() {
		for _, insn := range vec.Bytecodes {
			or, ok := insn.(*bytecode.Bitwise[W])
			if !ok || or.Op != bytecode.OP_OR || or.Right.IsConstant() {
				continue
			}
			//
			right := or.Right.AsRegister()
			if or.Left == right {
				continue
			}
			// Both operands must be single-writer shifts consumed only here.
			if writes[or.Left] != 1 || writes[right] != 1 || uses[or.Left] != 1 || uses[right] != 1 {
				continue
			}
			//
			shl, okL := defOf[or.Left].(*bytecode.Bitwise[W])
			shr, okR := defOf[right].(*bytecode.Bitwise[W])
			//
			if !okL || !okR {
				continue
			} else if shl.Op == bytecode.OP_SHR {
				shl, shr = shr, shl
			}
			//
			if shl.Op != bytecode.OP_SHL || shr.Op != bytecode.OP_SHR || shl.Left != shr.Left {
				continue
			}
			// The rotated source must be stable across both shifts.
			var (
				x     = shl.Left
				width = uint(or.Bitwidth)
			)
			//
			if writes[x] > 1 || regs[x].IsNative() || regs[x].Bitwidth().Unwrap() != width {
				continue
			} else if uint(shl.Bitwidth) != width || uint(shr.Bitwidth) != width {
				continue
			}
			//
			if rot, ok := matchRotation(shl, shr, width, regs, writes, defOf, consts); ok {
				rots[or] = rot
				dead[shl] = true
				dead[shr] = true
			}
		}
	}
	//
	return rots, dead
}

// matchRotation classifies a (SHL, SHR) pair over source x and width w as a
// constant or dynamic rotation, per the rules described on scanRotations.
func matchRotation[W word.Word[W]](shl, shr *bytecode.Bitwise[W], width uint,
	regs []descriptor.Register[W],
	writes map[bytecode.RegisterId]uint,
	defOf map[bytecode.RegisterId]Bytecode[W],
	consts map[bytecode.RegisterId]W,
) (rotation, bool) {
	// Shift amounts are always registers (constant operands are only
	// supported for AND/OR/XOR).
	shlAmt, shrAmt := shl.Right.AsRegister(), shr.Right.AsRegister()
	// Constant case: both amounts known, summing to the width.
	cl, okL := consts[shlAmt]
	cr, okR := consts[shrAmt]
	//
	if okL && okR && cl.BitLen() <= 32 && cr.BitLen() <= 32 {
		s, t := uint(cl.Uint64()), uint(cr.Uint64())
		//
		if s+t == width {
			return rotation{source: shl.Left, constAmount: s, isConst: true}, true
		}
		//
		return rotation{}, false
	}
	// Dynamic case: power-of-two widths only, so that an amount register of
	// width <= log2(width) always denotes an exact rotation.
	if width == 0 || width&(width-1) != 0 {
		return rotation{}, false
	}
	//
	depth := uint(bits.Len(width - 1))
	// (x << n) | (x >> (w - n)): rotate left by n.
	if n := shlAmt; isWidthMinus(shrAmt, width, n, writes, defOf, consts) &&
		isRotAmount(n, depth, regs, writes) {
		return rotation{source: shl.Left, amount: n, op: bytecode.OP_SHL}, true
	}
	// (x << (w - n)) | (x >> n): rotate right by n.
	if n := shrAmt; isWidthMinus(shlAmt, width, n, writes, defOf, consts) &&
		isRotAmount(n, depth, regs, writes) {
		return rotation{source: shr.Left, amount: n, op: bytecode.OP_SHR}, true
	}
	//
	return rotation{}, false
}

// isRotAmount checks a dynamic rotation amount register: stable (at most one
// write), non-native, and narrow enough to enter the rotation chain directly
// (width in [1, depth]).
func isRotAmount[W word.Word[W]](n bytecode.RegisterId, depth uint,
	regs []descriptor.Register[W], writes map[bytecode.RegisterId]uint,
) bool {
	if writes[n] > 1 || regs[n].IsNative() {
		return false
	}
	//
	w := regs[n].Bitwidth().Unwrap()
	//
	return w >= 1 && w <= depth
}

// isWidthMinus reports whether register m is structurally "width - n": a
// single-writer two-source SUB with zero constant whose first source holds
// the constant width and whose second source is n.
func isWidthMinus[W word.Word[W]](m bytecode.RegisterId, width uint, n bytecode.RegisterId,
	writes map[bytecode.RegisterId]uint,
	defOf map[bytecode.RegisterId]Bytecode[W],
	consts map[bytecode.RegisterId]W,
) bool {
	if writes[m] != 1 {
		return false
	}
	//
	sub, ok := defOf[m].(*bytecode.Arith[W])
	if !ok || sub.Op != bytecode.OP_SUB || len(sub.Target) != 1 || len(sub.Source) != 2 {
		return false
	}
	//
	if sub.Constant.Cmp64(0) != 0 || sub.Source[1] != n {
		return false
	}
	//
	w, ok := consts[sub.Source[0]]
	//
	return ok && w.Cmp64(uint64(width)) == 0
}
