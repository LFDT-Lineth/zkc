package picus

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/verify/picus/pcl"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// MirPicusTranslator captures any state needed to lower an MIR schema to a program in PCL (Picus Constraint Language).
// Core structure is borrowed from `lower.go` which converts MIR to AIR
type MirPicusTranslator[F field.Element[F]] struct {
	// Modules we are lowering from
	mirSchema mir.Schema[F]
	// Picus program we are building
	picusProgram *pcl.Program[F]
	// Tracks the current path conditions during lowering. This is necessary because of the
	// ⊥ symbol which is a constraint indicating that the path condition at the location is false
	pathConditions []pcl.Formula[F]
}

// NewMirPicusTranslator constructs a picus translator.
func NewMirPicusTranslator[F field.Element[F]](mirSchema mir.Schema[F]) *MirPicusTranslator[F] {
	return &MirPicusTranslator[F]{
		mirSchema:      mirSchema,
		picusProgram:   pcl.NewProgram[F](ModulusOf[F]()),
		pathConditions: make([]pcl.Formula[F], 0),
	}
}

// Translate translates the provided MIR Schema to a Picus Program.
func (p *MirPicusTranslator[F]) Translate() *pcl.Program[F] {
	// Perform a 1-1 translation between MIR modules and Picus modules
	var i uint
	for i = 0; i < p.mirSchema.Width(); i++ {
		p.TranslateModule(i)
	}

	return p.picusProgram
}

// TranslateModule compiles an MIR Module to a Picus Module.
// `i` denotes the index of the MIR module in the schema.
func (p *MirPicusTranslator[F]) TranslateModule(i uint) {
	// get the MIR module
	mirModule := p.mirSchema.Module(i)
	if mirModule.IsSynthetic() {
		// synthetic modules are not supported now
		// need to examine constraints with synthetic modules
		// to determine if they can flow through this translation procedure
		panic("Cannot translate synthetic modules now")
	}
	// initialize the corresponding PCL module
	picusModule := p.picusProgram.AddModule(mirModule.Name().String())
	// register inputs and outputs from MIR inputs/outputs
	for _, register := range mirModule.Registers() {
		picusVar := pcl.V[F](register.Name())
		if register.IsInput() {
			picusModule.AddInput(picusVar)
		} else if register.IsOutput() {
			picusModule.AddOutput(picusVar)
		}
	}

	// build PCL constraints from MIR constraints
	for iter := mirModule.Constraints(); iter.HasNext(); {
		constraint := iter.Next().(mir.Constraint[F])
		p.translateConstraint(constraint, picusModule, mirModule)
	}
}

// translateConstraint translates MIR constraints into PCL constraints.
// The built constraints are implicitly added to `picusModule`
func (p *MirPicusTranslator[F]) translateConstraint(c mir.Constraint[F],
	picusModule *pcl.Module[F], mirModule schema.Module[F],
) {
	// Check what kind of constraint we have
	switch v := c.Unwrap().(type) {
	case mir.RangeConstraint[F]:
		p.translateRangeConstraint(v, picusModule, mirModule)
	case mir.VanishingConstraint[F]:
		p.translateVanishing(v, picusModule, mirModule)
	default:
		panic(fmt.Sprintf("Unhandled constraint: %s", c.Unwrap()))
	}
}

// translateRangeConstraint translates a MIR range constraint `r` to a PCL less than constraint.
func (p *MirPicusTranslator[F]) translateRangeConstraint(r mir.RangeConstraint[F],
	picusModule *pcl.Module[F], mirModule schema.Module[F],
) {
	for i, e := range r.Sources {
		expr := p.lowerTerm(e, mirModule)
		// 1. Get the `big.Int` representation of the max unisgned value for a given bitwidth.
		// 2. Create a field element from the big integer.
		// 3. Construct a PCL constant from the field element.
		upperBound := pcl.C(field.BigInt[F](*MaxValueBig(int(r.Bitwidths[i]))))
		// Add (assert (<= `expr` `upperBound`))
		picusModule.AddLeqConstraint(expr, upperBound)
	}
}

// translateVanishing translates an MIR vanishing constraint into one or more PCL constraints.
func (p *MirPicusTranslator[F]) translateVanishing(v mir.VanishingConstraint[F],
	picusModule *pcl.Module[F], mirModule schema.Module[F],
) {
	if v.Domain.HasValue() {
		// TODO: need to handle this. Row specific constraints will require generating Picus modules which collect
		// all constraints that apply to the specific row.
		panic("row specific constraints are not supported!")
	}
	// We translate the logical term to a collection of Picus constraints
	picusModule.Constraints = append(picusModule.Constraints, p.logicalTermToConstraints(v.Constraint, mirModule)...)
}

// logicalTermToConstraints translates a MIR logical term to one or more PCL constraints.
// Most operations have a one to one translation,
// but for others it makes more sense to break them up into multiple Picus constraints.
// For example, a vanishing constraint that is a conjunction of terms can be split into two Picus constraints.
// Picus doesn't allow ITEs to be combined with connectives i.e (/\ (= x y) /\ (if ...)) so an ITE needs to map
// to a PCL ITE constraint
func (p *MirPicusTranslator[F]) logicalTermToConstraints(t mir.LogicalTerm[F],
	mirModule schema.Module[F],
) []pcl.Constraint[F] {
	switch e := t.(type) {
	case *mir.Conjunct[F]:
		return p.conjunctToConstraints(e, mirModule)
	case *mir.Equal[F]:
		return p.equalToConstraints(e, mirModule)
	case *mir.Ite[F]:
		return p.iteToConstraints(e, mirModule)
	case *mir.Disjunct[F]:
		return p.disjunctToConstraints(e, mirModule)
	default:
		panic(fmt.Sprintf("Unhandled term: %v", e))
	}
}

// Translates an MIR if-then-else constraint into a PCL ite constraint.
func (p *MirPicusTranslator[F]) iteToConstraints(ite *mir.Ite[F], mirModule schema.Module[F]) []pcl.Constraint[F] {
	// Generate a Picus formula that corresponds to the ite guard.
	picusCondition := p.logicalTermToFormula(ite.Condition, mirModule)
	// Convert the `ite.TrueBranch` and add the guard to our path conditions.
	p.pathConditions = append(p.pathConditions, picusCondition)
	constraintsTrueBranch := p.logicalTermToConstraints(ite.TrueBranch, mirModule)
	// Drop the constraint from the current path condition.
	p.pathConditions = p.pathConditions[:len(p.pathConditions)-1]
	// Add the negation of the guard to the path conditions to process the False branch.
	p.pathConditions = append(p.pathConditions, pcl.NewNot[F](picusCondition))
	// Convert the `ite.FalseBranch`.
	constraintsFalseBranch := p.logicalTermToConstraints(ite.FalseBranch, mirModule)
	// Drop the negation of the guard from path condition
	p.pathConditions = p.pathConditions[:len(p.pathConditions)-1]

	return []pcl.Constraint[F]{
		pcl.NewIfElse(picusCondition, constraintsTrueBranch, constraintsFalseBranch),
	}
}

// Translate an equality MIR constraint to a PCL equality constraint
func (p *MirPicusTranslator[F]) equalToConstraints(eq *mir.Equal[F], mirModule schema.Module[F]) []pcl.Constraint[F] {
	return []pcl.Constraint[F]{pcl.NewPicusConstraint[F](p.convertEqual(eq, mirModule))}
}

// conjunctToConstraints translates an MIR conjunction (/\ m_1 .. m_n)
// to a collection of Picus constraints [p_1, ..., p_n].
// Right now Picus works better if the constraints are split
// into different assertions so we do that here.
func (p *MirPicusTranslator[F]) conjunctToConstraints(conj *mir.Conjunct[F],
	mirModule schema.Module[F],
) []pcl.Constraint[F] {
	picusConstraints := make([]pcl.Constraint[F], 0)
	for _, conjunct := range conj.Args {
		picusConstraints = append(picusConstraints,
			p.logicalTermToConstraints(conjunct, mirModule)...)
	}

	return picusConstraints
}

// disjunctToConstraints translates an MIR disjunction (V m_1 ... m_n)
// to a Picus disjunction. There is a special case that needs to be handled for
// an empty disjunction which corresponds to ⊥. This code assumes that
// constraint only appears in `if-then-else`'s.
func (p *MirPicusTranslator[F]) disjunctToConstraints(disj *mir.Disjunct[F],
	mirModule schema.Module[F],
) []pcl.Constraint[F] {
	if len(disj.Args) == 0 {
		if len(p.pathConditions) == 0 {
			panic("Empty disjunct should correspond to impossible path condition. As such, path conditions cannot be empty")
		}

		return []pcl.Constraint[F]{
			pcl.NewPicusConstraint[F](pcl.NewNot[F](pcl.FoldAnd(p.pathConditions))),
		}
	}

	picusConstraints := make([]pcl.Constraint[F], 1)
	picusConstraints[0] = pcl.NewPicusConstraint[F](p.convertDisjunct(disj, mirModule))

	return picusConstraints
}

// Converts an MIR LogicalTerm to a Picus LogicalTerm
func (p *MirPicusTranslator[F]) logicalTermToFormula(t mir.LogicalTerm[F], mirModule schema.Module[F]) pcl.Formula[F] {
	switch e := t.(type) {
	case *mir.Conjunct[F]:
		return p.convertConjunct(e, mirModule)
	case *mir.Disjunct[F]:
		return p.convertDisjunct(e, mirModule)
	case *mir.Equal[F]:
		return p.convertEqual(e, mirModule)
	case *mir.NotEqual[F]:
		return p.converNotEqual(e, mirModule)
	default:
		panic(fmt.Sprintf("Unhandled term: %v", e))
	}
}

// Converts an MIR conjunction to a PCL conjunction formula.
func (p *MirPicusTranslator[F]) convertConjunct(conj *mir.Conjunct[F], mirModule schema.Module[F]) pcl.Formula[F] {
	return pcl.FoldAnd(p.logicalTermsToFormulas(conj.Args, mirModule))
}

// Converts an MIR disjunction to a PCL conjunction formula.
func (p *MirPicusTranslator[F]) convertDisjunct(conj *mir.Disjunct[F], mirModule schema.Module[F]) pcl.Formula[F] {
	return pcl.FoldOr(p.logicalTermsToFormulas(conj.Args, mirModule))
}

// Converts an MIR equals to a PCL equals formula
func (p *MirPicusTranslator[F]) convertEqual(eq *mir.Equal[F], mirModule schema.Module[F]) pcl.Formula[F] {
	lhs := p.lowerTerm(eq.Lhs, mirModule)
	rhs := p.lowerTerm(eq.Rhs, mirModule)

	return pcl.NewEq[F](lhs, rhs)
}

// Converts an MIR not equals to a PCL not equals formula
func (p *MirPicusTranslator[F]) converNotEqual(neq *mir.NotEqual[F], mirModule schema.Module[F]) pcl.Formula[F] {
	lhs := p.lowerTerm(neq.Lhs, mirModule)
	rhs := p.lowerTerm(neq.Rhs, mirModule)

	return pcl.NewNeq[F](lhs, rhs)
}

// Converts an array of MIR terms to an array of PCL terms
func (p *MirPicusTranslator[F]) logicalTermsToFormulas(terms []mir.LogicalTerm[F],
	mirModule schema.Module[F],
) []pcl.Formula[F] {
	nterms := make([]pcl.Formula[F], 0)

	for i := range len(terms) {
		nterms = append(nterms, p.logicalTermToFormula(terms[i], mirModule))
	}

	return nterms
}

// Converts an MIR term to a PCL expression
func (p *MirPicusTranslator[F]) lowerTerm(t mir.Term[F], module schema.Module[F]) pcl.Expr[F] {
	switch e := t.(type) {
	case *mir.Add[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Add, args)
	case *mir.Constant[F]:
		return pcl.C(e.Value)
	case *mir.RegisterAccess[F]:
		name := module.Register(e.Register()).Name()
		if strings.Contains(name, " ") {
			name = fmt.Sprintf("\"%s\"", name)
		}

		if e.RelativeShift() != 0 {
			name = fmt.Sprintf("%s_%d", name, e.RelativeShift())
		}

		return pcl.V[F](name)
	case *mir.Mul[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Mul, args)
	case *mir.Sub[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Sub, args)
	default:
		panic(fmt.Sprintf("unknown MIR expression \"%v\"", e))
	}
}

// Lower a set of zero or more MIR expressions.
func (p *MirPicusTranslator[F]) lowerTerms(exprs []mir.Term[F], mirModule schema.Module[F]) []pcl.Expr[F] {
	nexprs := make([]pcl.Expr[F], len(exprs))

	for i := range len(exprs) {
		nexprs[i] = p.lowerTerm(exprs[i], mirModule)
	}

	return nexprs
}
