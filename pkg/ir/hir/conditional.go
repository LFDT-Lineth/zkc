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
package hir

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// NaryFn describes a function which constructs an MIR term from a given set of zero or more terms.
type NaryFn func([]mirTerm) mirTerm

// BinaryLogicalFn describes a function whichs a logical MIR term from exactly two terms.
type BinaryLogicalFn func(l, r mirTerm) mir.LogicalTerm[word.BigEndian]

// UnconditionalTerm returns a term which has no condition associated with it.
func UnconditionalTerm(value mirTerm) IfTerm {
	var ncase = ifTermCase{term.True[word.BigEndian, mir.LogicalTerm[word.BigEndian]](), value}
	//
	return IfTerm{[]ifTermCase{ncase}}
}

// IfThenElse constructs an IfTerm representing an if-then else expression.
func IfThenElse(cond mir.LogicalTerm[word.BigEndian], tt, ff IfTerm) IfTerm {
	var (
		n       = len(tt.cases)
		m       = len(ff.cases)
		ncases  = make([]ifTermCase, n+m)
		negCond = term.Negation(cond)
	)
	// True branches
	for i, c := range tt.cases {
		condition := term.Conjunction(cond, c.condition).Simplify(false)
		ncases[i] = ifTermCase{condition, c.target}
	}
	// False branches
	for i, c := range ff.cases {
		condition := term.Conjunction(negCond, c.condition).Simplify(false)
		ncases[i+n] = ifTermCase{condition, c.target}
	}
	// Done
	return IfTerm{ncases}
}

// IfEqElse constructs an IfTerm representing an if-eq expression.
func IfEqElse(lhs IfTerm, rhs mirTerm, tt, ff mirTerm) IfTerm {
	var (
		n      = len(lhs.cases)
		ncases = make([]ifTermCase, 2*n)
	)
	// True branches
	for i, c := range lhs.cases {
		condition := term.Conjunction(c.condition,
			term.Equals[word.BigEndian, mir.LogicalTerm[word.BigEndian]](c.target, rhs))
		ncases[i] = ifTermCase{condition, tt}
	}
	// False branches
	for i, c := range lhs.cases {
		condition := term.Conjunction(c.condition,
			term.NotEquals[word.BigEndian, mir.LogicalTerm[word.BigEndian]](c.target, rhs))
		ncases[i+n] = ifTermCase{condition, ff}
	}
	// Done
	return IfTerm{ncases}
}

// MapIfTerms applies a given function to each target of the given argument
// terms, effectively producing their cross product.
func MapIfTerms(fn NaryFn, terms ...IfTerm) IfTerm {
	return IfTerm{
		constructTerms(0, terms, fn, make([]ifTermCase, len(terms))),
	}
}

// DisjunctIfTerms is similar to MapIfTerms but produces the logical disjunction
// of all constructed logical terms.
func DisjunctIfTerms(fn BinaryLogicalFn, lhs, rhs IfTerm) mir.LogicalTerm[word.BigEndian] {
	var terms []mir.LogicalTerm[word.BigEndian]
	//
	for _, lCase := range lhs.cases {
		for _, rCase := range rhs.cases {
			target := fn(lCase.target, rCase.target)
			terms = append(terms, term.Conjunction(lCase.condition, rCase.condition, target))
		}
	}
	//
	return term.Disjunction(terms...).Simplify(false)
}

// IfTerm represents a set of one or more conditional terms.  Observe that the
// conditions are expected to be total.  Hence, if there is only one term, then
// its condition must be true.
type IfTerm struct {
	cases []ifTermCase
}

// Equate returns a logical condition that constraints the target register to
// hold the values represented by this term on each row.
func (p *IfTerm) Equate(target register.Id, bitwidth uint) mir.LogicalTerm[word.BigEndian] {
	var (
		terms     = make([]mir.LogicalTerm[word.BigEndian], len(p.cases))
		targetVar = term.NewRegisterAccess[word.BigEndian, mirTerm](target, bitwidth, 0)
	)
	//
	for i, c := range p.cases {
		ith := term.Equals[word.BigEndian, mir.LogicalTerm[word.BigEndian]](targetVar, c.target)
		terms[i] = term.Conjunction(c.condition, ith)
	}
	//
	return term.Disjunction(terms...).Simplify(false)
}

// Map a given function over the targets of this set of conditional terms.
func (p *IfTerm) Map(fn func(mirTerm) mirTerm) IfTerm {
	var ncases = make([]ifTermCase, len(p.cases))
	//
	for i, c := range p.cases {
		ncases[i] = ifTermCase{
			c.condition,
			fn(c.target),
		}
	}
	//
	return IfTerm{ncases}
}

type ifTermCase struct {
	condition mir.LogicalTerm[word.BigEndian]
	target    mirTerm
}

func constructTerms(i int, terms []IfTerm, fn NaryFn, cases []ifTermCase) []ifTermCase {
	var ncases []ifTermCase
	//
	if i == len(terms) {
		var (
			args       = make([]mirTerm, len(cases))
			conditions = make([]mir.LogicalTerm[word.BigEndian], len(cases))
		)
		//
		for i, c := range cases {
			args[i] = c.target
			conditions[i] = c.condition
		}
		// Apply constructor
		return []ifTermCase{{term.Conjunction(conditions...), fn(args)}}
	}
	//
	for _, c := range terms[i].cases {
		cases[i] = c
		// Recursively construct terms for this position
		ith := constructTerms(i+1, terms, fn, cases)
		// Append them all together
		ncases = append(ncases, ith...)
	}
	//
	return ncases
}
