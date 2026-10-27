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
package bexp

import "math/big"

// Term represents an abstraction over boolean expressions.
type Term[T any] interface {
	// Create new variable.
	Variable(string) T
	// Create new constant.
	Number(big.Int) T
	// Logical
	Or(...T) T
	And(...T) T
	Truth(bool) T
	// Relational
	Equals(T) T
	NotEquals(T) T
	LessThan(T) T
	LessThanEquals(T) T
	// Arithmetic
	Add(...T) T
	Mul(...T) T
	Sub(...T) T
}
