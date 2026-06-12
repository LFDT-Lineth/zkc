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
package word

import (
	"cmp"
	"fmt"
	"math"
	"math/big"
	"math/bits"

	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
)

// Uint represents an unbound unsigned integer.
type Uint struct {
	value big.Int
}

// And implementation for Word interface.
func (p Uint) And(w Uint) Uint {
	var res big.Int
	res.And(&p.value, &w.value)
	//
	return Uint{res}
}

// Add implementation for Word interface.
func (p Uint) Add(w Uint) (Uint, bool) {
	var res big.Int
	//
	res.Add(&p.value, &w.value)
	//
	return Uint{res}, false
}

// AddMod implementation for Word interface.
func (p Uint) AddMod(w, m Uint) Uint {
	var res big.Int
	//
	res.Add(&p.value, &w.value)
	res.Mod(&res, &m.value)
	//
	return Uint{res}
}

// Bandwidth implementation for Word interface.
func (p Uint) Bandwidth() uint {
	return math.MaxUint
}

// BigInt implementation for Word interface.
func (p Uint) BigInt() *big.Int {
	return &p.value
}

// Cmp implementation for Word interface.
func (p Uint) Cmp(o Uint) int {
	return p.value.Cmp(&o.value)
}

// Cmp64 implementation for Word interface.
func (p Uint) Cmp64(o uint64) int {
	if p.value.IsUint64() {
		return cmp.Compare(p.value.Uint64(), o)
	}
	//
	return 1
}

// Div implementation for Word interface.
func (p Uint) Div(w Uint) Uint {
	if w.value.Sign() == 0 {
		panic("division by zero")
	}
	//
	var res big.Int
	res.Div(&p.value, &w.value)
	//
	return Uint{res}
}

// DwMul implementation for Word interface.
func (p Uint) DwMul(w Uint) (lo, hi Uint) {
	panic("todo")
}

// DwShr64 implementation for Word interface.
func (p Uint) DwShr64(w Uint, n uint64) (lo, hi Uint) {
	lo = p.Shr64(n)
	hi = w.Shr64(n)
	lo = lo.Or(w.Slice(uint(n)))
	//
	return lo, hi
}

// FitsWithin implementation for Word interface.
func (p Uint) FitsWithin(bitwidth uint) bool {
	return uint(p.value.BitLen()) <= bitwidth
}

// Not implementation for Word interface.
func (p Uint) Not(bitwidth uint) Uint {
	// Compute bitwise complement within width: (2^width - 1) XOR value
	var (
		mask = new(big.Int)
		res  big.Int
	)
	// (1 << 2^n) - 1
	mask.Lsh(&one, bitwidth)
	mask.Sub(mask, &one)
	//
	res.Xor(&p.value, mask)
	//
	return Uint{res}
}

// Or implementation for Word interface.
func (p Uint) Or(w Uint) Uint {
	var res big.Int
	res.Or(&p.value, &w.value)
	//
	return Uint{res}
}

// Mul implementation for Word interface.
func (p Uint) Mul(w Uint) (Uint, bool) {
	var (
		res big.Int
	)
	res.Mul(&p.value, &w.value)
	//
	return Uint{res}, false
}

// MulMod implementation for Word interface.
func (p Uint) MulMod(w, m Uint) Uint {
	var res big.Int
	//
	res.Mul(&p.value, &w.value)
	res.Mod(&res, &m.value)
	//
	return Uint{res}
}

// Rem implementation for Word interface.
func (p Uint) Rem(w Uint) Uint {
	if w.value.Sign() == 0 {
		panic("division by zero")
	}
	//
	var res big.Int
	res.Mod(&p.value, &w.value)
	//
	return Uint{res}
}

// Shl implementation for Word interface.
func (p Uint) Shl(width uint, n Uint) Uint {
	var res big.Int
	res.Lsh(&p.value, uint(n.Uint64()))
	// Mask result to width bits.
	mask := new(big.Int).Sub(util_math.Pow2(width), big.NewInt(1))
	res.And(&res, mask)
	//
	return Uint{res}
}

// Shl64 implementation for Word interface.
func (p Uint) Shl64(n uint64) Uint {
	var res big.Int
	res.Lsh(&p.value, uint(n))
	//
	return Uint{res}
}

// Shr implementation for Word interface.
func (p Uint) Shr(n Uint) Uint {
	var res big.Int
	res.Rsh(&p.value, uint(n.Uint64()))
	//
	return Uint{res}
}

// Shr64 implementation for Word interface.
func (p Uint) Shr64(n uint64) Uint {
	var val big.Int
	val.Rsh(&p.value, uint(n))
	//
	return Uint{val}
}

// Slice implementation for Word interface.
func (p Uint) Slice(width uint) Uint {
	return Uint{lowBits(width, p.value)}
}

// Uint64 implementation for Word interface.
func (p Uint) Uint64() uint64 {
	if p.value.IsUint64() {
		return p.value.Uint64()
	}
	//
	panic(fmt.Sprintf("word cannot be expressed as uint64 (0x%s)", p.value.Text(16)))
}

// SetUint64 assigns a given big integer to this unsigned integer.
func (p Uint) SetUint64(val uint64) Uint {
	var w big.Int
	w.SetUint64(val)
	//
	return Uint{w}
}

// SetBigInt assigns a given big integer to this unsigned integer; observe that
// this will panic if the given big integer is negative.
func (p Uint) SetBigInt(val *big.Int) Uint {
	// Sanity check
	if val.Sign() < 0 {
		panic("cannot assign negatve integer")
	}
	// Assign
	p.value = *val

	return p
}

// Sub implementation for Word interface.
func (p Uint) Sub(w Uint) (Uint, bool) {
	var (
		res big.Int
	)
	//
	res.Sub(&p.value, &w.value)
	//
	return Uint{res}, res.Sign() < 0
}

// SubMod implementation for Word interface.
func (p Uint) SubMod(w, m Uint) Uint {
	var res big.Int
	//
	res.Sub(&p.value, &w.value)
	res.Mod(&res, &m.value)
	//
	return Uint{res}
}

// Xor implementation for Word interface.
func (p Uint) Xor(w Uint) Uint {
	var res big.Int
	res.Xor(&p.value, &w.value)
	//
	return Uint{res}
}

// Text implementation for Word interface
func (p Uint) Text(base int) string {
	return p.value.Text(base)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p Uint) GobEncode() ([]byte, error) {
	return p.value.GobEncode()
}

// nolint
func (p *Uint) GobDecode(data []byte) error {
	return p.value.GobDecode(data)
}

// lowBits returns the low `width` bits of value (i.e. value mod 2^width).
// For example, given value 10111000 and width=4 the result is 1000.
func lowBits(width uint, value big.Int) big.Int {
	var slice big.Int
	// Fast paths: the result fits within a single uint64.
	if width <= 64 {
		if value.IsUint64() {
			slice.SetUint64(value.Uint64() & mask64(width))
			return slice
		}
		// For a multi-word value, the least-significant machine word already
		// holds every bit we keep, so there's no need to touch the high words.
		if width <= bits.UintSize {
			slice.SetUint64(uint64(value.Bits()[0]) & mask64(width))
			return slice
		}
	}
	// General path: mask off everything at or above bit `width`.
	mask := new(big.Int).Lsh(&one, width)
	mask.Sub(mask, &one)
	slice.And(&value, mask)
	//
	return slice
}

var one big.Int

func init() {
	one = *big.NewInt(1)
}
