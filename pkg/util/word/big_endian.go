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
	"math/big"
)

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// BigEndian captures the notion of an array of bytes represented in big endian
// form.  This is really just a wrapper for convenience, and to help clarify the
// underlying byte order.
type BigEndian struct {
	val big.Int
}

// NewBigEndian constructs a new big endian byte array.
func NewBigEndian(bytes []byte) BigEndian {
	var val big.Int
	val.SetBytes(TrimLeadingZeros(bytes))
	//
	return BigEndian{val}
}

// BigInt implementation for word.Word interface.
func (p BigEndian) BigInt() *big.Int {
	return &p.val
}

// Add implementation for field.Element interface
func (p BigEndian) Add(o BigEndian) BigEndian {
	var res big.Int
	res.Add(&p.val, &o.val)
	//
	return BigEndian{res}
}

// Mul implementation for field.Element interface
func (p BigEndian) Mul(o BigEndian) BigEndian {
	var res big.Int
	res.Mul(&p.val, &o.val)
	//
	return BigEndian{res}
}

// IsOne implementation for field.Element interface
func (p BigEndian) IsOne() bool {
	return p.Cmp64(1) == 0
}

// Inverse implementation for field.Element interface
func (p BigEndian) Inverse() BigEndian {
	panic("unsupported operation")
}

// Rsh performs a right bitshift of a given amount on this value.
func (p BigEndian) Rsh(n uint) BigEndian {
	var res big.Int
	res.Rsh(&p.val, n)
	//
	return BigEndian{res}
}

// Sub implementation for field.Element interface
func (p BigEndian) Sub(o BigEndian) BigEndian {
	var res big.Int
	res.Sub(&p.val, &o.val)
	//
	return BigEndian{res}
}

// Modulus implementation for field.Element interface
func (p BigEndian) Modulus() *big.Int {
	panic("unsupported operation")
}

// ByteWidth implementation for the Word interface.
func (p BigEndian) ByteWidth() uint {
	return ByteWidth(uint(p.val.BitLen()))
}

// Cmp64 implementation for Word interface.
func (p BigEndian) Cmp64(o uint64) int {
	if p.val.IsUint64() {
		return cmp.Compare(p.val.Uint64(), o)
	}
	//
	return 1
}

// Cmp implements a comparison by regarding the word as an unsigned integer.
func (p BigEndian) Cmp(o BigEndian) int {
	return p.val.Cmp(&o.val)
}

// Equals implementation for the hash.Hasher interface.
func (p BigEndian) Equals(o BigEndian) bool {
	return p.Cmp(o) == 0
}

// FitsWithin implementation for Word interface.
func (p BigEndian) FitsWithin(bitwidth uint) bool {
	return uint(p.val.BitLen()) <= bitwidth
}

// Hash implementation for the hash.Hasher interface.
func (p BigEndian) Hash() uint64 {
	// FNV1a hash implementation
	hash := offset64
	//
	for _, c := range p.val.Bytes() {
		hash ^= uint64(c)
		hash *= prime64
	}
	//
	return hash
}

// IsZero implementation for the Word interface
func (p BigEndian) IsZero() bool {
	return p.val.Sign() == 0
}

// PutBytes implementation for Word interface.
func (p BigEndian) PutBytes(bytes []byte) []byte {
	var (
		n = uint(len(bytes))
		m = ByteWidth(uint(p.val.BitLen()))
	)
	// Sanity check space
	if n < m {
		bytes = make([]byte, m)
	}
	//
	return p.val.FillBytes(bytes)
}

// SetBytes implementation for Word interface.
func (p BigEndian) SetBytes(bytes []byte) BigEndian {
	var val big.Int
	val.SetBytes(TrimLeadingZeros(bytes))
	//
	return BigEndian{val}
}

// SetUint64 implementation for Word interface.
func (p BigEndian) SetUint64(value uint64) BigEndian {
	var bytes [8]byte
	// Write big endian bytes
	binary.BigEndian.PutUint64(bytes[:], value)
	// Done
	return p.SetBytes(bytes[:])
}

// Uint64 implementation for Word interface.
func (p BigEndian) Uint64() uint64 {
	return p.val.Uint64()
}

// Bytes implementation for Word interface.
func (p BigEndian) Bytes() []byte {
	if p.val.Sign() < 0 {
		panic("cannot convert negative integer into unsigned bytes")
	}
	//
	return p.val.Bytes()
}

func (p BigEndian) String() string {
	return p.val.String()
}

// Text returns a string representation of this word in a given base.
func (p BigEndian) Text(base int) string {
	return p.val.Text(base)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode a big endian word.  This allows it to be marshalled into a binary form.
func (p *BigEndian) GobEncode() (data []byte, err error) {
	return p.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *BigEndian) GobDecode(data []byte) error {
	p.val.SetBytes(data)
	//
	return nil
}
