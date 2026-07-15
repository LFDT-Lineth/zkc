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
	"encoding/binary"
	"fmt"
	"math/big"
	"math/bits"
	"strconv"
)

// Uint128 represents an unsigned integer backed by a pair of native uint64
// values, where hi holds the most-significant 64 bits and lo the
// least-significant 64 bits.  Operations are implemented directly on the two
// words (using math/bits) so that, unlike a big.Int backing, the common
// arithmetic paths neither allocate nor touch BigInt().
type Uint128 struct {
	hi uint64
	lo uint64
}

var _ Word[Uint128] = Uint128{}

// Add implementation for Word interface.
func (p Uint128) Add(w Uint128) (Uint128, bool) {
	lo, carry := bits.Add64(p.lo, w.lo, 0)
	hi, carry := bits.Add64(p.hi, w.hi, carry)
	//
	return Uint128{hi, lo}, carry != 0
}

// Add64 implementation for Word interface.
func (p Uint128) Add64(w uint64) (Uint128, bool) {
	lo, carry := bits.Add64(p.lo, w, 0)
	hi, carry := bits.Add64(p.hi, 0, carry)
	//
	return Uint128{hi, lo}, carry != 0
}

// AddMod implementation for Word interface.
func (p Uint128) AddMod(w, m Uint128) Uint128 {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Reduce both inputs into [0, m) so their sum is bounded by 2*(m-1) < 2^129
	// and therefore needs at most a single subtraction of m.
	a := p.rem(m)
	b := w.rem(m)
	sum, carry := a.Add(b)
	// A carry means the true sum is >= 2^128 > m, so a reduction is required;
	// otherwise reduce only when the (wrapped) sum is itself >= m.  In the carry
	// case sum < m necessarily holds, and Sub wraps to exactly sum+2^128-m.
	if carry || sum.Cmp(m) >= 0 {
		sum, _ = sum.Sub(m)
	}
	//
	return sum
}

// And implementation for Word interface.
func (p Uint128) And(w Uint128) Uint128 {
	return Uint128{p.hi & w.hi, p.lo & w.lo}
}

// Bandwidth implementation for Word interface.
func (p Uint128) Bandwidth() uint {
	return 128
}

// BigInt implementation for Word interface.
func (p Uint128) BigInt() *big.Int {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[0:8], p.hi)
	binary.BigEndian.PutUint64(buf[8:16], p.lo)
	//
	return new(big.Int).SetBytes(buf[:])
}

// BitLen implementation for Word interface.
func (p Uint128) BitLen() uint {
	if p.hi == 0 {
		return uint(bits.Len64(p.lo))
	}
	//
	return uint(bits.Len64(p.hi)) + 64
}

// Cmp implementation for Word interface.
func (p Uint128) Cmp(o Uint128) int {
	if c := cmp.Compare(p.hi, o.hi); c != 0 {
		return c
	}
	//
	return cmp.Compare(p.lo, o.lo)
}

// Cmp64 implementation for Word interface.
func (p Uint128) Cmp64(o uint64) int {
	if p.hi != 0 {
		return 1
	}
	//
	return cmp.Compare(p.lo, o)
}

// Div implementation for Word interface.
func (p Uint128) Div(w Uint128) Uint128 {
	if w.isZero() {
		panic("division by zero")
	}
	//
	q, _ := p.quoRem(w)
	//
	return q
}

// DwDiv implementation for Word interface.
func (p Uint128) DwDiv(lo, d Uint128) (Uint128, Uint128) {
	if d.isZero() {
		panic("division by zero")
	} else if p.Cmp(d) >= 0 {
		panic("quotient overflow")
	}
	// Fast path: the divisor fits within 64 bits, and hence (given the
	// precondition p < d) so does p.  Chain bits.Div64 across the remaining
	// limbs, each step leaving a remainder strictly less than the divisor.
	if d.hi == 0 {
		q1, r := bits.Div64(p.lo, lo.hi, d.lo)
		q0, r := bits.Div64(r, lo.lo, d.lo)
		//
		return Uint128{q1, q0}, Uint128{0, r}
	}
	// Slow path: bitwise long division over the full 256-bit dividend.  The
	// truncation of the quotient to 128 bits is exact here since p < d.
	return quoRem256by128(p, lo, d)
}

// DwRem implementation for Word interface.
func (p Uint128) DwRem(lo, d Uint128) Uint128 {
	if d.isZero() {
		panic("division by zero")
	}
	// Fast path: the divisor fits within 64 bits.  First reduce the high word
	// (which, unlike for DwDiv, may be arbitrarily large), then chain
	// bits.Rem64 across the low word's limbs.
	if d.hi == 0 {
		r0 := p.rem(d)
		r := bits.Rem64(r0.lo, lo.hi, d.lo)
		r = bits.Rem64(r, lo.lo, d.lo)
		//
		return Uint128{0, r}
	}
	// Slow path: bitwise long division over the full 256-bit dividend.  The
	// remainder is exact regardless of any quotient truncation.
	_, r := quoRem256by128(p, lo, d)
	//
	return r
}

// FitsWithin implementation for Word interface.
func (p Uint128) FitsWithin(bitwidth uint) bool {
	switch {
	case bitwidth >= 128:
		return true
	case bitwidth >= 64:
		return p.hi>>(bitwidth-64) == 0
	default:
		return p.hi == 0 && p.lo>>bitwidth == 0
	}
}

// Mul implementation for Word interface.
func (p Uint128) Mul(w Uint128) (hi, lo Uint128) {
	// Schoolbook 128x128 -> 256 bit multiply built from four 64x64 -> 128 bit
	// partial products.  Each bits.Mul64 returns its result as (high, low) word.
	a1, a0 := bits.Mul64(p.lo, w.lo) // contributes at 2^0
	b1, b0 := bits.Mul64(p.lo, w.hi) // contributes at 2^64
	c1, c0 := bits.Mul64(p.hi, w.lo) // contributes at 2^64
	d1, d0 := bits.Mul64(p.hi, w.hi) // contributes at 2^128
	// Word 0 (bits 0..63).
	r0 := a0
	// Word 1 (bits 64..127) = a1 + b0 + c0.
	r1, k0 := bits.Add64(a1, b0, 0)
	r1, k1 := bits.Add64(r1, c0, 0)
	// Word 2 (bits 128..191) = b1 + c1 + d0 + carries out of word 1.
	r2, k2 := bits.Add64(b1, c1, 0)
	r2, k3 := bits.Add64(r2, d0, 0)
	r2, k4 := bits.Add64(r2, k0+k1, 0)
	// Word 3 (bits 192..255) = d1 + carries out of word 2.  This cannot overflow
	// since the full product is bounded by (2^128-1)^2 < 2^256.
	r3 := d1 + k2 + k3 + k4
	//
	return Uint128{r3, r2}, Uint128{r1, r0}
}

// MulMod implementation for Word interface.
func (p Uint128) MulMod(w, m Uint128) Uint128 {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Form the full 256-bit product, then reduce it modulo m.
	hi, lo := p.Mul(w)
	//
	return hi.DwRem(lo, m)
}

// Not implementation for Word interface.
func (p Uint128) Not(bitwidth uint) Uint128 {
	return Uint128{^p.hi, ^p.lo}.mask(bitwidth)
}

// Or implementation for Word interface.
func (p Uint128) Or(w Uint128) Uint128 {
	return Uint128{p.hi | w.hi, p.lo | w.lo}
}

// Rem implementation for Word interface.
func (p Uint128) Rem(w Uint128) Uint128 {
	if w.isZero() {
		panic("division by zero")
	}
	//
	_, r := p.quoRem(w)
	//
	return r
}

// Shl implementation for Word interface.
func (p Uint128) Shl(width uint, n Uint128) Uint128 {
	// A shift of 128 or more bits leaves nothing within the word's width.
	if n.hi != 0 || n.lo >= 128 {
		return Uint128{}
	}
	//
	return p.shl128(uint(n.lo)).mask(width)
}

// Shl64 implementation for Word interface.
func (p Uint128) Shl64(n uint64) (hi Uint128, lo Uint128) {
	switch {
	case n == 0:
		return Uint128{}, p
	case n < 128:
		// Low 128 bits of (p << n); the bits shifted past bit 127 form hi.
		return p.Shr64(128 - n), p.shl128(uint(n))
	case n < 256:
		// Everything lands at or above bit 128.
		return p.shl128(uint(n - 128)), Uint128{}
	default:
		return Uint128{}, Uint128{}
	}
}

// Shr implementation for Word interface.
func (p Uint128) Shr(n Uint128) Uint128 {
	if n.hi != 0 {
		return Uint128{}
	}
	//
	return p.Shr64(n.lo)
}

// Shr64 implementation for Word interface.
func (p Uint128) Shr64(n uint64) Uint128 {
	switch {
	case n == 0:
		return p
	case n < 64:
		return Uint128{p.hi >> n, (p.lo >> n) | (p.hi << (64 - n))}
	case n < 128:
		return Uint128{0, p.hi >> (n - 64)}
	default:
		return Uint128{}
	}
}

// Slice implementation for Word interface.
func (p Uint128) Slice(width uint) Uint128 {
	return p.mask(width)
}

// SetBigInt implementation for Word interface; panics if the value is negative
// or does not fit within 128 bits.
func (p Uint128) SetBigInt(val *big.Int) Uint128 {
	if val.Sign() < 0 {
		panic("cannot assign negative integer")
	} else if val.BitLen() > 128 {
		panic(fmt.Sprintf("value 0x%s exceeds uint128 bandwidth", val.Text(16)))
	}
	//
	var buf [16]byte
	val.FillBytes(buf[:])
	//
	return Uint128{
		hi: binary.BigEndian.Uint64(buf[0:8]),
		lo: binary.BigEndian.Uint64(buf[8:16]),
	}
}

// SetUint64 implementation for Word interface.
func (p Uint128) SetUint64(val uint64) Uint128 {
	return Uint128{0, val}
}

// Sub implementation for Word interface.
func (p Uint128) Sub(w Uint128) (Uint128, bool) {
	lo, borrow := bits.Sub64(p.lo, w.lo, 0)
	hi, borrow := bits.Sub64(p.hi, w.hi, borrow)
	//
	return Uint128{hi, lo}, borrow != 0
}

// Sub64 implementation for Word interface.
func (p Uint128) Sub64(w uint64) (Uint128, bool) {
	lo, borrow := bits.Sub64(p.lo, w, 0)
	hi, borrow := bits.Sub64(p.hi, 0, borrow)
	//
	return Uint128{hi, lo}, borrow != 0
}

// SubMod implementation for Word interface.
func (p Uint128) SubMod(w, m Uint128) Uint128 {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Reduce inputs into [0, m) so that the difference, taken modulo m, fits
	// naturally into a Uint128.
	a := p.rem(m)
	b := w.rem(m)
	//
	if a.Cmp(b) >= 0 {
		d, _ := a.Sub(b)
		return d
	}
	// a < b, so (a-b) mod m == m - (b-a).
	d, _ := b.Sub(a)
	res, _ := m.Sub(d)
	//
	return res
}

// Uint64 implementation for Word interface; panics if the value does not fit
// within 64 bits.
func (p Uint128) Uint64() uint64 {
	if p.hi != 0 {
		panic(fmt.Sprintf("word cannot be expressed as uint64 (0x%s)", p.Text(16)))
	}
	//
	return p.lo
}

// Xor implementation for Word interface.
func (p Uint128) Xor(w Uint128) Uint128 {
	return Uint128{p.hi ^ w.hi, p.lo ^ w.lo}
}

// Text implementation for Word interface.
func (p Uint128) Text(base int) string {
	// Fast path: a single word value formats without touching big.Int.
	if p.hi == 0 {
		return strconv.FormatUint(p.lo, base)
	}
	//
	return p.BigInt().Text(base)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p Uint128) GobEncode() ([]byte, error) {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[0:8], p.hi)
	binary.BigEndian.PutUint64(buf[8:16], p.lo)
	//
	return buf[:], nil
}

// nolint
func (p *Uint128) GobDecode(data []byte) error {
	if len(data) != 16 {
		return fmt.Errorf("invalid uint128 gob encoding: expected 16 bytes, got %d", len(data))
	}
	//
	p.hi = binary.BigEndian.Uint64(data[0:8])
	p.lo = binary.BigEndian.Uint64(data[8:16])
	//
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// isZero returns true when this word is zero.
func (p Uint128) isZero() bool {
	return p.hi == 0 && p.lo == 0
}

// bitLen returns the number of bits required to represent this word (i.e. the
// index of the most-significant set bit plus one, or zero when the word is
// zero).
func (p Uint128) bitLen() int {
	if p.hi != 0 {
		return 64 + bits.Len64(p.hi)
	}
	//
	return bits.Len64(p.lo)
}

// mask returns the low `bitwidth` bits of this word, clearing everything at or
// above bit `bitwidth`.
func (p Uint128) mask(bitwidth uint) Uint128 {
	switch {
	case bitwidth >= 128:
		return p
	case bitwidth >= 64:
		return Uint128{p.hi & mask64(bitwidth-64), p.lo}
	default:
		return Uint128{0, p.lo & mask64(bitwidth)}
	}
}

// shl128 shifts this word left by k bits (0 <= k < 128), keeping only the low
// 128 bits of the result.
func (p Uint128) shl128(k uint) Uint128 {
	switch {
	case k == 0:
		return p
	case k < 64:
		return Uint128{(p.hi << k) | (p.lo >> (64 - k)), p.lo << k}
	default: // 64 <= k < 128
		return Uint128{p.lo << (k - 64), 0}
	}
}

// rem returns this word reduced modulo m (assuming m is non-zero).
func (p Uint128) rem(m Uint128) Uint128 {
	_, r := p.quoRem(m)
	//
	return r
}

// quoRem returns the quotient and remainder of dividing this word by v.  The
// caller must ensure v is non-zero.
func (p Uint128) quoRem(v Uint128) (q, r Uint128) {
	// Fast path: the divisor fits within 64 bits.
	if v.hi == 0 {
		d := v.lo
		if p.hi == 0 {
			return Uint128{0, p.lo / d}, Uint128{0, p.lo % d}
		}
		// 128-bit / 64-bit division.  bits.Div64 requires its high word to be
		// strictly less than the divisor, which holds since r0 = p.hi % d < d.
		qHi := p.hi / d
		r0 := p.hi % d
		qLo, r1 := bits.Div64(r0, p.lo, d)
		//
		return Uint128{qHi, qLo}, Uint128{0, r1}
	}
	// The divisor exceeds 64 bits, so the quotient is guaranteed to fit within
	// 64 bits.  Anything smaller than the divisor divides to zero.
	if p.Cmp(v) < 0 {
		return Uint128{}, p
	}
	// Shift-subtract long division.  The alignment shift is at most 63 (since v
	// has at least 65 significant bits and p at most 128), so the whole quotient
	// fits in a single word.
	shift := p.bitLen() - v.bitLen()
	vs := v.shl128(uint(shift))
	r = p
	//
	var quo uint64
	//
	for i := shift; i >= 0; i-- {
		if r.Cmp(vs) >= 0 {
			r, _ = r.Sub(vs)
			quo |= uint64(1) << uint(i)
		}
		//
		vs = vs.Shr64(1)
	}
	//
	return Uint128{0, quo}, r
}

// quoRem256by128 divides the 256-bit value (hi*2^128 + lo) by m, returning
// the low 128 bits of the quotient along with the remainder (in [0, m)).  The
// caller must ensure m is non-zero.  Note that the quotient is truncated to
// 128 bits; it is exact only when hi < m.  This is a bitwise long division:
// the running remainder is always strictly less than m, so the only state
// that can spill past bit 127 when it is doubled is captured by the carry bit
// and folded back in via the comparison against m.
func quoRem256by128(hi, lo, m Uint128) (q, r Uint128) {
	// Locate the most-significant set bit of the 256-bit dividend so that we
	// start from there rather than always iterating all 256 positions.
	var top int
	//
	switch {
	case hi.hi != 0:
		top = 192 + bits.Len64(hi.hi) - 1
	case hi.lo != 0:
		top = 128 + bits.Len64(hi.lo) - 1
	case lo.hi != 0:
		top = 64 + bits.Len64(lo.hi) - 1
	case lo.lo != 0:
		top = bits.Len64(lo.lo) - 1
	default:
		return Uint128{}, Uint128{}
	}
	//
	for i := top; i >= 0; i-- {
		// r = (r << 1) | bit_i, remembering whether the doubling overflowed bit
		// 127.
		carryOut := r.hi >> 63
		r = Uint128{(r.hi << 1) | (r.lo >> 63), (r.lo << 1) | bitAt256(hi, lo, i)}
		// A carry means the true (129-bit) value is >= 2^128 > m and so always
		// needs reducing; in that case Sub wraps to exactly value-m.
		if carryOut != 0 || r.Cmp(m) >= 0 {
			r, _ = r.Sub(m)
			// Each subtraction corresponds to quotient bit i, of which only the
			// low 128 are representable.
			switch {
			case i >= 64:
				q.hi |= uint64(1) << uint(i-64)
			default:
				q.lo |= uint64(1) << uint(i)
			}
		}
	}
	//
	return q, r
}

// bitAt256 returns bit i (as 0 or 1) of the 256-bit value (hi*2^128 + lo).
func bitAt256(hi, lo Uint128, i int) uint64 {
	switch {
	case i >= 192:
		return (hi.hi >> uint(i-192)) & 1
	case i >= 128:
		return (hi.lo >> uint(i-128)) & 1
	case i >= 64:
		return (lo.hi >> uint(i-64)) & 1
	default:
		return (lo.lo >> uint(i)) & 1
	}
}
