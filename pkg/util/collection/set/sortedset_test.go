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
package set

import (
	"fmt"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

func Test_SortedSet_00(t *testing.T) {
	check_SortedSet_Insert(t, 5, 10)
	check_SortedSet_InsertSorted(t, 5, 10)
}

func Test_SortedSet_01(t *testing.T) {
	// Really hammer it.
	for i := 0; i < 100000; i++ {
		t.Run(fmt.Sprintf("i=%d", i), func(t *testing.T) {
			check_SortedSet_Insert(t, 10, 32)
			check_SortedSet_InsertSorted(t, 10, 32)
		})
	}
}

func Test_SortedSet_02(t *testing.T) {
	check_SortedSet_Insert(t, 100, 32)
	check_SortedSet_InsertSorted(t, 50, 32)
}

func Test_SortedSet_03(t *testing.T) {
	check_SortedSet_Insert(t, 1000, 64)
	check_SortedSet_InsertSorted(t, 500, 64)
}

func Test_SortedSet_04(t *testing.T) {
	check_SortedSet_Insert(t, 100000, 1024)
	check_SortedSet_InsertSorted(t, 50000, 1024)
}

func TestSlow_SortedSet_05(t *testing.T) {
	check_SortedSet_Insert(t, 100000, 4096)
	check_SortedSet_InsertSorted(t, 50000, 4096)
}

func TestSlow_SortedSet_06(t *testing.T) {
	check_SortedSet_Insert(t, 100000, 16384)
	check_SortedSet_InsertSorted(t, 50000, 16384)
}

// ===================================================================
// Test Helpers
// ===================================================================

func array_contains(items []uint, element uint) bool {
	for _, e := range items {
		if e == element {
			return true
		}
	}
	// Not present
	return false
}

func check_SortedSet_Insert(t *testing.T, n uint, m uint) {
	//
	t.Parallel()
	//
	items := util.GenerateRandomUints(n, m)
	aset := toSortedSet(items)
	anyset := toAnySortedSet(items)

	for i := uint(0); i < m; i++ {
		l := array_contains(items, i)
		r := aset.Contains(i)
		// Check set
		if !l && r {
			t.Errorf("unexpected item %d", i)
		} else if l && !r {
			t.Errorf("missing item %d", i)
		}
		// Check anyset
		r = anyset.Contains(Order[uint]{Item: i})
		if !l && r {
			t.Errorf("unexpected item %d (any)", i)
		} else if l && !r {
			t.Errorf("missing item %d (any)", i)
		}
	}
}

func check_SortedSet_InsertSorted(t *testing.T, n uint, m uint) {
	left := util.GenerateRandomUints(n, m)
	right := util.GenerateRandomUints(n, m)
	aset := toSortedSet(left)
	anyset := toAnySortedSet(left)

	aset.InsertSorted(toSortedSet(right))
	anyset.InsertSorted(toAnySortedSet(right))
	//
	for i := uint(0); i < m; i++ {
		l := array_contains(left, i) || array_contains(right, i)
		r := aset.Contains(i)
		// Check set
		if !l && r {
			t.Errorf("unexpected item %d", i)
		} else if l && !r {
			t.Errorf("missing item %d", i)
		}
		// Check any set
		r = anyset.Contains(Order[uint]{Item: i})
		if !l && r {
			t.Errorf("unexpected item %d (any)", i)
		} else if l && !r {
			t.Errorf("missing item %d (any)", i)
		}
	}
}

func toSortedSet(items []uint) *SortedSet[uint] {
	set := NewSortedSet[uint]()
	for _, v := range items {
		set.Insert(v)
	}

	return set
}

func toAnySortedSet(items []uint) *AnySortedSet[Order[uint]] {
	aset := NewAnySortedSet[Order[uint]]()
	for _, v := range items {
		aset.Insert(Order[uint]{Item: v})
	}

	return aset
}
