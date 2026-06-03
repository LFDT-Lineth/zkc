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
package mir

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

var (
	biZERO big.Int = *big.NewInt(0)
	biONE  big.Int = *big.NewInt(1)
)

// Polynomial provides a useful alias
type Polynomial = agnostic.DynamicPolynomial

// ============================================================================
// Term => Polynomial
// ============================================================================

// Translate a term into a polynomial.
func termToPolynomial[F field.Element[F]](term Term[F], mapping register.Map) Polynomial {
	switch t := term.(type) {
	case *Add[F]:
		return termAddToPolynomial(*t, mapping)
	case *Constant[F]:
		return termConstantToPolynomial(t.Value, mapping)
	case *RegisterAccess[F]:
		return termRegAccessToPolynomial(*t)
	case *Mul[F]:
		return termMulToPolynomial(*t, mapping)
	case *Sub[F]:
		return termSubToPolynomial(*t, mapping)
	case *VectorAccess[F]:
		return termVecAccessToPolynomial(*t, mapping)
	default:
		panic("unreachable")
	}
}

func termAddToPolynomial[F field.Element[F]](term Add[F], mapping register.Map) Polynomial {
	var result Polynomial
	//
	for i, e := range term.Args {
		ith := termToPolynomial(e, mapping)
		//
		if i == 0 {
			result = ith
		} else {
			result = result.Add(ith)
		}
	}
	//
	return result
}

func termConstantToPolynomial[F field.Element[F]](constant F, _ register.Map) Polynomial {
	var (
		result Polynomial
		value  big.Int
	)
	value.SetBytes(constant.Bytes())
	monomial := poly.NewMonomial[register.AccessId](value)
	//
	return result.Set(monomial)
}

func termMulToPolynomial[F field.Element[F]](term Mul[F], mapping register.Map) Polynomial {
	var result Polynomial
	//
	for i, e := range term.Args {
		ith := termToPolynomial(e, mapping)
		//
		if i == 0 {
			result = ith
		} else {
			result = result.Mul(ith)
		}
	}
	//
	return result
}

func termRegAccessToPolynomial[F field.Element[F]](term RegisterAccess[F]) Polynomial {
	var (
		identifier = term.Register().
				AccessOf(term.BitWidth()).
				Shift(term.RelativeShift()).
				Mask(term.MaskWidth())
		//
		monomial = poly.NewMonomial(biONE, identifier)
		result   Polynomial
	)
	//
	return result.Set(monomial)
}

func termSubToPolynomial[F field.Element[F]](term Sub[F], mapping register.Map) Polynomial {
	var result Polynomial
	//
	for i, e := range term.Args {
		ith := termToPolynomial(e, mapping)
		//
		if i == 0 {
			result = ith
		} else {
			result = result.Sub(ith)
		}
	}
	//
	return result
}

func termVecAccessToPolynomial[F field.Element[F]](term VectorAccess[F], _ register.Map) Polynomial {
	var (
		result Polynomial
		shift  uint = 0
	)
	// Ensure polynomial initialise for case when there are no variables.
	result = result.Const64(0)
	//
	for i, v := range term.Vars {
		var (
			regWidth = v.MaskWidth()
			ith      = termRegAccessToPolynomial(*v)
		)
		// Add to poly
		if i == 0 {
			result = ith
		} else {
			// Shift ith term
			ith = ith.MulScalar(util_math.Pow2(shift))
			// Add ith term
			result = result.Add(ith)
		}
		// Increase shift
		shift += regWidth
	}
	// Done
	return result
}

// ============================================================================
// Polynomial => Term
// ============================================================================

// Switch to determine whether or not to apply the factoring algorithm. Overall,
// this benefits of using this algorithm thus far have been negligible and,
// hence, it is disabled by default.  However, the algorithm has been tested and
// appears to work.
const applyPolynomialFactoring = false

func polynomialToTerm[F field.Element[F]](poly Polynomial) Term[F] {
	if applyPolynomialFactoring {
		return factoredPolynomialToTerm[F](poly)
	}
	//
	return unfactoredPolynomialToTerm[F](poly)
}

// Translate a term into a polynomial.
func factoredPolynomialToTerm[F field.Element[F]](poly Polynomial) Term[F] {
	var r Term[F]
	//
	if rid, n := findCommonFactor(poly); n >= 2 {
		// Split polynomial into factored potion and unfactored portion
		factor, remainder := factorPolynomial(rid, poly)
		// Recursively translate the factored and unfactored portions
		lhs := factoredPolynomialToTerm[F](factor)
		rhs := factoredPolynomialToTerm[F](remainder)
		// Now recombine
		r = term.RawRegisterAccess[F, Term[F]](rid.Id(), rid.BitWidth(), rid.RelativeShift()).Mask(rid.MaskWidth())
		//
		return term.Sum(term.Product[F](lhs, r), rhs)
	}
	// No factors available, default to direct translation
	return unfactoredPolynomialToTerm[F](poly)
}

// Identify a variable which occurs in the most monomials out of any, returning
// that variable and the number of occurrences.
func findCommonFactor(poly Polynomial) (register.AccessId, uint) {
	var (
		uses    = make(map[register.AccessId]uint)
		maxUses uint
	)
	//
	for i := range poly.Len() {
		term := poly.Term(i)
		for j := range term.Len() {
			var (
				count = uint(0)
				jth   = term.Nth(j)
			)
			// check whether seen already
			if j > 0 && term.Nth(j-1).Cmp(jth) == 0 {
				continue
			} else if c, ok := uses[jth]; ok {
				count = c
			}
			//
			uses[jth] = count + 1
			maxUses = max(maxUses, count+1)
		}
	}
	// Extract first match
	for r, c := range uses {
		if c == maxUses {
			return r, maxUses
		}
	}
	// Default
	return register.AccessId{}, 0
}

// Factor a polynomial with respect to a given variable.  For example, consider
// factoring the polynomial "x.y + x.z + y" with respect to x.  Then, this will
// return "y+z" (the factor) and "y" (the remainder).
func factorPolynomial(rid register.AccessId, poly Polynomial) (Polynomial, Polynomial) {
	var (
		factor, remainder Polynomial
	)
	//
	for i := range poly.Len() {
		term := poly.Term(i)
		if term.Contains(rid) {
			nterm := term.FactorOut(rid)
			//
			if factor == nil {
				factor = factor.Set(nterm)
			} else {
				factor.AddTerm(nterm)
			}
		} else {
			if remainder == nil {
				remainder = remainder.Set(term)
			} else {
				remainder.AddTerm(term)
			}
		}
	}
	//
	return factor, remainder
}

func unfactoredPolynomialToTerm[F field.Element[F]](poly Polynomial) Term[F] {
	var (
		pos []Term[F]
		neg []Term[F]
	)
	//
	for i := range poly.Len() {
		ith := poly.Term(i)
		//
		if ith.IsNegative() {
			neg = append(neg, monomialToTerm[F](poly.Term(i)))
		} else {
			pos = append(pos, monomialToTerm[F](poly.Term(i)))
		}
	}
	// Handle negative monomials (if applicable)
	if len(neg) != 0 {
		return term.Subtract(term.Sum(pos...), term.Sum(neg...))
	}
	//
	return term.Sum(pos...)
}

func monomialToTerm[F field.Element[F]](monomial agnostic.DynamicMonomial) Term[F] {
	var (
		terms = make([]Term[F], monomial.Len()+1)
		tmp   = monomial.Coefficient()
		coeff F
	)
	// Add coefficient
	terms[0] = term.Const[F, Term[F]](coeff.SetBytes(tmp.Bytes()))
	//
	for i := range monomial.Len() {
		ith := monomial.Nth(i)
		ith_term := term.RawRegisterAccess[F, Term[F]](ith.Id(), ith.BitWidth(), ith.RelativeShift())
		terms[i+1] = ith_term.Mask(ith.MaskWidth())
	}
	//
	return term.Product(terms...)
}
