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
package poly

import (
	"fmt"
	"math/big"
	"testing"
)

type Poly = *ArrayPoly[Var]

type Pt struct {
	x, y, z int
}

// POINTS is a collection of points
var POINTS = []int{-3, -2, -1, 0, 1, 2, 3, 4, 5, 12, 17, 87, 102, 103, 104}

var X_POINTS []Pt
var XY_POINTS []Pt
var XYZ_POINTS []Pt

func init() {
	var n = len(POINTS)
	//
	X_POINTS = make([]Pt, n)
	//
	for i, x := range POINTS {
		X_POINTS[i] = Pt{x: x}
	}
	//
	XY_POINTS = make([]Pt, n*n)
	//
	for i, x := range POINTS {
		for j, y := range POINTS {
			XY_POINTS[(i*n)+j] = Pt{x: x, y: y}
		}
	}
	//
	XYZ_POINTS = make([]Pt, n*n*n)
	//
	for i, x := range POINTS {
		for j, y := range POINTS {
			for k, z := range POINTS {
				XYZ_POINTS[(i*n*n)+(j*n)+k] = Pt{x: x, y: y, z: z}
			}
		}
	}
}

// ============================================================================
// Single Variable Tests
// ============================================================================

func Test_PolyEval_01(t *testing.T) {
	check(t, "123", X_POINTS, func(x, y, z int) int { return 123 })
}

func Test_PolyEval_02(t *testing.T) {
	check(t, "x", X_POINTS, func(x, y, z int) int { return x })
}

func Test_PolyEval_03(t *testing.T) {
	check(t, "x + 1", X_POINTS, func(x, y, z int) int { return x + 1 })
}

func Test_PolyEval_04(t *testing.T) {
	check(t, "x + 3 + 1", X_POINTS, func(x, y, z int) int { return x + 4 })
}

func Test_PolyEval_05(t *testing.T) {
	check(t, "x - 1", X_POINTS, func(x, y, z int) int { return x - 1 })
}

func Test_PolyEval_06(t *testing.T) {
	check(t, "x - 9 - 1", X_POINTS, func(x, y, z int) int { return x - 10 })
}

func Test_PolyEval_07(t *testing.T) {
	check(t, "2 * x", X_POINTS, func(x, y, z int) int { return 2 * x })
}
func Test_PolyEval_08(t *testing.T) {
	check(t, "x + x", X_POINTS, func(x, y, z int) int { return 2 * x })
}

func Test_PolyEval_09(t *testing.T) {
	check(t, "x + (x - x)", X_POINTS, func(x, y, z int) int { return x })
}

func Test_PolyEval_10(t *testing.T) {
	check(t, "2 * (x + 1)", X_POINTS, func(x, y, z int) int { return 2 * (x + 1) })
}

func Test_PolyEval_11(t *testing.T) {
	check(t, "(2 * x) + 1", X_POINTS, func(x, y, z int) int { return (2 * x) + 1 })
}

func Test_PolyEval_12(t *testing.T) {
	check(t, "x * x", X_POINTS, func(x, y, z int) int { return x * x })
}

// ============================================================================
// Double Variable Tests
// ============================================================================

func Test_PolyEval_20(t *testing.T) {
	check(t, "x + y", XY_POINTS, func(x, y, z int) int { return x + y })
}

func Test_PolyEval_21(t *testing.T) {
	check(t, "(2 * x) + y", XY_POINTS, func(x, y, z int) int { return x + x + y })
}

func Test_PolyEval_22(t *testing.T) {
	check(t, "x + (2 * y)", XY_POINTS, func(x, y, z int) int { return x + y + y })
}

func Test_PolyEval_23(t *testing.T) {
	check(t, "x + 1 + (2 * y)", XY_POINTS, func(x, y, z int) int { return x + 1 + y + y })
}
func Test_PolyEval_24(t *testing.T) {
	check(t, "x * y", XY_POINTS, func(x, y, z int) int { return x * y })
}
func Test_PolyEval_25(t *testing.T) {
	check(t, "(x * x * x) - (y * y)", XY_POINTS, func(x, y, z int) int { return (x * x * x) - (y * y) })
}
func Test_PolyEval_26(t *testing.T) {
	check(t, "(2 * x * x) + y", XY_POINTS, func(x, y, z int) int { return (2 * x * x) + y })
}

// ============================================================================
// Triple Variable Tests
// ============================================================================
func Test_PolyEval_40(t *testing.T) {
	check(t, "x + y + z", XYZ_POINTS, func(x, y, z int) int { return x + y + z })
}

func Test_PolyEval_41(t *testing.T) {
	check(t, "x + (y - z)", XYZ_POINTS, func(x, y, z int) int { return x + (y - z) })
}

func Test_PolyEval_42(t *testing.T) {
	check(t, "x - (y + z)", XYZ_POINTS, func(x, y, z int) int { return x - (y + z) })
}

func Test_PolyEval_43(t *testing.T) {
	check(t, "(x - y) + z", XYZ_POINTS, func(x, y, z int) int { return (x - y) + z })
}

func Test_PolyEval_44(t *testing.T) {
	check(t, "(x - y) + z + (2 * x)", XYZ_POINTS, func(x, y, z int) int { return x + x + x - y + z })
}

// ============================================================================
// Helpers
// ============================================================================

// Check the evaluation of a polynomial at evaluation given points.
func check(t *testing.T, input string, points []Pt, fn func(int, int, int) int) {
	// Parse the polynomial, producing one or more errors.
	if p, errs := Parse(input); len(errs) != 0 {
		t.Error(errs)
	} else {
		// Evaluate the polynomial at the given points, recalling that the first
		// point is always the outcome.
		for _, pnt := range points {
			env := make(map[string]big.Int)
			env["x"] = *big.NewInt(int64(pnt.x))
			env["y"] = *big.NewInt(int64(pnt.y))
			env["z"] = *big.NewInt(int64(pnt.z))
			actual := Eval(p, func(v Var) big.Int { return env[v.name] })
			expected := big.NewInt(int64(fn(pnt.x, pnt.y, pnt.z)))
			// Evaluate and check
			if actual.Cmp(expected) != 0 {
				err := fmt.Sprintf("incorrect evaluation (was %s, expected %s)", actual.String(), expected.String())
				t.Error(err)
			}
		}
	}
}
