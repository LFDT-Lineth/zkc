package pcl

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Constraint is an interface to mark Picus constraints
type Constraint[F field.Element[F]] interface {
	isConstraint()
	Lisp() sexp.SExp // PCL is similar to smtlib which is LISP-like
}

// Assert tepresents a Picus `assert` statement which asserts
// boolean combinations polynomial (in)-equalities
type Assert[F field.Element[F]] struct {
	Formula Formula[F]
}

// isConstraint implements the `Constraint` interface
func (*Assert[F]) isConstraint() {}

// Lisp creates a sexps such as `(assert (= x 1))`
func (v *Assert[F]) Lisp() sexp.SExp {
	return sexp.NewList(
		[]sexp.SExp{
			sexp.NewSymbol("assert"),
			v.Formula.Lisp(),
		})
}

// NewPicusConstraint constructs an assertion
func NewPicusConstraint[F field.Element[F]](formula Formula[F]) *Assert[F] {
	return &Assert[F]{
		Formula: formula,
	}
}

// ConstraintBlock is an array of constraints
type ConstraintBlock[F field.Element[F]] []Constraint[F]

// Lisp creates sexps such as `(e1 e2 e3)`
func (cb *ConstraintBlock[F]) Lisp() sexp.SExp {
	elements := make([]sexp.SExp, len(*cb))
	for i, constraint := range *cb {
		elements[i] = constraint.Lisp()
	}

	return sexp.NewList(elements)
}

// IfElse defines a PCL if-then-else statement
type IfElse[F field.Element[F]] struct {
	cond        Formula[F]
	trueBranch  ConstraintBlock[F]
	falseBranch ConstraintBlock[F]
}

// Implements constraint interface
func (*IfElse[F]) isConstraint() {}

// Lisp constructs an sexp such as (if (..) (..))
func (ite *IfElse[F]) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("if"),
		ite.cond.Lisp(),
		ite.trueBranch.Lisp(),
		ite.falseBranch.Lisp(),
	})
}

// NewIfElse constructs an if-then-else constraint
func NewIfElse[F field.Element[F]](cond Formula[F],
	thenConstraints []Constraint[F], elseConstraints []Constraint[F],
) *IfElse[F] {
	return &IfElse[F]{
		cond:        cond,
		trueBranch:  ConstraintBlock[F](thenConstraints),
		falseBranch: ConstraintBlock[F](elseConstraints),
	}
}
