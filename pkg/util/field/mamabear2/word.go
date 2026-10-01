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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// Cmp64 returns 1 if x > y, 0 if x = y, and -1 if x < y.
func (x Element) Cmp64(y uint64) int {
	a := x.ToUint64()
	//
	if a < y {
		return -1
	} else if a > y {
		return 1
	}

	return 0
}

// Equals implementation for hash.Hasher interface
func (x Element) Equals(o Element) bool {
	return x == o
}

// Hash implementation for hash.Hasher interface
func (x Element) Hash() uint64 {
	// FNV1a hash implementation (unrolled)
	hash := offset64
	//
	return (hash ^ x[0]) * prime64
}

// FitsWithin implementation for word.Word interface.  This concerns the
// numerical value of the element, hence it must be derived from Uint64 (which
// converts out of Montgomery form) rather than from the raw limb.  Shifting by
// 64 or more is well defined in Go for an unsigned value, yielding zero.
func (x Element) FitsWithin(bitwidth uint) bool {
	return (x.Uint64() >> bitwidth) == 0
}

// SetBytes implementation for word.Word interface.  The bytes are interpreted
// as a big-endian unsigned integer, which is reduced modulo m.
func (x Element) SetBytes(bs []byte) Element {
	var (
		v       uint64
		trimmed = word.TrimLeadingZeros(bs)
	)
	// Fast path: at most 64 bits, which New reduces directly.
	if len(trimmed) <= nbBytes {
		for _, b := range trimmed {
			v = (v << 8) | uint64(b)
		}
		//
		return New(v)
	}
	// Slow path: Horner's method, keeping v reduced.  Since v < m < 2⁵⁰, then
	// v.2⁸ + b < 2⁵⁸ which cannot overflow.
	for _, b := range trimmed {
		v = ((v << 8) | uint64(b)) % Modulus
	}
	//
	return New(v)
}

// SetUint64 implementation for word.Word interface.  The value is reduced
// modulo m.
func (x Element) SetUint64(val uint64) Element {
	return New(val)
}

// Uint64 implementation for word.Word interface.
func (x Element) Uint64() uint64 {
	return x.ToUint64()
}

// BigInt implementation for word.Word interface.
func (x Element) BigInt() *big.Int {
	var val big.Int
	//
	return val.SetUint64(x.Uint64())
}
