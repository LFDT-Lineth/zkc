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
	"fmt"
	"math/big"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

type mirTerm = mir.Term[word.BigEndian]
type mirLogicalTerm = mir.LogicalTerm[word.BigEndian]
type mirModuleBuilder = mir.ModuleBuilder[word.BigEndian]
type mirRegisterAccess = mir.RegisterAccess[word.BigEndian]
type mirVectorAccess = mir.VectorAccess[word.BigEndian]

// LowerToMir lowers (or refines) an HIR schema into an MIR schema.  That means
// lowering all the columns and constraints, whilst adding additional columns /
// constraints as necessary to preserve the original semantics.
func LowerToMir[E register.ConstMap](externs []E, modules []Module) []mir.Module[word.BigEndian] {
	var lowering = NewMirLowering(externs, modules)
	//
	return lowering.Lower()
}

// MirLowering captures all auxiliary state required in the process of lowering
// modules from HIR to MIR.  This state is because, as part of the lowering
// process, we may introduce some number of additional modules (e.g. for
// managing type proofs).
type MirLowering struct {
	// Modules we are lowering from
	hirModules []Module
	// Modules we are lowering to
	mirSchema mir.SchemaBuilder[word.BigEndian]
}

// NewMirLowering constructs an initial state for lowering a given MIR schema.
func NewMirLowering[E register.ConstMap](externs []E, modules []Module) MirLowering {
	var (
		mirSchema = ir.NewSchemaBuilder[word.BigEndian, mir.Constraint[word.BigEndian], mirTerm](externs...)
	)
	// Initialise MIR modules
	for _, m := range modules {
		mirSchema.NewModule(m.Name(), m.AllowPadding(), m.IsPublic(), m.IsSynthetic(), m.IsStatic(), m.IsNative(), m.Keys())
	}
	//
	return MirLowering{
		modules,
		mirSchema,
	}
}

// Lower the MIR schema provide when this lowering instance was created into an
// equivalent MIR schema.
func (p *MirLowering) Lower() []mir.Module[word.BigEndian] {
	var n = len(p.mirSchema.Externs())
	// Initialise modules
	for i := range len(p.hirModules) {
		p.initialiseModule(uint(i), uint(i+n))
	}
	// Lower modules
	for i := range len(p.hirModules) {
		p.lowerModule(uint(i), uint(i+n))
	}
	// Build concrete modules from schema
	return ir.BuildSchema[mir.Module[word.BigEndian]](p.mirSchema)
}

// InitialiseModule simply initialises all registers within the module, but does
// not lower any constraint or assignments.
func (p *MirLowering) initialiseModule(hirIndex, mirIndex uint) {
	var (
		hirModule = p.hirModules[hirIndex]
		mirModule = p.mirSchema.Module(mirIndex)
	)
	// Initialise registers in MIR module
	mirModule.NewRegisters(hirModule.Registers()...)
}

// LowerModule lowers the given MIR module into the corresponding MIR module.
// This includes all constraints and assignments.
func (p *MirLowering) lowerModule(hirIndex, mirIndex uint) {
	var (
		hirModule = p.hirModules[hirIndex]
		mirModule = p.mirSchema.Module(mirIndex)
	)
	// Lower assignments.
	for iter := hirModule.Assignments(); iter.HasNext(); {
		mirModule.AddAssignment(iter.Next())
	}
	// Lower constraints
	for iter := hirModule.Constraints(); iter.HasNext(); {
		// Following should always hold
		constraint := iter.Next().(Constraint)
		//
		p.lowerConstraint(constraint, mirModule)
	}
}

// Lower a constraint to the MIR level.
func (p *MirLowering) lowerConstraint(c Constraint, mirModule mirModuleBuilder) {
	// Check what kind of constraint we have
	switch v := c.constraint.(type) {
	case Assertion:
		p.lowerAssertion(v, mirModule)
	case FunctionCall:
		p.lowerFunctionCall(v, mirModule)
	case InterleavingConstraint:
		p.lowerInterleavingConstraint(v, mirModule)
	case LookupConstraint:
		p.lowerLookupConstraint(v, mirModule)
	case PermutationConstraint:
		p.lowerPermutationConstraint(v, mirModule)
	case RangeConstraint:
		p.lowerRangeConstraint(v, mirModule)
	case SortedConstraint:
		p.lowerSortedConstraint(v, mirModule)
	case VanishingConstraint:
		p.lowerVanishingConstraint(v, mirModule)
	default:
		// Should be unreachable as no other constraint types can be added to a
		// schema.
		panic("unreachable")
	}
}

// Lowering an assertion is straightforward since its not a true constraint.
func (p *MirLowering) lowerAssertion(v Assertion, module mirModuleBuilder) {
	module.AddConstraint(mir.NewAssertion[word.BigEndian](v.Handle, v.Context, v.Domain, v.Property))
}

func (p *MirLowering) lowerFunctionCall(v FunctionCall, module mirModuleBuilder) {
	var (
		nargs        = len(v.Arguments)
		nrets        = len(v.Returns)
		sources      = make([]lookup.Vector[word.BigEndian, *mirRegisterAccess], 1)
		targets      = make([]lookup.Vector[word.BigEndian, *mirRegisterAccess], 1)
		selector     = util.None[*mirRegisterAccess]()
		calleeModule = p.mirSchema.Module(v.Callee)
	)
	// Expand arguments and returns
	sourceTerms := p.expandTerms(module, append(v.Arguments, v.Returns...)...)
	// Expand selector (if applicable)
	if v.Selector.HasValue() {
		sel := p.expandLogicalTerm(v.Selector.Unwrap(), module)
		selector = util.Some(sel)
	}
	// Construct source vector
	sources[0] = lookup.NewVector(v.Caller, selector, sourceTerms...)
	// Construct target vector
	targetTerms := make([]*mirRegisterAccess, nargs+nrets)
	//
	for i := range nargs + nrets {
		var (
			rid      = register.NewId(uint(i))
			bitwidth = calleeModule.Register(rid).Width()
		)
		//
		targetTerms[i] = term.RawRegisterAccess[word.BigEndian, mirTerm](rid, bitwidth, 0)
	}
	// Done
	targets[0] = lookup.NewVector(v.Callee, util.None[*mirRegisterAccess](), targetTerms...)
	// Add constraint
	module.AddConstraint(mir.NewLookupConstraint(v.Handle, targets, sources))
}

// Lower a vanishing constraint to the MIR level.  This is relatively
// straightforward and simply relies on lowering the expression being
// constrained.
func (p *MirLowering) lowerVanishingConstraint(v VanishingConstraint, module mirModuleBuilder) {
	var term = p.lowerLogical(v.Constraint, module)
	//
	module.AddConstraint(
		mir.NewVanishingConstraint(v.Handle, v.Context, v.Domain, term))
}

// Lower a permutation constraint to the MIR level.  This is trivial because
// permutation constraints do not currently support complex forms.
func (p *MirLowering) lowerPermutationConstraint(v PermutationConstraint, module mirModuleBuilder) {
	module.AddConstraint(
		mir.NewPermutationConstraint[word.BigEndian](v.Handle, v.Context, v.Targets, v.Sources),
	)
}

// Lower a range constraint to the MIR level.  Since range constraints at the
// MIR level can only access columns directly, we must expand the source
// expressions into computed columns with corresponding constraints.
func (p *MirLowering) lowerRangeConstraint(v RangeConstraint, module mirModuleBuilder) {
	var term = p.expandTerms(module, v.Sources...)
	//
	module.AddConstraint(
		mir.NewRangeConstraint(v.Handle, v.Context, term, v.Bitwidths))
}

// Lower an interleaving constraint to the MIR level.  Since interleaving
// constraints at the MIR level can only access columns directly, we must expand
// the source expressions into computed columns with corresponding constraints.
func (p *MirLowering) lowerInterleavingConstraint(c InterleavingConstraint, mod mirModuleBuilder) {
	//
	// Lower sources
	sources := p.expandTermsAsVectors(c.Sources, mod)
	// Lower target
	target := p.expandTermAsVector(c.Target, mod)
	// Add constraint
	mod.AddConstraint(
		mir.NewInterleavingConstraint(c.Handle, c.TargetContext, c.SourceContext, target, sources))
}

// Lower a lookup constraint to the MIR level.  Since lookup constraints at the
// MIR level can only access columns directly, we must expand the source
// expressions into computed columns with corresponding constraints.
func (p *MirLowering) lowerLookupConstraint(c LookupConstraint, mirModule mirModuleBuilder) {
	var (
		sources = make([]lookup.Vector[word.BigEndian, *mirRegisterAccess], len(c.Sources))
		targets = make([]lookup.Vector[word.BigEndian, *mirRegisterAccess], len(c.Targets))
	)
	// Lower sources
	for i, ith := range c.Sources {
		sources[i] = p.lowerLookupVector(ith)
	}
	// Lower targets
	for i, ith := range c.Targets {
		targets[i] = p.lowerLookupVector(ith)
	}
	// Add constraint
	mirModule.AddConstraint(mir.NewLookupConstraint(c.Handle, targets, sources))
}

func (p *MirLowering) lowerLookupVector(vec lookup.Vector[word.BigEndian, Term],
) lookup.Vector[word.BigEndian, *mirRegisterAccess] {
	var (
		module   = p.mirSchema.Module(vec.Module)
		terms    = make([]*mirRegisterAccess, len(vec.Terms))
		selector util.Option[*mirRegisterAccess]
	)
	//
	if vec.HasSelector() {
		sel := p.expandTerm(vec.Selector.Unwrap(), module)
		selector = util.Some(sel)
	}
	//
	for i, e := range vec.Terms {
		// Check for unsafe operation (e.g. cast)
		var (
			unsafe = selector.HasValue() && term.IsUnsafeExpr[word.BigEndian, LogicalTerm, Term](e)
			expr   = e
		)
		//
		if unsafe {
			expr = term.Product(vec.Selector.Unwrap(), expr)
		}
		//
		terms[i] = p.expandTerm(expr, module)
	}
	//
	return lookup.NewVector(vec.Module, selector, terms...)
}

// Lower a sorted constraint to the MIR level.  Since sorting constraints at the
// MIR level can only access columns directly, we must expand the source
// expressions into computed columns with corresponding constraints.
func (p *MirLowering) lowerSortedConstraint(c SortedConstraint, module mirModuleBuilder) {
	var (
		terms    = p.expandTerms(module, c.Sources...)
		selector util.Option[*mirRegisterAccess]
	)
	//
	if c.Selector.HasValue() {
		sel := p.expandTerm(c.Selector.Unwrap(), module)
		selector = util.Some(sel)
	}
	// Add constraint
	module.AddConstraint(
		mir.NewSortedConstraint(c.Handle, c.Context, c.BitWidth, selector, terms, c.Signs, c.Strict))
}

func (p *MirLowering) lowerLogical(e LogicalTerm, module mirModuleBuilder) mirLogicalTerm {
	//
	switch e := e.(type) {
	case *Conjunct:
		return term.Conjunction(p.lowerLogicals(e.Args, module)...)
	case *Disjunct:
		return term.Disjunction(p.lowerLogicals(e.Args, module)...)
	case *Equal:
		return p.lowerEquality(true, e.Lhs, e.Rhs, module)
	case *Ite:
		return p.lowerIte(e, module)
	case *Negate:
		return term.Negation(p.lowerLogical(e.Arg, module))
	case *NotEqual:
		return p.lowerEquality(false, e.Lhs, e.Rhs, module)
	default:
		name := reflect.TypeOf(e).Name()
		panic(fmt.Sprintf("unknown HIR expression \"%s\"", name))
	}
}

func (p *MirLowering) lowerOptionalLogical(e LogicalTerm, module mirModuleBuilder) mirLogicalTerm {
	if e == nil {
		return nil
	}
	//
	return p.lowerLogical(e, module)
}

func (p *MirLowering) lowerLogicals(terms []LogicalTerm, module mirModuleBuilder,
) []mirLogicalTerm {
	//
	nexprs := make([]mirLogicalTerm, len(terms))

	for i := range len(terms) {
		nexprs[i] = p.lowerLogical(terms[i], module)
	}

	return nexprs
}

func (p *MirLowering) lowerEquality(sign bool, left Term, right Term, module mirModuleBuilder,
) mirLogicalTerm {
	//
	var fn = func(lhs, rhs mirTerm) mirLogicalTerm {
		//
		if sign {
			return term.Equals[word.BigEndian, mirLogicalTerm](lhs, rhs)
		}
		//
		return term.NotEquals[word.BigEndian, mirLogicalTerm](lhs, rhs)
	}
	//
	return p.lowerBinaryLogical(left, right, fn, module)
}

func (p *MirLowering) lowerIte(expr *Ite, module mirModuleBuilder) mirLogicalTerm {
	var (
		condition   = p.lowerLogical(expr.Condition, module)
		trueBranch  = p.lowerOptionalLogical(expr.TrueBranch, module)
		falseBranch = p.lowerOptionalLogical(expr.FalseBranch, module)
	)
	//
	return term.IfThenElse(condition, trueBranch, falseBranch)
}

func (p *MirLowering) lowerBinaryLogical(lhs, rhs Term, fn BinaryLogicalFn, module mirModuleBuilder,
) mirLogicalTerm {
	//
	var (
		lTerm = p.lowerTerm(lhs, module)
		rTerm = p.lowerTerm(rhs, module)
	)
	//
	return DisjunctIfTerms(fn, lTerm, rTerm)
}

func (p *MirLowering) expandTermsAsVectors(es []Term, module mirModuleBuilder) []*mirVectorAccess {
	vecs := make([]*mirVectorAccess, len(es))
	//
	for i, e := range es {
		vecs[i] = p.expandTermAsVector(e, module)
	}
	//
	return vecs
}

func (p *MirLowering) expandTermAsVector(e Term, module mirModuleBuilder) *mirVectorAccess {
	return term.RawVectorAccess(p.expandTerms(module, e))
}

func (p *MirLowering) expandTerms(mirModule mirModuleBuilder, es ...Term) (terms []*mirRegisterAccess) {
	//
	terms = make([]*mirRegisterAccess, len(es))
	//
	for i, e := range es {
		terms[i] = p.expandTerm(e, mirModule)
	}
	//
	return terms
}

func (p *MirLowering) expandLogicalTerm(le LogicalTerm, module mirModuleBuilder) *mir.RegisterAccess[word.BigEndian] {
	var (
		truth     = term.Const64[word.BigEndian, Term](1)
		falsehood = term.Const64[word.BigEndian, Term](0)
		expr      = term.IfElse(le, truth, falsehood)
	)
	// Expand if-term
	return p.expandTerm(expr, module)
}

// Expand an arbitrary term into a column as necessary.  This is used to lower
// constraints by compiling out expressions, such that the lowered constraint
// only operates over column accesses (i.e. because this is the form required
// for the AIR layer used by the prover).  To do this, requires two pieces:
// first, the expression is evaluated using an assignment which stores the
// result into what is essentially a temporary column; second, a constraint is
// used to enforce the relationship between that column and the original
// expression.
func (p *MirLowering) expandTerm(e Term, module mirModuleBuilder) *mir.RegisterAccess[word.BigEndian] {
	// Check whether this really requires expansion (or not).
	if rid, width, shift, mask, ok := isRegisterAccess(e); ok {
		// No, expansion not required
		return term.RawRegisterAccess[word.BigEndian, mirTerm](rid, width, shift).Mask(mask)
	} else if c, ok := term.IsConstant64(e); ok && (c == 0 || c == 1) {
		var (
			ca            = module.ConstRegister(uint8(c))
			bitwidth uint = uint(c)
		)
		// Expansion not required
		return term.RawRegisterAccess[word.BigEndian, mirTerm](ca, bitwidth, 0)
	}
	// Yes, expansion is really necessary
	var (
		expr   = p.lowerTerm(e, module)
		values = e.ValueRange()
		// Determine bitwidth required for target register
		bitwidth, sign = values.BitWidth()
		// Determine computed column name
		name = e.Lisp(true, module).String(false)
		// Look up column
		index, ok = module.HasRegister(name)
		// Default padding (for now)
		padding big.Int = ir.PaddingFor(e, module)
	)
	// Sanity check
	if sign {
		panic("cannot determine bitwidth of (signed) term")
	}
	//
	if !ok {
		// Convert expression into a generic computation
		computation := term.NewComputation[word.BigEndian, LogicalTerm](e)
		// Declared a new computed column
		index = module.NewRegister(register.NewComputed(name, bitwidth, padding))
		// Add assignment for filling said computed column
		module.AddAssignment(
			assignment.NewComputedRegister[word.BigEndian](computation, true, module.Id(), index))
		// Construct v == [e]
		eq_e_v := expr.Equate(index, bitwidth)
		// Ensure v == e, where v is value of computed column.
		module.AddConstraint(
			mir.NewVanishingConstraint(name, module.Id(), util.None[int](), eq_e_v))
	}
	// FIXME: eventually we just want to return the index
	return term.RawRegisterAccess[word.BigEndian, mirTerm](index, bitwidth, 0)
}

// Determine whether this is a direct register access, or not.
func isRegisterAccess(term Term) (rid register.Id, width uint, shift int, mask uint, ok bool) {
	switch t := term.(type) {
	case *RegisterAccess:
		return t.Register(), t.BitWidth(), t.RelativeShift(), t.MaskWidth(), true
	case *Cast:
		if r, w, s, m, ok := isRegisterAccess(t.Arg); ok {
			// sanity check cast makes sense
			if m < t.BitWidth {
				// TODO: provide a proper error message
				panic("cast out-of-bounds")
			}
			//
			return r, w, s, t.BitWidth, true
		}
	}
	//
	return register.UnusedId(), 0, 0, 0, false
}

// Lower a given HIR expression into one or more "conditional" MIR expressions.
// In the majority of cases, an HIR expression is lowered into a single
// unconditional MIR expression.  However, the presence of nested "if" terms
// introduces the need to separate out terms with conditions.  Consider the
// following minimal example:
//
// (== X
//
//	(if (== 0 Y) 0 7))
//
// In this case, we lower into (effectively the following two MIR conditional
// expressions:
//
// (1) (== X 0) when (== 0 Y)
// (2) (== X 7) when (!= 0 Y)
//
// The reason for doing this is to enable lowering to a polynomial expression
// (i.e. since these cannot contain nested normalisation expressions, etc).  An
// alternative approach would be to expand nested normalisations arising into
// inverse columns as they arise.  However, this was deemed to be less than
// desirable because it introduces products of the form (x*x⁻¹) which are
// expensive in the context of small fields.
func (p *MirLowering) lowerTerm(e Term, mirModule mirModuleBuilder) IfTerm {
	//
	switch e := e.(type) {
	case *Add:
		fn := func(args []mirTerm) mirTerm {
			return term.Sum(args...)
		}
		//
		return p.lowerTerms(fn, mirModule, e.Args...)
	case *Cast:
		if r, ok := e.Arg.(*RegisterAccess); ok {
			var (
				reg = mirModule.Register(r.Register())
			)
			// Sanity check cast makes sense
			if reg.Width() < e.BitWidth {
				// TODO: provide a proper error message
				panic("cast out-of-bounds")
			}
			// Construct access for the given register
			t := term.RawRegisterAccess[word.BigEndian, mirTerm](r.Register(), reg.Width(), r.RelativeShift())
			// Implement cast by masking register
			return UnconditionalTerm(t.Mask(e.BitWidth))
		}
		//
		return p.lowerTerm(e.Arg, mirModule)
		//
	case *Constant:
		return UnconditionalTerm(term.Const[word.BigEndian, mirTerm](e.Value))
	case *RegisterAccess:
		var t = term.RawRegisterAccess[word.BigEndian, mirTerm](e.Register(), e.BitWidth(), e.RelativeShift())
		// Carry forward any mask
		return UnconditionalTerm(t.Mask(e.MaskWidth()))
	case *Exp:
		return p.lowerExpTo(e, mirModule)
	case *IfZero:
		condition := p.lowerLogical(e.Condition, mirModule)
		trueBranch := p.lowerTerm(e.TrueBranch, mirModule)
		falseBranch := p.lowerTerm(e.FalseBranch, mirModule)
		//
		return IfThenElse(condition, trueBranch, falseBranch)
	case *LabelledConst:
		return UnconditionalTerm(term.Const[word.BigEndian, mirTerm](e.Value))
	case *Mul:
		fn := func(args []mirTerm) mirTerm {
			return term.Product(args...)
		}
		//
		return p.lowerTerms(fn, mirModule, e.Args...)
	case *Norm:
		var (
			zero = term.Const[word.BigEndian, mirTerm](field.Zero[word.BigEndian]())
			one  = term.Const[word.BigEndian, mirTerm](field.One[word.BigEndian]())
			arg  = p.lowerTerm(e.Arg, mirModule)
		)
		//
		return IfEqElse(arg, zero, zero, one)
	case *Sub:
		fn := func(args []mirTerm) mirTerm {
			return term.Subtract(args...)
		}
		//
		return p.lowerTerms(fn, mirModule, e.Args...)
	case *VectorAccess:
		return UnconditionalTerm(p.lowerVectorAccess(e))
	default:
		name := reflect.TypeOf(e).Name()
		panic(fmt.Sprintf("unknown HIR expression \"%s\"", name))
	}
}

// Lower a set of zero or more HIR expressions.
func (p *MirLowering) lowerTerms(fn NaryFn, mirModule mirModuleBuilder, exprs ...Term) IfTerm {
	var nexprs = make([]IfTerm, len(exprs))
	//
	for i := range len(exprs) {
		nexprs[i] = p.lowerTerm(exprs[i], mirModule)
	}
	//
	return MapIfTerms(fn, nexprs...)
}

// LowerTo lowers an exponent expression to the MIR level by lowering the
// argument, and then constructing a multiplication.  This is because the AIR
// level does not support an explicit exponent operator.
func (p *MirLowering) lowerExpTo(e *Exp, mirModule mirModuleBuilder) IfTerm {
	var (
		// Lower expression being raised
		expr = p.lowerTerm(e.Arg, mirModule)
	)
	//
	return expr.Map(func(arg mirTerm) mirTerm {
		// Multiply it out k times
		es := make([]mirTerm, e.Pow)
		//
		for i := uint64(0); i < e.Pow; i++ {
			es[i] = arg
		}
		// Done
		return term.Product[word.BigEndian](es...)
	})
}

func (p *MirLowering) lowerVectorAccess(e *VectorAccess) mirTerm {
	var (
		vars = make([]*term.RegisterAccess[word.BigEndian, mirTerm], len(e.Vars))
	)
	//
	for i, v := range e.Vars {
		ith := term.RawRegisterAccess[word.BigEndian, mirTerm](v.Register(), v.BitWidth(), v.RelativeShift())
		//
		vars[i] = ith.Mask(v.MaskWidth())
	}
	//
	return term.NewVectorAccess(vars)
}
