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

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
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
func Concretize[F1 Element[F1], F2 Element[F2]](mods []Module[F1]) Schema[F2] {
	var nModules = make([]Module[F2], len(mods))
	//
	for i, m := range mods {
		nModules[i] = concretizeModule[F1, F2](m)
	}
	// compile constant registers.
	InitialiseConstantRegisters(0, nModules)
	//
	return schema.NewUniformSchema(nModules)
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
	r = r.Init(m.Name(), m.AllowPadding(), m.IsPublicOutput(), m.IsPrivateOutput(), m.IsSynthetic(), m.IsNative(),
		m.IsStatic())
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
	// TODO: we actually never concretize assignments any more.
	panic(fmt.Sprintf("unknown assignment: %s\n", reflect.TypeOf(assign).String()))
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
	case LookupConstraint[F1]:
		// NOTE: lookup vectors are made up of registers and, hence, are
		// independent of the underlying field.
		return NewLookupConstraint[F2](c.Handle, c.Targets, c.Sources)
	case RangeConstraint[F1]:
		// NOTE: as for lookups, range constraints are made up of registers and,
		// hence, are independent of the underlying field.
		return NewRangeConstraint[F2](c.Handle, c.Context, c.Sources, c.Bitwidths)
	case VanishingConstraint[F1]:
		term := concretizeLogicalTerm[F1, F2](c.Constraint)
		//
		return NewVanishingConstraint(c.Handle, c.Context, c.Domain, term)
	default:
		panic("unreachable")
	}
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

func concretizeVectorAccess[F1 Element[F1], F2 Element[F2]](expr *VectorAccess[F1]) *VectorAccess[F2] {
	var regs = concretizeRegisterAccesses[F1, F2](expr.Vars)
	return term.RawVectorAccess(regs)
}

func concretizeRegisterAccess[F1 Element[F1], F2 Element[F2]](expr *RegisterAccess[F1]) *RegisterAccess[F2] {
	return term.RawRegisterAccess[F2, Term[F2]](expr.Register(), expr.BitWidth(), expr.RelativeShift())
}

func concretizeRegisterAccesses[F1 Element[F1], F2 Element[F2]](exprs []*RegisterAccess[F1]) []*RegisterAccess[F2] {
	var nterms = make([]*RegisterAccess[F2], len(exprs))
	//
	for i, t := range exprs {
		nterms[i] = term.RawRegisterAccess[F2, Term[F2]](t.Register(), t.BitWidth(), t.RelativeShift())
	}
	//
	return nterms
}
