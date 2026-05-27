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

	"github.com/consensys/go-corset/pkg/ir"
	"github.com/consensys/go-corset/pkg/ir/assignment"
	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/agnostic"
	"github.com/consensys/go-corset/pkg/schema/constraint"
	"github.com/consensys/go-corset/pkg/schema/constraint/interleaving"
	"github.com/consensys/go-corset/pkg/schema/constraint/permutation"
	"github.com/consensys/go-corset/pkg/schema/constraint/ranged"
	"github.com/consensys/go-corset/pkg/schema/constraint/sorted"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/word"
)

// Subdivide all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
//
// As part of the subdivision process, constraints may also need to be divided
// when they exceed the maximum permitted bandwidth.  For example, consider a
// simple constraint such as "x = y + 1" using 16bit registers x,y.  Subdividing
// for a bandwidth of 10bits and a maximum register width of 8bits means
// splitting each register into two limbs, and transforming our constraint into:
//
// 256*x'1 + x'0 = 256*y'1 + y'0 + 1
//
// However, as it stands, this constraint exceeds our bandwidth requirement
// since it requires at least 17bits of information to safely evaluate each
// side.  Thus, the constraint itself must be subdivided into two parts:
//
// 256*c + x'0 = y'0 + 1  // lower
//
//	x'1 = y'1 + c  // upper
//
// Here, c is a 1bit register introduced as part of the transformation to act as
// a "carry" between the two constraints.
func Subdivide[F field.Element[F], E register.ConstMap](mapping module.LimbsMap, externs []E,
	mods []Module[F]) []Module[F] {
	//
	var (
		builder = ir.NewSchemaBuilder[F, Constraint[F], Term[F]](externs...)
	)
	// Initialise subdivided modules using register limbs rather than the
	for i, m := range mods {
		// original registers.
		var (
			eid = uint(i + len(externs))
			mid = builder.NewModule(m.Name(), m.AllowPadding(), m.IsPublic(), m.IsSynthetic(),
				m.IsStatic(), m.IsNative(), 0)
			module   = builder.Module(mid)
			limbsMap = mapping.Module(mid).LimbsMap()
		)
		// Sanity check module identifier is consistent
		if mid != eid {
			panic(fmt.Sprintf("inconsistent module identifier (%d vs %d)", mid, eid))
		}
		// Initialise all register limbs.
		module.NewRegisters(limbsMap.Registers()...)
		// Assign static contents (if applicable)
		if m.IsStatic() {
			module.SetStaticContents(m.StaticContents())
		}
	}
	// Construct subdivider
	subdivider := &Subdivider[F]{builder, mapping}
	// Subdivide modules
	for i, m := range mods {
		mid := uint(i + len(externs))
		subdivider.SubdivideModule(mid, m)
	}
	// Done
	return ir.BuildSchema[Module[F]](builder)
}

// Subdivider is responsible for subdividing modules to ensure they fit within a
// given target field configuration (as determined by the mapping).  More
// specificially, any registers used within (and constraints, etc) are
// subdivided as necessary to ensure a maximum bandwidth requirement is met.
// Here, bandwidth refers to the maximum number of data bits which can be stored
// in the underlying field. As a simple example, the prime field F_7 has a
// bandwidth of 2bits.  To target a specific prime field, two parameters are
// used: the maximum bandwidth (as determined by the prime); the maximum
// register width (which should be smaller than the bandwidth).  The maximum
// register width determines the maximum permitted width of any register after
// subdivision.  Since every register value will be stored as a field element,
// it follows that the maximum width cannot be greater than the bandwidth.
// However, in practice, we want it to be marginally less than the bandwidth to
// ensure there is some capacity for calculations involving registers.
type Subdivider[F field.Element[F]] struct {
	// Subdivided (i.e. new) modules
	modules SchemaBuilder[F]
	// Predetermined mapping
	mapping module.LimbsMap
}

// SubdivideModule subdivides all registers, constraints and assignments within
// a given module.
func (p *Subdivider[F]) SubdivideModule(mid module.Id, rawModule Module[F]) {
	var module = p.modules.Module(mid)
	// subdivide assignments
	for _, c := range rawModule.RawAssignments() {
		module.AddAssignment(p.subdivideAssignment(c))
	}
	// subdivide constraints
	for _, c := range rawModule.RawConstraints() {
		module.AddConstraint(p.subdivideConstraint(c))
	}
}

// FreshAllocator creates a fresh allocator for the given module.
func (p *Subdivider[F]) FreshAllocator(mid module.Id) agnostic.RegisterAllocator {
	return register.NewAllocator[agnostic.Computation](p.modules.Module(mid))
}

// FlushAllocator causes the given register allocator to crystalise any
// allocated registers into the corresponding module.
func (p *Subdivider[F]) FlushAllocator(mid module.Id, alloc agnostic.RegisterAllocator) {
	var (
		module = p.modules.Module(mid)
		n      = len(module.Registers())
		regs   = alloc.Registers()
	)
	// Allocate *new* registers into module
	rids := module.NewRegisters(regs[n:]...)
	// include any additional assignments required for carry lines
	for _, a := range alloc.Assignments() {
		module.AddAssignment(assignment.NewComputedRegister[F](a.Right, true, mid, a.Left...))
	}
	// constrain all new registers
	for i, rid := range rids {
		var (
			ith        = regs[n+i]
			terms      = []*RegisterAccess[F]{term.RawRegisterAccess[F, Term[F]](rid, ith.Width(), 0)}
			bitwidths  = []uint{ith.Width()}
			handle     = fmt.Sprintf("%s:u%d", ith.Name(), ith.Width())
			constraint = ranged.NewConstraint[F](handle, mid, terms, bitwidths)
		)
		//
		module.AddConstraint(Constraint[F]{constraint})
	}
}

// ZeroRegister returns a register in the given module whose value is always
// the constant zero.
func (p *Subdivider[F]) ZeroRegister(mid module.Id) register.Id {
	var module = p.modules.Module(mid)
	// Access zero register for given module
	return module.ConstRegister(0)
}

// ============================================================================
// Assignments
// ============================================================================

func (p *Subdivider[F]) subdivideAssignment(a schema.Assignment[F]) schema.Assignment[F] {
	switch a := a.(type) {
	case *assignment.ComputedRegister[F]:
		return p.subdivideComputedRegister(a)
	case *assignment.NativeComputation[F]:
		return p.subdivideNativeComputation(a)
	case *assignment.SortedPermutation[F]:
		return p.subdivideSortedPermutation(a)
	default:
		panic("unreachable")
	}
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideComputedRegister(cr *assignment.ComputedRegister[F]) schema.Assignment[F] {
	var (
		ntargets []register.Id
		modmap   = p.mapping.Module(cr.Module)
		expr     = term.SubdivideExpr[word.BigEndian, constraint.Property](cr.Expr, modmap)
	)
	//
	for _, target := range cr.Targets {
		ntargets = append(ntargets, modmap.LimbIds(target)...)
	}
	//
	return assignment.NewComputedRegister[F](expr, cr.Direction, cr.Module, ntargets...)
}

func (p *Subdivider[F]) subdivideNativeComputation(cr *assignment.NativeComputation[F]) schema.Assignment[F] {
	var (
		targets = SubdivideRegisterRefs[F](p.mapping, cr.Targets...)
		sources = SubdivideRegisterRefs[F](p.mapping, cr.Sources...)
	)
	//
	return assignment.NewNativeComputation[F](cr.Function, targets, sources)
}

func (p *Subdivider[F]) subdivideSortedPermutation(sp *assignment.SortedPermutation[F]) schema.Assignment[F] {
	var (
		sources []register.Ref
		targets []register.Ref
		signs   []bool
	)
	//
	for i := range len(sp.Sources) {
		var (
			source = sp.Sources[i]
			target = sp.Targets[i]
			//
			sourceMapping = p.mapping.Module(source.Module())
			targetMapping = p.mapping.Module(target.Module())
			sourceLimbs   = sourceMapping.LimbIds(source.Register())
			targetLimbs   = targetMapping.LimbIds(target.Register())
		)
		// Sanity check for now
		if len(sourceLimbs) != len(targetLimbs) {
			panic("encountered irregular permutation constraint")
		}
		// Append limbs in reverse order to ensure most significant limb comes first.
		for j := len(sourceLimbs); j > 0; j-- {
			sources = append(sources, register.NewRef(source.Module(), sourceLimbs[j-1]))
			targets = append(targets, register.NewRef(target.Module(), targetLimbs[j-1]))
			//
			if i < len(sp.Signs) {
				signs = append(signs, sp.Signs[i])
			}
		}
	}
	//
	return assignment.NewSortedPermutation[F](targets, signs, sources)
}

// SubdivideRegisterRefs subdivides a set of register references according to a
// given mapping.
func SubdivideRegisterRefs[F field.Element[F]](mapping module.LimbsMap, refs ...register.Refs) []register.Refs {
	var (
		nrefs = make([]register.Refs, len(refs))
	)
	//
	for i, ref := range refs {
		nrefs[i] = ref.Apply(mapping.Module(ref.Module()))
	}
	//
	return nrefs
}

// ============================================================================
// Constraints
// ============================================================================

func (p *Subdivider[F]) subdivideConstraint(c Constraint[F]) Constraint[F] {
	var constraint schema.Constraint[F]
	switch c := c.constraint.(type) {
	case Assertion[F]:
		constraint = p.subdivideAssertion(c)
	case InterleavingConstraint[F]:
		constraint = p.subdivideInterleaving(c)
	case LookupConstraint[F]:
		constraint = p.subdivideLookup(c)
	case PermutationConstraint[F]:
		constraint = p.subdividePermutation(c)
	case RangeConstraint[F]:
		constraint = p.subdivideRange(c)
	case SortedConstraint[F]:
		constraint = p.subdivideSorted(c)
	case VanishingConstraint[F]:
		constraint = p.subdivideVanishing(c)
	default:
		panic("unreachable")
	}
	//
	return Constraint[F]{constraint}
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideAssertion(c Assertion[F]) Assertion[F] {
	var (
		module = p.mapping.Module(c.Context)
		prop   = term.SubdivideLogical[word.BigEndian, constraint.Property, term.Computation[word.BigEndian]](
			c.Property, module)
	)
	// Construct split constraint
	return constraint.NewAssertion[F](c.Handle, c.Context, c.Domain, prop)
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideInterleaving(c InterleavingConstraint[F]) InterleavingConstraint[F] {
	var (
		targetModule = p.mapping.Module(c.TargetContext)
		sourceModule = p.mapping.Module(c.SourceContext)
		target       = subdivideVectorAccess(c.Target, targetModule)
		sources      = subdivideVectorAccesses(c.Sources, sourceModule)
	)
	// Done
	return interleaving.NewConstraint(c.Handle, c.TargetContext, c.SourceContext, target, sources)
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdividePermutation(c PermutationConstraint[F]) PermutationConstraint[F] {
	var (
		module  = p.mapping.Module(c.Context)
		sources []register.Id
		targets []register.Id
	)
	//
	for i := range len(c.Sources) {
		var (
			sourceLimbs = module.LimbIds(c.Sources[i])
			targetLimbs = module.LimbIds(c.Targets[i])
		)
		// Sanity check for now
		if len(sourceLimbs) != len(targetLimbs) {
			panic("encountered irregular permutation constraint")
		}
		//
		sources = append(sources, sourceLimbs...)
		targets = append(targets, targetLimbs...)
	}
	//
	return permutation.NewConstraint[F](c.Handle, c.Context, targets, sources)
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideRange(c RangeConstraint[F]) RangeConstraint[F] {
	var (
		modmap    = p.mapping.Module(c.Context)
		terms     []*RegisterAccess[F]
		bitwidths []uint
	)
	//
	for i, source := range c.Sources {
		var (
			split    = subdivideRawRegisterAccess(source, modmap)
			bitwidth = c.Bitwidths[i]
		)
		// Include all registers
		terms = append(terms, split...)
		// Split bitwidths
		for _, jth := range split {
			var limbWidth = jth.MaskWidth()
			//
			bitwidths = append(bitwidths, min(bitwidth, limbWidth))
			//
			if bitwidth >= limbWidth {
				bitwidth -= limbWidth
			} else {
				bitwidth = 0
			}
		}
	}
	//
	return ranged.NewConstraint(c.Handle, c.Context, terms, bitwidths)
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideSorted(c SortedConstraint[F]) SortedConstraint[F] {
	var (
		modmap   = p.mapping.Module(c.Context)
		signs    []bool
		sources  []*RegisterAccess[F]
		selector = util.None[*RegisterAccess[F]]()
		bitwidth uint
	)
	// Split sources
	for i, source := range c.Sources {
		var split = subdivideRawRegisterAccess(source, modmap)
		// Append in reverse order to ensure most signicant limb comes first.
		for j := len(split); j > 0; j-- {
			var (
				jth       = split[j-1]
				limbWidth = modmap.Limb(jth.Register()).Width()
			)
			//
			sources = append(sources, jth)
			// Update sign (if applicable)
			if i < len(c.Signs) {
				signs = append(signs, c.Signs[i])
			}
			// Update bitwidth
			bitwidth = max(bitwidth, min(limbWidth, jth.MaskWidth()))
		}
	}
	// Split optional selector
	if c.Selector.HasValue() {
		tmp := subdivideRawRegisterAccess(c.Selector.Unwrap(), modmap)
		//
		if len(tmp) != 1 {
			panic(fmt.Sprintf("encountered irregular selectored with %d limbs.", len(tmp)))
		}
		//
		selector = util.Some(tmp[0])
	}
	// Done
	return sorted.NewConstraint(c.Handle, c.Context, bitwidth, selector, sources, signs, c.Strict)
}

// ============================================================================
// Term
// ============================================================================

func subdivideTerm[F field.Element[F]](expr Term[F], mapping register.LimbsMap) Term[F] {
	switch t := expr.(type) {
	case *Add[F]:
		return term.Sum(subdivideTerms(t.Args, mapping)...)
	case *Constant[F]:
		return t
	case *RegisterAccess[F]:
		return subdivideRegisterAccess(t, mapping)
	case *Mul[F]:
		return term.Product(subdivideTerms(t.Args, mapping)...)
	case *Sub[F]:
		return term.Subtract(subdivideTerms(t.Args, mapping)...)
	case *VectorAccess[F]:
		return subdivideVectorAccess(t, mapping)
	default:
		panic("unreachable")
	}
}

func subdivideTerms[F field.Element[F]](terms []Term[F], mapping register.LimbsMap) []Term[F] {
	var nterms []Term[F] = make([]Term[F], len(terms))
	//
	for i := range len(terms) {
		nterms[i] = subdivideTerm(terms[i], mapping)
	}
	//
	return nterms
}

func subdivideRegisterAccess[F field.Element[F]](expr *RegisterAccess[F], mapping register.LimbsMap) Term[F] {
	var (
		// Construct appropriate terms
		terms = subdivideRawRegisterAccess(expr, mapping)
	)
	// Check whether vector required, or not
	if len(terms) == 1 {
		// NOTE: we cannot return the original term directly, as its index may
		// differ under the limb mapping.
		return terms[0]
	}
	//
	return term.NewVectorAccess(terms)
}

func subdivideVectorAccesses[F field.Element[F]](terms []*VectorAccess[F], mapping register.LimbsMap,
) []*VectorAccess[F] {
	//
	var (
		nterms = make([]*VectorAccess[F], len(terms))
	)
	// Split sources
	for i, src := range terms {
		nterms[i] = subdivideVectorAccess(src, mapping)
	}
	//
	return nterms
}

func subdivideVectorAccess[F field.Element[F]](expr *VectorAccess[F], mapping register.LimbsMap) *VectorAccess[F] {
	var terms []*RegisterAccess[F]
	//
	for _, v := range expr.Vars {
		var ith = subdivideRawRegisterAccess(v, mapping)
		//
		terms = append(terms, ith...)
	}
	//
	return term.RawVectorAccess(terms)
}

func subdivideRawRegisterAccesses[F field.Element[F]](terms []*RegisterAccess[F], mapping register.LimbsMap,
) []*VectorAccess[F] {
	//
	var (
		vecs = make([]*VectorAccess[F], len(terms))
	)
	//
	for i, t := range terms {
		ith := subdivideRawRegisterAccess(t, mapping)
		vecs[i] = term.RawVectorAccess(ith)
	}
	//
	return vecs
}

func subdivideRawRegisterAccess[F field.Element[F]](expr *RegisterAccess[F], mapping register.LimbsMap,
) []*RegisterAccess[F] {
	//
	var (
		// Determine limbs for this register
		limbs = mapping.LimbIds(expr.Register())
		// Construct appropriate terms
		terms []*RegisterAccess[F]
		//
		bitwidth = expr.MaskWidth()
	)
	//
	for i, limbId := range limbs {
		var (
			limb      = mapping.Limb(limbId)
			limbWidth = min(bitwidth, limb.Width())
		)
		// NOTE: following ensures at least one limb is always added for any
		// register.  This is necessary to ensure we never completely eliminate
		// a register.  Perhaps surprisingly, it is possible for a register to
		// have a bitwidth of 0.  This happens for "constant registers" (i.e.
		// registers whose value constant).
		if limbWidth > 0 || i == 0 {
			// Construct register access
			ith := term.RawRegisterAccess[F, Term[F]](limbId, limb.Width(), expr.RelativeShift())
			// Mask off any unrequired bits
			terms = append(terms, ith.Mask(limbWidth))
		}
		//
		bitwidth -= limbWidth
	}
	//
	return terms
}
