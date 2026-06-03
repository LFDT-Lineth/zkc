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
	"math/rand"
	"slices"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/assert"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
)

func init() {
	// make sure the interface is adhered to.
	_ = Element[koalabear.Element](koalabear.Element{})
	_ = Element[bls12_377.Element](bls12_377.Element{})
}

func TestBatchInvert(t *testing.T) {
	s := make(elementArray, 4000)
	sInv := make(elementArray, len(s))
	scratch := make(elementArray, len(s))

	for i := range s {
		s[i] = koalabear.Element{rand.Uint32()}
		if s[i][0] >= koalabear.Modulus {
			s[i][0] = 0 // getting a zero with considerable probability
		}

		sInv[i] = s[i].Inverse()

		copy(scratch[:i], s)
		BatchInvert(scratch[:i])

		for j := range i {
			assert.Equal(t, sInv[j][0], scratch[j][0], "on slice %v, at index %d", s[:i], j)
		}
	}
}

type elementArray []koalabear.Element

func (e elementArray) BitWidth() uint {
	panic("not implemented")
}

func (e elementArray) Clone() array.MutArray[koalabear.Element] {
	return slices.Clone(e)
}

func (e elementArray) Get(u uint) koalabear.Element {
	return e[u]
}

func (e elementArray) Len() uint {
	return uint(len(e))
}

func (e elementArray) Slice(u uint, u2 uint) array.Array[koalabear.Element] {
	return e[u:u2]
}

func (e elementArray) Append(t koalabear.Element) array.MutArray[koalabear.Element] {
	panic("not implemented")
}

func (e elementArray) Set(u uint, t koalabear.Element) array.MutArray[koalabear.Element] {
	e[u] = t
	return e
}

func (e elementArray) Pad(u uint, u2 uint, t koalabear.Element) array.MutArray[koalabear.Element] {
	panic("not implemented")
}
