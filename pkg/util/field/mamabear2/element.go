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

// Package mamabear2 implements the MamaBear field (m = 2⁴⁹ - 2³⁴ + 1) directly
// on a single uint64 limb, rather than by wrapping gnark-crypto.  Every
// operation takes and returns elements by value, without passing pointers,
// such that calls can be fully inlined.
package mamabear2

import (
	"math/big"
	"math/bits"
	"strconv"
)

// Element of a prime order field, represented in Montgomery form to speed up
// multiplications.  The limb is always fully reduced (i.e. less than the
// modulus), hence every element has a unique representation.
type Element [1]uint64 // defined as an array to prevent mistaken use of arithmetic operators, or naive assignments.

const (
	// Modulus m = 2⁴⁹ - 2³⁴ + 1
	Modulus           = 562932773552129
	rModM             = 17179836415     // r (mod m), where r = 2⁶⁴
	rSqModM           = 207231713267    // r² (mod m)
	negModulusInvModR = 562932773552127 // -Modulus⁻¹ (mod r), used for Montgomery reduction
	nbBytes           = 8
)

var (
	bigModulus = new(big.Int).SetUint64(Modulus)
)

// In what follows nᵣ = 64 is the number of bits in r, whilst nₘ = 49 is the
// number of bits in m.

// Add x + y
func (x Element) Add(y Element) Element {
	res := x[0] + y[0] // nₘ + 1 bits, so cannot overflow

	if reduced, borrow := bits.Sub64(res, Modulus, 0); borrow == 0 {
		res = reduced
	}

	return Element{res}
}

// Sub x - y
func (x Element) Sub(y Element) Element {
	res, borrow := bits.Sub64(x[0], y[0], 0)
	if borrow != 0 {
		res += Modulus
	}

	return Element{res}
}

// montgomeryReduce hi:lo -> hi:lo.R⁻¹ (mod m)
// 0 ≤ hi:lo < m.R      ( nₘ + nᵣ bits )
func montgomeryReduce(hi, lo uint64) Element {
	// textbook Montgomery reduction
	t := lo * negModulusInvModR // t = x * (-Modulus⁻¹) (mod r)
	tHi, tLo := bits.Mul64(t, Modulus)
	// By construction lo + tLo = 0 (mod r), hence the low word of x + t.m is
	// discarded and only its carry retained.
	_, carry := bits.Add64(lo, tLo, 0)
	// (x + t.m) / r < 2m < 2⁵⁰, so this cannot overflow.
	res := hi + tHi + carry

	if reduced, borrow := bits.Sub64(res, Modulus, 0); borrow == 0 {
		res = reduced
	}

	return Element{res}
}

// ToUint64 returns the numerical (non-Montgomery) value of x.
func (x Element) ToUint64() uint64 {
	// Montgomery reduction specialised for a single word.  Since x < m, then
	// (x + t.m) / r < m + 1.  Furthermore, equality with m would require x = 0
	// (mod m), in which case t = 0 and the result is 0.  Hence, the result is
	// always fully reduced.
	t := x[0] * negModulusInvModR
	tHi, tLo := bits.Mul64(t, Modulus)
	_, carry := bits.Add64(x[0], tLo, 0)

	return tHi + carry
}

// Modulus implementation for the Element interface
func (x Element) Modulus() *big.Int {
	return bigModulus
}

// Mul x * y
func (x Element) Mul(y Element) Element {
	return montgomeryReduce(bits.Mul64(x[0], y[0]))
}

// New returns an element of the field corresponding to the natural number x
// (mod m).
func New(x uint64) Element {
	// x < r and r² (mod m) < m, hence x.r² < m.r as required by montgomeryReduce.
	// This calls montgomeryReduce directly (rather than via Mul) to keep New
	// within the inlining budget.
	return montgomeryReduce(bits.Mul64(x, rSqModM))
}

// Cmp compares the numerical values of x and y.  Montgomery form does not
// preserve order, hence both are converted first.  This avoids cmp.Compare,
// whose NaN checks push it over the inlining budget.
func (x Element) Cmp(y Element) int {
	a, b := x.ToUint64(), y.ToUint64()
	//
	if a < b {
		return -1
	} else if a > b {
		return 1
	}

	return 0
}

// Half x -> x/2 (mod m).
func (x Element) Half() Element {
	if x[0]%2 == 0 {
		return Element{x[0] / 2}
	}
	// the modulus is less than 2⁶³ so this is safe.
	return Element{(x[0] + Modulus) / 2}
}

// Inverse x -> x⁻¹ (mod m) or 0 if x = 0
func (x Element) Inverse() Element {
	// Algorithm 16 in "Efficient Software-Implementation of Finite Fields with Applications to Cryptography"
	if x[0] == 0 {
		return Element{0}
	}

	u := x[0]
	v := uint64(Modulus)

	var c Element
	// Since x actually contains x.R, we have to multiply the result by R² to get x⁻¹R⁻¹R² = x⁻¹R.
	b := Element{rSqModM}

	for (u != 1) && (v != 1) {
		for u%2 == 0 {
			u /= 2
			b = b.Half()
		}

		for v%2 == 0 {
			v /= 2
			c = c.Half()
		}

		if diff, borrow := bits.Sub64(u, v, 0); borrow == 0 {
			u = diff
			b = b.Sub(c)
		} else {
			v -= u
			c = c.Sub(b)
		}
	}

	if u == 1 {
		return b
	}

	return c
}

// String returns the value of x based 10.
func (x Element) String() string {
	return strconv.FormatUint(x.ToUint64(), 10)
}

// Text returns the value of x in the given base.
func (x Element) Text(base int) string {
	return strconv.FormatUint(x.ToUint64(), base)
}

// Bytes returns the big-endian encoded value of the Element, possibly with leading zeros.
func (x Element) Bytes() []byte {
	res := make([]byte, nbBytes)
	v := x.ToUint64()

	for i := range nbBytes {
		res[i] = byte(v >> ((nbBytes - 1 - i) * 8))
	}

	return res
}

// IsZero checks whether x = 0.
func (x Element) IsZero() bool {
	return x[0] == 0
}

// IsOne checks whether x = 1.
func (x Element) IsOne() bool {
	return x[0] == rModM
}
