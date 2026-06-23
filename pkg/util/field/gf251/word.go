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
package gf251

import (
	"cmp"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// Cmp64 returns 1 if x > y, 0 if x = y, and -1 if x < y.
func (x Element) Cmp64(y uint64) int {
	return cmp.Compare(uint64(x.ToUint32()), y)
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
	return (hash ^ uint64(x[0])) * prime64
}

// FitsWithin implementation for word.Word interface.
func (x Element) FitsWithin(bitwidth uint) bool {
	return (x[0] >> bitwidth) == 0
}

// SetBytes implementation for word.Word interface.
func (x Element) SetBytes(bs []byte) Element {
	var v uint32
	//
	for _, b := range word.TrimLeadingZeros(bs) {
		v = (v << 8) | uint32(b)
	}
	//
	return New(v)
}

// SetUint64 implementation for word.Word interface
func (x Element) SetUint64(val uint64) Element {
	var elem Element
	//
	return elem.AddUint32(uint32(val))
}

// Uint64 implementation for word.Word interface.
func (x Element) Uint64() uint64 {
	return uint64(x.ToUint32())
}

// BigInt implementation for word.Word interface.
func (x Element) BigInt() *big.Int {
	var val big.Int
	//
	return val.SetUint64(x.Uint64())
}
