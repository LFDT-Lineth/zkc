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
package array

import (
	"reflect"
	"testing"
)

func Test_RemoveMatching_01(t *testing.T) {
	check_RemoveMatching(t, []int{1, 2}, 1, []int{2})
}

func Test_RemoveMatching_02(t *testing.T) {
	check_RemoveMatching(t, []int{1, 2}, 2, []int{1})
}

func Test_RemoveMatching_03(t *testing.T) {
	check_RemoveMatching(t, []int{1, 2, 3}, 1, []int{2, 3})
}

func Test_RemoveMatching_04(t *testing.T) {
	check_RemoveMatching(t, []int{2, 1, 3}, 1, []int{2, 3})
}

func Test_RemoveMatching_05(t *testing.T) {
	check_RemoveMatching(t, []int{2, 3, 1}, 1, []int{2, 3})
}

func Test_RemoveMatching_06(t *testing.T) {
	check_RemoveMatching(t, []int{1, 2, 3, 1}, 1, []int{2, 3})
}

func Test_RemoveMatching_07(t *testing.T) {
	check_RemoveMatching(t, []int{2, 1, 3, 1}, 1, []int{2, 3})
}
func Test_RemoveMatching_08(t *testing.T) {
	check_RemoveMatching(t, []int{2, 3, 1, 1}, 1, []int{2, 3})
}

func Test_RemoveMatching_09(t *testing.T) {
	check_RemoveMatching(t, []int{1, 2, 1, 3}, 1, []int{2, 3})
}

func Test_RemoveMatching_10(t *testing.T) {
	check_RemoveMatching(t, []int{1, 1, 2, 3}, 1, []int{2, 3})
}

func check_RemoveMatching(t *testing.T, original []int, item int, expected []int) {
	actual := RemoveMatching(original, func(ith int) bool { return ith == item })
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("removing %d from %v got %v", item, original, actual)
	}
}

// ============================================================================
// Reverse
// ============================================================================

func Test_Reverse_01(t *testing.T) {
	check_Reverse(t, []int{}, []int{})
}

func Test_Reverse_02(t *testing.T) {
	check_Reverse(t, []int{1}, []int{1})
}

func Test_Reverse_03(t *testing.T) {
	check_Reverse(t, []int{1, 2}, []int{2, 1})
}

func Test_Reverse_04(t *testing.T) {
	check_Reverse(t, []int{1, 2, 3}, []int{3, 2, 1})
}

func Test_Reverse_05(t *testing.T) {
	check_Reverse(t, []int{1, 2, 3, 4}, []int{4, 3, 2, 1})
}

func check_Reverse(t *testing.T, original []int, expected []int) {
	actual := Reverse(original)
	// Check functional reversal
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("reversing %v gave %v", original, actual)
	}
	// Check in place reversal
	ReverseInPlace(original)
	//
	if !reflect.DeepEqual(original, expected) {
		t.Errorf("reversing (in place) %v gave %v", original, expected)
	}
}
