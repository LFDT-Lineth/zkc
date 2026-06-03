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
package micro

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

// SplitPolynomial splits the registers in a given polynomial into their limbs,
// producing an equivalent (but not necessarily identical) polynomial.  For
// example, suppose that X and Y split into limbs X'1, X'0 and Y'1, Y'0.  Then
// the polynomial 2*X + Y splits into 512*X'1 + 2*X'0 + 256*Y'1 + Y'0.
func SplitPolynomial(p Polynomial, env register.LimbsMap) Polynomial {
	var npoly Polynomial
	//
	for i := range p.Len() {
		ith := SplitMonomial(p.Term(i), env)
		//
		if i == 0 {
			npoly = ith
		} else {
			npoly = npoly.Add(ith)
		}
	}
	// Sanity check p was not zero.
	if npoly != nil {
		// No, p was not zero.
		return npoly
	}
	// Yes, p was zero (hence, had no terms).
	return p
}

// SplitMonomial splits a given monomial (e.g. 2*x*y) according to a given
// register-to-limb mapping.  For example, suppose x is u16 and maps to x'0 and
// x'1 (both u8), whilst y maps to itself.  Then, the resulting polynomial is:
//
// 2*(x'0 + 256*x'1)*y --> (2*x'0*y) + (512*x'1*y)
//
// Of course, things get more involved when more than one register is being
// split, but the basic idea above applies.
func SplitMonomial(p agnostic.StaticMonomial, env register.LimbsMap) Polynomial {
	var res Polynomial
	// FIXME: what to do with the coefficient?  This is a problem because its
	// not clear how we should split this.  Presumably it should be split
	// according to the maximum register width.
	res = res.Set(poly.NewMonomial[register.Id](p.Coefficient()))
	//
	for i := range p.Len() {
		// Determine limbs corresponding to the given constraint.
		limbs := env.LimbIds(p.Nth(i))
		// Construct polynomial representing limbs
		ith := LimbPolynomial(limbs, env)
		//
		res = res.Mul(ith)
	}
	//
	return res
}

// LimbPolynomial constructs a polynomial from the given limbs which represents
// the value of the original register.  For example, suppose x is a u16 register
// which splits into two u8 limbs x'0 and x'1.  Then, the constructed "limb
// polynomial" is simply x'0 + 256*x'1 (recall that x'0 is the last significant
// limb).
func LimbPolynomial(limbs []register.Id, env register.LimbsMap) Polynomial {
	var (
		res Polynomial
		// Offset is used to determine the coefficient for the next limb.
		offset big.Int = *big.NewInt(1)
		//
		terms = make([]agnostic.StaticMonomial, len(limbs))
	)
	//
	for i, rid := range limbs {
		var (
			coeff big.Int
			reg   = env.Limb(rid)
		)
		// Clone coefficient
		coeff.Set(&offset)
		// Construct term
		terms[i] = poly.NewMonomial(coeff, rid)
		// Shift offset up
		offset.Lsh(&offset, reg.Width())
	}
	// Done
	return res.Set(terms...)
}
