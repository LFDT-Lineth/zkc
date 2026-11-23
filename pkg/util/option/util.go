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
package option

// Equitable captures the notion of a datatype whose values can be compared for
// equality.
type Equitable[T any] interface {
	// Check whether two items are equal (or not).
	Equals(T) bool
}

// Hashable provides a generic definition of a hashing function suitable.
type Hashable[T any] interface {
	// Return a suitable hashcode.
	Hash() uint64
}

// Equal checks whether two (equitable) options are equal, or not.
func Equal[T Equitable[T]](l, r Option[T]) bool {
	if l.HasValue() && r.HasValue() {
		return l.Unwrap().Equals(r.Unwrap())
	}
	//
	return l.IsEmpty() == r.IsEmpty()
}

// Hash returns a simple hash for a (hashable) option value, where none is given
// the value zero.
func Hash[T Hashable[T]](o Option[T]) uint64 {
	if o.HasValue() {
		return o.Unwrap().Hash()
	}
	//
	return 0
}
