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
package field

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
)

// BatchInvert efficiently inverts the list of elements s, in place.
func BatchInvert[T Element[T]](s []T) {
	if len(s) == 0 {
		return
	}
	//
	var (
		zero = Zero[T]()
		one  = One[T]()
		last = uint(len(s) - 1)
		// identifies entries which are zero
		isZero = bit.NewSet(uint(len(s)))

		m = make([]T, len(s)) // m[i] = s[i] * s[i+1] * ...
	)
	//
	isZero.Set(last, s[last].IsZero())

	if isZero.Get(last) {
		s[last] = one
	}

	m[last] = s[last]

	for i := len(s) - 2; i >= 0; i-- {
		isZero.Set(uint(i), s[uint(i)].IsZero())

		if isZero.Get(uint(i)) {
			s[uint(i)] = one
		}

		m[i] = m[i+1].Mul(s[uint(i)])
	}

	inv := m[0].Inverse() // inv = s[0]⁻¹ * s[1]⁻¹ * ...

	for i := range len(s) - 1 {
		// inv = s[i]⁻¹ * s[i+1]⁻¹ * ...
		newInv := inv.Mul(s[i])
		s[i] = inv.Mul(m[i+1])
		inv = newInv
		// inv = s[i+1]⁻¹ * s[i+2]⁻¹ * ...
		if isZero.Get(uint(i)) {
			s[i] = zero
		}
	}

	s[last] = inv

	if isZero.Get(last) {
		s[last] = zero
	}
}
