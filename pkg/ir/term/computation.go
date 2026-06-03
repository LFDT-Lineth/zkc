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
package term

import (
	"encoding/gob"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Computation represents an "unbound" term.  That is, it captures any possible
// term (i.e. rather than a fixed set as for MIR or AIR, etc).
type Computation[F any] interface {
	Expr[F, Computation[F]]
}

// LogicalComputation represents an "unbound" term.  That is, it captures any
// possible term (i.e. rather than a fixed set as for MIR or AIR, etc).
type LogicalComputation[F any] interface {
	Logical[F, LogicalComputation[F]]
}

// ============================================================================
// Computation
// ============================================================================

// NewComputation takes an arbitrary term and converts in into an instance of
// computation by wrapping it.
func NewComputation[F field.Element[F], S Logical[F, S], T Expr[F, T]](term Expr[F, T]) Computation[F] {
	switch t := term.(type) {
	case *Add[F, T]:
		args := NewComputations[F, S](t.Args)
		return Sum(args...)
	case *Cast[F, T]:
		arg := NewComputation[F, S](t.Arg)
		return CastOf(arg, t.BitWidth)
	case *Constant[F, T]:
		return Const[F, Computation[F]](t.Value)
	case *Exp[F, T]:
		arg := NewComputation[F, S](t.Arg)
		return Exponent(arg, t.Pow)
	case *IfZero[F, S, T]:
		condition := NewLogicalComputation[F, S, T](t.Condition)
		trueBranch := NewComputation[F, S](t.TrueBranch)
		falseBranch := NewComputation[F, S](t.FalseBranch)
		// Done
		return IfElse(condition, trueBranch, falseBranch)
	case *LabelledConst[F, T]:
		return LabelledConstant[F, Computation[F]](t.Label, t.Value)
	case *Mul[F, T]:
		args := NewComputations[F, S](t.Args)
		return Product(args...)
	case *Norm[F, T]:
		arg := NewComputation[F, S, T](t.Arg)
		return Normalise(arg)
	case *RegisterAccess[F, T]:
		return RawRegisterAccess[F, Computation[F]](t.Register(), t.BitWidth(), t.RelativeShift()).Mask(t.MaskWidth())
	case *Sub[F, T]:
		args := NewComputations[F, S](t.Args)
		return Subtract(args...)
	case *VectorAccess[F, T]:
		var nterms = make([]*RegisterAccess[F, Computation[F]], len(t.Vars))
		//
		for i, v := range t.Vars {
			nterms[i] = RawRegisterAccess[F, Computation[F]](v.Register(), v.BitWidth(), v.RelativeShift()).Mask(v.MaskWidth())
		}
		//
		return NewVectorAccess(nterms)
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", term.Lisp(false, nil).String(false)))
	}
}

// NewComputations constructs an array of zero or more computations.
func NewComputations[F field.Element[F], S Logical[F, S], T Expr[F, T]](terms []T) []Computation[F] {
	var computations = make([]Computation[F], len(terms))
	//
	for i, t := range terms {
		computations[i] = NewComputation[F, S](t)
	}
	//
	return computations
}

// ============================================================================
// Logical Comptuation
// ============================================================================

// NewLogicalComputation takes an arbitrary logical term and converts in into an
// instance of computation by wrapping it.
func NewLogicalComputation[F field.Element[F], S Logical[F, S], T Expr[F, T]](term Logical[F, S],
) LogicalComputation[F] {
	switch t := term.(type) {
	case *Conjunct[F, S]:
		args := NewLogicalComputations[F, S, T](t.Args)
		return Conjunction(args...)
	case *Disjunct[F, S]:
		args := NewLogicalComputations[F, S, T](t.Args)
		return Disjunction(args...)
	case *Equal[F, S, T]:
		lhs := NewComputation[F, S](t.Lhs)
		rhs := NewComputation[F, S](t.Rhs)

		return Equals[F, LogicalComputation[F]](lhs, rhs)
	case *Ite[F, S]:
		var trueBranch, falseBranch LogicalComputation[F]

		condition := NewLogicalComputation[F, S, T](t.Condition)
		//
		if t.TrueBranch != nil {
			trueBranch = NewLogicalComputation[F, S, T](t.TrueBranch)
		}
		//
		if t.FalseBranch != nil {
			falseBranch = NewLogicalComputation[F, S, T](t.FalseBranch)
		}
		//
		return IfThenElse(condition, trueBranch, falseBranch)
	case *Negate[F, S]:
		arg := NewLogicalComputation[F, S, T](t.Arg)
		return Negation(arg)
	case *NotEqual[F, S, T]:
		lhs := NewComputation[F, S](t.Lhs)
		rhs := NewComputation[F, S](t.Rhs)

		return NotEquals[F, LogicalComputation[F]](lhs, rhs)
	default:
		panic(fmt.Sprintf("unknown computation encountered: %s", term.Lisp(false, nil).String(false)))
	}
}

// NewLogicalComputations constructs an array of zero or more computations.
func NewLogicalComputations[F field.Element[F], S Logical[F, S], T Expr[F, T]](terms []S) []LogicalComputation[F] {
	var computations = make([]LogicalComputation[F], len(terms))
	//
	for i, t := range terms {
		computations[i] = NewLogicalComputation[F, S, T](t)
	}
	//
	return computations
}

// ComputationTerm provides a convenient alias for a big endian term.
type ComputationTerm = Expr[word.BigEndian, Computation[word.BigEndian]]

// LogicalComputationTerm provides a convenient alias for a big endian logical term.
type LogicalComputationTerm = Logical[word.BigEndian, LogicalComputation[word.BigEndian]]

func init() {
	gob.Register(ComputationTerm(&Add[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Sub[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Mul[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Cast[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Exp[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(
		&IfZero[word.BigEndian, LogicalComputation[word.BigEndian], Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Constant[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&LabelledConst[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&Norm[word.BigEndian, Computation[word.BigEndian]]{}))
	gob.Register(ComputationTerm(&RegisterAccess[word.BigEndian, Computation[word.BigEndian]]{}))

	gob.Register(LogicalComputationTerm(&Conjunct[word.BigEndian, LogicalComputation[word.BigEndian]]{}))
	gob.Register(LogicalComputationTerm(&Disjunct[word.BigEndian, LogicalComputation[word.BigEndian]]{}))
	gob.Register(LogicalComputationTerm(
		&Equal[word.BigEndian, LogicalComputation[word.BigEndian], Computation[word.BigEndian]]{}))
	gob.Register(LogicalComputationTerm(
		&NotEqual[word.BigEndian, LogicalComputation[word.BigEndian], Computation[word.BigEndian]]{}))
}
