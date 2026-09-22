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
package field

import (
	"bytes"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/assert"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/goldilocks"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
)

func init() {
	// make sure the interface is adhered to.
	_ = Element[koalabear.Element](koalabear.Element{})
	_ = Element[bls12_377.Element](bls12_377.Element{})
}

// TestFitsWithin checks FitsWithin reports whether the *numerical* value of an
// element fits a given bitwidth.  Every one of these fields holds its value in
// Montgomery form, so an implementation reading the raw representation reports
// nonsense for small values (e.g. the Montgomery form of 1 is a large limb).
func TestFitsWithin(t *testing.T) {
	// Widths are checked up to each field's bit capacity, since values at or
	// above a field's modulus wrap around.
	fits := func(t *testing.T, name string, fitsWithin func(*big.Int, uint) bool, capacity uint) {
		// Zero fits any width, including zero.
		assert.True(t, fitsWithin(big.NewInt(0), 0), "%s: 0 in u0", name)
		assert.True(t, fitsWithin(big.NewInt(0), capacity), "%s: 0 in u%d", name, capacity)
		// Nothing else fits a zero width.
		assert.False(t, fitsWithin(big.NewInt(1), 0), "%s: 1 in u0", name)
		// Boundary at each width below the field's capacity: 2^k-1 fits k bits,
		// whilst 2^k does not.  Sweeping every width exercises each limb
		// boundary for the multi-limb fields.
		for k := uint(1); k < capacity; k++ {
			at := new(big.Int).Lsh(big.NewInt(1), k)
			below := new(big.Int).Sub(at, big.NewInt(1))
			//
			assert.True(t, fitsWithin(below, k), "%s: %s in u%d", name, below, k)
			assert.False(t, fitsWithin(at, k), "%s: %s in u%d", name, at, k)
			// A value needing k bits does not fit k-1 bits.
			assert.False(t, fitsWithin(below, k-1), "%s: %s in u%d", name, below, k-1)
		}
	}
	//
	fits(t, "koalabear", func(v *big.Int, w uint) bool {
		var e koalabear.Element
		return e.SetBytes(v.Bytes()).FitsWithin(w)
	}, 30)
	//
	fits(t, "gf8209", func(v *big.Int, w uint) bool {
		var e gf8209.Element
		return e.SetBytes(v.Bytes()).FitsWithin(w)
	}, 13)
	//
	fits(t, "gf251", func(v *big.Int, w uint) bool {
		var e gf251.Element
		return e.SetBytes(v.Bytes()).FitsWithin(w)
	}, 7)
	//
	fits(t, "goldilocks", func(v *big.Int, w uint) bool {
		var e goldilocks.Element
		return e.SetBytes(v.Bytes()).FitsWithin(w)
	}, 63)
	// The modulus is 253 bits wide, so 252 is the largest width every value is
	// guaranteed to be representable below.  This is the only field whose values
	// span multiple limbs, hence the only one exercising the wide branches.
	fits(t, "bls12_377", func(v *big.Int, w uint) bool {
		var e bls12_377.Element
		return e.SetBytes(v.Bytes()).FitsWithin(w)
	}, 252)
}

func TestBatchInvert(t *testing.T) {
	s := make(elementArray, 4000)
	sInv := make(elementArray, len(s))
	scratch := make(elementArray, len(s))

	for i := range s {
		s[i] = koalabear.Element{rand.Uint32()}
		if s[i][0] >= koalabear.Modulus {
			s[i][0] = 0 // getting a zero with considerable probability
		}

		sInv[i] = s[i].Inverse()

		copy(scratch[:i], s)
		BatchInvert(scratch[:i])

		for j := range i {
			assert.Equal(t, sInv[j][0], scratch[j][0], "on slice %v, at index %d", s[:i], j)
		}
	}
}

type elementArray []koalabear.Element

func (e elementArray) BitWidth() uint {
	panic("not implemented")
}

func (e elementArray) Get(u uint) koalabear.Element {
	return e[u]
}

func (e elementArray) Len() uint {
	return uint(len(e))
}

func (e elementArray) Decode(uint, *bytes.Buffer) error {
	panic("not implemented")
}

func (e elementArray) Encode(*bytes.Buffer) {
	panic("not implemented")
}

func (e elementArray) Append(t koalabear.Element) {
	panic("not implemented")
}

func (e elementArray) Set(u uint, t koalabear.Element) {
	e[u] = t
}

func (e elementArray) Pad(u uint, u2 uint, t koalabear.Element) array.Array[koalabear.Element] {
	panic("not implemented")
}

func (e elementArray) String() string {
	panic("no implemeented")
}
