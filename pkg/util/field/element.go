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
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// An Element of a prime-order field.
type Element[Operand any] interface {
	fmt.Stringer
	word.Word[Operand]
	// Add x+y
	Add(y Operand) Operand
	// Return the value of this word as a big integer.
	BigInt() *big.Int
	// Check whether this value is zero (or not).
	IsZero() bool
	// Check whether this value is one (or not).
	IsOne() bool
	// Return the modulus for the field in question.
	Modulus() *big.Int
	// Compute x * y
	Mul(y Operand) Operand
	// Compute x⁻¹, or 0 if x = 0.
	Inverse() Operand
	// Compute x - y
	Sub(y Operand) Operand
	// Text returns the numerical value of x in the given base.
	Text(base int) string
}

// Zero constructs a field element representing 0
func Zero[F Element[F]]() F {
	var element F
	//
	return element
}

// One constructs a field element representing 1
func One[F Element[F]]() F {
	var element F
	//
	return element.SetUint64(1)
}

// BigInt constructs a field element from a given big.Int
func BigInt[F Element[F]](val big.Int) F {
	var (
		element F
	)
	//
	element = element.SetBytes(val.Bytes())
	// Handle negative values
	if val.Sign() < 0 {
		panic("negative value encountered")
	}
	//
	return element
}

// BigInts constructs an array of field elements from a given array of big.Int.
func BigInts[F Element[F]](values []big.Int) []F {
	var elements = make([]F, len(values))
	//
	for i, val := range values {
		elements[i] = BigInt[F](val)
	}
	//
	return elements
}

// Uint64 construct a field element from a given uint64
func Uint64[F Element[F]](val uint64) F {
	var element F
	//
	return element.SetUint64(val)
}

// FromBigEndianBytes constructs a word from an array of bytes given in big endian order.
func FromBigEndianBytes[F Element[F]](bytes []byte) F {
	var element F
	//
	return element.SetBytes(bytes)
}

// TwoPowN constructs a field element representing 2^n
func TwoPowN[F Element[F]](n uint) F {
	var two F
	//
	return Pow(two.SetUint64(2), uint64(n))
}
