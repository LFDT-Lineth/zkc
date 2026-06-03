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
	"math"
	"math/big"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	air_gadgets "github.com/LFDT-Lineth/zkc/pkg/ir/air/gadgets"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
)

// LowerToAir lowers (or refines) an MIR schema into an AIR schema.  That means
// lowering all the columns and constraints, whilst adding additional columns /
// constraints as necessary to preserve the original semantics.
func LowerToAir[F field.Element[F]](schema Schema[F], fieldBandwith uint, config OptimisationConfig) air.Schema[F] {
	lowering := NewAirLowering(fieldBandwith, schema)
	// Configure optimisations
	lowering.ConfigureOptimisation(config)
	//
	return lowering.Lower()
}

// AirLowering captures all auxiliary state required in the process of lowering
// modules from MIR to AIR.  This state is because, as part of the lowering
// process, we may introduce some number of additional modules (e.g. for
// managing type proofs).
type AirLowering[F field.Element[F]] struct {
	config OptimisationConfig
	// Maximum field bandwidth
	fieldBandwidth uint
	// Modules we are lowering from
	mirSchema Schema[F]
	// Modules we are lowering to
	airSchema air.SchemaBuilder[F]
}

// NewAirLowering constructs an initial state for lowering a given MIR schema.
func NewAirLowering[F field.Element[F]](fieldBandwidth uint, mirSchema Schema[F]) AirLowering[F] {
	var (
		airSchema = ir.NewSchemaBuilder[F, air.Constraint[F], air.Term[F], air.Module[F]]()
	)
	// Initialise AIR modules
	for _, m := range mirSchema.RawModules() {
		airSchema.NewModule(m.Name(), m.AllowPadding(), m.IsPublic(), m.IsSynthetic(), m.IsStatic(), m.IsNative(), m.Keys())
	}
	//
	return AirLowering[F]{
		DEFAULT_OPTIMISATION_LEVEL,
		fieldBandwidth,
		mirSchema,
		airSchema,
	}
}

// ConfigureOptimisation configures the amount of optimisation to apply during
// the lowering process.
func (p *AirLowering[F]) ConfigureOptimisation(config OptimisationConfig) {
	p.config = config
}

// Lower the MIR schema provide when this lowering instance was created into an
// equivalent AIR schema.
func (p *AirLowering[F]) Lower() air.Schema[F] {
	// Initialise modules
	for i := 0; i < int(p.mirSchema.Width()); i++ {
		p.InitialiseModule(uint(i))
	}
	// Lower modules
	for i := 0; i < int(p.mirSchema.Width()); i++ {
		p.LowerModule(uint(i))
	}
	// Build concrete modules from schema
	modules := ir.BuildSchema[air.Module[F]](p.airSchema)
	// Done
	return schema.NewUniformSchema(modules)
}

// InitialiseModule simply initialises all registers within the module, but does
// not lower any constraint or assignments.
func (p *AirLowering[F]) InitialiseModule(index uint) {
	var (
		mirModule = p.mirSchema.Module(index)
		airModule = p.airSchema.Module(index)
	)
	// Initialise registers in AIR module
	airModule.NewRegisters(mirModule.Registers()...)
	// Initialise static contents (if applicable)
	if mirModule.IsStatic() {
		airModule.SetStaticContents(mirModule.StaticContents())
	}
}

// LowerModule lowers the given MIR module into the corresponding AIR module.
// This includes all constraints and assignments.
func (p *AirLowering[F]) LowerModule(index uint) {
	var (
		mirModule = p.mirSchema.Module(index)
		airModule = p.airSchema.Module(index)
		// record of true bitwidths
		bitwidths = determineTrueBitwidths(mirModule)
	)
	// Add assignments.  At this time, there is nothing to do in terms of
	// lowering.  Observe that this must be done *before* lowering assignments
	// to ensure a correct ordering.  For example, if a constraint refers to one
	// of these assigned columns and generates a corresponding computed column
	// (e.g. for the inverse).
	for iter := mirModule.Assignments(); iter.HasNext(); {
		airModule.AddAssignment(iter.Next())
	}
	// Lower constraints
	for iter := mirModule.Constraints(); iter.HasNext(); {
		// Following should always hold
		constraint := iter.Next().(Constraint[F])
		//
		p.lowerConstraintToAir(constraint, airModule, bitwidths)
	}
}

// Lower a constraint to the AIR level.
func (p *AirLowering[F]) lowerConstraintToAir(c Constraint[F], airModule air.ModuleBuilder[F], bitwidths []uint) {
	// Check what kind of constraint we have
	switch v := c.constraint.(type) {
	case Assertion[F]:
		p.lowerAssertionToAir(v, airModule)
	case InterleavingConstraint[F]:
		p.lowerInterleavingConstraintToAir(v, airModule)
	case LookupConstraint[F]:
		p.lowerLookupConstraintToAir(v, airModule)
	case PermutationConstraint[F]:
		p.lowerPermutationConstraintToAir(v, airModule)
	case RangeConstraint[F]:
		p.lowerRangeConstraintToAir(v, airModule)
	case SortedConstraint[F]:
		p.lowerSortedConstraintToAir(v, airModule)
	case VanishingConstraint[F]:
		p.lowerVanishingConstraintToAir(v, airModule, bitwidths)
	default:
		// Should be unreachable as no other constraint types can be added to a
		// schema.
		panic("unreachable")
	}
}

// Lowering an assertion is straightforward since its not a true constraint.
func (p *AirLowering[F]) lowerAssertionToAir(v Assertion[F], airModule air.ModuleBuilder[F]) {
	airModule.AddConstraint(air.NewAssertion[F](v.Handle, v.Context, v.Domain, v.Property))
}

// Lower a vanishing constraint to the AIR level.  This is relatively
// straightforward and simply relies on lowering the expression being
// constrained.  This may result in the generation of computed columns, e.g. to
// hold inverses, etc.
func (p *AirLowering[F]) lowerVanishingConstraintToAir(v VanishingConstraint[F], airModule air.ModuleBuilder[F],
	bitwidths []uint) {
	//
	var (
		terms = p.lowerAndSimplifyLogicalTo(v.Constraint, airModule, bitwidths)
	)
	//
	for i, air_expr := range terms {
		// Construct suitable handle to distinguish this case
		handle := fmt.Sprintf("%s#%d", v.Handle, i)
		// Add constraint
		airModule.AddConstraint(
			air.NewVanishingConstraint(handle, v.Context, v.Domain, air_expr))
	}
}

// Lower a permutation constraint to the AIR level.  This is trivial because
// permutation constraints do not currently support complex forms.
func (p *AirLowering[F]) lowerPermutationConstraintToAir(v PermutationConstraint[F], airModule air.ModuleBuilder[F]) {
	airModule.AddConstraint(
		air.NewPermutationConstraint[F](v.Handle, v.Context, v.Targets, v.Sources),
	)
}

// Lower a range constraint to the AIR level.  The challenge here is that a
// range constraint at the AIR level cannot use arbitrary expressions; rather it
// can only constrain columns directly.  Therefore, whenever a general
// expression is encountered, we must generate a computed column to hold the
// value of that expression, along with appropriate constraints to enforce the
// expected value.
func (p *AirLowering[F]) lowerRangeConstraintToAir(v RangeConstraint[F], airModule air.ModuleBuilder[F]) {
	// Extract target expression
	for i, e := range v.Sources {
		// Apply bitwidth gadget
		ref := register.NewRef(airModule.Id(), e.Register())
		// Construct gadget
		gadget := air_gadgets.NewBitwidthGadget(&p.airSchema).
			WithMaxRangeConstraint(p.config.MaxRangeConstraint)
		//
		gadget.Constrain(ref, v.Bitwidths[i])
	}
}

// Lower an interleaving constraint to the AIR level.  The challenge here is
// that interleaving constraints at the AIR level cannot use arbitrary
// expressions; rather, they can only access columns directly.  Therefore,
// whenever a general expression is encountered, we must generate a computed
// column to hold the value of that expression, along with appropriate
// constraints to enforce the expected value.
func (p *AirLowering[F]) lowerInterleavingConstraintToAir(c InterleavingConstraint[F],
	airModule air.ModuleBuilder[F]) {
	var (
		n = len(c.Target.Vars)
	)
	//
	for i := range n {
		var (
			sources = make([]*air.ColumnAccess[F], len(c.Sources))
			ith     = c.Target.Vars[i]
		)
		// Lower sources
		for j, src := range c.Sources {
			var (
				jth      = src.Vars[i]
				jth_term = term.RawRegisterAccess[F, air.Term[F]](jth.Register(), jth.BitWidth(), jth.RelativeShift())
			)
			// Apply any mask
			sources[j] = jth_term.Mask(jth.MaskWidth())
		}
		// Lower target
		var (
			ith_term = term.RawRegisterAccess[F, air.Term[F]](ith.Register(), ith.BitWidth(), ith.RelativeShift())
			// Apply any mask
			target = ith_term.Mask(ith.MaskWidth())
		)
		// Add constraint
		airModule.AddConstraint(
			air.NewInterleavingConstraint(c.Handle, c.TargetContext, c.SourceContext, *target, sources))
	}
}

// Lower a lookup constraint to the AIR level.  The challenge here is that
// lookup constraints at the AIR level cannot use arbitrary expressions; rather,
// they can only access columns directly.  Therefore, whenever a general
// expression is encountered, we must generate a computed column to hold the
// value of that expression, along with appropriate constraints to enforce the
// expected value.
func (p *AirLowering[F]) lowerLookupConstraintToAir(c LookupConstraint[F], airModule air.ModuleBuilder[F]) {
	var (
		sources = make([]lookup.Vector[F, *air.ColumnAccess[F]], len(c.Sources))
		targets = make([]lookup.Vector[F, *air.ColumnAccess[F]], len(c.Targets))
	)
	// Lower sources
	for i, ith := range c.Sources {
		sources[i] = p.expandLookupVectorToAir(ith)
	}
	// Lower targets
	for i, ith := range c.Targets {
		targets[i] = p.expandLookupVectorToAir(ith)
	}
	// Add constraint
	airModule.AddConstraint(air.NewLookupConstraint(c.Handle, targets, sources))
}

func (p *AirLowering[F]) expandLookupVectorToAir(vector LookupVector[F],
) lookup.Vector[F, *air.ColumnAccess[F]] {
	var (
		terms    = p.lowerRegisterAccesses(vector.Terms...)
		selector util.Option[*air.ColumnAccess[F]]
	)
	//
	if vector.HasSelector() {
		sel := p.lowerRegisterAccesses(vector.Selector.Unwrap())[0]
		selector = util.Some(sel)
	}
	//
	return lookup.NewVector(vector.Module, selector, terms...)
}

// Lower a sorted constraint to the AIR level.  The challenge here is that there
// is not concept of sorting constraints at the AIR level.  Instead, we have to
// generate the necessary machinery to enforce the sorting constraint.
func (p *AirLowering[F]) lowerSortedConstraintToAir(c SortedConstraint[F], airModule air.ModuleBuilder[F]) {
	var (
		sources = make([]register.Id, len(c.Sources))
	)
	//
	for i, source := range c.Sources {
		var ith_width = source.MaskWidth()
		// Sanity check
		if i < len(c.Signs) && ith_width > c.BitWidth {
			msg := fmt.Sprintf("incompatible bitwidths (%d vs %d)", ith_width, c.BitWidth)
			panic(msg)
		}
		//
		sources[i] = source.Register()
	}
	// finally add the sorting constraint
	gadget := air_gadgets.NewLexicographicSortingGadget[F](c.Handle, sources, c.BitWidth).
		WithSigns(c.Signs...).
		WithStrictness(c.Strict).
		WithMaxRangeConstraint(p.config.MaxRangeConstraint)
	// Add (optional) selector
	if c.Selector.HasValue() {
		selector := p.lowerTermTo(c.Selector.Unwrap(), airModule)
		gadget.WithSelector(selector)
	}
	// Done
	gadget.Apply(airModule.Id(), &p.airSchema)
}

func (p *AirLowering[F]) lowerRegisterAccesses(terms ...*RegisterAccess[F]) []*air.ColumnAccess[F] {
	var nterms = make([]*air.ColumnAccess[F], len(terms))
	//
	for i, ith := range terms {
		ith_term := term.RawRegisterAccess[F, air.Term[F]](ith.Register(), ith.BitWidth(), ith.RelativeShift())
		// Apply any mask
		nterms[i] = ith_term.Mask(ith.MaskWidth())
	}
	//
	return nterms
}

func (p *AirLowering[F]) lowerAndSimplifyLogicalTo(term LogicalTerm[F],
	airModule air.ModuleBuilder[F], bitwidths []uint) []air.Term[F] {
	// Expand term to remove all syntactic sugage
	term = p.expandLogical(true, term)
	// Apply all reasonable simplifications
	term = term.Simplify(false)
	// Lower properly
	return simplify(p.lowerLogical(term, airModule, bitwidths))
}

func (p *AirLowering[F]) expandLogical(sign bool, e LogicalTerm[F]) LogicalTerm[F] {
	//
	switch e := e.(type) {
	case *Conjunct[F]:
		return p.expandConjunction(sign, e)
	case *Disjunct[F]:
		return p.expandDisjunction(sign, e)
	case *Equal[F]:
		return p.expandEquality(sign, e.Lhs, e.Rhs)
	case *Ite[F]:
		return p.expandIteTo(sign, e)
	case *Negate[F]:
		return p.expandLogical(!sign, e.Arg)
	case *NotEqual[F]:
		return p.expandEquality(!sign, e.Lhs, e.Rhs)
	default:
		name := reflect.TypeOf(e).Name()
		panic(fmt.Sprintf("unknown MIR expression \"%s\"", name))
	}
}

func (p *AirLowering[F]) expandLogicals(sign bool, terms ...LogicalTerm[F]) []LogicalTerm[F] {
	//
	nexprs := make([]LogicalTerm[F], len(terms))

	for i := range len(terms) {
		nexprs[i] = p.expandLogical(sign, terms[i])
	}

	return nexprs
}

func (p *AirLowering[F]) expandConjunction(sign bool, e *Conjunct[F]) LogicalTerm[F] {
	var terms = p.expandLogicals(sign, e.Args...)
	//
	if sign {
		return term.Conjunction(terms...)
	}
	//
	return term.Disjunction(terms...)
}

func (p *AirLowering[F]) expandDisjunction(sign bool, e *Disjunct[F]) LogicalTerm[F] {
	var terms = p.expandLogicals(sign, e.Args...)
	//
	if sign {
		//
		return term.Disjunction(terms...)
	}
	//
	return term.Conjunction(terms...)
}

func (p *AirLowering[F]) expandEquality(sign bool, left Term[F], right Term[F]) LogicalTerm[F] {
	if sign {
		return term.Equals[F, LogicalTerm[F]](left, right)
	}
	//
	return term.NotEquals[F, LogicalTerm[F]](left, right)
}

func (p *AirLowering[F]) expandIteTo(sign bool, e *Ite[F]) LogicalTerm[F] {
	if sign {
		return p.expandPositiveIteTo(e)
	}
	//
	return p.expandNegativeIteTo(e)
}

func (p *AirLowering[F]) expandPositiveIteTo(e *Ite[F]) LogicalTerm[F] {
	var (
		terms []LogicalTerm[F]
	)
	// Handle true branch (if applicable)
	if e.TrueBranch != nil {
		falseCondition := p.expandLogical(false, e.Condition)
		trueBranch := p.expandLogical(true, e.TrueBranch)
		terms = append(terms, term.Disjunction(falseCondition, trueBranch))
	}
	// Handle false branch (if applicable)
	if e.FalseBranch != nil {
		trueCondition := p.expandLogical(true, e.Condition)
		falseBranch := p.expandLogical(true, e.FalseBranch)
		terms = append(terms, term.Disjunction(trueCondition, falseBranch))
	}
	//
	return term.Conjunction(terms...)
}

// !ITE(A,B,C) => !((!A||B) && (A||C))
//
//	=> !(!A||B) || !(A||C)
//	=> (A&&!B) || (!A&&!C)
func (p *AirLowering[F]) expandNegativeIteTo(e *Ite[F]) LogicalTerm[F] {
	var (
		terms []LogicalTerm[F]
	)
	// Handle true branch (if applicable)
	if e.TrueBranch != nil {
		trueCondition := p.expandLogical(true, e.Condition)
		notTrueBranch := p.expandLogical(false, e.TrueBranch)
		terms = append(terms, term.Conjunction(trueCondition, notTrueBranch))
	}
	// Handle false branch (if applicable)
	if e.FalseBranch != nil {
		falseCondition := p.expandLogical(false, e.Condition)
		notFalseBranch := p.expandLogical(false, e.FalseBranch)
		terms = append(terms, term.Conjunction(falseCondition, notFalseBranch))
	}
	//
	return term.Disjunction(terms...)
}

func (p *AirLowering[F]) lowerLogical(e LogicalTerm[F], airMod air.ModuleBuilder[F], bitwidths []uint) []air.Term[F] {
	//
	switch e := e.(type) {
	case *Conjunct[F]:
		return p.lowerConjunct(e, airMod, bitwidths)
	case *Disjunct[F]:
		return p.lowerDisjunct(e, airMod, bitwidths)
	case *Equal[F]:
		return p.lowerEqualityTo(e, airMod, bitwidths)
	case *NotEqual[F]:
		return p.lowerNonEqualityTo(e, airMod, bitwidths)
	default:
		name := reflect.TypeOf(e).Name()
		panic(fmt.Sprintf("unknown MIR expression \"%s\"", name))
	}
}

func (p *AirLowering[F]) lowerLogicals(terms []LogicalTerm[F], airMod air.ModuleBuilder[F], bitwidths []uint,
) [][]air.Term[F] {
	nexprs := make([][]air.Term[F], len(terms))

	for i := range len(terms) {
		nexprs[i] = p.lowerLogical(terms[i], airMod, bitwidths)
	}

	return nexprs
}

func (p *AirLowering[F]) lowerConjunct(e *Conjunct[F], airMod air.ModuleBuilder[F], bitwidths []uint) []air.Term[F] {
	var (
		worklist []air.Term[F]
		sums     []air.Term[F]
	)
	// flattern conjuncts
	for _, ts := range p.lowerLogicals(e.Args, airMod, bitwidths) {
		worklist = array.AppendAll(worklist, ts...)
	}
	//
	for len(worklist) > 0 {
		// determine length of next conjunct
		n := p.nextSumConjunct(bitwidths, worklist)
		// construct next sum
		sums = append(sums, term.Sum(worklist[:n]...))
		// Remove n terms from worklist
		worklist = worklist[n:]
	}
	//
	return sums
}

func (p *AirLowering[F]) lowerDisjunct(e *Disjunct[F], airMod air.ModuleBuilder[F], bitwidths []uint) []air.Term[F] {
	var (
		zero     = term.Const64[F, Term[F]](0)
		nterms   []LogicalTerm[F]
		worklist []Term[F]
	)
	// Split out suitable non-zero checks
	for _, t := range e.Args {
		if ra := isNonZeroCheck(t); ra != nil {
			worklist = append(worklist, ra)
		} else {
			nterms = append(nterms, t)
		}
	}
	// Combine non-zero checks together
	for len(worklist) > 0 {
		// determine length of next packet
		n := p.nextNonZeroCheck(worklist, bitwidths)
		// construct next non-zero check
		check := term.NotEquals[F, LogicalTerm[F]](term.Sum(worklist[:n]...), zero)
		// append check
		nterms = append(nterms, check)
		// Remove n terms from worklist
		worklist = worklist[n:]
	}
	// Continue as before
	return disjunction(p.lowerLogicals(nterms, airMod, bitwidths)...)
}

func (p *AirLowering[F]) lowerEqualityTo(e *Equal[F], airModule air.ModuleBuilder[F], bitwidths []uint,
) []air.Term[F] {
	//
	var (
		lhs air.Term[F] = p.lowerTermTo(e.Lhs, airModule)
		rhs air.Term[F] = p.lowerTermTo(e.Rhs, airModule)
	)
	//
	return []air.Term[F]{term.Subtract(lhs, rhs)}
}

func (p *AirLowering[F]) lowerNonEqualityTo(e *NotEqual[F], airModule air.ModuleBuilder[F], bitwidths []uint,
) []air.Term[F] {
	// //
	var (
		lhs air.Term[F] = p.lowerTermTo(e.Lhs, airModule)
		rhs air.Term[F] = p.lowerTermTo(e.Rhs, airModule)
		eq              = term.Subtract(lhs, rhs)
	)
	//
	one := term.Const64[F, air.Term[F]](1)
	// construct norm(eq)
	norm_eq := p.normalise(eq, airModule)
	// construct 1 - norm(eq)
	return []air.Term[F]{term.Subtract(one, norm_eq)}
}

// Inner form is used for recursive calls and does not repeat the constant
// propagation phase.
func (p *AirLowering[F]) lowerTermTo(e Term[F], airModule air.ModuleBuilder[F]) air.Term[F] {
	//
	switch e := e.(type) {
	case *Add[F]:
		args := p.lowerTerms(e.Args, airModule)
		return term.Sum(args...)
	case *Constant[F]:
		return term.Const[F, air.Term[F]](e.Value)
	case *RegisterAccess[F]:
		return term.RawRegisterAccess[F, air.Term[F]](e.Register(), e.BitWidth(), e.RelativeShift()).Mask(e.MaskWidth())
	case *Mul[F]:
		args := p.lowerTerms(e.Args, airModule)
		return term.Product(args...)
	case *Sub[F]:
		args := p.lowerTerms(e.Args, airModule)
		return term.Subtract(args...)
	case *VectorAccess[F]:
		return p.lowerVectorAccess(e, airModule)
	default:
		name := reflect.TypeOf(e).Name()
		panic(fmt.Sprintf("unknown MIR expression \"%s\"", name))
	}
}

// Lower a set of zero or more MIR expressions.
func (p *AirLowering[F]) lowerTerms(exprs []Term[F], airModule air.ModuleBuilder[F]) []air.Term[F] {
	nexprs := make([]air.Term[F], len(exprs))

	for i := range len(exprs) {
		nexprs[i] = p.lowerTermTo(exprs[i], airModule)
	}

	return nexprs
}

func (p *AirLowering[F]) lowerVectorAccess(e *VectorAccess[F], airModule air.ModuleBuilder[F]) air.Term[F] {
	var (
		terms []air.Term[F] = make([]air.Term[F], len(e.Vars))
		shift               = uint(0)
	)
	//
	for i, v := range e.Vars {
		var (
			limb  = airModule.Register(v.Register())
			width = v.MaskWidth()
			ith   *air.ColumnAccess[F]
		)
		// Ensure limbwidth normalised
		ith = term.RawRegisterAccess[F, air.Term[F]](v.Register(), limb.Width(), v.RelativeShift()).Mask(width)
		// Apply shift
		terms[i] = term.Product(shiftTerm(ith, shift))
		//
		shift = shift + width
	}
	//
	return term.Sum(terms...)
}

func shiftTerm[F field.Element[F]](expr air.Term[F], width uint) air.Term[F] {
	if width == 0 {
		return expr
	}
	// Compute 2^width
	n := field.TwoPowN[F](width)
	//
	return term.Product(term.Const[F, air.Term[F]](n), expr)
}

func (p *AirLowering[F]) normalise(arg air.Term[F], airModule air.ModuleBuilder[F]) air.Term[F] {
	bounds := arg.ValueRange()
	// Check whether normalisation actually required.  For example, if the
	// argument is just a binary column then a normalisation is not actually
	// required.
	if p.config.InverseEliminiationLevel > 0 && bounds.Within(util_math.NewInterval64(0, 1)) {
		// arg ∈ {0,1} ==> normalised already :)
		return arg
	} else if p.config.InverseEliminiationLevel > 0 && bounds.Within(util_math.NewInterval64(-1, 1)) {
		// arg ∈ {-1,0,1} ==> (arg*arg) ∈ {0,1}
		return term.Product(arg, arg)
	}
	// Determine appropriate shift
	shift := 0
	// Apply shift normalisation (if enabled)
	if p.config.ShiftNormalisation {
		// Determine shift ranges
		min, max := arg.ShiftRange()
		// determine shift amount
		if max < 0 {
			shift = max
		} else if min > 0 {
			shift = min
		}
	}
	// Construct an expression representing the normalised value of e.  That is,
	// an expression which is 0 when e is 0, and 1 when e is non-zero.
	arg = arg.ApplyShift(-shift).Simplify(false)
	norm := air_gadgets.Normalise(arg, airModule)
	//
	return norm.ApplyShift(shift)
}

// Simplify a bunch of logical terms
func simplify[F field.Element[F]](terms []air.Term[F]) []air.Term[F] {
	var nterms []air.Term[F] = make([]air.Term[F], len(terms))
	//
	for i, t := range terms {
		nterms[i] = t.Simplify(false)
	}
	//
	return nterms
}

func (p *AirLowering[F]) nextSumConjunct(bitwidths []uint, terms []air.Term[F]) (n uint) {
	//
	var (
		sum big.Int
	)
	//
	for i := 0; i < len(terms); i++ {
		var (
			ith    = terms[i]
			values = valueRangeOf(ith, bitwidths)
			minVal = values.MinValue()
			maxVal = values.MaxValue()
			signed = !minVal.IsNotAnInfinity() || minVal.Sign() < 0
		)
		// Check for signed value
		if signed || !maxVal.IsNotAnInfinity() {
			// terminate packet.  Observe that, we need to make sure at least
			// one item is included in the next packet.
			return uint(max(1, i))
		}
		// Update sum value
		tmp := maxVal.IntVal()
		//
		sum.Add(&sum, &tmp)
		// Check sum still within field bandwidth
		if uint(sum.BitLen()) > p.fieldBandwidth {
			// terminate here
			return uint(max(1, i))
		}
	}
	// consume all terms
	return uint(len(terms))
}

func (p *AirLowering[F]) nextNonZeroCheck(checks []Term[F], bitwidths []uint) (n uint) {
	var (
		// Bitwidth of current check
		sum big.Int
	)
	//
	for i := 0; i < len(checks); i++ {
		var (
			ith          = checks[i].(*RegisterAccess[F])
			ith_bitwidth = bitwidths[ith.Register().Unwrap()]
		)
		// Sanity check bitwidth
		if ith_bitwidth == math.MaxUint {
			// terminate packet.  Observe that, we need to make sure at least
			// one item is included in the next packet.
			return uint(max(1, i))
		}
		//
		ithRange := valueRangeOfBits(ith_bitwidth)
		ithMax := ithRange.MaxIntValue()
		//
		sum.Add(&sum, &ithMax)
		//
		if sum.BitLen() > int(p.fieldBandwidth) {
			// terminate packet here
			return uint(max(1, i))
		}
	}
	// Consume all checks
	return uint(len(checks))
}

func isNonZeroCheck[F field.Element[F]](term LogicalTerm[F]) *RegisterAccess[F] {
	if t, ok := term.(*NotEqual[F]); ok {
		var candidate Term[F]
		//
		if isZero(t.Lhs) {
			candidate = t.Rhs
		} else if isZero(t.Rhs) {
			candidate = t.Lhs
		} else {
			return nil
		}
		// Final check
		if ra, ok := candidate.(*RegisterAccess[F]); ok {
			return ra
		}
	}
	//
	return nil
}

func isZero[F field.Element[F]](term Term[F]) bool {
	if c, ok := term.(*Constant[F]); ok {
		return c.Value.IsZero()
	}
	//
	return false
}

// Construct the disjunction lhs v rhs, where both lhs and rhs can be
// conjunctions of terms.
func disjunction[F field.Element[F]](terms ...[]air.Term[F]) []air.Term[F] {
	// Base cases
	switch len(terms) {
	case 0:
		// NOTE: return non-zero value to indicate a failure.
		return []air.Term[F]{term.Const64[F, air.Term[F]](1)}
	case 1:
		return terms[0]
	}
	//
	var (
		nterms []air.Term[F]
		lhs    = terms[0]
		rhs    = disjunction(terms[1:]...)
	)
	// FIXME: this is where things can get expensive, and it would be useful to
	// explore whether extractNormalisedCondition could help here.
	for _, l := range lhs {
		for _, r := range rhs {
			disjunct := term.Product(l, r)
			nterms = append(nterms, disjunct)
		}
	}
	//
	return nterms
}

func valueRangeOf[F field.Element[F]](term air.Term[F], bitwidths []uint) util_math.Interval {
	switch t := term.(type) {
	case *air.Add[F]:
		var res util_math.Interval

		for i, arg := range t.Args {
			ith := valueRangeOf(arg, bitwidths)
			if i == 0 {
				res.Set(ith)
			} else {
				res.Add(ith)
			}
		}
		//
		return res
	case *air.ColumnAccess[F]:
		var rid = t.Register().Unwrap()
		// NOTE: the following is necessary because MaxUint is permitted as a signal
		// that the given register has no fixed bitwidth.  Rather, it can consume
		// all possible values of the underlying field element.
		if rid >= uint(len(bitwidths)) || bitwidths[rid] == math.MaxUint {
			return util_math.INFINITY
		}
		//
		return valueRangeOfBits(bitwidths[rid])
	case *air.Constant[F]:
		var c big.Int
		// Extract big integer from field element
		c.SetBytes(t.Value.Bytes())
		// Return as interval
		return util_math.NewInterval(c, c)
	case *air.Mul[F]:
		var res util_math.Interval

		for i, arg := range t.Args {
			ith := valueRangeOf(arg, bitwidths)
			if i == 0 {
				res.Set(ith)
			} else {
				res.Mul(ith)
			}
		}
		//
		return res
	case *air.Sub[F]:
		var res util_math.Interval

		for i, arg := range t.Args {
			ith := valueRangeOf(arg, bitwidths)
			if i == 0 {
				res.Set(ith)
			} else {
				res.Sub(ith)
			}
		}
		//
		return res
	default:
		panic("unknown AIR term encountered")
	}
}

func valueRangeOfBits(bitwidth uint) util_math.Interval {
	var bound = big.NewInt(2)
	//
	bound.Exp(bound, big.NewInt(int64(bitwidth)), nil)
	// Subtract 1 because interval is inclusive.
	bound.Sub(bound, &biONE)
	// Done
	return util_math.NewInterval(biZERO, *bound)
}

// This function goes through all the registers of the module to determine which
// have type constraints and records their maximum bitwidths arising.
func determineTrueBitwidths[F field.Element[F]](mirModule schema.Module[F]) []uint {
	var bitwidths = make([]uint, mirModule.Width())
	// initialise with maximum widths
	for i := range bitwidths {
		bitwidths[i] = math.MaxUint
	}
	//
	for iter := mirModule.Constraints(); iter.HasNext(); {
		// Following should always hold
		c := iter.Next().(Constraint[F])
		// Check what kind of constraint we have
		if v, ok := c.constraint.(RangeConstraint[F]); ok {
			// apply constraints
			for i, e := range v.Sources {
				rid := e.Register().Unwrap()
				// sanity check
				if bitwidths[rid] != math.MaxUint {
					panic("duplicate range constraint detected")
				}
				// Update bitwidth accordingly
				bitwidths[rid] = min(bitwidths[rid], v.Bitwidths[i])
			}
		}
	}
	//
	return bitwidths
}
