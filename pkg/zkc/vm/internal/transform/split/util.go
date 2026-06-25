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
// > fn f(x'1:u16, x'0:u16, y'1:u16,y'0:u16) -> (r'1:u16,r'0:u16) { ... }
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
	for i, c := range descriptor.SplitConstant(constant, mapping.Field().RegisterWidth) {
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

// targetWidth determines the bitwidth of the first target.  If no target
// exists, it returns a large bitwidth to prevent further selection.
func targetWidth[W any](targets []RegisterId, mapping descriptor.RegisterMap[W]) uint {
	if len(targets) == 0 {
		return math.MaxUint
	}
	//
	return mapping.Register(targets[0]).Bitwidth().Unwrap()
}
