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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DecomposeSubtractions rewrites every integer subtraction having more than one
// "negative term" (i.e. more than one subtrahend register, or a subtrahend plus
// a non-zero constant) into a chain of subtractions, each having exactly one
// negative term.  For example (all u32):
//
//	z = x - y - w        =>   t = x - y ; z = t - w
//	z = x - y - w - 1    =>   t0 = x - y ; t1 = t0 - w ; z = t1 - 1
//
// This must run before register splitting.  The borrow line introduced by
// split.Subtraction only encodes a borrow of a single 2^regWidth unit correctly:
// it stores the two's-complement sign-extension bits of the (possibly negative)
// chunk result rather than the true borrow count.  A subtraction with k negative
// terms can borrow up to k units across a limb boundary, which the single-borrow
// model mis-handles.  Restricting every subtraction to one negative term
// guarantees a 1-bit borrow, which splits correctly.
//
// Each intermediate is a fresh register wide enough to hold the partial result.
//
// Only subtractions that register splitting will actually touch are rewritten:
// a subtraction whose operands and target all fit within the field's register
// width is left alone (splitting is a no-op for it, and decomposing would alter
// its two's-complement wrap-on-underflow semantics).  For genuinely-split
// subtractions the result is required to be non-negative (the VM rejects a
// negative result that no longer fits the fixed-width target), so every partial
// is bounded by the minuend and no intermediate wrap-around occurs.
func DecomposeSubtractions[W word.Word[W]](cfg field.Config, program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())
	//
	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = decomposeSubtractionFunction[W](cfg.RegisterWidth, fn)
		}
	}
	//
	return descriptor.NewProgram(program.Field(), out...)
}

func decomposeSubtractionFunction[W word.Word[W]](regWidth uint, fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = newRegAllocator(fn.Registers())
	)
	//
	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return decomposeSubtractionCode[W](regWidth, b, alloc)
		})
	}
	//
	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.IsNative(), nvecs)
}

func decomposeSubtractionCode[W word.Word[W]](regWidth uint, b Bytecode[W], alloc *regAllocator[W]) []Bytecode[W] {
	insn, ok := b.(*bytecode.Arith[W])
	// Only integer subtractions are of interest.
	if !ok || insn.Op != bytecode.OP_SUB || len(insn.Source) == 0 {
		return []Bytecode[W]{b}
	}
	//
	var (
		zero        W
		hasConstant = insn.Constant.Cmp64(0) != 0
		subtrahends = insn.Source[1:]
		// Number of negative terms (subtrahends plus a non-zero constant).
		negs = len(subtrahends)
	)
	//
	if hasConstant {
		negs++
	}
	// A single (or zero) negative term already splits correctly (1-bit borrow);
	// native (field) operands are never split; and a subtraction that fits
	// entirely within the register width is not split at all (and must keep its
	// wrap-on-underflow semantics), so leave all of these untouched.
	if negs < 2 || !alloc.Register(insn.Source[0]).Bitwidth().HasValue() ||
		decomposeWidth(insn, alloc) <= regWidth {
		return []Bytecode[W]{b}
	}
	//
	var (
		codes []Bytecode[W]
		acc   = insn.Source[0]
		width = decomposeWidth(insn, alloc)
		n     = len(subtrahends)
	)
	// Subtract each subtrahend register, one at a time, into a fresh temporary
	// (the last step writes the original target when there is no constant).
	for i, sub := range subtrahends {
		var target []bytecode.RegisterId
		//
		if i == n-1 && !hasConstant {
			target = insn.Target
		} else {
			target = []bytecode.RegisterId{alloc.Allocate("t", util.Some(width))}
		}
		//
		codes = append(codes, bytecode.SubVecConst(target, []bytecode.RegisterId{acc, sub}, zero))
		acc = target[0]
	}
	// Subtract the constant last, as its own single-term step.
	if hasConstant {
		codes = append(codes, bytecode.SubVecConst(insn.Target, []bytecode.RegisterId{acc}, insn.Constant))
	}
	//
	return codes
}

// decomposeWidth returns a bitwidth large enough to hold every partial result of
// the decomposed chain without spurious overflow: the maximum width across the
// operands and the target.  Native (field) operands are excluded (handled by the
// caller); their absence here yields the widest fixed-width operand.
func decomposeWidth[W word.Word[W]](insn *bytecode.Arith[W], alloc *regAllocator[W]) uint {
	var width uint
	//
	for _, r := range insn.Source {
		width = max(width, alloc.Register(r).Bitwidth().UnwrapOr(0))
	}
	//
	for _, r := range insn.Target {
		width = max(width, alloc.Register(r).Bitwidth().UnwrapOr(0))
	}
	//
	return width
}
