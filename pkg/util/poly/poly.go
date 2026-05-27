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
package poly

import (
	"bytes"
	"math/big"

	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/source/sexp"
)

// Polynomial represents a sum of terms of a type T of variables.
type Polynomial[S util.Comparable[S], T Term[S, T], P any] interface {
	// Len returns the number of terms in this polynomial.
	Len() uint

	// Term returns the ith term in this polynomial.
	Term(uint) T

	// Initialise this polynomial from zero or more terms, returnining this.
	Set(...T) P

	// Initialise this polynomial from a 64bit constant.
	Const64(uint64) P

	// Cmp provides an ordering over polynomials.
	Cmp(o P) int

	// IsZero returns an indication as to whether this polynomial is equivalent
	// to zero (or not).  This is a three valued logic system which can return
	// either "yes", "no" or "maybe" where: (i) "yes" means the polynomial
	// always evaluates to zero; (ii) "no" means the polynomial never evaluates
	// to zero; (iii) "maybe" indicates the polynomial may sometimes evaluate to
	// zero.  When the return ok holds then res indicates either yes or not.
	// Otherwise, the result is maybe.
	IsZero() (res bool, ok bool)

	// Add another polynomial onto this polynomial, such that this polynomial is
	// updated in place.
	Add(P) P

	// Subtract another polynomial from this polynomial, such that this
	// polynomial is updated in place.
	Sub(P) P

	// Multiply this polynomial by another polynomial, such that this polynomial
	// is updated in place.
	Mul(P) P

	// Negate each mononial in this polynomial.
	Negate() P

	// For a given bitwidth n, divide a polynomial by 2^n produces a quotient and
	// remainder.  For example, dividing 256*x1+x0 by 2^8 gives x1 remainder x0.
	// This algorithm is somehow akin to "shifting" a polynomial downwards.  For
	// example, consider our example again:
	//
	//	 15             8 7               0
	//	+----------------+-----------------+
	//	|     2^8*x1     |        x0       |
	//	+----------------+-----------------+
	//
	// Then, shifting this down by 8bits gives:
	//
	//	                  7               0
	//	                 +-----------------+
	//	>>>>>>>>>>>>>>>> |        x1       |
	//	                 +-----------------+
	//
	// And we are left with a remainder as well.
	Shr(n uint) (quot P, rem P)
}

// Eval evaluates a given polynomial with a given environment (i.e. mapping of variables to values)
func Eval[S util.Comparable[S], T Term[S, T], P Polynomial[S, T, P]](poly P, env func(S) big.Int) *big.Int {
	val := big.NewInt(0)
	// Sum evaluated terms
	for i := uint(0); i < poly.Len(); i++ {
		ith := evalTerm(poly.Term(i), env)
		val.Add(val, ith)
	}
	// Done
	return val
}

func evalTerm[S util.Comparable[S], T Term[S, T]](term T, env func(S) big.Int) *big.Int {
	var (
		acc   big.Int
		coeff big.Int = term.Coefficient()
	)
	// Initialise accumulator
	acc.Set(&coeff)
	//
	for j := uint(0); j < term.Len(); j++ {
		jth := env(term.Nth(j))
		acc.Mul(&acc, &jth)
	}
	//
	return &acc
}

// String constructs a suitable string representation for a given polynomial
// assuming an environment which maps identifiers to strings.
func String[S util.Comparable[S], T Term[S, T], P Polynomial[S, T, P]](poly P, env func(S) string) string {
	var buf bytes.Buffer
	//
	if poly.Len() == 0 {
		return "0"
	}
	//
	for i := range poly.Len() {
		ith := poly.Term(i)
		coeff := ith.Coefficient()
		//
		if i != 0 && coeff.Sign() >= 0 {
			buf.WriteString("+")
		} else if i != 0 {
			buf.WriteString("-")
			coeff.Neg(&coeff)
		}
		// Various cases to improve readability
		if ith.Len() == 0 {
			buf.WriteString(coeff.String())
		} else if coeff.Cmp(big.NewInt(1)) != 0 {
			buf.WriteString("(")
			buf.WriteString(coeff.String())
			//
			for j := range ith.Len() {
				buf.WriteString("*")
				//
				buf.WriteString(env(ith.Nth(j)))
			}
			//
			buf.WriteString(")")
		} else if ith.Len() == 1 {
			buf.WriteString(env(ith.Nth(0)))
		} else {
			buf.WriteString("(")
			//
			for j := range ith.Len() {
				if j != 0 {
					buf.WriteString("*")
				}
				//
				buf.WriteString(env(ith.Nth(j)))
			}
			//
			buf.WriteString(")")
		}
	}
	//
	return buf.String()
}

var one = big.NewInt(1)

// Lisp constructs a suitable lisp representation for a given polynomial
// assuming an environment which maps identifiers to strings.
func Lisp[S util.Comparable[S], T Term[S, T], P Polynomial[S, T, P]](poly P, env func(S) string) sexp.SExp {
	var terms []sexp.SExp
	//
	terms = append(terms, sexp.NewSymbol("+"))
	//
	for i := range poly.Len() {
		var (
			ith   = poly.Term(i)
			coeff = ith.Coefficient()
			isOne = coeff.Cmp(one) == 0
		)
		// Case analysis
		switch {
		case isOne && ith.Len() == 0:
			terms = append(terms, sexp.NewSymbol(coeff.String()))
		case isOne && ith.Len() == 1:
			terms = append(terms, sexp.NewSymbol(env(ith.Nth(0))))
		default:
			term := []sexp.SExp{sexp.NewSymbol("*")}
			//
			if !isOne {
				term = append(term, sexp.NewSymbol(coeff.String()))
			}
			// Append variables
			for j := range ith.Len() {
				jth := env(ith.Nth(j))
				term = append(term, sexp.NewSymbol(jth))
			}
			//
			terms = append(terms, sexp.NewList(term))
		}
	}
	//
	return sexp.NewList(terms)
}
