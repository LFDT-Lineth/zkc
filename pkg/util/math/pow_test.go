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
package math

import "testing"

func Test_Pow_0(t *testing.T) {
	check(0, t)
}

func Test_Pow_1(t *testing.T) {
	check(1, t)
}

func Test_Pow_2(t *testing.T) {
	check(2, t)
}

func Test_Pow_3(t *testing.T) {
	check(3, t)
}

func Test_Pow_4(t *testing.T) {
	check(4, t)
}

func Test_Pow_5(t *testing.T) {
	check(5, t)
}

func check(base uint64, t *testing.T) {
	for i := uint64(0); i < 10; i++ {
		// Bruteforce solution
		e := bruteForce(base, i)
		// Check for a match
		if x := PowUint64(base, i); x != e {
			t.Errorf("2^%d == %d != %d", i, x, e)
		}
	}
}

func bruteForce(base, exp uint64) uint64 {
	acc := uint64(1)
	for i := uint64(0); i < exp; i++ {
		acc *= base
	}

	return acc
}
