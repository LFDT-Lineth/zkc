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
package hash

import (
	"bytes"
	"fmt"
	"hash/fnv"
)

// A reasonably simple hashset implementation which permits collisions.  Observe
// that, for example, hashicorp's go-set is *not* a suitable replacement here,
// since that does not handle collisions.  Specifically, it assumes the hash
// function always uniquely identifies the data in question.  I don't want to
// make that assumption here.

// Hasher provides a generic definition of a hashing function suitable for use
// within the hashset.  This is similar to the Hasher interface provided in
// go-set, except that it additionally includes equality.
type Hasher[T any] interface {
	// Check whether two items are equal (or not).
	Equals(T) bool
	// Return a suitable hashcode.
	Hash() uint64
}

// Uint64Key provides a simple wrapper around a uint64.
type Uint64Key struct {
	value uint64
}

// NewUint64Key constructs a new bytes key.
func NewUint64Key(key uint64) Uint64Key {
	return Uint64Key{key}
}

// Equals compares two Uint64Keys to check whether they represent the same
// underlying byte array (or not).
func (p Uint64Key) Equals(other Uint64Key) bool {
	return p.value == other.value
}

// Hash generates a 64-bit hashcode from the underlying value.
func (p Uint64Key) Hash() uint64 {
	// This is a deliberate act to limit the qualitfy of this hash function.
	return p.value
}

func (p Uint64Key) String() string {
	return fmt.Sprintf("%d", p.value)
}

// ============================================================================
// BytesKey Implementation
// ============================================================================

var _ Hasher[BytesKey] = BytesKey{}

// BytesKey wraps a bytes array as something which can be safely placed into a
// HashSet.
type BytesKey struct {
	bytes []byte
}

// NewBytesKey constructs a new bytes key.
func NewBytesKey(bytes []byte) BytesKey {
	return BytesKey{bytes}
}

// Equals compares two BytesKeys to check whether they represent the same
// underlying byte array (or not).
func (p BytesKey) Equals(other BytesKey) bool {
	return bytes.Equal(p.bytes, other.bytes)
}

// Hash generat6es a 64-bit hashcode from the underlying bytes array.
func (p BytesKey) Hash() uint64 {
	hash := fnv.New64a()
	hash.Write(p.bytes)
	// Done
	return hash.Sum64()
}

var _ Hasher[BytesKey] = BytesKey{}

// ============================================================================
// ArrayKey Implementation
// ============================================================================

const (
	offset64 uint64 = 14695981039346656037
	prime64  uint64 = 1099511628211
)

// Array provides a mechanism for hashing hashes (or other uint64 values).
type Array[F Hasher[F]] struct {
	elements []F
}

// NewArray constructs a new bytes key.
func NewArray[F Hasher[F]](hashes []F) Array[F] {
	return Array[F]{hashes}
}

// Equals compares two arrays to check whether they represent the same
// underlying byte array (or not).
func (p Array[F]) Equals(other Array[F]) bool {
	var (
		n = len(p.elements)
		m = len(other.elements)
	)
	//
	if n != m {
		return false
	}
	//
	for i := range n {
		if !p.elements[i].Equals(other.elements[i]) {
			return false
		}
	}
	//
	return true
}

// Elements returns the underlying elements.
func (p Array[F]) Elements() []F {
	return p.elements
}

// Hash generat6es a 64-bit hashcode from the underlying bytes array.
func (p Array[F]) Hash() uint64 {
	// FNV1a hash implementation
	hash := offset64
	//
	for _, c := range p.elements {
		hash ^= c.Hash()
		hash *= prime64
	}
	//
	return hash
}
