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
	"math/big"

	"github.com/consensys/gnark-crypto/field/goldilocks"
)

var (
	// Modulus defines the prime used for this finite field.  Observe that
	// goldilocks.Modulus() allocates a fresh big.Int on every call, so the value
	// is cached here once and shared thereafter.  Callers must treat it as
	// read-only.
	Modulus = goldilocks.Modulus()
)

// Element wraps fr.Element to conform
// to the field.Element interface.
type Element struct {
	goldilocks.Element
}

// Add x + y
func (x Element) Add(y Element) Element {
	var res goldilocks.Element
	//
	res.Add(&x.Element, &y.Element)
	//
	return Element{res}
}

// Cmp returns 1 if x > y, 0 if x = y, and -1 if x < y.
func (x Element) Cmp(y Element) int {
	return x.Element.Cmp(&y.Element)
}

// Inverse x⁻¹, or 0 if x = 0.
func (x Element) Inverse() Element {
	var elem goldilocks.Element
	//
	elem.Inverse(&x.Element)
	//
	return Element{elem}
}

// IsOne implementation for the Element interface
func (x Element) IsOne() bool {
	return x.Element.IsOne()
}

// IsZero implementation for the Element interface
func (x Element) IsZero() bool {
	return x.Element.IsZero()
}

// Modulus implementation for the Element interface.  This returns the shared
// Modulus value, rather than allocating a fresh big.Int per call, and must
// therefore be treated as read-only by callers.
func (x Element) Modulus() *big.Int {
	return Modulus
}

// Mul x * y
func (x Element) Mul(y Element) Element {
	var elem goldilocks.Element
	//
	elem.Mul(&x.Element, &y.Element)
	//
	return Element{elem}
}

// Sub x - y
func (x Element) Sub(y Element) Element {
	var elem goldilocks.Element
	//
	elem.Sub(&x.Element, &y.Element)
	//
	return Element{elem}
}

// Bytes returns the big-endian encoded value of the Element, possibly with leading zeros.
func (x Element) Bytes() []byte {
	return x.Marshal()
}

func (x Element) String() string {
	return x.Element.String()
}

// Text implementation for the Element interface
func (x Element) Text(base int) string {
	return x.Element.Text(base)
}
