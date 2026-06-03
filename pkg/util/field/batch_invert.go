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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
)

// BatchInvert efficiently inverts the list of elements s, in place.
func BatchInvert[T Element[T]](s array.MutArray[T]) array.MutArray[T] {
	if s.Len() == 0 {
		return s
	}
	//
	var (
		zero = Zero[T]()
		one  = One[T]()
		// identifies entries which are zero
		isZero = bit.NewSet(s.Len())

		m = make([]T, s.Len()) // m[i] = s[i] * s[i+1] * ...
	)
	//
	isZero.Set(s.Len()-1, s.Get(s.Len()-1).IsZero())

	if isZero.Get(s.Len() - 1) {
		s = s.Set(s.Len()-1, one)
	}

	m[s.Len()-1] = s.Get(s.Len() - 1)

	for i := int(s.Len()) - 2; i >= 0; i-- {
		isZero.Set(uint(i), s.Get(uint(i)).IsZero())

		if isZero.Get(uint(i)) {
			s = s.Set(uint(i), one)
		}

		m[i] = m[i+1].Mul(s.Get(uint(i)))
	}

	inv := m[0].Inverse() // inv = s[0]⁻¹ * s[1]⁻¹ * ...

	for i := range s.Len() - 1 {
		// inv = s[i]⁻¹ * s[i+1]⁻¹ * ...
		newInv := inv.Mul(s.Get(i))
		s = s.Set(i, inv.Mul(m[i+1]))
		inv = newInv
		// inv = s[i+1]⁻¹ * s[i+2]⁻¹ * ...
		if isZero.Get(i) {
			s = s.Set(i, zero)
		}
	}

	s = s.Set(s.Len()-1, inv)

	if isZero.Get(s.Len() - 1) {
		s = s.Set(s.Len()-1, zero)
	}
	//
	return s
}
