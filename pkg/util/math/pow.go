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
package math

import (
	"math/big"
	"math/bits"
)

// NextPowerOfTwo returns the smallest power of two greater than or equal to n,
// treating zero as one.
func NextPowerOfTwo(n uint) uint {
	if n <= 1 {
		return 1
	}
	//
	return 1 << bits.Len(n-1)
}

// PowUint64 raises a given base raised to a given power.
func PowUint64(base uint64, exp uint64) uint64 {
	result := uint64(1)
	//
	for {
		if exp&1 == 1 {
			result *= base
		}
		// div 2
		exp >>= 1
		//
		if exp == 0 {
			break
		}
		//
		base *= base
	}

	return result
}

// Pow2 computes two raised to a given power (i.e. 2^n)
func Pow2(n uint) *big.Int {
	var m = big.NewInt(2)
	//
	m.Exp(m, big.NewInt(int64(n)), nil)
	//
	return m
}

// NegPow2 computes minus two reaised to a given power (i.e. -2^n)
func NegPow2(n uint) *big.Int {
	val := Pow2(n)
	return val.Neg(val)
}

// FloorLog2 computes the floor of the base-2 logarithm of a given number, returning 0 for 0.
func FloorLog2(n uint) uint {
	if n == 0 {
		return 0
	}

	return uint(bits.Len(n) - 1)
}
