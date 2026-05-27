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
package poly

import "math/big"

// Term represents a product (or monomial) within a polynomial.
type Term[S any, T any] interface {
	// Coefficient returns the coefficient of this term.
	Coefficient() big.Int
	// Len returns the number of variables in this polynomial term.
	Len() uint
	// Negate this term
	Negate() T
	// Nth returns the nth variable in this polynomial term.
	Nth(uint) S
	// Matches determines whether or not the variables of this term match those
	// of the other.
	Matches(other T) bool
	// Check whether this term has a negative coefficient (or not)
	IsNegative() bool
}
