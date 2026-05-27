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
	"math/big"
	"slices"

	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/set"
)

// ArrayPoly is the simpliest (and least efficient) polynomial implementation.
// It provides a reference against which other (more efficient) implementations
// can be compared.  Observe that an unitialised ArrayPoly variable corresponds
// with zero.
type ArrayPoly[S util.Comparable[S]] struct {
	terms set.AnySortedSet[Monomial[S]]
}

// Len returns the number of terms in this polynomial.
func (p *ArrayPoly[S]) Len() uint {
	if p == nil {
		return 0
	}
	//
	return uint(len(p.terms))
}

// Term returns the ith term in this polynomial.
func (p *ArrayPoly[S]) Term(ith uint) Monomial[S] {
	return p.terms[ith]
}

// Cmp implementation for Comparable interface
func (p *ArrayPoly[S]) Cmp(other *ArrayPoly[S]) int {
	//
	return slices.CompareFunc(p.terms, other.terms, func(l Monomial[S], r Monomial[S]) int {
		return l.Cmp(r)
	})
}

// Set initialises this polynomial from zero or more terms.
func (p *ArrayPoly[S]) Set(terms ...Monomial[S]) *ArrayPoly[S] {
	var poly ArrayPoly[S]
	//
	for _, t := range terms {
		poly.AddTerm(t)
	}
	//
	if p != nil {
		p.terms = poly.terms
		return p
	}
	//
	return &poly
}

// Const64 Initialise this polynomial from a 64bit constant.
func (p *ArrayPoly[S]) Const64(val uint64) *ArrayPoly[S] {
	// Special case for zero
	if val == 0 {
		return &ArrayPoly[S]{}
	}
	//
	coeff := big.NewInt(int64(val))
	//
	return p.Set(NewMonomial[S](*coeff))
}

// Clone performs a deep copy of this polynomial
func (p *ArrayPoly[S]) Clone() *ArrayPoly[S] {
	nterms := make([]Monomial[S], len(p.terms))
	//
	for i := range nterms {
		nterms[i] = p.terms[i].Clone()
	}
	//
	return &ArrayPoly[S]{nterms}
}

// Equal performs structural equality between two polynomials.  That is, they
// are consider the same provide they have identical structure.
func (p *ArrayPoly[S]) Equal(other *ArrayPoly[S]) bool {
	if len(p.terms) != len(other.terms) {
		return false
	}
	//
	for i := range len(p.terms) {
		if !p.terms[i].Equal(other.terms[i]) {
			return false
		}
	}
	//
	return true
}

// Signed determines whether or not this polynomial can evaluate to both
// positive and negative values.  Currently, this is defined simply as whether
// or not a contained monomial has a negative coefficient.
func (p *ArrayPoly[S]) Signed() bool {
	for i := range len(p.terms) {
		if p.terms[i].IsNegative() {
			return true
		}
	}
	//
	return false
}

// IsZero returns an indication as to whether this polynomial is equivalent
// to zero (or not).  This is a three valued logic system which can return
// either "yes", "no" or "maybe" where: (i) "yes" means the polynomial
// always evaluates to zero; (ii) "no" means the polynomial never evaluates
// to zero; (iii) "maybe" indicates the polynomial may sometimes evaluate to
// zero.  When the return ok holds then res indicates either yes or not.
// Otherwise, the result is maybe.
func (p *ArrayPoly[S]) IsZero() (res bool, ok bool) {
	panic("todo")
}

// Negate polynomial
func (p *ArrayPoly[S]) Negate() *ArrayPoly[S] {
	var res ArrayPoly[S]
	//
	for _, t := range p.terms {
		res.AddTerm(t.Negate())
	}
	//
	return &res
}

// Add another polynomial onto this polynomial.
func (p *ArrayPoly[S]) Add(other *ArrayPoly[S]) *ArrayPoly[S] {
	var res = p.Clone()
	//
	for i := uint(0); i < other.Len(); i++ {
		res.AddTerm(other.Term(i))
	}
	//
	return res
}

// AddScalar adds a signed scalar value to this polynomial
func (p *ArrayPoly[S]) AddScalar(scalar *big.Int) *ArrayPoly[S] {
	var res = p.Clone()
	//
	res.AddTerm(NewMonomial[S](*scalar))
	//
	return res
}

// Sub another polynomial from this polynomil
func (p *ArrayPoly[S]) Sub(other *ArrayPoly[S]) *ArrayPoly[S] {
	var res = p.Clone()
	//
	for i := uint(0); i < other.Len(); i++ {
		res.SubTerm(other.Term(i))
	}
	//
	return res
}

// Mul this polynomial by another polynomial.
func (p *ArrayPoly[S]) Mul(other *ArrayPoly[S]) *ArrayPoly[S] {
	var res ArrayPoly[S]
	//
	for _, ith := range p.terms {
		for _, jth := range other.terms {
			res.AddTerm(ith.Mul(jth))
		}
	}
	//
	return &res
}

// MulScalar multiplies this polynomial by scalar.
func (p *ArrayPoly[S]) MulScalar(scalar *big.Int) *ArrayPoly[S] {
	var res ArrayPoly[S]
	//
	for _, ith := range p.terms {
		res.AddTerm(ith.MulScalar(scalar))
	}
	//
	return &res
}

// AddTerm adds a single term into this polynomial.
func (p *ArrayPoly[S]) AddTerm(other Monomial[S]) {
	// Avoid adding an empty monomial
	if !other.IsZero() {
		for i, term := range p.terms {
			if term.Matches(other) {
				ith := &p.terms[i]
				// Add term at this position
				ith.coefficient.Add(&ith.coefficient, &other.coefficient)
				// Check whether its now zero (or not)
				if ith.IsZero() {
					p.terms = array.RemoveAt(p.terms, uint(i))
				}
				//
				return
			}
		}
		//
		p.terms.Insert(other.Clone())
	}
}

// SubTerm subtracts a single term from this polynomial.
func (p *ArrayPoly[S]) SubTerm(other Monomial[S]) {
	// Avoid subtracting an empty monomial
	if !other.IsZero() {
		for i, term := range p.terms {
			if term.Matches(other) {
				ith := &p.terms[i]
				// Sub term at this position
				ith.coefficient.Sub(&ith.coefficient, &other.coefficient)
				// Check whether its now zero (or not)
				if ith.IsZero() {
					p.terms = array.RemoveAt(p.terms, uint(i))
				}
				//
				return
			}
		}
		// Append negation to end
		p.terms.Insert(other.Neg())
	}
}

// Shr performs a "shift right" operation on this polynomial.
func (p *ArrayPoly[S]) Shr(n uint) (*ArrayPoly[S], *ArrayPoly[S]) {
	var (
		quotients  []Monomial[S]
		remainders []Monomial[S]
	)
	//
	for i := range p.Len() {
		var ith = p.Term(i)
		// Divide coefficient by bitwidth using shifts for efficiency.
		quot, rem := ith.Shr(n)
		//
		if !quot.IsZero() {
			quotients = append(quotients, quot)
		}
		//
		if !rem.IsZero() {
			remainders = append(remainders, rem)
		}
	}
	//
	return &ArrayPoly[S]{quotients}, &ArrayPoly[S]{remainders}
}
