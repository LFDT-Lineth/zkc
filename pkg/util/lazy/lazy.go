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
package lazy

// Value returns a value when invoked
type Value[T any] func() T

// Const returns a lazy value which always returns a constant
func Const[T any](v T) Value[T] {
	return func() T {
		return v
	}
}

// Read returns a lazy value which returns the value of an array at a given
// index.
func Read[T any](index int, array []T) Value[T] {
	return func() T {
		return array[index]
	}
}

// IfElse provides a make-shift ternary operator
func IfElse[T any](value bool, ifTrue Value[T], ifFalse Value[T]) T {
	if value {
		return ifTrue()
	}
	//
	return ifFalse()
}

// IfDefault provides a make-shift ternary operator
func IfDefault[T any](value bool, ifTrue Value[T]) T {
	var val T
	//
	if value {
		return ifTrue()
	}
	//
	return val
}
