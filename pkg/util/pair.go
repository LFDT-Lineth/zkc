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
package util

// Pair provides a simple encapsulation of two items paired together.
type Pair[S any, T any] struct {
	Left  S
	Right T
}

// NewPair returns a new instance of Pair by value.
func NewPair[S any, T any](left S, right T) Pair[S, T] {
	return Pair[S, T]{left, right}
}

// NewPairRef returns a reference to a new instance of Pair.
func NewPairRef[S any, T any](left S, right T) *Pair[S, T] {
	var p = NewPair(left, right)
	return &p
}

// Split returns both the left and right elements of this pair.
func (p *Pair[S, T]) Split() (S, T) {
	return p.Left, p.Right
}
