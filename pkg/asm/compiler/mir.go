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
package compiler

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/consensys/go-corset/pkg/asm/program"
	"github.com/consensys/go-corset/pkg/ir"
	"github.com/consensys/go-corset/pkg/ir/mir"
	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/constraint/lookup"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
)

// ModuleBuilder is used within this translator for building the various modules
// which are contained within the mixed MIR schema.
type ModuleBuilder[F field.Element[F]] = ir.ModuleBuilder[F, mir.Constraint[F], mir.Term[F]]

// MirModule provides a wrapper around a corset-level module declaration.
type MirModule[F field.Element[F]] struct {
	Module ModuleBuilder[F]
}

// Initialise this module
func (p MirModule[F]) Initialise(mid uint, fn MicroComponent) MirModule[F] {
	builder := ir.NewModuleBuilder[F, mir.Constraint[F], mir.Term[F]](
		fn.Name(), mid, false, fn.IsPublic(), false, false, false, fn.NumInputs())
	//
	switch fn := fn.(type) {
	case *MicroFunction:
		// Add corresponding assignment for this function.
		builder.AddAssignment(program.NewAssignment[F](mid, fn))
	default:
		panic("unknown component encountered")
	}
	//
	p.Module = builder
	//
	return p
}

// NewAssignment adds a new assignment to this module.
func (p MirModule[F]) NewAssignment(assignment schema.Assignment[F]) {
	p.Module.AddAssignment(assignment)
}

// NewColumn constructs a new column of the given name and bitwidth within
// this module.
func (p MirModule[F]) NewColumn(kind register.Type, name string, bitwidth uint, padding big.Int,
) register.Id {
	//
	var (
		// Add new register
		rid = p.Module.NewRegister(register.New(kind, name, bitwidth, padding))
	)
	//
	terms := []*term.RegisterAccess[F, mir.Term[F]]{term.RawRegisterAccess[F, mir.Term[F]](rid, bitwidth, 0)}
	// Add corresponding range constraint to enforce bitwidth
	switch kind {
	case register.INPUT_REGISTER, register.OUTPUT_REGISTER, register.COMPUTED_REGISTER:
		p.Module.AddConstraint(
			mir.NewRangeConstraint(name, p.Module.Id(), terms, []uint{bitwidth}))
	case register.ONE_REGISTER:
		p.addConstantConstraint(1, rid, bitwidth)
	case register.ZERO_REGISTER:
		p.addConstantConstraint(0, rid, bitwidth)
	default:
		panic("unknown register kind encountered")
	}
	// Done
	return rid
}

// NewUnusedColumn constructs an empty (i.e. unused) column identifier.
func (p MirModule[F]) NewUnusedColumn() register.Id {
	return register.UnusedId()
}

// NewConstraint constructs a new vanishing constraint with the given name
// within this module.
func (p MirModule[F]) NewConstraint(name string, domain util.Option[int], constraint MirExpr[F]) {
	e := constraint.logical.Simplify(false)
	//
	p.Module.AddConstraint(
		mir.NewVanishingConstraint(name, p.Module.Id(), domain, e))
}

// NewLookup constructs a new lookup constraint
func (p MirModule[F]) NewLookup(name string, from []register.Id, target MirModule[F], to []register.Id,
	enable util.Option[register.Id]) {
	//
	var (
		sources       = wrapMirRegisterAccesses[F](p.Module, from...)
		targets       = wrapMirRegisterAccesses[F](target.Module, to...)
		targetVectors []lookup.Vector[F, *mir.RegisterAccess[F]]
		sourceVectors []lookup.Vector[F, *mir.RegisterAccess[F]]
	)
	//
	if enable.IsEmpty() {
		targetVectors = append(targetVectors, lookup.UnfilteredVector(target.Module.Id(), targets...))
	} else {
		var (
			enReg = target.Module.Register(enable.Unwrap())
			en    = term.RawRegisterAccess[F, mir.Term[F]](enable.Unwrap(), enReg.Width(), 0)
		)
		//
		targetVectors = append(targetVectors, lookup.FilteredVector(target.Module.Id(), en, targets...))
	}
	//
	sourceVectors = append(sourceVectors, lookup.UnfilteredVector(p.Module.Id(), sources...))
	//
	p.Module.AddConstraint(mir.NewLookupConstraint(name, targetVectors, sourceVectors))
}

// String returns an appropriately formatted representation of the module.
func (p MirModule[F]) String() string {
	var builder strings.Builder
	//
	for _, r := range p.Module.Registers() {
		fmt.Fprintf(&builder, "var %s\n", r.String())
	}
	//
	return builder.String()
}

func (p MirModule[F]) addConstantConstraint(value uint64, rid register.Id, bitwidth uint) {
	name := fmt.Sprintf("%d", value)
	//
	p.Module.AddConstraint(mir.NewVanishingConstraint(name, p.Module.Id(), util.None[int](),
		term.Equals[F, mir.LogicalTerm[F], mir.Term[F]](
			term.NewRegisterAccess[F, mir.Term[F]](rid, bitwidth, 0),
			term.Const64[F, mir.Term[F]](value))))
}

// MirExpr is a wrapper around a corset expression which provides the
// necessary interface.
type MirExpr[F field.Element[F]] struct {
	expr    mir.Term[F]
	logical mir.LogicalTerm[F]
}

// AsLogical extracts a logical constraint from this expression.
func (p MirExpr[F]) AsLogical() mir.LogicalTerm[F] {
	return p.logical.Simplify(false)
}

// Add constructs a sum between this expression and zero or more
func (p MirExpr[F]) Add(exprs ...MirExpr[F]) MirExpr[F] {
	args := unwrapSplitMirExpr(p, exprs...)
	return MirExpr[F]{term.Sum(args...), nil}
}

// And constructs a conjunction between this expression and zero or more
// expressions.
func (p MirExpr[F]) And(exprs ...MirExpr[F]) MirExpr[F] {
	args := unwrapSplitMirLogicals(p, exprs...)
	return MirExpr[F]{nil, term.Conjunction(args...)}
}

// Equals constructs an equality between two expressions.
func (p MirExpr[F]) Equals(rhs MirExpr[F]) MirExpr[F] {
	if p.expr == nil {
		panic("invalid left argument")
	} else if rhs.expr == nil {
		panic("invalid right argument")
	}
	//
	logical := term.Equals[F, mir.LogicalTerm[F]](p.expr, rhs.expr)
	//
	return MirExpr[F]{nil, logical}
}

// Then constructs an implication between two expressions.
func (p MirExpr[F]) Then(trueBranch MirExpr[F]) MirExpr[F] {
	logical := term.IfThenElse(p.logical, trueBranch.logical, nil)
	return MirExpr[F]{nil, logical}
}

// ThenElse constructs an if-then-else expression with this expression
// acting as the condition.
func (p MirExpr[F]) ThenElse(trueBranch MirExpr[F], falseBranch MirExpr[F]) MirExpr[F] {
	logical := term.IfThenElse(p.logical, trueBranch.logical, falseBranch.logical)
	return MirExpr[F]{nil, logical}
}

// Multiply constructs a product between this expression and zero or more
// expressions.
func (p MirExpr[F]) Multiply(exprs ...MirExpr[F]) MirExpr[F] {
	args := unwrapSplitMirExpr(p, exprs...)
	return MirExpr[F]{term.Product(args...), nil}
}

// NotEquals constructs a non-equality between two expressions.
func (p MirExpr[F]) NotEquals(rhs MirExpr[F]) MirExpr[F] {
	logical := term.NotEquals[F, mir.LogicalTerm[F]](p.expr, rhs.expr)
	return MirExpr[F]{nil, logical}
}

// Bool constructs a truth or falsehood
func (p MirExpr[F]) Bool(val bool) MirExpr[F] {
	if val {
		// empty conjunction is true
		return MirExpr[F]{nil, term.Conjunction[F, mir.LogicalTerm[F]]()}
	}
	// empty disjunction is false
	return MirExpr[F]{nil, term.Disjunction[F, mir.LogicalTerm[F]]()}
}

// BigInt constructs a constant expression from a big integer.
func (p MirExpr[F]) BigInt(number big.Int) MirExpr[F] {
	// Not power of 2
	var (
		num F
		n   big.Int
	)
	//
	if number.Sign() < 0 {
		n.Add(&number, num.Modulus())
	} else {
		n = number
	}
	//
	num = num.SetBytes(n.Bytes())
	//
	return MirExpr[F]{term.Const[F, mir.Term[F]](num), nil}
}

// Or constructs a disjunction between this expression and zero or more
// expressions.
func (p MirExpr[F]) Or(exprs ...MirExpr[F]) MirExpr[F] {
	args := unwrapSplitMirLogicals(p, exprs...)
	return MirExpr[F]{nil, term.Disjunction(args...)}
}

// Variable constructs a variable with a given shift.
func (p MirExpr[F]) Variable(index register.Id, bitwidth uint, shift int) MirExpr[F] {
	return MirExpr[F]{term.NewRegisterAccess[F, mir.Term[F]](index, bitwidth, shift), nil}
}

func (p MirExpr[F]) String(func(register.Id) string) string {
	if p.expr != nil {
		return p.expr.Lisp(false, nil).String(false)
	} else if p.logical != nil {
		return p.logical.Lisp(false, nil).String(false)
	} else {
		return "nil"
	}
}

func unwrapSplitMirExpr[F field.Element[F]](head MirExpr[F], tail ...MirExpr[F]) []mir.Term[F] {
	cexprs := make([]mir.Term[F], len(tail)+1)
	//
	cexprs[0] = head.expr
	//
	for i, e := range tail {
		cexprs[i+1] = e.expr
		//
		if e.logical != nil {
			panic("logical expression encountered")
		}
	}
	//
	return cexprs
}

func unwrapSplitMirLogicals[F field.Element[F]](head MirExpr[F], tail ...MirExpr[F]) []mir.LogicalTerm[F] {
	cexprs := make([]mir.LogicalTerm[F], len(tail)+1)
	//
	cexprs[0] = head.logical
	//
	for i, e := range tail {
		cexprs[i+1] = e.logical
		//
		if e.expr != nil {
			panic("arithmetic expression encountered")
		}
	}
	//
	return cexprs
}

func wrapMirRegisterAccesses[F field.Element[F]](mapping register.Map, regs ...register.Id) []*mir.RegisterAccess[F] {
	var vars = make([]*mir.RegisterAccess[F], len(regs))
	//
	for i, rid := range regs {
		var reg = mapping.Register(rid)
		//
		vars[i] = term.RawRegisterAccess[F, mir.Term[F]](rid, reg.Width(), 0)
	}
	//
	return vars
}
