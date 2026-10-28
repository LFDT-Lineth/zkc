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
package word

import (
	"cmp"
	"math"
	"math/rand/v2"
	"testing"
)

func Test_BigEndian_00(t *testing.T) {
	checkBigEndian(t, 0, 1)
}

func Test_BigEndian_01(t *testing.T) {
	for i := uint64(0); i < 10; i++ {
		for j := uint64(0); j < 10; j++ {
			checkBigEndian(t, i, j)
		}
	}
}
func Test_BigEndian_02(t *testing.T) {
	for i := uint64(0); i < 100; i++ {
		for j := uint64(0); j < 100; j++ {
			checkBigEndian(t, i, j)
		}
	}
}

func Test_BigEndian_03(t *testing.T) {
	for i := uint64(0); i < 1000; i++ {
		for j := uint64(0); j < 1000; j++ {
			checkBigEndian(t, i, j)
		}
	}
}

func Test_BigEndian_04(t *testing.T) {
	var max uint = uint(math.MaxInt64)
	//
	for i := uint64(0); i < 10000; i++ {
		l := rand.UintN(max)
		r := rand.UintN(max)
		checkBigEndian(t, uint64(l), uint64(r))
	}
}
func checkBigEndian(t *testing.T, lhs uint64, rhs uint64) {
	var (
		lw = Uint64[BigEndian](lhs)
		rw = Uint64[BigEndian](rhs)
	)
	//
	checkCmp(t, lw, rw, lhs, rhs)
	checkCmp64(t, lw, lhs, rhs)
}

func checkCmp(t *testing.T, lw, rw BigEndian, lhs, rhs uint64) {
	//
	c1 := lw.Cmp(rw)
	c2 := cmp.Compare(lhs, rhs)
	//
	if c1 != c2 {
		t.Errorf("invalid comparison: %d ~ %d = %d (expected %d)", lhs, rhs, c1, c2)
	}
}

func checkCmp64(t *testing.T, lw BigEndian, lhs, rhs uint64) {
	//
	c1 := lw.Cmp64(rhs)
	c2 := cmp.Compare(lhs, rhs)
	//
	if c1 != c2 {
		t.Errorf("invalid comparison: %d ~ %d = %d (expected %d)", lhs, rhs, c1, c2)
	}
}
