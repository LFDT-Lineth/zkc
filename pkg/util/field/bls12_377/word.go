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
package bls12_377

import "math/big"

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// Equals implementation for hash.Hasher interface
func (x Element) Equals(o Element) bool {
	return x.Element == o.Element
}

// Hash implementation for hash.Hasher interface
func (x Element) Hash() uint64 {
	// FNV1a hash implementation (unrolled)
	hash := offset64
	//
	hash = (hash ^ x.Element[0]) * prime64
	hash = (hash ^ x.Element[1]) * prime64
	hash = (hash ^ x.Element[2]) * prime64
	hash = (hash ^ x.Element[3]) * prime64
	//
	return hash
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
