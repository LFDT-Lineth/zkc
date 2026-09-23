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
	"cmp"
	"math/big"
)

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// Cmp64 returns 1 if x > y, 0 if x = y, and -1 if x < y.
func (x Element) Cmp64(y uint64) int {
	if x.IsUint64() {
		return cmp.Compare(x.Uint64(), y)
	}
	//
	return 1
}

// Equals implementation for hash.Hasher interface
func (x Element) Equals(o Element) bool {
	return x.Element == o.Element
}

// Hash implementation for hash.Hasher interface
func (x Element) Hash() uint64 {
	// FNV1a hash implementation
	return (offset64 ^ x.Element[0]) * prime64
}

// FitsWithin implementation for word.Word interface.  This concerns the
// numerical value of the element, hence it must be derived from Uint64 (which
// converts out of Montgomery form) rather than from the raw limb.  Note that
// goldilocks.BitLen reads the raw limb without converting, unlike Bits / Cmp,
// so it is not usable here.  Shifting by 64 or more is well defined in Go for
// an unsigned value, yielding zero, so wide bitwidths need no special case.
func (x Element) FitsWithin(bitwidth uint) bool {
	return (x.Uint64() >> bitwidth) == 0
}

// SetBytes implementation for word.Word interface.
func (x Element) SetBytes(bytes []byte) Element {
	x.Element.SetBytes(bytes)
	//
	return x
}

// SetUint64 implementation for word.Word interface.
func (x Element) SetUint64(val uint64) Element {
	x.Element.SetUint64(val)
	//
	return x
}

// Uint64 implementation for word.Word interface.
func (x Element) Uint64() uint64 {
	return x.Element.Uint64()
}

// BigInt implementation for word.Word interface.
func (x Element) BigInt() *big.Int {
	var (
		val   big.Int
		bytes = x.Element.Bytes()
	)
	//
	return val.SetBytes(bytes[:])
}
