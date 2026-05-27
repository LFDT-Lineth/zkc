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
	"reflect"

	"github.com/consensys/go-corset/pkg/ir/assignment"
	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/constraint/lookup"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
)

// Element provides a convenient shorthand.
type Element[F any] = field.Element[F]

// Concretize converts an MIR schema for a given field F1 into an MIR schema for
// another field F2.  This is awkward as we have to rebuild the entire
// Intermediate Representation in order to match the type appropriately. In
// doing this, we take some opportunities to simplify, such as removing labelled
// constants (which no longer make sense).  Furthermore, this stage can
// technically fail if the relevant constraints cannot be correctly concretized.
// For example, they contain a constant which does not fit within the field.
func Concretize[F1 Element[F1], F2 Element[F2], E register.ConstMap](mapping module.LimbsMap, externs []E,
	mods []Module[F1]) []Module[F2] {
	var (
		nModules = make([]Module[F2], len(mods))
	)
	//
	for i, m := range Subdivide(mapping, externs, mods) {
		// Concretize subdivided module.
		nModules[i] = concretizeModule[F1, F2](m)
	}
	//
	return nModules
}

func concretizeModule[F1 Element[F1], F2 Element[F2]](m Module[F1]) Module[F2] {
	var (
		r Module[F2]
		// Concreteize Assignments
		assignments = concretizeAssignments[F1, F2](m.RawAssignments())
		// Concreteize Constraints
		constraints = concretizeConstraints[F1, F2](m.RawConstraints())
	)
	// Initialise new module
	r = r.Init(m.Name(), m.AllowPadding(), m.IsPublic(), m.IsSynthetic(), m.IsNative(), m.IsStatic(), m.Keys())
	// Add concretized components
	r.AddRegisters(m.Registers()...)
	r.AddAssignments(assignments...)
	r.AddConstraints(constraints...)
	// Propagate static contents (if any)
	if m.IsStatic() {
		panic("concretizing static modules not supported")
	}
	// Done
	return r
}

// ============================================================================
// Assignments
// ============================================================================

func concretizeAssignments[F1 Element[F1], F2 Element[F2]](assigns []schema.Assignment[F1]) []schema.Assignment[F2] {
	var rs = make([]schema.Assignment[F2], len(assigns))
	//
	for i, a := range assigns {
		rs[i] = concretizeAssignment[F1, F2](a)
	}
	//
	return rs
}

func concretizeAssignment[F1 Element[F1], F2 Element[F2]](assign schema.Assignment[F1]) schema.Assignment[F2] {
	switch a := assign.(type) {
	case *assignment.ComputedRegister[F1]:
		return assignment.NewComputedRegister[F2](a.Expr, a.Direction, a.Module, a.Targets...)
	case *assignment.NativeComputation[F1]:
		return assignment.NewNativeComputation[F2](a.Function, a.Targets, a.Sources)
	case *assignment.SortedPermutation[F1]:
		return assignment.NewSortedPermutation[F2](a.Targets, a.Signs, a.Sources)
	default:
		panic(fmt.Sprintf("unknown assignment: %s\n", reflect.TypeOf(a).String()))
	}
}

// ============================================================================
// Constraints
// ============================================================================

func concretizeConstraints[F1 Element[F1], F2 Element[F2]](constraints []Constraint[F1]) []Constraint[F2] {
	var rs = make([]Constraint[F2], len(constraints))
	//
	for i, c := range constraints {
		rs[i] = concretizeConstraint[F1, F2](c)
	}
	//
	return rs
}

func concretizeConstraint[F1 Element[F1], F2 Element[F2]](constraint Constraint[F1]) Constraint[F2] {
	//
	switch c := constraint.Unwrap().(type) {
	case Assertion[F1]:
		//
		return NewAssertion[F2](c.Handle, c.Context, c.Domain, c.Property)
	case InterleavingConstraint[F1]:
		target := concretizeVectorAccess[F1, F2](c.Target)
		sources := concretizeVectorAccesses[F1, F2](c.Sources)
		//
		return NewInterleavingConstraint(c.Handle, c.TargetContext, c.SourceContext, target, sources)
	case LookupConstraint[F1]:
		targets := concretizeLookupVectors[F1, F2](c.Targets)
		sources := concretizeLookupVectors[F1, F2](c.Sources)
		//
		return NewLookupConstraint(c.Handle, targets, sources)
	case PermutationConstraint[F1]:
		return NewPermutationConstraint[F2](c.Handle, c.Context, c.Targets, c.Sources)
	case RangeConstraint[F1]:
		var terms = concretizeRegisterAccesses[F1, F2](c.Sources)
		//
		return NewRangeConstraint(c.Handle, c.Context, terms, c.Bitwidths)
	case SortedConstraint[F1]:
		var (
			sources  = concretizeRegisterAccesses[F1, F2](c.Sources)
			selector = util.None[*RegisterAccess[F2]]()
		)
		//
		if c.Selector.HasValue() {
			selector = util.Some(concretizeRegisterAccess[F1, F2](c.Selector.Unwrap()))
		}
		//
		return NewSortedConstraint(c.Handle, c.Context, c.BitWidth, selector, sources, c.Signs, c.Strict)
	case VanishingConstraint[F1]:
		term := concretizeLogicalTerm[F1, F2](c.Constraint)
		//
		return NewVanishingConstraint(c.Handle, c.Context, c.Domain, term)
	default:
		panic("unreachable")
	}
}

func concretizeLookupVectors[F1 Element[F1], F2 Element[F2]](vecs []LookupVector[F1]) []LookupVector[F2] {
	var nvecs = make([]LookupVector[F2], len(vecs))
	//
	for i, vec := range vecs {
		nvecs[i] = concretizeLookupVector[F1, F2](vec)
	}
	//
	return nvecs
}

func concretizeLookupVector[F1 Element[F1], F2 Element[F2]](vec LookupVector[F1]) LookupVector[F2] {
	var (
		sources                                   = concretizeRegisterAccesses[F1, F2](vec.Terms)
		selector util.Option[*RegisterAccess[F2]] = util.None[*RegisterAccess[F2]]()
	)
	//
	if vec.Selector.HasValue() {
		selector = util.Some(concretizeRegisterAccess[F1, F2](vec.Selector.Unwrap()))
	}
	//
	return lookup.NewVector(vec.Module, selector, sources...)
}

// ============================================================================
// LogicalTerms
// ============================================================================

func concretizeLogicalTerm[F1 Element[F1], F2 Element[F2]](t LogicalTerm[F1]) LogicalTerm[F2] {
	switch t := t.(type) {
	case *Conjunct[F1]:
		return term.Conjunction(concretizeLogicalTerms[F1, F2](t.Args)...)
	case *Disjunct[F1]:
		return term.Disjunction(concretizeLogicalTerms[F1, F2](t.Args)...)
	case *Equal[F1]:
		lhs := concretizeTerm[F1, F2](t.Lhs)
		rhs := concretizeTerm[F1, F2](t.Rhs)
		//
		return term.Equals[F2, LogicalTerm[F2]](lhs, rhs)
	case *Ite[F1]:
		var tb, fb LogicalTerm[F2]
		//
		cond := concretizeLogicalTerm[F1, F2](t.Condition)
		//
		if t.TrueBranch != nil {
			tb = concretizeLogicalTerm[F1, F2](t.TrueBranch)
		}
		//
		if t.FalseBranch != nil {
			fb = concretizeLogicalTerm[F1, F2](t.FalseBranch)
		}
		//
		return term.IfThenElse(cond, tb, fb)
	case *Negate[F1]:
		return term.Negation(concretizeLogicalTerm[F1, F2](t.Arg))
	case *NotEqual[F1]:
		lhs := concretizeTerm[F1, F2](t.Lhs)
		rhs := concretizeTerm[F1, F2](t.Rhs)
		//
		return term.NotEquals[F2, LogicalTerm[F2]](lhs, rhs)
	default:
		panic("unreachable")
	}
}

func concretizeLogicalTerms[F1 Element[F1], F2 Element[F2]](terms []LogicalTerm[F1]) []LogicalTerm[F2] {
	var nterms = make([]LogicalTerm[F2], len(terms))
	//
	for i, t := range terms {
		nterms[i] = concretizeLogicalTerm[F1, F2](t)
	}
	//
	return nterms
}

// ============================================================================
// Terms
// ============================================================================

func concretizeTerm[F1 Element[F1], F2 Element[F2]](t Term[F1]) Term[F2] {
	var tmp F2
	//
	switch t := t.(type) {
	case *Add[F1]:
		return term.Sum(concretizeTerms[F1, F2](t.Args)...)
	case *Constant[F1]:
		// NOTE: could fail if  F1 value does not fit into F2 value.
		return term.Const[F2, Term[F2]](tmp.SetBytes(t.Value.Bytes()))
	case *RegisterAccess[F1]:
		return concretizeRegisterAccess[F1, F2](t)
	case *Mul[F1]:
		return term.Product(concretizeTerms[F1, F2](t.Args)...)
	case *Sub[F1]:
		return term.Subtract(concretizeTerms[F1, F2](t.Args)...)
	case *VectorAccess[F1]:
		return concretizeVectorAccess[F1, F2](t)
	default:
		panic("unreachable")
	}
}

func concretizeTerms[F1 Element[F1], F2 Element[F2]](terms []Term[F1]) []Term[F2] {
	var nterms = make([]Term[F2], len(terms))
	//
	for i, t := range terms {
		nterms[i] = concretizeTerm[F1, F2](t)
	}
	//
	return nterms
}

func concretizeVectorAccesses[F1 Element[F1], F2 Element[F2]](terms []*VectorAccess[F1]) []*VectorAccess[F2] {
	var nterms = make([]*VectorAccess[F2], len(terms))
	//
	for i, t := range terms {
		nterms[i] = concretizeVectorAccess[F1, F2](t)
	}
	//
	return nterms
}

func concretizeVectorAccess[F1 Element[F1], F2 Element[F2]](expr *VectorAccess[F1]) *VectorAccess[F2] {
	var regs = concretizeRegisterAccesses[F1, F2](expr.Vars)
	return term.RawVectorAccess(regs)
}

func concretizeRegisterAccess[F1 Element[F1], F2 Element[F2]](expr *RegisterAccess[F1]) *RegisterAccess[F2] {
	access := term.RawRegisterAccess[F2, Term[F2]](expr.Register(), expr.BitWidth(), expr.RelativeShift())
	// Apply any mask
	return access.Mask(expr.MaskWidth())
}

func concretizeRegisterAccesses[F1 Element[F1], F2 Element[F2]](exprs []*RegisterAccess[F1]) []*RegisterAccess[F2] {
	var nterms = make([]*RegisterAccess[F2], len(exprs))
	//
	for i, t := range exprs {
		nterms[i] = term.RawRegisterAccess[F2, Term[F2]](t.Register(), t.BitWidth(), t.RelativeShift()).Mask(t.MaskWidth())
	}
	//
	return nterms
}
