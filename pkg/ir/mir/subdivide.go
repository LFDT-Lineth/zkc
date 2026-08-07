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

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
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
func Subdivide[F field.Element[F]](mapping module.LimbsMap, mods []Module[F]) []Module[F] {
	//
	var (
		builder = ir.NewSchemaBuilder[F, Constraint[F], Term[F]]()
	)
	// Initialise subdivided modules using register limbs rather than the
	for _, m := range mods {
		// original registers.
		var (
			mid = builder.NewModule(m.Name(), m.AllowPadding(), m.IsPublicOutput(), m.IsPrivateOutput(),
				m.IsSynthetic(), m.IsStatic(), m.IsNative(), 0)
			module   = builder.Module(mid)
			limbsMap = mapping.Module(mid).LimbsMap()
		)
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
		mid := uint(i)
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
		module.AddAssignment(assignment.NewComputedRegister[F](a.Right, mid, a.Left...))
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
	default:
		panic("unreachable")
	}
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideComputedRegister(cr *assignment.ComputedRegister[F]) schema.Assignment[F] {
	var (
		ntargets []register.Id
		modmap   = p.mapping.Module(cr.Module)
		expr     = term.SubdivideExpr[word.BigEndian, term.LogicalComputation[word.BigEndian]](cr.Expr, modmap)
	)
	//
	for _, target := range cr.Targets {
		ntargets = append(ntargets, modmap.LimbIds(target)...)
	}
	//
	return assignment.NewComputedRegister[F](expr, cr.Module, ntargets...)
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
	case LookupConstraint[F]:
		constraint = p.subdivideLookup(c)
	case RangeConstraint[F]:
		constraint = p.subdivideRange(c)
	case VanishingConstraint[F]:
		constraint = p.subdivideVanishing(c)
	default:
		panic("unreachable")
	}
	//
	return Constraint[F]{constraint}
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
