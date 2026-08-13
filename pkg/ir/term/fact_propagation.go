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
package term

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// absorbFacts performs unit propagation across the (already flattened, per-
// term simplified) arguments of a conjunction.  Any argument which is not
// itself a guarded (no-else) Ite holds unconditionally within this
// conjunction, and is therefore a "fact".  Any Ite-guarded argument whose
// condition matches (or is the negation of) a known fact has its guard
// eliminated: if the fact establishes the condition, the guard is redundant
// and drops out in favour of its body; if the fact contradicts the
// condition, the guard can never fire and the whole term is vacuously true.
//
// This generalises the reasoning dfa.Path.Die/Join performs specifically for
// a fail's own "dead" condition (see pkg/zkc/util/dfa/path.go) to any sibling
// fact/guard pair arising anywhere in an assembled conjunction, rather than
// requiring each producer of such facts to special-case the absorption
// itself.
func absorbFacts[F field.Element[F], T Logical[F, T]](terms []T, casts bool) []T {
	for {
		var (
			facts   = factsOf[F](terms)
			nterms  = make([]T, 0, len(terms))
			changed = false
		)
		//
		if len(facts) == 0 {
			return terms
		}
		//
		for _, term := range terms {
			var (
				l       Logical[F, T] = term
				ite, ok               = l.(*Ite[F, T])
			)
			//
			if !ok || ite.FalseBranch != nil {
				// Only the single-sided (no-else) guard form produced for
				// per-write constraints is handled here.
				nterms = append(nterms, term)
				continue
			}
			//
			switch matchFact(ite.Condition, facts) {
			case factHolds:
				if ite.TrueBranch != nil {
					nterms = append(nterms, ite.TrueBranch.(T))
				} else {
					nterms = append(nterms, True[F, T]())
				}
				//
				changed = true
			case factContradicted:
				nterms = append(nterms, True[F, T]())
				changed = true
			default:
				nterms = append(nterms, term)
			}
		}
		//
		if !changed {
			return terms
		}
		// Re-simplify/flatten/drop now-true terms: absorption may have
		// exposed new facts (enabling further rounds) or new nested
		// conjuncts.
		nterms = simplifyLogicalTerms(nterms, casts)
		nterms = array.Flatten(nterms, flattenConjunct[F, T])
		terms = array.RemoveMatching(nterms, IsTrue[F, T])
	}
}

// factMatch describes the outcome of comparing a guard condition against a
// set of known facts.
type factMatch uint8

const (
	factUnknown factMatch = iota
	factHolds
	factContradicted
)

// factsOf returns every term in the list which is not itself a guarded
// (no-else) Ite — i.e. holds unconditionally within this conjunction.
func factsOf[F field.Element[F], T Logical[F, T]](terms []T) []T {
	var facts []T
	//
	for _, term := range terms {
		var l Logical[F, T] = term
		//
		if _, ok := l.(*Ite[F, T]); !ok {
			facts = append(facts, term)
		}
	}
	//
	return facts
}

// matchFact determines whether a given guard condition is already
// established (factHolds), or ruled out (factContradicted), by one of the
// given known facts, based on a structural comparison of their canonical
// lisp forms.  This is deliberately purely syntactic (no general entailment
// checking): the facts and conditions compared here originate from the same
// translation machinery, so syntactically identical (or negated) forms are
// the common and worthwhile case to catch.
func matchFact[F field.Element[F], T Logical[F, T]](cond T, facts []T) factMatch {
	var (
		condKey = lispKey[F](cond)
		negKey  = lispKey[F](cond.Negate())
	)
	//
	for _, fact := range facts {
		switch lispKey[F](fact) {
		case condKey:
			return factHolds
		case negKey:
			return factContradicted
		}
	}
	//
	return factUnknown
}

// lispKey produces a canonical string form of a logical term, suitable for
// structural (syntactic) equality comparison.
func lispKey[F field.Element[F], T Logical[F, T]](term T) string {
	return term.Lisp(false, nil).String(false)
}
