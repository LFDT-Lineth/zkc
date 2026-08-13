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
	"math/big"
)

// Double represents an unsigned integer of twice the bandwidth of a given
// word type, where hi holds the most-significant limb and lo the
// least-significant limb.  For example, Double[Uint64] is a 128-bit word
// (equivalent to Uint128) and Double[Uint128] is a 256-bit word.  Operations
// are implemented generically in terms of the limb type's own Word
// operations, following the same limb-decomposition approach as Uint128.
type Double[W Word[W]] struct {
	hi, lo W
}

// Add implementation for Word interface.
func (p Double[W]) Add(w Double[W]) (Double[W], bool) {
	lo, carry := p.lo.Add(w.lo)
	hi, overflow := p.hi.Add(w.hi)
	// Propagate any carry out of the low limb into the high limb.
	if carry {
		var o bool
		//
		hi, o = hi.Add64(1)
		overflow = overflow || o
	}
	//
	return Double[W]{hi, lo}, overflow
}

// Add64 implementation for Word interface.
func (p Double[W]) Add64(w uint64) (Double[W], bool) {
	var tmp Double[W]
	return p.Add(tmp.SetUint64(w))
}

// AddMod implementation for Word interface.
func (p Double[W]) AddMod(w, m Double[W]) Double[W] {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Reduce both inputs into [0, m) so their sum is bounded by 2*(m-1) and
	// therefore needs at most a single subtraction of m.
	a := p.Rem(m)
	b := w.Rem(m)
	sum, carry := a.Add(b)
	// A carry means the true sum exceeds the bandwidth (and hence m), so a
	// reduction is required; otherwise reduce only when the (wrapped) sum is
	// itself >= m.  In the carry case sum < m necessarily holds, and Sub wraps
	// to exactly the reduced value.
	if carry || sum.Cmp(m) >= 0 {
		sum, _ = sum.Sub(m)
	}
	//
	return sum
}

// And implementation for Word interface.
func (p Double[W]) And(w Double[W]) Double[W] {
	return Double[W]{p.hi.And(w.hi), p.lo.And(w.lo)}
}

// Bandwidth implementation for Word interface.
func (p Double[W]) Bandwidth() uint {
	return 2 * p.lo.Bandwidth()
}

// BigInt implementation for Word interface.
func (p Double[W]) BigInt() *big.Int {
	res := new(big.Int).Lsh(p.hi.BigInt(), p.lo.Bandwidth())
	//
	return res.Or(res, p.lo.BigInt())
}

// BitLen implementation for Word interface.
func (p Double[W]) BitLen() uint {
	if hiLen := p.hi.BitLen(); hiLen != 0 {
		return hiLen + p.lo.Bandwidth()
	}
	//
	return p.lo.BitLen()
}

// Cmp implementation for Word interface.
func (p Double[W]) Cmp(o Double[W]) int {
	if c := p.hi.Cmp(o.hi); c != 0 {
		return c
	}
	//
	return p.lo.Cmp(o.lo)
}

// Cmp64 implementation for Word interface.
func (p Double[W]) Cmp64(o uint64) int {
	if p.BitLen() > 64 {
		return 1
	}
	//
	return cmp.Compare(p.Uint64(), o)
}

// Div implementation for Word interface.
func (p Double[W]) Div(w Double[W]) Double[W] {
	if w.isZero() {
		panic("division by zero")
	}
	//
	q, _ := p.quoRem(w)
	//
	return q
}

// DwDiv implementation for Word interface.
func (p Double[W]) DwDiv(lo, d Double[W]) (Double[W], Double[W]) {
	if d.isZero() {
		panic("division by zero")
	} else if p.Cmp(d) >= 0 {
		panic("quotient overflow")
	}
	// Fast path: the divisor fits within a single limb, and hence (given the
	// precondition p < d) so does p.  Chain the limb-level DwDiv across the
	// remaining limbs, each step leaving a remainder strictly less than the
	// divisor.
	if d.hi.BitLen() == 0 {
		var zero W
		//
		q1, r := p.lo.DwDiv(lo.hi, d.lo)
		q0, r := r.DwDiv(lo.lo, d.lo)
		//
		return Double[W]{q1, q0}, Double[W]{zero, r}
	}
	// Slow path: bitwise long division over the full 4n-bit dividend.  The
	// truncation of the quotient to 2n bits is exact here since p < d.
	return dwQuoRem(p, lo, d)
}

// DwRem implementation for Word interface.
func (p Double[W]) DwRem(lo, d Double[W]) Double[W] {
	if d.isZero() {
		panic("division by zero")
	}
	// Fast path: the divisor fits within a single limb.  First reduce the high
	// word (which, unlike for DwDiv, may be arbitrarily large), then chain the
	// limb-level DwRem across the low word's limbs.
	if d.hi.BitLen() == 0 {
		var zero W
		//
		r := p.Rem(d).lo.DwRem(lo.hi, d.lo)
		r = r.DwRem(lo.lo, d.lo)
		//
		return Double[W]{zero, r}
	}
	// Slow path: bitwise long division over the full 4n-bit dividend.  The
	// remainder is exact regardless of any quotient truncation.
	_, r := dwQuoRem(p, lo, d)
	//
	return r
}

// FitsWithin implementation for Word interface.
func (p Double[W]) FitsWithin(bitwidth uint) bool {
	if n := p.lo.Bandwidth(); bitwidth >= n {
		return p.hi.FitsWithin(bitwidth - n)
	}
	//
	return p.hi.BitLen() == 0 && p.lo.FitsWithin(bitwidth)
}

// Mul implementation for Word interface.
func (p Double[W]) Mul(w Double[W]) (hi, lo Double[W]) {
	// Schoolbook multiply built from four limb-level partial products, exactly
	// as for Uint128 but expressed over the generic limb type.  Each limb-level
	// Mul returns its result as a (high, low) limb pair.
	a1, a0 := p.lo.Mul(w.lo) // contributes at 2^0
	b1, b0 := p.lo.Mul(w.hi) // contributes at 2^n
	c1, c0 := p.hi.Mul(w.lo) // contributes at 2^n
	d1, d0 := p.hi.Mul(w.hi) // contributes at 2^2n
	// Limb 0 (bits 0..n-1).
	r0 := a0
	// Limb 1 (bits n..2n-1) = a1 + b0 + c0.
	r1, k0 := a1.Add(b0)
	r1, k1 := r1.Add(c0)
	// Limb 2 (bits 2n..3n-1) = b1 + c1 + d0 + carries out of limb 1.
	r2, k2 := b1.Add(c1)
	r2, k3 := r2.Add(d0)
	r2, k4 := r2.Add64(b2u(k0) + b2u(k1))
	// Limb 3 (bits 3n..4n-1) = d1 + carries out of limb 2.  This cannot
	// overflow since the full product is bounded by (2^2n - 1)^2 < 2^4n.
	r3, _ := d1.Add64(b2u(k2) + b2u(k3) + b2u(k4))
	//
	return Double[W]{r3, r2}, Double[W]{r1, r0}
}

// MulMod implementation for Word interface.
func (p Double[W]) MulMod(w, m Double[W]) Double[W] {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Form the full 4n-bit product, then reduce it modulo m.
	hi, lo := p.Mul(w)
	//
	return hi.DwRem(lo, m)
}

// Not implementation for Word interface.
func (p Double[W]) Not(bitwidth uint) Double[W] {
	n := p.lo.Bandwidth()
	//
	if bitwidth <= n {
		var zero W
		return Double[W]{zero, p.lo.Not(bitwidth)}
	}
	//
	return Double[W]{p.hi.Not(bitwidth - n), p.lo.Not(n)}
}

// Or implementation for Word interface.
func (p Double[W]) Or(w Double[W]) Double[W] {
	return Double[W]{p.hi.Or(w.hi), p.lo.Or(w.lo)}
}

// Rem implementation for Word interface.
func (p Double[W]) Rem(w Double[W]) Double[W] {
	if w.isZero() {
		panic("division by zero")
	}
	//
	_, r := p.quoRem(w)
	//
	return r
}

// Shl implementation for Word interface.
func (p Double[W]) Shl(width uint, n Double[W]) Double[W] {
	// A shift of the full bandwidth or more bits leaves nothing within the
	// word's width.
	if n.Cmp64(uint64(2*p.lo.Bandwidth())) >= 0 {
		return Double[W]{}
	}
	//
	return p.shl(n.Uint64()).Slice(width)
}

// Shl64 implementation for Word interface.
func (p Double[W]) Shl64(n uint64) (hi Double[W], lo Double[W]) {
	bandwidth := uint64(2 * p.lo.Bandwidth())
	//
	switch {
	case n == 0:
		return Double[W]{}, p
	case n < bandwidth:
		// Low bits of (p << n); the bits shifted past the bandwidth form hi.
		return p.Shr64(bandwidth - n), p.shl(n)
	case n < 2*bandwidth:
		// Everything lands at or above the bandwidth.
		return p.shl(n - bandwidth), Double[W]{}
	default:
		return Double[W]{}, Double[W]{}
	}
}

// Shr implementation for Word interface.
func (p Double[W]) Shr(n Double[W]) Double[W] {
	// A shift of the full bandwidth or more bits clears the word.
	if n.Cmp64(uint64(2*p.lo.Bandwidth())) >= 0 {
		return Double[W]{}
	}
	//
	return p.Shr64(n.Uint64())
}

// Shr64 implementation for Word interface.
func (p Double[W]) Shr64(n uint64) Double[W] {
	var (
		k    = uint64(p.lo.Bandwidth())
		zero W
	)
	//
	switch {
	case n == 0:
		return p
	case n < k:
		// Bits falling out of the high limb land in the top of the low limb; the
		// limb-level left shift computes them, with its low half already masked
		// to the limb bandwidth.
		_, c := p.hi.Shl64(k - n)
		return Double[W]{p.hi.Shr64(n), p.lo.Shr64(n).Or(c)}
	case n < 2*k:
		return Double[W]{zero, p.hi.Shr64(n - k)}
	default:
		return Double[W]{zero, zero}
	}
}

// Slice implementation for Word interface.
func (p Double[W]) Slice(width uint) Double[W] {
	n := p.lo.Bandwidth()
	//
	if width <= n {
		var zero W
		return Double[W]{zero, p.lo.Slice(width)}
	}
	//
	return Double[W]{p.hi.Slice(width - n), p.lo}
}

// SetBigInt implementation for Word interface; panics if the value is negative
// or does not fit within the double word's bandwidth.
func (p Double[W]) SetBigInt(val *big.Int) Double[W] {
	var (
		n  = p.lo.Bandwidth()
		hi big.Int
	)
	//
	if val.Sign() < 0 {
		panic("cannot assign negative integer")
	} else if uint(val.BitLen()) > 2*n {
		panic(fmt.Sprintf("value 0x%s exceeds double word bandwidth", val.Text(16)))
	}
	//
	lo := lowBits(n, *val)
	hi.Rsh(val, n)
	//
	return Double[W]{p.hi.SetBigInt(&hi), p.lo.SetBigInt(&lo)}
}

// SetUint64 implementation for Word interface.
func (p Double[W]) SetUint64(val uint64) Double[W] {
	var (
		n    = p.lo.Bandwidth()
		zero W
	)
	//
	if n >= 64 {
		return Double[W]{zero, p.lo.SetUint64(val)}
	}
	// The value spans both limbs; the limb-level SetUint64 panics if the high
	// part does not fit.
	return Double[W]{p.hi.SetUint64(val >> n), p.lo.SetUint64(val & mask64(n))}
}

// Sub implementation for Word interface.
func (p Double[W]) Sub(w Double[W]) (Double[W], bool) {
	lo, borrow := p.lo.Sub(w.lo)
	hi, underflow := p.hi.Sub(w.hi)
	// Propagate any borrow out of the low limb into the high limb.
	if borrow {
		var u bool
		//
		hi, u = hi.Sub64(1)
		underflow = underflow || u
	}
	//
	return Double[W]{hi, lo}, underflow
}

// Sub64 implementation for Word interface.
func (p Double[W]) Sub64(w uint64) (Double[W], bool) {
	var tmp Double[W]
	return p.Sub(tmp.SetUint64(w))
}

// SubMod implementation for Word interface.
func (p Double[W]) SubMod(w, m Double[W]) Double[W] {
	if m.isZero() {
		panic("modulus by zero")
	}
	// Reduce inputs into [0, m) so that the difference, taken modulo m, fits
	// naturally into the word.
	a := p.Rem(m)
	b := w.Rem(m)
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
func (p Double[W]) Uint64() uint64 {
	if p.BitLen() > 64 {
		panic(fmt.Sprintf("word cannot be expressed as uint64 (0x%s)", p.Text(16)))
	}
	//
	if p.hi.BitLen() == 0 {
		return p.lo.Uint64()
	}
	// Since the value fits within 64 bits and the high limb is non-zero, the
	// limb bandwidth is necessarily less than 64 here.
	return (p.hi.Uint64() << p.lo.Bandwidth()) | p.lo.Uint64()
}

// Xor implementation for Word interface.
func (p Double[W]) Xor(w Double[W]) Double[W] {
	return Double[W]{p.hi.Xor(w.hi), p.lo.Xor(w.lo)}
}

// Text implementation for Word interface.
func (p Double[W]) Text(base int) string {
	// Fast path: a value contained within the low limb formats directly.
	if p.hi.BitLen() == 0 {
		return p.lo.Text(base)
	}
	//
	return p.BigInt().Text(base)
}

// ============================================================================
// Helpers
// ============================================================================

// isZero returns true when this word is zero.
func (p Double[W]) isZero() bool {
	return p.hi.BitLen() == 0 && p.lo.BitLen() == 0
}

// shl shifts this word left by k bits, keeping only the low bits which fit
// within the word's bandwidth.
func (p Double[W]) shl(k uint64) Double[W] {
	var (
		n    = uint64(p.lo.Bandwidth())
		zero W
	)
	//
	switch {
	case k == 0:
		return p
	case k < n:
		// The bits shifted out of the low limb (returned as the high half of
		// the limb-level shift) carry into the high limb.
		c, lo := p.lo.Shl64(k)
		_, hi := p.hi.Shl64(k)
		//
		return Double[W]{hi.Or(c), lo}
	case k < 2*n:
		_, hi := p.lo.Shl64(k - n)
		return Double[W]{hi, zero}
	default:
		return Double[W]{zero, zero}
	}
}

// quoRem returns the quotient and remainder of dividing this word by v.  The
// caller must ensure v is non-zero.
func (p Double[W]) quoRem(v Double[W]) (q, r Double[W]) {
	// Fast path: the divisor fits within a single limb, so the quotient can be
	// computed limb-wise via the 2n-by-n division primitive (mirroring
	// Uint128.quoRem).
	if v.hi.BitLen() == 0 {
		var zero W
		//
		if p.hi.BitLen() == 0 {
			return Double[W]{zero, p.lo.Div(v.lo)}, Double[W]{zero, p.lo.Rem(v.lo)}
		}
		// The DwDiv precondition holds since rem = p.hi % v.lo < v.lo.
		qHi := p.hi.Div(v.lo)
		rem := p.hi.Rem(v.lo)
		qLo, rem := rem.DwDiv(p.lo, v.lo)
		//
		return Double[W]{qHi, qLo}, Double[W]{zero, rem}
	}
	// The divisor exceeds one limb, so the quotient is guaranteed to fit
	// within a single limb (plus one bit).  Anything smaller than the divisor
	// divides to zero.
	if p.Cmp(v) < 0 {
		return Double[W]{}, p
	}
	// Shift-subtract long division: align the divisor's most-significant bit
	// with the dividend's, then subtract-and-shift back down.  The aligned
	// divisor cannot overflow since p fits within the same bandwidth.
	var (
		shift = p.BitLen() - v.BitLen()
		vs    = v.shl(uint64(shift))
		one   Double[W]
	)
	//
	one = one.SetUint64(1)
	r = p
	//
	for i := int(shift); i >= 0; i-- {
		if r.Cmp(vs) >= 0 {
			r, _ = r.Sub(vs)
			q = q.Or(one.shl(uint64(i)))
		}
		//
		vs = vs.Shr64(1)
	}
	//
	return q, r
}

// dwQuoRem divides the 4n-bit value (hi << 2n | lo) by m, returning the low
// 2n bits of the quotient along with the remainder (in [0, m)).  The caller
// must ensure m is non-zero.  Note that the quotient is truncated to 2n bits;
// it is exact only when hi < m.  This is a bitwise long division (the generic
// analogue of quoRem256by128): the running remainder is always strictly less
// than m, so the only state that can spill past the bandwidth when it is
// doubled is captured by the carry bit and folded back in via the comparison
// against m.
func dwQuoRem[W Word[W]](hi, lo, m Double[W]) (q, r Double[W]) {
	var (
		bandwidth = 2 * m.lo.Bandwidth()
		one       Double[W]
		top       int
	)
	// Locate the most-significant set bit of the dividend so that we start
	// from there rather than always iterating all 4n positions.
	switch {
	case hi.BitLen() != 0:
		top = int(bandwidth+hi.BitLen()) - 1
	case lo.BitLen() != 0:
		top = int(lo.BitLen()) - 1
	default:
		return Double[W]{}, Double[W]{}
	}
	//
	one = one.SetUint64(1)
	//
	for i := top; i >= 0; i-- {
		// r = (r << 1) | bit_i, remembering whether the doubling overflowed the
		// bandwidth.
		carryOut := r.BitLen() == bandwidth
		r = r.shl(1)
		//
		if dwBitAt(hi, lo, i) {
			r = r.Or(one)
		}
		// A carry means the true value is >= 2^2n > m and so always needs
		// reducing; in that case Sub wraps to exactly value-m.
		if carryOut || r.Cmp(m) >= 0 {
			r, _ = r.Sub(m)
			// Each subtraction corresponds to quotient bit i, of which only the
			// low 2n are representable.
			if uint(i) < bandwidth {
				q = q.Or(one.shl(uint64(i)))
			}
		}
	}
	//
	return q, r
}

// dwBitAt returns true when bit i of the 4n-bit value (hi << 2n | lo) is set.
func dwBitAt[W Word[W]](hi, lo Double[W], i int) bool {
	if bandwidth := 2 * lo.lo.Bandwidth(); uint(i) >= bandwidth {
		return hi.bit(uint64(uint(i) - bandwidth))
	}
	//
	return lo.bit(uint64(i))
}

// bit returns true when the kth bit of this word is set.
func (p Double[W]) bit(k uint64) bool {
	return p.Shr64(k).lo.Slice(1).BitLen() != 0
}

// b2u converts a carry (or borrow) flag into a 0/1 integer.
func b2u(flag bool) uint64 {
	if flag {
		return 1
	}
	//
	return 0
}
