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
package logical

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

// Equation represents an equation between two polynomials, such as "0 == x + 1"
// or "0 != 2xy + 3", etc.
type Equation[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]] struct {
	// Sign indicates whether this is an Equality (==) or a non-Equality (!=).
	sign bool
	// Body of constraint.  Each of these terms must be positively signed.
	lhs, rhs P
}

// Vanishes constraints a new vanishing constraint (P == 0) for a given term P.
func Vanishes[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]](term P) Equation[S, T, P] {
	var (
		// NOTE: normalisation is quite weak, and does not normalise "x-y" versus
		// "y-x", etc.
		norm     = normalise(term)
		lhs, rhs = split(norm)
	)
	//
	return Equation[S, T, P]{true, lhs, rhs}
}

// LeftHandSide returns the left-hand side of this equation
func (p Equation[S, T, P]) LeftHandSide() P {
	return p.lhs
}

// RightHandSide returns the right-hand side of this equation
func (p Equation[S, T, P]) RightHandSide() P {
	return p.rhs
}

// CloseOver implementation for Atom interface
func (p Equation[S, T, P]) CloseOver(o Equation[S, T, P]) Equation[S, T, P] {
	return p
}

// Cmp implementation for Comparable interface
func (p Equation[S, T, P]) Cmp(o Equation[S, T, P]) int {
	if !p.sign && o.sign {
		return -1
	} else if p.sign && !o.sign {
		return 1
	} else if c := p.lhs.Cmp(o.lhs); c != 0 {
		return c
	}
	//
	return p.rhs.Cmp(o.rhs)
}

// Is implementation of Atom interface
func (p Equation[S, T, P]) Is(truth bool) bool {
	if !p.sign {
		truth = !truth
	}
	//
	if truth {
		return isZero(p.lhs) && isZero(p.rhs)
	}
	//
	return isNonZero(p.lhs) && isZero(p.rhs)
}

// Negate this Equality (i.e. turn it from "==" to "!=" or vice-versa)
func (p Equation[S, T, P]) Negate() Equation[S, T, P] {
	return Equation[S, T, P]{!p.sign, p.lhs, p.rhs}
}

// Sign returns the sign of this atom (either positive for equality or negative
// for non-equality).
func (p Equation[S, T, P]) Sign() bool {
	return p.sign
}

func (p Equation[S, T, P]) String(mapping func(S) string) string {
	var (
		lhs = poly.String(p.lhs, mapping)
		rhs = poly.String(p.rhs, mapping)
	)
	//
	if p.sign {
		return fmt.Sprintf("%s=%s", lhs, rhs)
	}
	//
	return fmt.Sprintf("%s≠%s", lhs, rhs)
}

func normalise[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]](term P) P {
	var signs int
	//
	for i := range term.Len() {
		if term.Term(i).IsNegative() {
			signs--
		} else {
			signs++
		}
	}
	//
	if signs < 0 {
		term = term.Negate()
	}
	//
	return term
}

func split[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]](term P) (lhs, rhs P) {
	var lhsTerms, rhsTerms []T
	//
	for i := range term.Len() {
		if t := term.Term(i); t.IsNegative() {
			lhsTerms = append(lhsTerms, t.Negate())
		} else {
			rhsTerms = append(rhsTerms, t)
		}
	}
	//
	return lhs.Set(lhsTerms...), rhs.Set(rhsTerms...)
}

func isZero[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]](p P) bool {
	// This works because polynomials which represent 0 are always empty.
	return p.Len() == 0
}

func isNonZero[S util.Comparable[S], T poly.Term[S, T], P poly.Polynomial[S, T, P]](p P) bool {
	return p.Len() == 1 && p.Term(0).Len() == 0
}
