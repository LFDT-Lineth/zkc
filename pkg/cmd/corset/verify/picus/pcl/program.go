package pcl

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

//====================
// Module / Program
//====================

// Module represents a PCL Module
type Module[F field.Element[F]] struct {
	Name        string
	Inputs      []Expr[F]
	Outputs     []Expr[F]
	Constraints []Constraint[F]
}

// NewModule constructs a PCL Module
func NewModule[F field.Element[F]](name string) *Module[F] {
	return &Module[F]{
		Name:        name,
		Inputs:      make([]Expr[F], 0),
		Outputs:     make([]Expr[F], 0),
		Constraints: make([]Constraint[F], 0),
	}
}

// IsEmpty returns true if the module is empty.
// Some modules in MIR/AIR are empty so they get translated to empty Picus modules
// i.e, modules with no constraints or defined inputs or outputs. This utility is used
// to prune those empty modules from the generated Picus program
func (m *Module[F]) IsEmpty() bool {
	return len(m.Constraints) == 0 && len(m.Inputs) == 0 && len(m.Outputs) == 0
}

// AddInput adds input to m's list of inputs.
func (m *Module[F]) AddInput(input Expr[F]) {
	m.Inputs = append(m.Inputs, input)
}

// AddOutput adds output to m's list of outputs.
func (m *Module[F]) AddOutput(output Expr[F]) {
	m.Outputs = append(m.Outputs, output)
}

// AddLtConstraint constructs an assertion (assert (< op1 op2))
func (m *Module[F]) AddLtConstraint(op1 Expr[F], op2 Expr[F]) {
	m.Constraints = append(m.Constraints, NewPicusConstraint[F](NewLt[F](op1, op2)))
}

// AddLeqConstraint constructs an assertion (assert (<= op1 op2))
func (m *Module[F]) AddLeqConstraint(op1 Expr[F], op2 Expr[F]) {
	m.Constraints = append(m.Constraints, NewPicusConstraint[F](NewLeq[F](op1, op2)))
}

// AddGeqConstraint constructs an assertion (assert (>= op1 op2))
func (m *Module[F]) AddGeqConstraint(op1 Expr[F], op2 Expr[F]) {
	m.Constraints = append(m.Constraints, NewPicusConstraint[F](NewGeq[F](op1, op2)))
}

// AddEqConstraint constructs an assertion (assert (= op1 op2))
func (m *Module[F]) AddEqConstraint(op1 Expr[F], op2 Expr[F]) {
	m.Constraints = append(m.Constraints, NewPicusConstraint[F](NewEq[F](op1, op2)))
}

// Program represents a PCL program. A PCL program is structured as follows:
// (begin-prime [prime])
// (begin-module m1)
// ..
// (end-module)
// ..
// (begin-module m2)
// ...
// (end-module)
type Program[F field.Element[F]] struct {
	Prime   *big.Int // field modulus as element
	Modules map[string]*Module[F]
}

// NewProgram constructs an empty PCL Program.
func NewProgram[F field.Element[F]](prime *big.Int) *Program[F] {
	return &Program[F]{
		Prime:   prime,
		Modules: make(map[string]*Module[F]),
	}
}

// AddModule adds module `moduleName` to `pp`.
func (pp *Program[F]) AddModule(moduleName string) *Module[F] {
	picusModule := NewModule[F](moduleName)
	pp.Modules[moduleName] = picusModule

	return picusModule
}
