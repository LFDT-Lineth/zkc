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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// HasNativeRegisterId checks whether a given set of registers contains at least
// one native register.
func HasNativeRegisterId[W word.Word[W]](regs []RegisterId, mapping RegisterMap[W]) bool {
	return array.ContainsMatching(regs, func(rid RegisterId) bool {
		return mapping.Register(rid).IsNative()
	})
}

// HasNativeRegister checks whether a given set of registers contains at least
// one native register.
func HasNativeRegister[W word.Word[W]](regs []Register[W]) bool {
	return array.ContainsMatching(regs, func(reg Register[W]) bool {
		return reg.IsNative()
	})
}

// CalculateAddBitwidth computes the minimal bitwidth required to hold the result
// of summing the given registers and constant.
//
// NOTE: none is returned when any source is a native (field-typed) register.
// Such a register has no fixed bitwidth — it can hold any field element up to
// the prime modulus — so the RHS has no finite width bound and could always
// overflow a fixed-width target.
func CalculateAddBitwidth[W word.Word[W]](sources []RegisterId, constant W, env RegisterMap[W]) util.Option[uint] {
	var (
		acc = func(lhs, rhs *big.Int) { lhs.Add(lhs, rhs) }
		max = CalculateMaximumValue(constant, sources, env, acc)
	)
	//
	return util.MapOption(max, func(val big.Int) uint { return uint(val.BitLen()) })
}

// CalculateSubBitwidth computes the minimal bitwidth required to hold the
// result of subtraction the first register from the sum of the remaining
// registers and constant.
//
// NOTE: none is returned when any source is a native (field-typed) register.
// Such a register has no fixed bitwidth — it can hold any field element up to
// the prime modulus — so the RHS has no finite width bound and could always
// overflow a fixed-width target.
func CalculateSubBitwidth[W word.Word[W]](sources []RegisterId, constant W, env RegisterMap[W]) util.Option[uint] {
	var (
		zero W
		acc  = func(lhs, rhs *big.Int) { lhs.Add(lhs, rhs) }
		lhs  = CalculateMaximumValue(zero, sources[:1], env, acc).Unwrap()
		rhs  = CalculateMaximumValue(constant, sources[1:], env, acc).Unwrap()
	)
	// Subtract one for rhs to account correctly for negative values.  This is
	// because negative values do not need to encode zero and, hence, can
	// account for one additional value.
	rhs.Sub(&rhs, big.NewInt(1))
	// Calculate bitwidth, including an additional bit for the sign
	return util.Some(1 + uint(max(lhs.BitLen(), rhs.BitLen())))
}

// CalculateMulBitwidth computes the minimal bitwidth required to hold the
// result of the product of the given registers and constant.
//
// NOTE: none is returned when any source is a native (field-typed) register.
// Such a register has no fixed bitwidth — it can hold any field element up to
// the prime modulus — so the RHS has no finite width bound and could always
// overflow a fixed-width target.
func CalculateMulBitwidth[W word.Word[W]](sources []RegisterId, constant W, env RegisterMap[W]) util.Option[uint] {
	var (
		acc = func(lhs, rhs *big.Int) { lhs.Mul(lhs, rhs) }
		max = CalculateMaximumValue(constant, sources, env, acc)
	)
	//
	return util.MapOption(max, func(val big.Int) uint { return uint(val.BitLen()) })
}

// CalculateMaximumValue computes the largest value which can be produced by the
// given set of registers and constant using the given accumulator function.
// Using this, its possible to determine the number of bits required to hold the
// resulting value.
//
// NOTE: none is returned when any source is a native (field-typed) register.
// Such a register has no fixed bitwidth — it can hold any field element up to
// the prime modulus — so the RHS has no finite width bound and could always
// overflow a fixed-width target.
func CalculateMaximumValue[W word.Word[W]](constant W, regs []RegisterId, regmap RegisterMap[W],
	acc func(*big.Int, *big.Int)) util.Option[big.Int] {
	//
	var maxVal big.Int
	// Initialise max value
	maxVal.Set(constant.BigInt())
	// Determine maximum expressible value
	for _, rod := range regs {
		var (
			v   = big.NewInt(1)
			reg = regmap.Register(rod)
		)
		// Check for native registers
		if reg.IsNative() {
			return util.None[big.Int]()
		}
		//
		v.Lsh(v, reg.Bitwidth().Unwrap())
		v.Sub(v, big.NewInt(1))
		// Accumulate maximum register value
		acc(&maxVal, v)
	}
	//
	return util.Some(maxVal)
}

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
