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
package hash

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

func Test_HashMap_01(t *testing.T) {
	items := []uint{1, 2, 3, 4, 3, 2, 1}
	check_HashMap(t, items)
}

func Test_HashMap_02(t *testing.T) {
	items := util.GenerateRandomUints(10, 32)
	check_HashMap(t, items)
}

func Test_HashMap_03(t *testing.T) {
	items := util.GenerateRandomUints(100, 32)
	check_HashMap(t, items)
}

func Test_HashMap_04(t *testing.T) {
	items := util.GenerateRandomUints(1000, 32)
	check_HashMap(t, items)
}

func Test_HashMap_05(t *testing.T) {
	items := util.GenerateRandomUints(100000, 32)
	check_HashMap(t, items)
}

func TestSlow_HashMap_08(t *testing.T) {
	items := util.GenerateRandomUints(100000, 64)
	check_HashMap(t, items)
}

func TestSlow_HashMap_09(t *testing.T) {
	items := util.GenerateRandomUints(100000, 128)
	check_HashMap(t, items)
}

// ===================================================================
// Test Helpers
// ===================================================================

func check_HashMap(t *testing.T, items []uint) {
	t.Parallel()
	//
	gmap := initGoMap(items)
	hmap := NewMap[testKey, uint](0)
	// Insert items
	for key, val := range gmap {
		hmap.Insert(testKey{key}, val)
	}
	// Sanity check number of unique items
	if hmap.Size() != uint(len(gmap)) {
		t.Errorf("expected %d items, got %d: %s", len(gmap), hmap.Size(), hmap.String())
	}
	// Sanity check containership
	for key, val := range gmap {
		if !hmap.ContainsKey(testKey{key}) {
			t.Errorf("missing key %d: %s", key, hmap.String())
		} else if v, ok := hmap.Get(testKey{key}); !ok {
			t.Errorf("missing item %d=>%d: %s", key, val, hmap.String())
		} else if v != val {
			t.Errorf("expecting %d=>%d, got %d=>%d: %s", key, val, key, v, hmap.String())
		}
	}
}

func initGoMap(items []uint) map[uint]uint {
	gmap := make(map[uint]uint)
	//
	for _, v := range items {
		if w, ok := gmap[v]; ok {
			gmap[v] = w + 1
		} else {
			gmap[v] = 1
		}
	}
	//
	return gmap
}
