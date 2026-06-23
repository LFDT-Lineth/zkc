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
package agnosticity

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Alias for big integer representation of 0.
var zero big.Int = *big.NewInt(0)

// NumberOfLimbs determines the number of register limbs required for a given
// bitwidth. For example, a 64bit register splits into two limbs for a maximum
// register width of 32bits. Observe that an e.g. 60bit register also splits
// into two limbs here as well, where the most significant limb is 28bits wide
// and the least significant is 32bits width.
func NumberOfLimbs(maxWidth uint, regWidth uint) uint {
	n := regWidth / maxWidth
	m := regWidth % maxWidth
	// Check for uneven split
	if m != 0 {
		return n + 1
	}
	//
	return n
}

// MaxNumberOfLimbs returns the maximum number of limbs required for any
// register in the given target registers.  For example, given registers r0 and
// r1 of bitwidths 16bits and 8bits (respectively), then 2 is maximum number of
// limbs for an 8bit maximum register width.
func MaxNumberOfLimbs(maxWidth uint, regs []Register, targets []RegisterId) uint {
	var n = uint(0)
	//
	for _, target := range targets {
		regWidth := regs[target.Unwrap()].Width()
		n = max(n, NumberOfLimbs(maxWidth, regWidth))
	}
	//
	return n
}

// SplitRegister splits a register into a number of limbs with the given maximum
// bitwidth.  For the resulting array, the least significant register is first.
// Since registers are always split to the maximum width as much as possible, it
// is only the most significant register which may (in some cases) have fewer
// bits than the maximum allowed.
func SplitRegister(maxWidth uint, r Register) []Register {
	var (
		nlimbs  = NumberOfLimbs(maxWidth, r.Width())
		limbs   = make([]Register, nlimbs)
		width   = r.Width()
		padding = SplitConstant(nlimbs, maxWidth, *r.Padding())
	)
	//
	maxWidth = determineLimbWidth(nlimbs, width)
	//
	for i := uint(0); i < nlimbs; i++ {
		ith_name := fmt.Sprintf("%s'%d", r.Name(), i)
		ith_width := min(maxWidth, width)
		limbs[i] = register.New(r.Kind(), ith_name, ith_width, padding[i])
		width -= maxWidth
	}
	//
	return limbs
}

// SplitConstant splits a given constant into a number of "limbs" of a given
// maximum width. For example, consider splitting the constant 0x7b2d into 8bit
// limbs.  Then, this function returns the array [0x2d,0x7b].
func SplitConstant(nLimbs uint, maxWidth uint, constant big.Int) []big.Int {
	var (
		bound = big.NewInt(2)
		acc   big.Int
		limbs []big.Int = make([]big.Int, nLimbs)
	)
	// Clone constant
	acc.Set(&constant)
	// Determine upper bound
	bound.Exp(bound, big.NewInt(int64(maxWidth)), nil)
	//
	for i := 0; acc.Cmp(&zero) != 0; i++ {
		var limb big.Int
		//limb.Set(&acc)
		limb.Mod(&acc, bound)
		limbs[i] = limb

		acc.Rsh(&acc, maxWidth)
	}
	//
	return limbs
}

func determineLimbWidth(nlimbs uint, width uint) uint {
	return bound(width / nlimbs)
}

func bound(width uint) uint {
	var (
		acc = uint(1)
	)
	// Find nearest power of 2 (upper bound)
	for ; acc < width; acc = acc * 2 {

	}
	//
	return acc
}
