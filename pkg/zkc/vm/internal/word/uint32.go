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

// Uint32 represents an unsigned integer backed by a native uint32.  Operations
// which naturally require a double-width intermediate (multiplication, double
// word division, etc.) use a uint64 for that intermediate so that, unlike a
// big.Int backing, the common arithmetic paths neither allocate nor touch
// BigInt().
type Uint32 struct {
	value uint32
}

var _ Word[Uint32] = Uint32{}

// Add implementation for Word interface.
func (p Uint32) Add(w Uint32) (Uint32, bool) {
	sum, carry := bits.Add32(p.value, w.value, 0)
	return Uint32{sum}, carry != 0
}

// Add64 implementation for Word interface.
func (p Uint32) Add64(w uint64) (Uint32, bool) {
	sum, carry := bits.Add64(uint64(p.value), w, 0)
	return Uint32{uint32(sum)}, carry != 0 || sum > mask64(32)
}

// AddMod implementation for Word interface.
func (p Uint32) AddMod(w, m Uint32) Uint32 {
	// A uint64 intermediate comfortably holds the sum of two uint32 values.
	sum := uint64(p.value) + uint64(w.value)
	//
	return Uint32{uint32(sum % uint64(m.value))}
}

// And implementation for Word interface.
func (p Uint32) And(w Uint32) Uint32 {
	return Uint32{p.value & w.value}
}

// Bandwidth implementation for Word interface.
func (p Uint32) Bandwidth() uint {
	return 32
}

// BigInt implementation for Word interface.
func (p Uint32) BigInt() *big.Int {
	return new(big.Int).SetUint64(uint64(p.value))
}

// BitLen implementation for Word interface.
func (p Uint32) BitLen() uint {
	return uint(bits.Len32(p.value))
}

// Config implementation for Word interface.
func (p Uint32) Config() Config {
	return WORD_UINT32
}

// Cmp implementation for Word interface.
func (p Uint32) Cmp(o Uint32) int {
	return cmp.Compare(p.value, o.value)
}

// Cmp64 implementation for Word interface.
func (p Uint32) Cmp64(o uint64) int {
	return cmp.Compare(uint64(p.value), o)
}

// Div implementation for Word interface.
func (p Uint32) Div(w Uint32) Uint32 {
	return Uint32{p.value / w.value}
}

// DwDiv implementation for Word interface.
func (p Uint32) DwDiv(lo, d Uint32) (Uint32, Uint32) {
	if d.value == 0 {
		panic("division by zero")
	} else if p.value >= d.value {
		panic("quotient overflow")
	}
	// The double word (p:lo) fits within a uint64, so a single native division
	// yields both quotient and remainder.  The precondition p < d guarantees the
	// quotient fits within 32 bits.
	dividend := uint64(p.value)<<32 | uint64(lo.value)
	q := dividend / uint64(d.value)
	r := dividend % uint64(d.value)
	//
	return Uint32{uint32(q)}, Uint32{uint32(r)}
}

// DwRem implementation for Word interface.
func (p Uint32) DwRem(lo, d Uint32) Uint32 {
	// The double word (p:lo) fits within a uint64, so the remainder is a single
	// native modulo.
	dividend := uint64(p.value)<<32 | uint64(lo.value)
	//
	return Uint32{uint32(dividend % uint64(d.value))}
}

// FitsWithin implementation for Word interface.
func (p Uint32) FitsWithin(bitwidth uint) bool {
	if bitwidth >= 32 {
		return true
	}
	//
	return p.value>>bitwidth == 0
}

// Mul implementation for Word interface.
func (p Uint32) Mul(w Uint32) (hi, lo Uint32) {
	full := uint64(p.value) * uint64(w.value)
	return Uint32{uint32(full >> 32)}, Uint32{uint32(full)}
}

// MulMod implementation for Word interface.
func (p Uint32) MulMod(w, m Uint32) Uint32 {
	// A uint64 intermediate comfortably holds the product of two uint32 values.
	prod := uint64(p.value) * uint64(w.value)
	//
	return Uint32{uint32(prod % uint64(m.value))}
}

// Not implementation for Word interface.
func (p Uint32) Not(bitwidth uint) Uint32 {
	return Uint32{(^p.value) & mask32(bitwidth)}
}

// Or implementation for Word interface.
func (p Uint32) Or(w Uint32) Uint32 {
	return Uint32{p.value | w.value}
}

// Rem implementation for Word interface.
func (p Uint32) Rem(w Uint32) Uint32 {
	return Uint32{p.value % w.value}
}

// Shl implementation for Word interface.
func (p Uint32) Shl(width uint, n Uint32) Uint32 {
	return Uint32{(p.value << n.value) & mask32(width)}
}

// Shl64 implementation for Word interface.
func (p Uint32) Shl64(n uint64) (hi Uint32, lo Uint32) {
	//
	if n >= 32 {
		hi.value = p.value << (n - 32)
	} else {
		hi.value = p.value >> (32 - n)
		lo.value = p.value << n
	}
	//
	return hi, lo
}

// Shr implementation for Word interface.
func (p Uint32) Shr(n Uint32) Uint32 {
	return p.Shr64(uint64(n.value))
}

// Shr64 implementation for Word interface.
func (p Uint32) Shr64(n uint64) Uint32 {
	return Uint32{p.value >> n}
}

// Slice implementation for Word interface.
func (p Uint32) Slice(width uint) Uint32 {
	return Uint32{p.value & mask32(width)}
}

// SetBigInt implementation for Word interface; panics if the value is negative
// or does not fit within 32 bits.
func (p Uint32) SetBigInt(val *big.Int) Uint32 {
	if val.Sign() < 0 {
		panic("cannot assign negative integer")
	} else if !val.IsUint64() || val.Uint64() > mask64(32) {
		panic(fmt.Sprintf("value 0x%s exceeds uint32 bandwidth", val.Text(16)))
	}
	//
	return Uint32{uint32(val.Uint64())}
}

// SetUint64 implementation for Word interface; panics if the value does not fit
// within 32 bits.
func (p Uint32) SetUint64(val uint64) Uint32 {
	if val > mask64(32) {
		panic(fmt.Sprintf("value 0x%x exceeds uint32 bandwidth", val))
	}
	//
	return Uint32{uint32(val)}
}

// Sub implementation for Word interface.
func (p Uint32) Sub(w Uint32) (Uint32, bool) {
	diff, borrow := bits.Sub32(p.value, w.value, 0)
	return Uint32{diff}, borrow != 0
}

// Sub64 implementation for Word interface.
func (p Uint32) Sub64(w uint64) (Uint32, bool) {
	diff, borrow := bits.Sub64(uint64(p.value), w, 0)
	return Uint32{uint32(diff)}, borrow != 0
}

// SubMod implementation for Word interface.
func (p Uint32) SubMod(w, m Uint32) Uint32 {
	// Reduce inputs into the range [0, m) so that the difference, taken modulo
	// m, fits naturally into a uint32.
	a := p.value % m.value
	b := w.value % m.value
	//
	if a >= b {
		return Uint32{a - b}
	}
	//
	return Uint32{m.value - (b - a)}
}

// Uint64 implementation for Word interface.
func (p Uint32) Uint64() uint64 {
	return uint64(p.value)
}

// Xor implementation for Word interface.
func (p Uint32) Xor(w Uint32) Uint32 {
	return Uint32{p.value ^ w.value}
}

// Text implementation for Word interface.
func (p Uint32) Text(base int) string {
	return strconv.FormatUint(uint64(p.value), base)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p Uint32) GobEncode() ([]byte, error) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], p.value)
	//
	return buf[:], nil
}

// nolint
func (p *Uint32) GobDecode(data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("invalid uint32 gob encoding: expected 4 bytes, got %d", len(data))
	}
	//
	p.value = binary.BigEndian.Uint32(data)
	//
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// mask32 returns a uint32 with the low `bitwidth` bits set.  When bitwidth is
// 32 or greater, all bits are set (this avoids the undefined behaviour of
// shifting a uint32 by 32).
func mask32(bitwidth uint) uint32 {
	if bitwidth >= 32 {
		return ^uint32(0)
	}
	//
	return (uint32(1) << bitwidth) - 1
}
