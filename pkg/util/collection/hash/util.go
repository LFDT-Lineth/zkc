// Copyright Consensys Incorporated
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
package hash

import (
	"encoding/binary"
	"hash/fnv"
)

// String returns a fast and deterministic (but non-cryptographic) has for a
// string.
func String(s string) uint64 {
	return Bytes([]byte(s)...)
}

// Bool returns a simple hash for a boolean value.
func Bool(flag bool) uint64 {
	var (
		h = fnv.New64a()
		b byte
	)
	//
	if flag {
		b = 1
	} else {
		b = 0
	}
	//
	h.Write([]byte{b})
	//
	return h.Sum64()
}

// Bytes returns a fast and deterministic (but non-cryptographic) has for a
// byte slice.
func Bytes(bytes ...byte) uint64 {
	h := fnv.New64a()
	h.Write(bytes)
	//
	return h.Sum64()
}

// Uint64s returns the hash of zero or more uint64 words.
func Uint64s(vals ...uint64) uint64 {
	var (
		h   = fnv.New64a()
		buf [8]byte
	)
	//
	for _, v := range vals {
		binary.BigEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	//
	return h.Sum64()
}
