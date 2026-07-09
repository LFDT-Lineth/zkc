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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

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

// splitSourceRegisters splits each source register into limb-indexed chunks
// and folds the corresponding limb of the instruction constant into each
// chunk.
func splitSourceRegisters[W word.Word[W]](mapping descriptor.LimbsMap[W], regs []RegisterId, constant W) Chunks[W] {
	var (
		chunks Chunks[W]
	)
	// Split source registers
	for _, reg := range regs {
		// split ith register into n limbs and then allocate them across the
		// chunks accordingly.
		for j, limb := range mapping.LimbIds(reg) {
			chunks.Apply(uint(j), appendRhsLimb[W](limb))
		}
	}
	// Split constant
	for i, c := range descriptor.SplitConstant(constant, mapping.RegisterWidth()) {
		chunks.Apply(uint(i), setRhsConstant(c))
	}
	//
	return chunks
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

// initialiseLineaChunks splits the source registers (and constant) into
// least-significant-first chunks, then assigns target limbs to each chunk
// according to the number of bits the corresponding RHS can produce.
func initialiseLineaChunks[W word.Word[W]](mapping descriptor.LimbsMap[W], alloc Allocator[W],
	targets, sources []RegisterId, constant W) (Chunks[W], []Bytecode[W]) {
	//
	var (
		bytecodes []Bytecode[W]
		// Extract register width
		regWidth = mapping.RegisterWidth()
		// Split source registers into initial chunks
		chunks = splitSourceRegisters(mapping, sources, constant)
		// Determine target limbs
		limbs = applyLimbsMapReversed(mapping, targets...)
	)
	//
	for i := uint(0); i < chunks.Len(); i++ {
		var (
			lhs   []RegisterId
			codes []Bytecode[W]
		)
		// pull out targets
		if len(limbs) > 0 {
			lhs, limbs, codes = selectAlignedLimbs(regWidth, limbs, alloc)
		} else {
			lhs = []RegisterId{alloc.ZeroRegister()}
		}
		// allocate selected targets
		chunks.Apply(i, setLhsLimbs[W](lhs...))
		//
		bytecodes = append(bytecodes, codes...)
	}
	// Handle cases where we have more targets than necessary.  This can arise
	// under normal circumstances, such as when assigning a small constant to a
	// wide target register.  In this case, we simple assign each target in this
	// "overhang" to zero.
	for len(limbs) > 0 {
		chunks.Append(setLhsLimbs[W](limbs[0]))
		limbs = limbs[1:]
	}
	//
	return chunks, bytecodes
}

// select aligned target registers
func selectAlignedLimbs[W word.Word[W]](bitwidth uint, targets []RegisterId, alloc Allocator[W],
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
			lo = alloc.Allocate("t", lastWidth-n)
			hi = alloc.Allocate("t", n)
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
