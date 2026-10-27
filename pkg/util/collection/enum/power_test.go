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
package enum

import (
	"testing"
)

func Test_Power_1_1(t *testing.T) {
	enumerator := Power(1, []uint{0})
	checkEnumerator(t, enumerator, [][]uint{{0}}, arrayEquals)
}

func Test_Power_1_2(t *testing.T) {
	enumerator := Power(1, []uint{0, 1})
	checkEnumerator(t, enumerator, [][]uint{{0}, {1}}, arrayEquals)
}

func Test_Power_1_3(t *testing.T) {
	enumerator := Power(1, []uint{0, 1, 2})
	checkEnumerator(t, enumerator, [][]uint{{0}, {1}, {2}}, arrayEquals)
}

func Test_Power_2_1(t *testing.T) {
	enumerator := Power(2, []uint{0})
	checkEnumerator(t, enumerator, [][]uint{{0, 0}}, arrayEquals)
}

func Test_Power_2_2(t *testing.T) {
	enumerator := Power(2, []uint{0, 1})
	checkEnumerator(t, enumerator, [][]uint{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, arrayEquals)
}

func Test_Power_2_3(t *testing.T) {
	enumerator := Power(2, []uint{0, 1, 2})
	checkEnumerator(t, enumerator, [][]uint{
		{0, 0}, {1, 0}, {2, 0}, {0, 1}, {1, 1}, {2, 1}, {0, 2}, {1, 2}, {2, 2}}, arrayEquals)
}

func Test_Power_3_1(t *testing.T) {
	enumerator := Power(3, []uint{0})
	checkEnumerator(t, enumerator, [][]uint{{0, 0, 0}}, arrayEquals)
}
func Test_Power_3_2(t *testing.T) {
	enumerator := Power(3, []uint{0, 1})
	checkEnumerator(t, enumerator, [][]uint{
		{0, 0, 0}, {1, 0, 0}, {0, 1, 0}, {1, 1, 0}, {0, 0, 1}, {1, 0, 1}, {0, 1, 1}, {1, 1, 1}}, arrayEquals)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkEnumerator[E any](t *testing.T, enumerator Enumerator[E], expected []E, eq func(E, E) bool) {
	for i := 0; i < len(expected); i++ {
		ith := enumerator.Next()
		if !eq(ith, expected[i]) {
			t.Errorf("expected %v, got %v", expected[i], ith)
		}
	}
	// Sanity check lengths match
	if enumerator.HasNext() {
		t.Errorf("expected %d elements, got more", len(expected))
	}
}

func arrayEquals[T comparable](lhs []T, rhs []T) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	// Check each item in turn
	for i := 0; i < len(lhs); i++ {
		if lhs[i] != rhs[i] {
			return false
		}
	}
	// Done
	return true
}
