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
package register

import (
	"fmt"
)

// Limb is just an alias for Register, but it helps to clarify when we are
// referring to a register after subdivision.
type Limb = Register

// LimbId is just an alias for register.RegisterId, but it helps to clarify when we are
// referring to a register after subdivision.
type LimbId = Id

// SplitIntoLimbs splits a register into a number of limbs with the given maximum
// bitwidth.  For the resulting array, the least significant register is first.
// Since registers are always split to the maximum width as much as possible, it
// is only the most significant register which may (in some cases) have fewer
// bits than the maximum allowed.
func SplitIntoLimbs(maxWidth uint, r Register) []Register {
	panic("delete me")
}

// LimbWidths determines the limb widths for any register of the given size.
func LimbWidths(maxWidth, regWidth uint) []uint {
	var (
		commonWidth  = commonLimbWidth(maxWidth, regWidth)
		nlimbs       = minNumberOfLimbs(commonWidth, regWidth)
		limbWidths   = make([]uint, nlimbs)
		bitsLeft     = regWidth
		accLimbWidth uint
	)
	//
	for i := range nlimbs {
		if i+1 != nlimbs {
			// internal limbs get common width
			limbWidths[i] = min(commonWidth, bitsLeft)
		} else {
			// last limb gets remaining bits
			limbWidths[i] = bitsLeft
		}
		//
		accLimbWidth += limbWidths[i]
		bitsLeft -= limbWidths[i]
		// Sanity check requirements met
		if limbWidths[i] > maxWidth {
			panic(fmt.Sprintf(
				"internal failure (limb width u%d exceeds maximum register width u%d)", limbWidths[i], maxWidth))
		}
	}
	// Sanity check
	if accLimbWidth != regWidth {
		panic(fmt.Sprintf(
			"internal failure (register width u%d does not match combined limb widths u%d)", regWidth, accLimbWidth))
	}
	//
	return limbWidths
}

// commonLimbWidth returns the "common" limb width when splitting a given
// register for a given maximum width.  Assume a given register splits into n
// limbs.  Assuming n > 1, then n-1 of these will have the same "common" width.
// The remaining limb is referred to as the residue, and may have a different
// width (usually smaller, but this is not a requirement).
func commonLimbWidth(maxRegisterWidth uint, registerWidth uint) uint {
	var (
		// Determine how many limbs required
		n = minNumberOfLimbs(maxRegisterWidth, registerWidth)
		//
		avgWidth = uint(1)
	)
	// Now, round up our average limb width to the nearest power of two.  This
	// is because we "prefer" widths to be powers of two.
	for ; (avgWidth*n) < registerWidth && (avgWidth*2 <= maxRegisterWidth); avgWidth = avgWidth * 2 {
	}
	//
	return avgWidth
}

// minNumberOfLimbs determines the minimum number of register limbs required for
// a given bitwidth. For example, a 64bit register splits into two limbs for a
// maximum register width of 32bits. Observe that an e.g. 60bit register also
// splits into two limbs here as well, where the most significant limb is 28bits
// wide and the least significant is 32bits width.
func minNumberOfLimbs(maxRegisterWidth uint, registerWidth uint) uint {
	n := registerWidth / maxRegisterWidth
	m := registerWidth % maxRegisterWidth
	// Check for uneven split
	if m != 0 {
		return n + 1
	}
	//
	return n
}
