// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.
//
// SPDX-License-Identifier: Apache-2.0

package split

import (
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LimbsMap provides a useful alias
type LimbsMap[W word.Word[W]] descriptor.LimbsMap[W]

// ApplyLimbsMap maps a set of (bytecode) registers onto their corresponding
// limb registers.  Observe that this splits the limbs in "declaration order"
// (also known as "big endian" ordering). For example, consider this
// declaration:
//
// > fn f(x:u32, y:u32) -> (r:u32) { ... }
//
// After splitting registers into u16 limbs, we have this:
//
// > fn f(x'1:u16,x'0:u16, y'1:u16,y'0:u16) -> (r'1:u16,r'0:u16) { ... }
//
// Observe that x'1 is the most significant limb of x, etc.  Thus, given the
// array "[x,y,r]", this function returns "[x'1,x'0,y'1,y'0,r'1,r'0]"
func ApplyLimbsMap[W any](limbsMap descriptor.LimbsMap[W], rids ...RegisterId) []RegisterId {
	var limbIds []RegisterId
	//
	for _, r := range rids {
		var limbs = limbsMap.LimbIds(r)
		// Reverse limbs to ensure declaration order
		limbIds = append(limbIds, array.Reverse(limbs)...)
	}
	//
	return limbIds
}

// ApplyLimbsMapReversed maps a set of (bytecode) registers onto their
// corresponding limb registers.  Observe that this splits the limbs according
// to their natural (or little endian) ordering.  Thus, given the array
// "[x,y,r]", this function returns "[x'0,x'1,y'0,y'1,r'0,r'1]".
func applyLimbsMapReversed[W any](limbsMap descriptor.LimbsMap[W], rids ...RegisterId) []RegisterId {
	var limbIds []RegisterId
	//
	for _, r := range rids {
		limbIds = append(limbIds, limbsMap.LimbIds(r)...)
	}
	//
	return limbIds
}

type limbMatrix[W word.Word[W]] struct {
	mapping       descriptor.LimbsMap[W]
	chunks        [][]util.Option[RegisterId]
	width, height int
}

func newLimbMatrix[W word.Word[W]](regs []RegisterId, mapping descriptor.LimbsMap[W]) *limbMatrix[W] {
	var (
		nregs  = len(regs)
		nlimbs int
	)
	// Determine maximum number of limbs of any register
	for _, reg := range regs {
		// split ith register into n limbs and then allocate them across the
		// chunks accordingly.
		nlimbs = max(nlimbs, len(mapping.LimbIds(reg)))
	}
	// initialise empty chunks array
	var chunks = make([][]util.Option[RegisterId], nlimbs)
	//
	for i := range chunks {
		chunks[i] = make([]util.Option[RegisterId], nregs)
	}
	// fill out chunks array
	for i, reg := range regs {
		for j, limb := range mapping.LimbIds(reg) {
			chunks[j][i] = util.Some(limb)
		}
	}
	// Done
	return &limbMatrix[W]{
		mapping,
		chunks,
		nlimbs,
		nregs,
	}
}

// SelectLimbs consumes as many register limbs as possible which fit within the
// given bitwidth, returning the selection along with what's left.  This
// function will always select at least one limb and (in this case only) the
// selected bitwidth can exceed that requested.
func selectLimbs[W any](bitwidth uint, targets []RegisterId, mapping descriptor.RegisterMap[W],
) (selected []RegisterId, remainder []RegisterId) {
	//
	var lhs []RegisterId
	// Always force at least one register to be selected
	if targetWidth(targets, mapping) > bitwidth {
		return []RegisterId{targets[0]}, targets[1:]
	}
	// Add more registers only if there is space.
	for targetWidth(targets, mapping) <= bitwidth {
		var (
			next  = targets[0]
			width = mapping.Register(next).Bitwidth().Unwrap()
		)
		//
		lhs = append(lhs, next)
		targets = targets[1:]
		bitwidth = bitwidth - width
	}
	//
	return lhs, targets
}

// RegisterStack abstracts a set of concatenated registers, and provides
// mechanism to extract them on a bit-by-bit basis.
type RegisterStack[W word.Word[W]] struct {
	// stack of registers
	stack []RegisterId
	// register allocator (used when splitting registers)
	alloc Allocator[W]
	// bytecodes created to handle splitting which should come either before or
	// after the given instruction.
	post []Bytecode[W]
}

// Size returns remaining height of stack
func (p *RegisterStack[W]) Size() uint {
	return uint(len(p.stack))
}

// Pop off next target
func (p *RegisterStack[W]) Pop() (res RegisterId) {
	var next = p.stack[0]
	//
	p.stack = p.stack[1:]
	//
	return next
}

// SelectExact at most n bits from the current register stack.  Specifically, if the
// target stack has enough bits, then it always returns exactly n bits.
func (p *RegisterStack[W]) SelectExact(nbits uint) (res []RegisterId) {
	var post []Bytecode[W]
	//
	if len(p.stack) > 0 {
		res, p.stack, post = selectAlignedTargetLimbs(nbits, p.stack, p.alloc)
		// append any required bytecodes
		p.post = append(p.post, post...)
	} else {
		res = []RegisterId{p.alloc.ZeroRegister()}
	}
	//
	return res
}

// SelectUpto selects upto n bits from the stack, but it does not need to be
// exact.
func (p *RegisterStack[W]) SelectUpto(nbits uint) (res []RegisterId) {
	if descriptor.HasNativeRegisterId(p.stack, p.alloc) {
		util.Assert(len(p.stack) == 1, "native register has limbs")
		return p.stack
	}
	//
	if len(p.stack) > 0 {
		res, p.stack = selectLimbs(nbits, p.stack, p.alloc)
	} else {
		res = []RegisterId{p.alloc.ZeroRegister()}
	}
	// Done
	return res
}

// selectAlignedTargetLimbs selects n registers from the given array of target
// registers, such that their combined width does not exceed the given target.
func selectAlignedTargetLimbs[W word.Word[W]](bitwidth uint, targets []RegisterId, alloc Allocator[W],
) (selected []RegisterId, remainder []RegisterId, context []Bytecode[W]) {
	//
	var (
		lhsWidth  uint
		lastWidth uint
	)
	// Consume upto the given bitwidth.
	for lhsWidth < bitwidth && len(targets) > 0 {
		// Determine width of target
		lastWidth = alloc.Register(targets[0]).Bitwidth().Unwrap()
		// Push target onto current lhs
		selected = append(selected, targets[0])
		// Pop target from targets queue
		targets = targets[1:]
		// Update lhs bitwidth
		lhsWidth += lastWidth
	}
	// Alignment check
	if lhsWidth > bitwidth {
		// In this case, we've pull off a register which is too big.  Therefore,
		// it needs to be split into two pieces.
		var (
			n  = lhsWidth - bitwidth
			m  = len(selected) - 1
			lo = alloc.Allocate("t", util.Some(lastWidth-n))
			hi = alloc.Allocate("t", util.Some(n))
		)
		//
		context = append(context, bytecode.Concat[W]([]RegisterId{selected[m]}, []RegisterId{lo, hi}))
		selected = append(selected[:m], lo)
		targets = array.Prepend(hi, targets)
	}
	//
	return selected, targets, context
}

// targetWidth determines the bitwidth of the first target.  If no target
// exists, it returns a large bitwidth to prevent further selection.
func targetWidth[W any](targets []RegisterId, mapping descriptor.RegisterMap[W]) uint {
	if len(targets) == 0 {
		return math.MaxUint
	}
	//
	return mapping.Register(targets[0]).Bitwidth().Unwrap()
}

func allocateTemporary[W word.Word[W]](bitwidth uint, mapping descriptor.LimbsMap[W], alloc Allocator[W],
) (RegisterId, descriptor.LimbsMap[W]) {
	var (
		zero      W
		temp      = descriptor.NewRegister(register.COMPUTED_REGISTER, "", util.Some(bitwidth), zero)
		tempLimbs = descriptor.SplitIntoLimbs(mapping.RegisterWidth(), temp)
		limbs     = make([]RegisterId, len(tempLimbs))
		rid       = RegisterId(mapping.Width())
		n         = len(limbs) - 1
	)
	// Allocate limbs in reverse order so most significant limb comes first
	// (i.e. to ensure allocation matches what happens normally).
	for i := range tempLimbs {
		limbs[n-i] = alloc.Allocate("t", tempLimbs[n-i].Bitwidth())
	}
	//
	return rid, limbsMapWrapper[W]{mapping, rid, temp, limbs}
}

// a minimal wrapper which allows us to "pretend" there is a mapping from an
// imaginary register to a given set of concrete limbs.  It doesn't matter that
// the register is imaginary as, after splitting, only the limbs will remain ---
// and they are real.
type limbsMapWrapper[W any] struct {
	descriptor.LimbsMap[W]
	// the imaginary mapping
	id    RegisterId
	reg   descriptor.Register[W]
	limbs []RegisterId
}

// Limbs implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) LimbIds(reg RegisterId) []descriptor.LimbId {
	if reg == p.id {
		return p.limbs
	}
	//
	return p.LimbsMap.LimbIds(reg)
}

// Register implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) Register(reg descriptor.LimbId) descriptor.Limb[W] {
	if reg == p.id {
		return p.reg
	}
	//
	return p.LimbsMap.Register(reg)
}

// Limb implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) Limb(reg descriptor.LimbId) descriptor.Limb[W] {
	panic("unsupported operation")
}

// Limbs implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) Limbs() []descriptor.Limb[W] {
	panic("unsupported operation")
}

// LimbsMap implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) LimbsRegisterMap() descriptor.RegisterMap[W] {
	panic("unsupported operation")
}

// Registers implementation for the LimbsMap interface
func (p limbsMapWrapper[W]) Registers() []descriptor.Limb[W] {
	panic("unsupported operation")
}
