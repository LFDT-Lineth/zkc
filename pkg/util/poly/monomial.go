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
	"slices"
	"sort"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

var zero big.Int

// Monomial represents a monomial within an array polynomial.
type Monomial[S util.Comparable[S]] struct {
	coefficient big.Int
	vars        []S
}

// NewMonomial constructs a new array term with a given coefficient and zero or
// more variables.
func NewMonomial[S util.Comparable[S]](coefficient big.Int, vars ...S) Monomial[S] {
	// Clone incoming variables
	vars = slices.Clone(vars)
	// Sort incoming variables
	sortVars(vars)
	//
	return Monomial[S]{coefficient, vars}
}

// Clone an array term
func (p Monomial[S]) Clone() Monomial[S] {
	var (
		val   big.Int
		nvars = make([]S, len(p.vars))
	)
	// Copy variables
	copy(nvars, p.vars)
	// Copy coefficient
	val.Set(&p.coefficient)
	//
	return Monomial[S]{val, nvars}
}

// Coefficient returns the coefficient of this term.
func (p Monomial[S]) Coefficient() big.Int {
	return p.coefficient
}

// Contains checks whether this monomial contains the given variable, or not.
func (p Monomial[S]) Contains(v S) bool {
	// employ binary search to find the item
	_, res := sort.Find(len(p.vars), func(i int) int {
		return v.Cmp(p.vars[i])
	})
	//
	return res
}

// Cmp implementation for the Comparable interface
func (p Monomial[S]) Cmp(other Monomial[S]) int {
	// Compare variables first.  Observe this is critical to ensuring correct
	// operation of the ArrayPoly.  That's because we have an invariant which
	// says we can change the coefficient of any moninial without changing its
	// position in the sorted set of monomials.
	if c := array.Compare(p.vars, other.vars); c != 0 {
		return c
	}
	//
	return p.coefficient.Cmp(&other.coefficient)
}

// FactorOut produces a fresh monomial containing one less occurrence of the
// given variable (if it is contained within).  Otherwise, it returns an
// identical monomial.
func (p Monomial[S]) FactorOut(v S) Monomial[S] {
	// employ binary search to find the item
	index, res := sort.Find(len(p.vars), func(i int) int {
		return v.Cmp(p.vars[i])
	})
	//
	if res {
		// Remove occurrence at matched index
		nvars := array.RemoveAt(p.vars, uint(index))
		// Construct fresh monomial
		return NewMonomial(p.coefficient, nvars...)
	}
	// Not contained, therefore construct fresh (but otherwise identical)
	// monomial
	return p.Clone()
}

// Equal performs structural equality between two mononomials.  That is, they
// are consider the same provide they have identical structure.
func (p Monomial[S]) Equal(other Monomial[S]) bool {
	if len(p.vars) != len(other.vars) {
		return false
	} else if p.coefficient.Cmp(&other.coefficient) != 0 {
		return false
	}
	//
	for i := range p.vars {
		if p.vars[i].Cmp(other.vars[i]) != 0 {
			return false
		}
	}
	//
	return true
}

// IsZero checks whether or not this monomial is zero.  Or, put another way,
// whether or not the coefficient of this monomial is zero.
func (p Monomial[S]) IsZero() bool {
	c := p.coefficient
	return c.BitLen() == 0
}

// IsNegative checks whether or not the coefficient for this monomial is
// negative.
func (p Monomial[S]) IsNegative() bool {
	c := p.coefficient
	return c.Cmp(&zero) < 0
}

// Negate the coefficient of this monomial
func (p Monomial[S]) Negate() Monomial[S] {
	c := p.Clone()
	c.coefficient.Neg(&c.coefficient)
	//
	return c
}

// Len returns the number of variables in this polynomial term.
func (p Monomial[S]) Len() uint {
	return uint(len(p.vars))
}

// Nth returns the nth variable in this polynomial term.
func (p Monomial[S]) Nth(index uint) S {
	return p.vars[index]
}

// Neg returns a negated copy of this monomial
func (p Monomial[S]) Neg() Monomial[S] {
	var res = p.Clone()
	// Negate Coefficient
	res.coefficient.Neg(&res.coefficient)
	// Done
	return res
}

// Mul returns a fresh monomial representing the multiplication of this monomial
// and another.
func (p Monomial[S]) Mul(other Monomial[S]) Monomial[S] {
	var res Monomial[S]
	// Multiply coefficients
	res.coefficient.Mul(&p.coefficient, &other.coefficient)
	// Append variables
	res.vars = array.MergeSorted(p.vars, other.vars)
	// Done
	return res
}

// MulScalar multiplies this monomial by scalar.
func (p Monomial[S]) MulScalar(scalar *big.Int) Monomial[S] {
	var res = p.Clone()
	// Multiply coefficients
	res.coefficient.Mul(&res.coefficient, scalar)
	// Done
	return res
}

// Matches determines whether or not the variables of this term match those
// of the other.
func (p Monomial[S]) Matches(other Monomial[S]) bool {
	if p.Len() != other.Len() {
		return false
	}
	//
	for i := uint(0); i < p.Len(); i++ {
		if p.vars[i].Cmp(other.Nth(i)) != 0 {
			return false
		}
	}
	//
	return true
}

// Shr performs a "shift right" on this monomial.
func (p Monomial[S]) Shr(n uint) (quot Monomial[S], rem Monomial[S]) {
	var (
		coeff               big.Int
		quotient, remainder big.Int
		neg                 = p.coefficient.Sign() < 0
	)
	// Handle negative values
	if neg {
		coeff.Abs(&p.coefficient)
	} else {
		coeff = p.coefficient
	}
	// Determine quotient and remainder
	quotient.Rsh(&coeff, n)
	remainder.Lsh(&quotient, n)
	remainder.Sub(&coeff, &remainder)
	// Handle negative values
	if neg {
		quotient.Neg(&quotient)
		remainder.Neg(&remainder)
	}
	// Done
	return Monomial[S]{quotient, p.vars}, Monomial[S]{remainder, p.vars}
}

// String constructs a suitable string representation for a given polynomial
// assuming an environment which maps identifiers to strings.
func (p Monomial[S]) String(env func(S) string) string {
	var (
		buf   bytes.Buffer
		coeff = p.Coefficient()
	)
	// Various cases to improve readability
	if p.Len() == 0 {
		buf.WriteString(coeff.String())
	} else if coeff.Cmp(big.NewInt(1)) != 0 {
		buf.WriteString("(")
		buf.WriteString(coeff.String())
		//
		for j := range p.Len() {
			buf.WriteString("*")
			//
			buf.WriteString(env(p.Nth(j)))
		}
		//
		buf.WriteString(")")
	} else if p.Len() == 1 {
		buf.WriteString(env(p.Nth(0)))
	} else {
		buf.WriteString("(")
		//
		for j := range p.Len() {
			if j != 0 {
				buf.WriteString("*")
			}
			//
			buf.WriteString(env(p.Nth(j)))
		}
		//
		buf.WriteString(")")
	}
	//
	return buf.String()
}

// Vars retursnt the variables of this monomial as an array.
func (p Monomial[S]) Vars() []S {
	return p.vars
}

func sortVars[S util.Comparable[S]](vars []S) {
	slices.SortFunc(vars, func(a, b S) int {
		return a.Cmp(b)
	})
}
