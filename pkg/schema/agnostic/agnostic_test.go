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
package agnostic

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

var xRegId = register.NewId(0)
var yRegId = register.NewId(1)
var zRegId = register.NewId(2)

func x(coeff int64) StaticPolynomial {
	return Var(xRegId, coeff)
}

func y(coeff int64) StaticPolynomial {
	return Var(yRegId, coeff)
}

func z(coeff int64) StaticPolynomial {
	return Var(zRegId, coeff)
}

// Var constructs a polynomial representing a given variable multiplied by a
// given coefficient.
func Var(id register.Id, coeff int64) StaticPolynomial {
	var (
		p StaticPolynomial
		c = *big.NewInt(coeff)
	)
	//
	p = p.Set(poly.NewMonomial(c, id))
	//
	return p
}

func Test_Poly_01(t *testing.T) {
	// x where x:u8 requires 8 bits
	check(t, 8, x(1), 8)
}
func Test_Poly_02(t *testing.T) {
	// 2x where x:u8 requires 9 bits
	check(t, 9, x(2), 8)
}
func Test_Poly_03(t *testing.T) {
	// x+y where x:u8, y:u8 requires 9 bits
	check(t, 9, x(1).Add(y(1)), 8, 8)
}
func Test_Poly_03a(t *testing.T) {
	// x+2y where x:u8, y:u8 requires 10 bits
	check(t, 10, x(1).Add(y(2)), 8, 8)
}
func Test_Poly_04(t *testing.T) {
	// x+y where x:u16, y:u8 requires 17 bits
	check(t, 17, x(1).Add(y(1)), 16, 8)
}
func Test_Poly_05(t *testing.T) {
	// x-y where x:u8, y:u8 requires 9 bits
	check(t, 9, x(1).Sub(y(1)), 8, 8)
}
func Test_Poly_06(t *testing.T) {
	// x-y where x:u16, y:u8 requires 17 bits
	check(t, 17, x(1).Sub(y(1)), 16, 8)
}
func Test_Poly_07(t *testing.T) {
	// -2x + xy
	check(t, 4, x(-2).Add(y(-2).Add(y(1).Mul(z(1)))), 2, 1, 1)
}
func Test_Poly_08(t *testing.T) {
	// -2x - 1 -2y + yz
	check(t, 5, x(-2).AddScalar(&minusOne).Add(y(-2).Add(y(1).Mul(z(1)))), 2, 1, 1)
}

func check(t *testing.T, bitwidth uint, p StaticPolynomial, widths ...uint) {
	var regs = make([]register.Register, len(widths))
	//
	for i := range regs {
		regs[i] = register.NewInput("?", widths[i], big.Int{})
	}
	// Determine computed bitwidth
	actual, _ := WidthOfPolynomial(p, ArrayEnvironment(regs))
	// Check for any differences
	if actual != bitwidth {
		err := fmt.Sprintf("invalid bitwidth (expected %d got %d)", bitwidth, actual)
		t.Error(err)
	}
}
