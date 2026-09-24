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
package goldilocks

import (
	"math/big"
	"math/bits"
	"math/rand/v2"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/assert"
)

// MODULUS is the Goldilocks prime, p = 2^64 - 2^32 + 1.
const MODULUS uint64 = 1<<64 - 1<<32 + 1

// new constructs an Element holding the given value.  Note this deliberately
// goes through SetUint64 (rather than the embedded limb) since the internal
// representation is Montgomery encoded.
func newElement(val uint64) Element {
	var e Element
	return e.SetUint64(val)
}

// randomValue samples a value uniformly from [0,p).
func randomValue() uint64 {
	return rand.Uint64N(MODULUS)
}

// bigModulus returns p as a big.Int.
func bigModulus() *big.Int {
	return new(big.Int).SetUint64(MODULUS)
}

// TestUint64RoundTrip checks that a value survives SetUint64 / Uint64 intact.
// This is the property the rest of the pipeline relies upon when moving values
// between the interpreter and the field, and it is not obviously true given the
// Montgomery encoding.
func TestUint64RoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 2, 255, 256, 1 << 31, 1 << 32, MODULUS - 1} {
		assert.Equal(t, v, newElement(v).Uint64(), "round trip of %d", v)
	}
	//
	for range 10000 {
		v := randomValue()
		assert.Equal(t, v, newElement(v).Uint64(), "round trip of %d", v)
	}
}

// TestAdd checks addition against big.Int arithmetic mod p, including the
// wraparound cases where the Montgomery representation must carry.
func TestAdd(t *testing.T) {
	var i, j big.Int
	//
	m := bigModulus()
	//
	for range 10000 {
		a, b := randomValue(), randomValue()
		//
		i.SetUint64(a).Add(&i, j.SetUint64(b)).Mod(&i, m)
		//
		actual := newElement(a).Add(newElement(b))
		//
		assert.Equal(t, i.Uint64(), actual.Uint64(), "%d + %d", a, b)
	}
	// p-1 + 1 wraps to zero.
	assert.True(t, newElement(MODULUS-1).Add(newElement(1)).IsZero())
}

// TestSub checks subtraction against big.Int arithmetic mod p, including the
// borrow cases where a < b.
func TestSub(t *testing.T) {
	var i, j big.Int
	//
	m := bigModulus()
	//
	for range 10000 {
		a, b := randomValue(), randomValue()
		//
		i.SetUint64(a).Sub(&i, j.SetUint64(b)).Mod(&i, m)
		//
		actual := newElement(a).Sub(newElement(b))
		//
		assert.Equal(t, i.Uint64(), actual.Uint64(), "%d - %d", a, b)
	}
	// 0 - 1 underflows to p-1.
	assert.Equal(t, MODULUS-1, newElement(0).Sub(newElement(1)).Uint64())
}

// TestMul checks multiplication against big.Int arithmetic mod p.  This is the
// operation most sensitive to the Montgomery encoding, since the wrapper must
// not apply a spurious extra reduction factor.
func TestMul(t *testing.T) {
	var i, j big.Int
	//
	m := bigModulus()
	//
	for range 10000 {
		a, b := randomValue(), randomValue()
		//
		i.SetUint64(a).Mul(&i, j.SetUint64(b)).Mod(&i, m)
		//
		actual := newElement(a).Mul(newElement(b))
		//
		assert.Equal(t, i.Uint64(), actual.Uint64(), "%d * %d", a, b)
	}
}

// TestInverse checks that x⁻¹ is a genuine multiplicative inverse, and that
// zero inverts to zero (as the Element interface requires, rather than
// panicking or yielding garbage).
func TestInverse(t *testing.T) {
	one := newElement(1)
	//
	for range 10000 {
		// avoid zero, handled separately below.
		a := randomValue()
		if a == 0 {
			continue
		}
		//
		x := newElement(a)
		// x * x⁻¹ = 1
		assert.Equal(t, one, x.Mul(x.Inverse()), "inverse of %d", a)
	}
	// Inverse of zero is zero.
	assert.True(t, newElement(0).Inverse().IsZero())
}

// TestCmp checks that comparison follows the numerical ordering of the values,
// rather than the ordering of their Montgomery representations.
func TestCmp(t *testing.T) {
	for range 10000 {
		a, b := randomValue(), randomValue()
		//
		var expected int
		//
		switch {
		case a > b:
			expected = 1
		case a < b:
			expected = -1
		}
		//
		assert.Equal(t, expected, newElement(a).Cmp(newElement(b)), "cmp %d and %d", a, b)
	}
}

// TestCmp64 checks Cmp64 agrees with the numerical ordering against a raw
// uint64.
func TestCmp64(t *testing.T) {
	for range 10000 {
		a, b := randomValue(), randomValue()
		//
		var expected int
		//
		switch {
		case a > b:
			expected = 1
		case a < b:
			expected = -1
		}
		//
		assert.Equal(t, expected, newElement(a).Cmp64(b), "cmp64 %d and %d", a, b)
	}
}

// TestFitsWithin checks FitsWithin reports whether the *numerical* value fits
// the given bitwidth.  The Montgomery representation of a small value is a
// large limb, so an implementation derived from the raw limb (e.g. via
// goldilocks.BitLen, which does not convert) reports nonsense here.
func TestFitsWithin(t *testing.T) {
	// Zero fits anything, including a zero bitwidth.
	assert.True(t, newElement(0).FitsWithin(0))
	assert.True(t, newElement(0).FitsWithin(32))
	// Nothing else fits a zero bitwidth.
	assert.False(t, newElement(1).FitsWithin(0))
	// Every element fits the full 64-bit width.
	assert.True(t, newElement(MODULUS-1).FitsWithin(64))
	// Check the boundary at each width: 2^k-1 fits k bits, 2^k does not.
	for k := uint(1); k < 64; k++ {
		below := uint64(1)<<k - 1
		at := uint64(1) << k
		//
		assert.True(t, newElement(below).FitsWithin(k), "%d in u%d", below, k)
		assert.False(t, newElement(at).FitsWithin(k), "%d in u%d", at, k)
	}
	// Random values agree with the bit length of the value itself.
	for range 10000 {
		v := randomValue()
		width := uint(bits.Len64(v))
		//
		assert.True(t, newElement(v).FitsWithin(width), "%d in u%d", v, width)
		//
		if width > 0 {
			assert.False(t, newElement(v).FitsWithin(width-1), "%d in u%d", v, width-1)
		}
	}
}

// TestByteRoundTrip checks that Bytes / SetBytes round trips, and that the
// encoding is the fixed 8-byte width the serialisation layer expects.
func TestByteRoundTrip(t *testing.T) {
	var e Element
	//
	for range 10000 {
		v := randomValue()
		x := newElement(v)
		bytes := x.Bytes()
		//
		assert.Equal(t, 8, len(bytes), "byte width of %d", v)
		assert.True(t, e.SetBytes(bytes).Equals(x), "byte round trip of %d", v)
	}
}

// TestBigInt checks BigInt reports the numerical value, not the Montgomery
// representation.  Several callers (e.g. the field cast lowering) compare this
// against the modulus, so a Montgomery leak here would be silently wrong.
func TestBigInt(t *testing.T) {
	m := bigModulus()
	//
	for range 10000 {
		v := randomValue()
		actual := newElement(v).BigInt()
		//
		assert.Equal(t, 0, actual.Cmp(new(big.Int).SetUint64(v)), "bigint of %d", v)
		// Every field element is canonical, i.e. strictly below p.
		assert.True(t, actual.Cmp(m) < 0, "bigint of %d exceeds modulus", v)
	}
}

// TestText checks the textual representation is the numerical value in the
// requested base.
func TestText(t *testing.T) {
	for range 1000 {
		v := randomValue()
		x := newElement(v)
		//
		for _, base := range []int{2, 10, 16} {
			expected := new(big.Int).SetUint64(v).Text(base)
			assert.Equal(t, expected, x.Text(base), "text base %d of %d", base, v)
		}
	}
}

// TestZeroOne checks the additive and multiplicative identities, and that the
// zero value of the struct is the field's zero (relied upon wherever an Element
// is declared via `var`).
func TestZeroOne(t *testing.T) {
	var uninitialised Element
	//
	zero := newElement(0)
	one := newElement(1)
	//
	assert.True(t, uninitialised.IsZero())
	assert.True(t, zero.IsZero())
	assert.False(t, one.IsZero())
	assert.False(t, zero.IsOne())
	assert.True(t, one.IsOne())
	//
	assert.Equal(t, zero, zero.Add(zero))
	assert.Equal(t, one, zero.Add(one))
	assert.Equal(t, one, one.Mul(one))
	assert.Equal(t, zero, one.Mul(zero))
}

// TestEqualsHash checks Equals distinguishes distinct values, and that Hash is
// consistent with Equals.  Lookup arguments depend on both: equal values must
// hash equally for correctness, and distinct values must generally hash
// distinctly or the lookup degrades to a linear scan.
func TestEqualsHash(t *testing.T) {
	// Records the value previously seen for each hash, so that a hash shared
	// between two distinct values can be reported.
	hashes := make(map[uint64]uint64)
	//
	for range 10000 {
		a, b := randomValue(), randomValue()
		x, y := newElement(a), newElement(b)
		// Equals tracks numerical equality.
		assert.Equal(t, a == b, x.Equals(y), "equals %d and %d", a, b)
		// Equal values must hash equally.
		if a == b {
			assert.Equal(t, x.Hash(), y.Hash(), "hash of %d and %d", a, b)
		}
		// FNV1a over a single limb is injective here (an xor followed by a
		// multiply by an odd prime, modulo 2^64), so distinct values must not
		// collide.  This also pins against a degenerate implementation which
		// discards the value entirely.
		for _, pair := range [][2]uint64{{a, x.Hash()}, {b, y.Hash()}} {
			val, hash := pair[0], pair[1]
			//
			if prev, ok := hashes[hash]; ok {
				assert.Equal(t, prev, val, "hash collision between %d and %d", prev, val)
			}
			//
			hashes[hash] = val
		}
	}
	// Independently constructed copies of the same value agree.
	x := newElement(12345)
	assert.True(t, x.Equals(newElement(12345)))
	assert.Equal(t, x.Hash(), newElement(12345).Hash())
}

// TestModulus checks the reported modulus is p, and that the Element method
// agrees with the package-level value.  Since both now share one big.Int,
// this also pins that callers observe a consistent value.
func TestModulus(t *testing.T) {
	var e Element
	//
	assert.Equal(t, 0, e.Modulus().Cmp(bigModulus()))
	assert.Equal(t, 0, Modulus.Cmp(bigModulus()))
	// p must be reported identically via either route.
	assert.Equal(t, 0, e.Modulus().Cmp(Modulus))
}
