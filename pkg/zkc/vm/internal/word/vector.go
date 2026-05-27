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
package word

// Vector encapsulates an array of words (e.g. for representing a constant which
// has been split).
type Vector[W Word[W]] struct {
	words []W
}

// NewVector constructs a new word vector from a given array of words.
func NewVector[W Word[W]](words ...W) Vector[W] {
	return Vector[W]{words}
}

// Len returns the size of this vector.
func (p *Vector[W]) Len() uint {
	return uint(len(p.words))
}

// Words provides direct access to the encapsulated word array.
func (p *Vector[W]) Words() []W {
	return p.words
}
