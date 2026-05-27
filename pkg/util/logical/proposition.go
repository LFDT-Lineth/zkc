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
	"math"
	"slices"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/set"
)

// Atom represents an indivisible part of a proposition.
type Atom[I any, A any] interface {
	array.Comparable[A]
	// Return the logical negation of this atom
	Negate() A
	// Check whether this atom is equivalent to logical truth or falsehood.
	Is(bool) bool
	// CloseOver this term and another, producing a potentially updated version
	// of this term.  For example, closing over "x=y" and "y=0" might given
	// "x=0", etc.
	CloseOver(o A) A
	// String returns a human-readable representation
	String(func(I) string) string
}

// Proposition provides an abstraction over logical statements made up from
// conjunctions and disjunctions of atoms (i.e. atomic formulae).  Currently,
// propositions are always stored in Disjunctive Normal Form (DNF).
type Proposition[I any, A Atom[I, A]] struct {
	conjuncts set.AnySortedSet[Conjunction[I, A]]
}

// Truth constructs either logical truth or logical false
func Truth[I any, A Atom[I, A]](val bool) Proposition[I, A] {
	if val {
		return Proposition[I, A]{nil}
	}
	//
	return Proposition[I, A]{[]Conjunction[I, A]{{nil}}}
}

// NewProposition constructs a proposition from a single atom.
func NewProposition[I any, A Atom[I, A]](atom A) Proposition[I, A] {
	var (
		disjuncts set.AnySortedSet[Conjunction[I, A]]
		conjuncts set.AnySortedSet[A]
	)
	//
	if atom.Is(true) {
		return Truth[I, A](true)
	} else if atom.Is(false) {
		return Truth[I, A](false)
	}
	//
	conjuncts.Insert(atom)
	disjuncts.Insert(Conjunction[I, A]{conjuncts})
	//
	return Proposition[I, A]{disjuncts}
}

// Clone this proposition making sure the resulting tree is disjoint
func (p *Proposition[I, A]) Clone() Proposition[I, A] {
	return Proposition[I, A]{slices.Clone(p.conjuncts)}
}

// Conjuncts returns the individual conjunctions which form this proposition.
func (p *Proposition[I, A]) Conjuncts() []Conjunction[I, A] {
	return p.conjuncts
}

// Equals returns true if the two propositions are identical.
func (p *Proposition[I, A]) Equals(other Proposition[I, A]) bool {
	if len(p.conjuncts) != len(other.conjuncts) {
		return false
	}
	//
	for i := range len(p.conjuncts) {
		if p.conjuncts[i].Cmp(other.conjuncts[i]) != 0 {
			return false
		}
	}
	//
	return true
}

// IsTrue checks whether or not this branch corresponds with logical truth or
// not.
func (p *Proposition[I, A]) IsTrue() bool {
	return len(p.conjuncts) == 0
}

// IsFalse checks whether or not this branch corresponds with logical false or
// not.
func (p *Proposition[I, A]) IsFalse() bool {
	return len(p.conjuncts) == 1 && len(p.conjuncts[0].atoms) == 0
}

// And returns the conjunction of two propositions.
func (p *Proposition[I, A]) And(other Proposition[I, A]) Proposition[I, A] {
	var br Proposition[I, A]
	//
	if p.IsFalse() || other.IsFalse() {
		return Truth[I, A](false)
	} else if p.IsTrue() {
		return other
	} else if other.IsTrue() {
		return *p
	}
	//
	for i, disjunct := range p.conjuncts {
		ith := andConjunctProposition(disjunct, other)
		//
		if i == 0 {
			br = ith
		} else {
			br = br.Or(ith)
		}
	}
	//
	return br
}

// Or returns the disjunction of two propositions.
func (p *Proposition[I, A]) Or(other Proposition[I, A]) Proposition[I, A] {
	var disjuncts set.AnySortedSet[Conjunction[I, A]]
	//
	if p.IsTrue() || other.IsTrue() {
		return Truth[I, A](true)
	} else if p.IsFalse() {
		return other
	} else if other.IsFalse() {
		return *p
	}
	//
	disjuncts.InsertSorted(&p.conjuncts)
	disjuncts.InsertSorted(&other.conjuncts)
	//
	return simplify(Proposition[I, A]{disjuncts})
}

// Negate returns the logical negation of this proposition.
func (p *Proposition[I, A]) Negate() Proposition[I, A] {
	var q Proposition[I, A]
	//
	if p.IsTrue() {
		return Truth[I, A](false)
	} else if p.IsFalse() {
		return Truth[I, A](true)
	}
	//
	for i, d := range p.conjuncts {
		ith := negateConjunct(d)
		//
		if i == 0 {
			q = ith
		} else {
			q = q.And(ith)
		}
	}
	//
	return q
}

func (p *Proposition[I, A]) String(mapping func(I) string) string {
	var (
		builder strings.Builder
		braces  = len(p.conjuncts) > 1
	)
	// check for true or false
	if p.IsFalse() {
		return "⊥"
	} else if p.IsTrue() {
		return "⊤"
	}
	//
	for i, c := range p.conjuncts {
		if i != 0 {
			builder.WriteString(" ∨ ")
		}
		//
		builder.WriteString(c.String(braces, mapping))
	}
	//
	return builder.String()
}

func simplify[I any, A Atom[I, A]](p Proposition[I, A]) Proposition[I, A] {
	var (
		n       = uint(len(p.conjuncts))
		changed = true
	)
	//
	for changed {
		changed = false
		// Outmost loop iterates unit terms, whilst innermost loop.
		for i := uint(0); i < n; i++ {
			for j := i + 1; j < n; j++ {
				if c, tautology := simplifyConjuncts(p, i, j); tautology {
					return Truth[I, A](true)
				} else {
					changed = changed || c
				}
			}
		}
	}
	// Resort the set, as it may be out of order after simplification has
	// completed.
	p.conjuncts = *set.RawAnySortedSet(p.conjuncts...)
	//
	return p
}

func simplifyConjuncts[I any, A Atom[I, A]](p Proposition[I, A], i, j uint) (bool, bool) {
	var (
		changed, tautology = unitPropagation(p, i, j)
	)
	//
	if !tautology {
		var (
			ith    = p.conjuncts[i]
			jth    = p.conjuncts[j]
			ithjth = ith.Implies(jth)
			jthith = jth.Implies(ith)
		)
		// NOTE: its possible that ith == jth here and, in such case, we'd
		// expect ithjth and jthith.
		switch {
		case ithjth && !jthith:
			p.conjuncts[j] = ith
		case !ithjth && jthith:
			p.conjuncts[i] = jth
		}
	}
	//
	return changed, tautology
}

func unitPropagation[I any, A Atom[I, A]](p Proposition[I, A], i, j uint) (bool, bool) {
	var (
		in = len(p.conjuncts[i].atoms)
		jn = len(p.conjuncts[j].atoms)
	)
	//
	if in == 1 && jn == 1 {
		var (
			ith = p.conjuncts[i].atoms[0]
			jth = p.conjuncts[j].atoms[0]
		)
		// Check for P || ~P
		return false, ith.Cmp(jth.Negate()) == 0
	} else if (in != 1 && jn != 1) || in == 0 || jn == 0 {
		return false, false
	} else if in > jn {
		i, j = j, i
	}
	// ASSERT: len(p.conjuncts[i].atoms) == 1
	var (
		ith = p.conjuncts[i].atoms[0].Negate()
		jth = p.conjuncts[j]
	)
	// Check whether anything to do
	if kth, ok := jth.Remove(ith); ok {
		p.conjuncts[j] = kth
		return true, false
	}
	//
	return false, false
}

func negateConjunct[I any, A Atom[I, A]](c Conjunction[I, A]) Proposition[I, A] {
	var br Proposition[I, A]
	//
	for i, a := range c.atoms {
		ith := NewProposition(a.Negate())
		//
		if i == 0 {
			br = ith
		} else {
			br = br.Or(ith)
		}
	}
	//
	return br
}

func andConjunctProposition[I any, A Atom[I, A]](c Conjunction[I, A], o Proposition[I, A]) Proposition[I, A] {
	var disjuncts set.AnySortedSet[Conjunction[I, A]]
	//
	for _, disjunct := range o.conjuncts {
		var nc Conjunction[I, A]
		nc.atoms.InsertSorted(&c.atoms)
		nc.atoms.InsertSorted(&disjunct.atoms)
		//
		if !nc.simplify() {
			continue
		}
		//
		disjuncts.Insert(nc)
	}
	// Sanity check
	if len(disjuncts) == 0 {
		return Truth[I, A](false)
	}
	// Done
	return Proposition[I, A]{disjuncts}
}

// ============================================================================
// conjunct
// ============================================================================

// Conjunction represents the conjunction of zero or more atoms.
type Conjunction[I any, A Atom[I, A]] struct {
	atoms set.AnySortedSet[A]
}

// Atoms returns the underlying atoms which are conjuncted together.
func (p Conjunction[I, A]) Atoms() []A {
	return p.atoms
}

// Cmp implementation for Comparable interface
func (p Conjunction[I, A]) Cmp(o Conjunction[I, A]) int {
	return array.Compare(p.atoms, o.atoms)
}

// Remove an atom from this conjunction (if it is contained within), or simply
// return this conjunction.
func (p Conjunction[I, A]) Remove(atom A) (Conjunction[I, A], bool) {
	if i := p.atoms.Find(atom); i != math.MaxUint {
		natoms := array.RemoveAt(p.atoms, i)
		// Yes removed.
		return Conjunction[I, A]{natoms}, true
	}
	// Nothing doing
	return p, false
}

// Implies checks whether this conjunction implies another.  For example, A
// implies (A B), whilst (A B) implies (A B C), etc.
func (p Conjunction[I, A]) Implies(other Conjunction[I, A]) bool {
	for _, a := range p.atoms {
		if !other.atoms.Contains(a) {
			return false
		}
	}
	//
	return true
}

func (p Conjunction[I, A]) String(braces bool, mapping func(I) string) string {
	var builder strings.Builder
	//
	braces = braces && len(p.atoms) > 1
	//
	if braces {
		builder.WriteString("(")
	}
	//
	for i, c := range p.atoms {
		if i != 0 {
			builder.WriteString(" ∧ ")
		}
		//
		builder.WriteString(c.String(mapping))
	}
	//
	if braces {
		builder.WriteString(")")
	}
	//
	return builder.String()
}

// Attempt to remove subsumed conditions.  Consider "x≠0 ∧ x=1 ∧ x≠y" for
// example.  In this case, the condition "x≠0" is subsumed by "x=1" and, hence,
// can be removed.  This returns false if proposition is equivalent to logical
// false.
func (p *Conjunction[I, A]) simplify() bool {
	var (
		done    = false
		changed = false
	)
	//
	for !done {
		done = true
		// This is an O(n^2) operation, but we just assume the number of
		// conjunctions (i.e. n) is small.
		for i, ci := range p.atoms {
			for _, cj := range p.atoms {
				cij := ci.CloseOver(cj)
				//
				if cij.Is(false) {
					return false
				} else if ci.Cmp(cij) != 0 {
					p.atoms[i] = cij
					changed = true
					done = false
				}
			}
		}
	}
	//
	if changed {
		// Remove any T values
		p.atoms = array.RemoveMatching(p.atoms, func(a A) bool {
			return a.Is(true)
		})
		// Resort as things may have gotten disturbed
		p.atoms = *set.RawAnySortedSet(p.atoms...)
	}
	//
	return true
}
