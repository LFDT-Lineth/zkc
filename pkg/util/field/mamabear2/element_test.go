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
package mamabear2

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/assert"
)

// samples returns operands covering the boundaries of the field (0, 1, m-1)
// and of the uint64 domain (m, values near 2⁶⁴), followed by random values
// across the full uint64 range.
func samples(n int) []uint64 {
	vals := []uint64{0, 1, 2, Modulus - 2, Modulus - 1, Modulus, Modulus + 1,
		1 << 48, 1<<49 - 1, 1 << 49, 2*Modulus - 1, math.MaxUint64 - 1, math.MaxUint64}
	//
	for range n {
		vals = append(vals, rand.Uint64N(Modulus), rand.Uint64())
	}
	//
	return vals
}

// check that a binary operation agrees with the corresponding big.Int
// operation (mod m), for all pairs of sampled operands.
func checkBinop(t *testing.T, name string, op func(x, y Element) Element,
	bop func(z, x, y *big.Int) *big.Int) {
	//
	var (
		m        = new(big.Int).SetUint64(Modulus)
		vals     = samples(100)
		bx, by   big.Int
		expected big.Int
	)
	//
	for _, a := range vals {
		for _, b := range vals {
			bx.SetUint64(a)
			by.SetUint64(b)
			bop(&expected, &bx, &by).Mod(&expected, m)
			//
			actual := op(New(a), New(b))
			assert.Equal(t, expected.Uint64(), actual.Uint64(), "%d %s %d", a, name, b)
			// Representation must be canonical (fully reduced)
			assert.True(t, actual[0] < Modulus, "%d %s %d not reduced", a, name, b)
		}
	}
}

func TestAdd(t *testing.T) {
	checkBinop(t, "+", Element.Add, (*big.Int).Add)
}

func TestSub(t *testing.T) {
	checkBinop(t, "-", Element.Sub, (*big.Int).Sub)
}

func TestMul(t *testing.T) {
	checkBinop(t, "*", Element.Mul, (*big.Int).Mul)
}

func TestSetUint64(t *testing.T) {
	for _, a := range samples(10000) {
		x := New(a)
		assert.Equal(t, a%Modulus, x.Uint64(), "%d", a)
		assert.True(t, x[0] < Modulus, "%d not reduced", a)
		assert.Equal(t, x, Element{}.SetUint64(a))
	}
}

func TestInverse(t *testing.T) {
	var (
		m        = new(big.Int).SetUint64(Modulus)
		expected big.Int
	)
	//
	assert.Equal(t, New(0), New(0).Inverse())
	//
	for _, a := range samples(100000) {
		if a%Modulus == 0 {
			continue
		}
		//
		expected.SetUint64(a).ModInverse(&expected, m)
		x := New(a).Inverse()
		assert.Equal(t, expected.Uint64(), x.Uint64(), "inverse of %d", a)
		assert.True(t, New(a).Mul(x).IsOne(), "%d * %d⁻¹ != 1", a, a)
	}
}

func TestHalf(t *testing.T) {
	for _, a := range samples(10000) {
		x := New(a)
		assert.Equal(t, x, x.Half().Add(x.Half()), "halving of %d", a)
	}
}

func TestCmp(t *testing.T) {
	vals := samples(100)
	//
	for _, a := range vals {
		for _, b := range vals {
			ra, rb := a%Modulus, b%Modulus
			x, y := New(a), New(b)
			//
			switch {
			case ra < rb:
				assert.Equal(t, -1, x.Cmp(y), "%d cmp %d", a, b)
			case ra > rb:
				assert.Equal(t, 1, x.Cmp(y), "%d cmp %d", a, b)
			default:
				assert.Equal(t, 0, x.Cmp(y), "%d cmp %d", a, b)
			}
			//
			assert.Equal(t, ra == rb, x.Equals(y), "%d equals %d", a, b)
		}
		// Cmp64 against a value beyond the field
		assert.Equal(t, -1, New(a).Cmp64(Modulus), "%d cmp64 m", a)
	}
}

func TestByteConversion(t *testing.T) {
	var (
		m = new(big.Int).SetUint64(Modulus)
		v big.Int
	)
	//
	for _, a := range samples(10000) {
		// element to bytes
		x := New(a)
		expected := v.SetUint64(a % Modulus).FillBytes(make([]byte, nbBytes))
		assert.Equal(t, expected, x.Bytes())
		// bytes to element
		assert.Equal(t, x, Element{}.SetBytes(v.SetUint64(a).Bytes()))
		assert.Equal(t, x, Element{}.SetBytes(expected))
	}
	// Values wider than 64 bits are reduced modulo m.
	for range 10000 {
		v.SetUint64(rand.Uint64()).Lsh(&v, 64).Add(&v, new(big.Int).SetUint64(rand.Uint64()))
		x := Element{}.SetBytes(v.Bytes())
		assert.Equal(t, new(big.Int).Mod(&v, m).Uint64(), x.Uint64(), "%s", v.String())
	}
	// Empty and zero-padded inputs
	assert.Equal(t, New(0), Element{}.SetBytes(nil))
	assert.Equal(t, New(1), Element{}.SetBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
}

func TestText(t *testing.T) {
	assert.Equal(t, "562932773552128", New(Modulus-1).String())
	assert.Equal(t, "1fffc00000000", New(Modulus-1).Text(16))
	assert.Equal(t, "562932773552128", New(Modulus-1).BigInt().String())
}

func TestZeroOne(t *testing.T) {
	zero := New(0)
	one := New(1)

	assert.True(t, zero.IsZero())
	assert.False(t, one.IsZero())
	assert.False(t, zero.IsOne())
	assert.True(t, one.IsOne())
	assert.True(t, New(Modulus).IsZero())
	assert.True(t, New(Modulus+1).IsOne())

	assert.Equal(t, zero, zero.Add(zero))
	assert.Equal(t, one, zero.Add(one))
	assert.Equal(t, one, one.Mul(one))
	assert.Equal(t, zero, one.Mul(zero))
}
