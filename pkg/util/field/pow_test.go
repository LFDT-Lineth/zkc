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
package field

import (
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
)

const POW_BASE_MAX uint = 65536
const POW_BASE_INC uint = 8

func Test_Pow_00(t *testing.T) {
	PowCheck(t, 1, 1)
}
func Test_Pow_01(t *testing.T) {
	PowCheck(t, 2, 1)
}
func Test_Pow_02(t *testing.T) {
	PowCheck(t, 2, 2)
}
func Test_Pow_03(t *testing.T) {
	PowCheck(t, 2, 3)
}
func Test_Pow_04(t *testing.T) {
	PowCheck(t, 2, 4)
}
func Test_Pow_05(t *testing.T) {
	PowCheck(t, 3, 1)
}
func Test_Pow_06(t *testing.T) {
	PowCheck(t, 3, 2)
}
func Test_Pow_07(t *testing.T) {
	PowCheck(t, 3, 3)
}
func Test_Pow_08(t *testing.T) {
	PowCheck(t, 3, 4)
}
func Test_Pow_09(t *testing.T) {
	PowCheck(t, 3, 5)
}

func Test_Pow_10(t *testing.T) {
	PowCheckLoop(t, 0)
}

func Test_Pow_12(t *testing.T) {
	PowCheckLoop(t, 1)
}

func Test_Pow_13(t *testing.T) {
	PowCheckLoop(t, 2)
}

func Test_Pow_14(t *testing.T) {
	PowCheckLoop(t, 3)
}

func Test_Pow_15(t *testing.T) {
	PowCheckLoop(t, 4)
}

func Test_Pow_16(t *testing.T) {
	PowCheckLoop(t, 5)
}

func Test_Pow_17(t *testing.T) {
	PowCheckLoop(t, 6)
}

func Test_Pow_18(t *testing.T) {
	PowCheckLoop(t, 7)
}

func PowCheckLoop(t *testing.T, first uint) {
	// Enable parallel testing
	t.Parallel()
	// Run through the loop
	for i := first; i < POW_BASE_MAX; i += POW_BASE_INC {
		for j := uint64(0); j < 256; j++ {
			PowCheck(t, i, j)
		}
	}
}

// Check pow computed correctly.  This is done by comparing against the existing
// gnark function.
func PowCheck(t *testing.T, base uint, pow uint64) {
	var (
		k        = big.NewInt(int64(pow))
		actual   bls12_377.Element
		expected = fr.NewElement(uint64(base))
	)
	// Initialise actual value
	actual = actual.SetUint64(uint64(base))
	// Compute actual using our optimised method
	actual = Pow(actual, pow)
	// Compute expected using existing gnark function
	expected.Exp(expected, k)
	// Final sanity check
	if actual.Element.Cmp(&expected) != 0 {
		t.Errorf("Pow(%d,%d)=%s (not %s)", base, pow, actual.String(), expected.String())
	}
}
