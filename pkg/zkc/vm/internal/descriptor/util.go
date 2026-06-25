// Copyright Consensys Software Inc.
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
package descriptor

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// SplitConstant splits a given constant into a number of "limbs". For example,
// consider splitting the constant 0x7b2d into 8-bit limbs.  Then, this function
// returns the array [0x2d,0x7b].  Observe that the least significant limb is
// always returned first (i.e. at index zero in the resulting array).
func SplitConstant[W word.Word[W]](constant W, width uint) []W {
	var (
		acc   = constant
		limbs []W
	)
	//
	for i := 0; acc.Cmp64(0) != 0; i++ {
		// Extract bottom bits
		limbs = append(limbs, acc.Slice(width))
		// Shift down
		acc = acc.Shr64(uint64(width))
	}
	//
	return limbs
}

// SplitIntoLimbs splits a register into a number of limbs with the given maximum
// bitwidth.  For the resulting array, the least significant register is first.
// Since registers are always split to the maximum width as much as possible, it
// is only the most significant register which may (in some cases) have fewer
// bits than the maximum allowed.
func SplitIntoLimbs[W word.Word[W]](maxWidth uint, r Register[W]) []Register[W] {
	// We do not split native registers
	if r.Bitwidth().IsEmpty() {
		return []Register[W]{r}
	}
	// Non-native register can be split, so proceed.
	var (
		bitwidth = r.bitwidth.Unwrap()
		limbs    []Register[W]
		// Split padding value
		padding = SplitConstant(r.Padding(), maxWidth)
	)
	//
	for i := 0; bitwidth > 0; i++ {
		var (
			ith_width   = min(maxWidth, bitwidth)
			ith_name    = fmt.Sprintf("%s'%d", r.Name(), i)
			ith_padding W
		)
		// Update padding (if applicable)
		if i < len(padding) {
			ith_padding = padding[i]
		}
		// construct limt
		limbs = append(limbs, NewRegister(r.kind, ith_name, util.Some(ith_width), ith_padding))
		//
		bitwidth -= ith_width
	}
	// Special case when register doesn't require splitting.  This is useful
	// because we want to retain the original register name exactly.
	if len(limbs) <= 1 {
		return []Register[W]{r}
	}
	//
	return limbs
}
