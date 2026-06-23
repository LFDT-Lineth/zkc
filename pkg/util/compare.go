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

// Comparable interface which can be implemented by non-primitive types.
type Comparable[T any] interface {
	// Cmp returns < 0 if this is less than other, or 0 if they are equal, or >
	// 0 if this is greater than other.
	Cmp(other T) int
}

// ComparableUint64 interface which can be implemented by non-primitive types.
type ComparableUint64 interface {
	// Cmp returns < 0 if this is less than other, or 0 if they are equal, or >
	// 0 if this is greater than other.
	Cmp64(other uint64) int
}

// Comparator provides a generic concept of comparing two values (e.g. l < r).
type Comparator[W Comparable[W]] interface {
	// Cmp returns true when the underlying comparison (e.g. l < r) holds.
	Cmp(l, r W) bool
}

// ============================================================================

// Equal comparator holds when l == r
type Equal[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p Equal[W]) Cmp(l, r W) bool {
	return l.Cmp(r) == 0
}

// ============================================================================

// NotEqual comparator holds when l != r
type NotEqual[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p NotEqual[W]) Cmp(l, r W) bool {
	return l.Cmp(r) != 0
}

// ============================================================================

// LessThan comparator holds when l < r
type LessThan[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p LessThan[W]) Cmp(l, r W) bool {
	return l.Cmp(r) < 0
}

// ============================================================================

// LessThanOrEqual comparator holds when l <= r
type LessThanOrEqual[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p LessThanOrEqual[W]) Cmp(l, r W) bool {
	return l.Cmp(r) <= 0
}

// ============================================================================

// GreaterThan comparator holds when l > r
type GreaterThan[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p GreaterThan[W]) Cmp(l, r W) bool {
	return l.Cmp(r) > 0
}

// ============================================================================

// GreaterThanOrEqual comparator holds when l >= r
type GreaterThanOrEqual[W Comparable[W]] struct {
}

// Cmp implementation for Comparable interface
func (p GreaterThanOrEqual[W]) Cmp(l, r W) bool {
	return l.Cmp(r) >= 0
}
