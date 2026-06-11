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

// Union represents a value which is either of the first type or of the second
// type.
type Union[S, T any] struct {
	// Indicates first present
	sign bool
	// Left value
	first S
	// Right value
	second T
}

// Union1 constructs a union holding a value of the first type.
func Union1[S, T any](value S) Union[S, T] {
	var empty T
	//
	return Union[S, T]{true, value, empty}
}

// Union2 constructs a union holding a value of the second type.
func Union2[S, T any](value T) Union[S, T] {
	var empty S
	//
	return Union[S, T]{false, empty, value}
}

// HasFirst indicates whether this union holds a value of the first type (or
// not).
func (u Union[S, T]) HasFirst() bool {
	return u.sign
}

// HasSecond indicates whether this union holds a value of the second type (or
// not).
func (u Union[S, T]) HasSecond() bool {
	return !u.sign
}

// First returns the contained value of the first type.  If the union does not
// hold a value of the first type, then this will panic.
func (u Union[S, T]) First() S {
	if u.sign {
		return u.first
	}
	//
	panic("cannot take first item, as union holds second")
}

// Second returns the contained value of the second type.  If the union does not
// hold a value of the second type, then this will panic.
func (u Union[S, T]) Second() T {
	if !u.sign {
		return u.second
	}
	//
	panic("cannot take second item, as union holds first")
}
