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
package util

import (
	"math/rand/v2"
)

// GenerateRandomUints generates n random unsigned integers in the range 0..m.
func GenerateRandomUints(n, m uint) []uint {
	items := make([]uint, n)

	for i := range n {
		items[i] = rand.UintN(m)
	}

	return items
}

// GenerateRandomBytes generates n random bytes.
func GenerateRandomBytes(n uint) []byte {
	items := make([]byte, n)

	for i := range n {
		items[i] = byte(rand.UintN(256))
	}

	return items
}

// GenerateRandomUint64s generates n random 64bit unsigned integers in the range
// 0..m.
func GenerateRandomUint64s(n, m uint64) []uint64 {
	items := make([]uint64, n)

	for i := range n {
		items[i] = rand.Uint64N(m)
	}

	return items
}

// GenerateRandomInts generates n random unsigned integers in the range -m..m.
func GenerateRandomInts(n uint, m int) []int {
	items := make([]int, n)

	for i := uint(0); i < n; i++ {
		items[i] = rand.IntN(2*m) - m
	}

	return items
}

// GenerateRandomElements generates n elements selected at random from the given array.
func GenerateRandomElements[E any](n uint, elems []E) []E {
	items := make([]E, n)
	m := uint(len(elems))

	for i := uint(0); i < n; i++ {
		index := rand.UintN(m)
		items[i] = elems[index]
	}

	return items
}

// SampleElements selects exactly n elements from the given array when its
// length is greater (otherwise returns the array untouched).
func SampleElements[E any](n uint, elems []E) []E {
	if n >= uint(len(elems)) {
		return elems
	}
	//
	m := len(elems)
	res := make([]E, 0, n)
	// Number of items still to choose
	r := int(n)
	// Selection sampling (Knuth's Algorithm S): include each element with
	// probability r / (m - j), which yields a uniform distribution over all
	// n-element subsets whilst preserving order.
	for j := 0; r > 0; j++ {
		if rand.IntN(m-j) < r {
			res = append(res, elems[j])
			r--
		}
	}
	//
	return res
}
