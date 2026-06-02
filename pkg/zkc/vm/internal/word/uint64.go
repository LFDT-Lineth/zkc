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

// Uint64 represents an unsigned integer backed by a native uint64.
type Uint64 struct {
	value uint64
}

var _ Word[Uint64] = Uint64{}

// Add implementation for Word interface.
func (p Uint64) Add(w Uint64) (Uint64, bool) {
	sum, carry := bits.Add64(p.value, w.value, 0)
	return Uint64{sum}, carry != 0
}

// AddMod implementation for Word interface.
func (p Uint64) AddMod(w, m Uint64) Uint64 {
	if m.value == 0 {
		panic("modulus by zero")
	}
	// Use 128-bit intermediate to avoid overflow.
	lo, hi := bits.Add64(p.value, w.value, 0)
	_, rem := bits.Div64(hi, lo, m.value)
	//
	return Uint64{rem}
}

// And implementation for Word interface.
func (p Uint64) And(w Uint64) Uint64 {
	return Uint64{p.value & w.value}
}

// Bandwidth implementation for Word interface.
func (p Uint64) Bandwidth() uint {
	return 64
}

// BigInt implementation for Word interface.
func (p Uint64) BigInt() *big.Int {
	return new(big.Int).SetUint64(p.value)
}

// Cmp implementation for Word interface.
func (p Uint64) Cmp(o Uint64) int {
	return cmp.Compare(p.value, o.value)
}

// Cmp64 implementation for Word interface.
func (p Uint64) Cmp64(o uint64) int {
	return cmp.Compare(p.value, o)
}

// Div implementation for Word interface.
func (p Uint64) Div(w Uint64) Uint64 {
	if w.value == 0 {
		panic("division by zero")
	}
	//
	return Uint64{p.value / w.value}
}

// FitsWithin implementation for Word interface.
func (p Uint64) FitsWithin(bitwidth uint) bool {
	if bitwidth >= 64 {
		return true
	}
	//
	return p.value>>bitwidth == 0
}

// Mul implementation for Word interface.
func (p Uint64) Mul(w Uint64) (Uint64, bool) {
	hi, lo := bits.Mul64(p.value, w.value)
	return Uint64{lo}, hi != 0
}

// MulMod implementation for Word interface.
func (p Uint64) MulMod(w, m Uint64) Uint64 {
	if m.value == 0 {
		panic("modulus by zero")
	}
	// Use 128-bit intermediate to avoid overflow.
	hi, lo := bits.Mul64(p.value, w.value)
	_, rem := bits.Div64(hi, lo, m.value)
	//
	return Uint64{rem}
}

// Not implementation for Word interface.
func (p Uint64) Not(bitwidth uint) Uint64 {
	return Uint64{(^p.value) & mask64(bitwidth)}
}

// Or implementation for Word interface.
func (p Uint64) Or(w Uint64) Uint64 {
	return Uint64{p.value | w.value}
}

// Rem implementation for Word interface.
func (p Uint64) Rem(w Uint64) Uint64 {
	if w.value == 0 {
		panic("division by zero")
	}
	//
	return Uint64{p.value % w.value}
}

// Shl implementation for Word interface.
func (p Uint64) Shl(width uint, n Uint64) Uint64 {
	return Uint64{(p.value << n.value) & mask64(width)}
}

// Shl64 implementation for Word interface.
func (p Uint64) Shl64(n uint64) Uint64 {
	return Uint64{(p.value << n)}
}

// Shr implementation for Word interface.
func (p Uint64) Shr(n Uint64) Uint64 {
	return p.Shr64(n.value)
}

// Shr64 implementation for Word interface.
func (p Uint64) Shr64(n uint64) Uint64 {
	return Uint64{p.value >> n}
}

// Slice implementation for Word interface.
func (p Uint64) Slice(width uint) Uint64 {
	return Uint64{p.value & mask64(width)}
}

// SetBigInt implementation for Word interface; panics if the value is negative
// or does not fit within 64 bits.
func (p Uint64) SetBigInt(val *big.Int) Uint64 {
	if val.Sign() < 0 {
		panic("cannot assign negative integer")
	} else if !val.IsUint64() {
		panic(fmt.Sprintf("value 0x%s exceeds uint64 bandwidth", val.Text(16)))
	}
	//
	return Uint64{val.Uint64()}
}

// SetUint64 implementation for Word interface.
func (p Uint64) SetUint64(val uint64) Uint64 {
	return Uint64{val}
}

// Sub implementation for Word interface.
func (p Uint64) Sub(w Uint64) (Uint64, bool) {
	diff, borrow := bits.Sub64(p.value, w.value, 0)
	return Uint64{diff}, borrow != 0
}

// SubMod implementation for Word interface.
func (p Uint64) SubMod(w, m Uint64) Uint64 {
	if m.value == 0 {
		panic("modulus by zero")
	}
	// Reduce inputs into the range [0, m) so that the difference, taken modulo
	// m, fits naturally into a uint64.
	a := p.value % m.value
	b := w.value % m.value
	//
	if a >= b {
		return Uint64{a - b}
	}
	//
	return Uint64{m.value - (b - a)}
}

// Uint64 implementation for Word interface.
func (p Uint64) Uint64() uint64 {
	return p.value
}

// Xor implementation for Word interface.
func (p Uint64) Xor(w Uint64) Uint64 {
	return Uint64{p.value ^ w.value}
}

// Text implementation for Word interface.
func (p Uint64) Text(base int) string {
	return strconv.FormatUint(p.value, base)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p Uint64) GobEncode() ([]byte, error) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], p.value)
	//
	return buf[:], nil
}

// nolint
func (p *Uint64) GobDecode(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("invalid uint64 gob encoding: expected 8 bytes, got %d", len(data))
	}
	//
	p.value = binary.BigEndian.Uint64(data)
	//
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// mask64 returns a uint64 with the low `bitwidth` bits set.  When bitwidth is
// 64 or greater, all bits are set (this avoids the undefined behaviour of
// shifting a uint64 by 64).
func mask64(bitwidth uint) uint64 {
	if bitwidth >= 64 {
		return ^uint64(0)
	}
	//
	return (uint64(1) << bitwidth) - 1
}
