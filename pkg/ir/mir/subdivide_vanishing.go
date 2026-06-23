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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	log "github.com/sirupsen/logrus"
)

// EXPLODING_MULTIPLIER determines the multipler to use for logging "exploding"
// constraints.  This is essentially an aid to debugging.
var EXPLODING_MULTIPLIER = uint(10)

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideVanishing(vc VanishingConstraint[F]) VanishingConstraint[F] {
	//
	var (
		modmap = p.mapping.Module(vc.Context)
		// Extract allocator
		alloc = p.FreshAllocator(vc.Context)
		// Split all registers occurring in the logical term.
		targets, context = splitLogicalTerm(vc.Constraint, term.True[F, LogicalTerm[F]](), modmap, alloc)
		// Determine size of original tree
		n = sizeOfTree(vc.Constraint, modmap)
		// Determine size of split tree
		m = sizeOfTree(targets, alloc) + sizeOfTree(context, alloc)
		//
		multiplier = float64(m) / float64(n)
	)
	// Check for any exploding constraints
	if multiplier > float64(EXPLODING_MULTIPLIER) {
		multiplier := fmt.Sprintf("%.2f", multiplier)
		log.Debug("exploding (x", multiplier, ") constraint \"", vc.Handle, "\" in module \"", modmap.Name(), "\" detected.")
	}
	// Flush allocator
	p.FlushAllocator(vc.Context, alloc)
	//
	return vanishing.NewConstraint(vc.Handle, vc.Context, vc.Domain, term.Conjunction(context, targets).Simplify(false))
}

func splitLogicalTerm[F field.Element[F]](expr LogicalTerm[F], path LogicalTerm[F], mapping register.LimbsMap,
	env agnostic.RegisterAllocator) (target LogicalTerm[F], context LogicalTerm[F]) {
	//
	switch t := expr.(type) {
	case *Conjunct[F]:
		targets, context := splitLogicalTerms(t.Args, path, mapping, env)
		return term.Conjunction(targets...), context
	case *Disjunct[F]:
		targets, context := splitLogicalTerms(t.Args, path, mapping, env)
		return term.Disjunction(targets...), context
	case *Equal[F]:
		return splitEquality(true, t.Lhs, t.Rhs, path, mapping, env)
	case *Ite[F]:
		condition, ctx1 := splitLogicalTerm(t.Condition, path, mapping, env)
		truePath := term.Conjunction(condition, path)
		falsePath := term.Conjunction(condition.Negate(), path)
		trueBranch, ctx2 := splitOptionalLogicalTerm(t.TrueBranch, truePath, mapping, env)
		falseBranch, ctx3 := splitOptionalLogicalTerm(t.FalseBranch, falsePath, mapping, env)
		//
		return term.IfThenElse(condition, trueBranch, falseBranch), term.Conjunction(ctx1, ctx2, ctx3)
	case *Negate[F]:
		target, context = splitLogicalTerm(t.Arg, path, mapping, env)
		return term.Negation(target), context
	case *NotEqual[F]:
		return splitEquality(false, t.Lhs, t.Rhs, path, mapping, env)
	default:
		panic("unreachable")
	}
}

func splitOptionalLogicalTerm[F field.Element[F]](expr LogicalTerm[F], path LogicalTerm[F],
	mapping register.LimbsMap, env agnostic.RegisterAllocator) (target LogicalTerm[F], context LogicalTerm[F]) {
	//
	if expr == nil {
		return nil, term.True[F, LogicalTerm[F]]()
	}
	//
	return splitLogicalTerm(expr, path, mapping, env)
}

func splitLogicalTerms[F field.Element[F]](terms []LogicalTerm[F], path LogicalTerm[F],
	mapping register.LimbsMap, env agnostic.RegisterAllocator) (targets []LogicalTerm[F], context LogicalTerm[F]) {
	//
	var (
		nterms = make([]LogicalTerm[F], len(terms))
		nctx   = make([]LogicalTerm[F], len(terms))
	)
	//
	for i := range len(terms) {
		nterms[i], nctx[i] = splitLogicalTerm(terms[i], path, mapping, env)
	}
	//
	return nterms, term.Conjunction(nctx...)
}

func splitEquality[F field.Element[F]](sign bool, lhs, rhs Term[F], path LogicalTerm[F], mapping register.LimbsMap,
	alloc agnostic.RegisterAllocator) (target LogicalTerm[F], context LogicalTerm[F]) {
	//
	var (
		lhsTerm = subdivideTerm(lhs, mapping)
		rhsTerm = subdivideTerm(rhs, mapping)
		// Split terms accordingl to mapping, and translate into polynomials
		lhsPoly = termToPolynomial(lhsTerm, mapping.LimbsMap())
		rhsPoly = termToPolynomial(rhsTerm, mapping.LimbsMap())
		//
		// Construct equality for spltting
		equation = agnostic.NewEquation(lhsPoly, rhsPoly)
		// Split the equation
		tgtEqns, ctxEqns = equation.Split(mapping.Field(), alloc)
		// Prepare resulting conjunct / disjunct
		tgtTerms = make([]LogicalTerm[F], len(tgtEqns))
		ctxTerms = make([]LogicalTerm[F], len(ctxEqns))
	)
	// Check whether any splitting actually occurred.  If not, then keep the
	// original form to protect against expansion impacting performance.
	if len(tgtEqns) == 1 && len(ctxEqns) == 0 {
		if sign {
			return term.Equals[F, LogicalTerm[F]](lhsTerm, rhsTerm), term.True[F, LogicalTerm[F]]()
		}
		//
		return term.NotEquals[F, LogicalTerm[F]](lhsTerm, rhsTerm), term.True[F, LogicalTerm[F]]()
	}
	// Splitting actually occurred, hence translate target equations and
	// context.
	for i, eq := range tgtEqns {
		// reconstruct original term
		l := polynomialToTerm[F](eq.LeftHandSide)
		r := polynomialToTerm[F](eq.RightHandSide)
		//
		if sign {
			tgtTerms[i] = term.Equals[F, LogicalTerm[F]](l, r)
		} else {
			tgtTerms[i] = term.NotEquals[F, LogicalTerm[F]](l, r)
		}
	}
	// Translate contextual equations
	for i, eq := range ctxEqns {
		// reconstruct original term
		l := polynomialToTerm[F](eq.LeftHandSide)
		r := polynomialToTerm[F](eq.RightHandSide)
		//
		ctxTerms[i] = term.Equals[F, LogicalTerm[F]](l, r)
	}
	// construct contextual constraints
	context = term.IfThenElse(path, term.Conjunction(ctxTerms...), nil)
	// Done (for now)
	if sign {
		return term.Conjunction(tgtTerms...), context
	}
	//
	return term.Disjunction(tgtTerms...), context
}

func sizeOfTree[F field.Element[F]](term LogicalTerm[F], mapping register.Map) uint {
	switch t := term.(type) {
	case *Conjunct[F]:
		return sizeOfTrees(t.Args, mapping)
	case *Disjunct[F]:
		return sizeOfTrees(t.Args, mapping)
	case *Equal[F]:
		return 1
	case *Ite[F]:
		size := sizeOfTree(t.Condition, mapping)
		//
		if t.TrueBranch != nil {
			size += sizeOfTree(t.TrueBranch, mapping)
		}
		//
		if t.FalseBranch != nil {
			size += sizeOfTree(t.FalseBranch, mapping)
		}
		//
		return size
	case *Negate[F]:
		return sizeOfTree(t.Arg, mapping)
	case *NotEqual[F]:
		return 1
	default:
		panic("unknown logical term encountered")
	}
}

func sizeOfTrees[F field.Element[F]](terms []LogicalTerm[F], mapping register.Map) uint {
	var size uint
	//
	for _, term := range terms {
		size += sizeOfTree(term, mapping)
	}
	//
	return size
}
