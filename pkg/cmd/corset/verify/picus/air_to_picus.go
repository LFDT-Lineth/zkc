package picus

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/verify/picus/pcl"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// AirPicusTranslator captures any state needed to lower an AIR schema
// to a program in PCL (Picus Constraint Language). Core structure is borrowed from `mir_to_picus.go`.
type AirPicusTranslator[F field.Element[F]] struct {
	// Modules we are lowering from
	airSchema air.Schema[F]
	// Picus program we are building
	picusProgram *pcl.Program[F]
}

// NewAirPicusTranslator constructs an Air->Picus translator.
func NewAirPicusTranslator[F field.Element[F]](airSchema air.Schema[F]) *AirPicusTranslator[F] {
	return &AirPicusTranslator[F]{
		airSchema:    airSchema,
		picusProgram: pcl.NewProgram[F](ModulusOf[F]()),
	}
}

// Translate the provided MIR Schema to a Picus Program.
func (p *AirPicusTranslator[F]) Translate() *pcl.Program[F] {
	// Perform a 1-1 translation between MIR modules and Picus modules
	var i uint
	for i = 0; i < p.airSchema.Width(); i++ {
		p.TranslateModule(i)
	}

	return p.picusProgram
}

// TranslateModule compiles an AIR Module to a Picus Module.
// `i` denotes the index of the AIR module in the schema.
func (p *AirPicusTranslator[F]) TranslateModule(i uint) {
	// get the MIR module
	airModule := p.airSchema.Module(i)
	if airModule.IsSynthetic() {
		return
	}
	// initialize the corresponding PCL module
	picusModule := p.picusProgram.AddModule(airModule.Name().String())
	// register inputs and outputs from MIR inputs/outputs
	for _, register := range airModule.Registers() {
		picusVar := pcl.V[F](register.Name())
		if register.IsInput() {
			picusModule.AddInput(picusVar)
		} else if register.IsOutput() {
			picusModule.AddOutput(picusVar)
		}
	}

	// build PCL constraints from MIR constraints
	for iter := airModule.Constraints(); iter.HasNext(); {
		constraint := iter.Next().(air.Constraint[F])
		p.translateConstraint(constraint, picusModule, airModule)
	}
}

// translateConstraints translates MIR constraints into PCL constraints.
// The built constraints are implicitly added to `picusModule`
func (p *AirPicusTranslator[F]) translateConstraint(c air.Constraint[F],
	picusModule *pcl.Module[F], airModule schema.Module[F],
) {
	// Check what kind of constraint we have
	switch v := c.(type) {
	case air.VanishingConstraint[F]:
		p.translateVanishing(v, picusModule, airModule)
	case air.LookupConstraint[F]:
		p.translateLookup(v, picusModule)
	case air.RangeConstraint[F]:
		p.translateRangeConstraint(v, picusModule, airModule)
	default:
		panic(fmt.Sprintf("Unhandled constraint: %s", v.Name()))
	}
}

// translateRangeConstraint translates a MIR range constraint `r` to a PCL less than constraint.
func (p *AirPicusTranslator[F]) translateRangeConstraint(r air.RangeConstraint[F],
	picusModule *pcl.Module[F], airModule schema.Module[F],
) {
	var (
		exprs     = r.Unwrap().Sources
		bitwidths = r.Unwrap().Bitwidths
	)

	for i, e := range exprs {
		expr := p.lowerTerm(e, airModule)
		// 1. Get the `big.Int` representation of the max unisgned value for a given bitwidth.
		// 2. Create a field element from the big integer.
		// 3. Construct a PCL constant from the field element.
		upperBound := pcl.C(field.BigInt[F](*MaxValueBig(int(bitwidths[i]))))
		// Add (assert (<= `expr` `upperBound`))
		picusModule.AddLeqConstraint(expr, upperBound)
	}
}

// translateVanishing translates an AIR vanishing constraint into a PCL constraint.
func (p *AirPicusTranslator[F]) translateVanishing(v air.VanishingConstraint[F],
	picusModule *pcl.Module[F], airModule schema.Module[F],
) {
	if v.Unwrap().Domain.HasValue() {
		// TODO: need to handle this. Row specific constraints will require
		// generating Picus modules which collec all constraints that apply
		// to the specific row.
		panic("row specific constraints are not supported!")
	}
	// We translate the logical term to a collection of Picus constraints
	picusModule.Constraints = append(picusModule.Constraints, p.logicalTermToConstraint(v.Unwrap().Constraint, airModule))
}

// translateLookup translates an AIR lookup constraint into a PCL constraint.
func (p *AirPicusTranslator[F]) translateLookup(v air.LookupConstraint[F], picusModule *pcl.Module[F]) {
	if len(v.Unwrap().Targets) == 1 {
		target := v.Unwrap().Targets[0]

		targetModule := p.airSchema.Module(target.Module)
		if targetModule.Name().Name != "u128" {
			panic(fmt.Sprintf("Unhandled lookup target: %s", targetModule.Name()))
		}

		source := v.Unwrap().Sources[0]
		sourceModule := p.airSchema.Module(source.Module)
		sourceTerm := p.lowerTerm(source.Ith(0), sourceModule)
		upperBound := pcl.C(field.BigInt[F](*MaxValueBig(128)))
		picusModule.AddLeqConstraint(sourceTerm, upperBound)
	}
}

// logicalTermToConstraint translates an AIR logical term to a PCL constraints.
// Unlike the MIR translation this should be a one-to-one translation.
func (p *AirPicusTranslator[F]) logicalTermToConstraint(t air.LogicalTerm[F],
	airModule schema.Module[F],
) pcl.Constraint[F] {
	expr := p.lowerTerm(t.Term, airModule)
	return pcl.NewPicusConstraint[F](pcl.NewEq[F](expr, pcl.Zero[F]()))
}

// lowerTerm converts an AIR term to a PCL expression
func (p *AirPicusTranslator[F]) lowerTerm(t air.Term[F], module schema.Module[F]) pcl.Expr[F] {
	switch e := t.(type) {
	case *air.Add[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Add, args)
	case *air.Constant[F]:
		return pcl.C(e.Value)
	case *air.ColumnAccess[F]:
		name := module.Register(e.Register()).Name()
		if strings.Contains(name, " ") {
			name = fmt.Sprintf("\"%s\"", name)
		}

		if e.RelativeShift() != 0 {
			name = fmt.Sprintf("%s_%d", name, e.RelativeShift())
		}

		return pcl.V[F](name)
	case *air.Mul[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Mul, args)
	case *air.Sub[F]:
		args := p.lowerTerms(e.Args, module)
		return pcl.FoldBinaryE(pcl.Sub, args)
	default:
		panic(fmt.Sprintf("unknown AIR expression \"%v\"", e))
	}
}

// lowerTerms lowers a set of zero or more AIR expressions.
func (p *AirPicusTranslator[F]) lowerTerms(exprs []air.Term[F], airModule schema.Module[F]) []pcl.Expr[F] {
	nexprs := make([]pcl.Expr[F], len(exprs))

	for i := range len(exprs) {
		nexprs[i] = p.lowerTerm(exprs[i], airModule)
	}

	return nexprs
}
