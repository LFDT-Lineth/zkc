package pcl

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

//=======================
// Boolean Constraint AST
//=======================

// Symbols used when pretty-printing to S-expressions.
const (
	negSymbol     = "-"   // negSymbol is the unary negation symbol.
	notSymbol     = "!"   // notSymbol is the boolean negation symbol.
	iffSymbol     = "<=>" // iffSymbol is the boolean equivalence symbol.
	impliesSymbol = "=>"  // impliesSymbol is the boolean implication symbol.
	andSymbol     = "&&"  // andSymbol is the boolean conjunction symbol.
	orSymbol      = "||"  // orSymbol is the boolean disjunction symbol.
)

// RelOp enumerates relational predicates in PCL
type RelOp int

const (
	opEq RelOp = iota // =
	opNe              // !=
	opLt              // <
	opLe              // <=
	opGt              // >
	opGe              // >=
)

// String returns the s-expression symbol for `RelOp`
func (op RelOp) String() string {
	switch op {
	case opEq:
		return "="
	case opNe:
		return "!="
	case opLt:
		return "<"
	case opLe:
		return "<="
	case opGt:
		return ">"
	case opGe:
		return ">="
	default:
		panic(fmt.Sprintf("Unknown relational op: %d", op))
	}
}

// Formula is an interface to mark Picus formulas. In PCL formulas appear in four places
// 1. (Assertions) - Circuit constraints (vanishing constraints here).
// 2. (Postconditions) - Formulas that Picus must prove are entailed by the module constraints
// 3. (Assumptions) - Formulas Picus assumes hold on the parameters of the module
// 3. (Conditional Guards)
type Formula[F field.Element[F]] interface {
	isFormula()
	Lisp() sexp.SExp
}

// Pred is an atomic predicate of the form (Left OP Right).
type Pred[F field.Element[F]] struct {
	Op    RelOp
	Left  Expr[F]
	Right Expr[F]
}

// Lisp renders the predicate as an S-expression: (<op> <left> <right>).
func (p *Pred[F]) Lisp() sexp.SExp {
	return sexp.NewList(
		[]sexp.SExp{
			sexp.NewSymbol(p.Op.String()),
			p.Left.Lisp(),
			p.Right.Lisp(),
		})
}

func (*Pred[F]) isFormula() {}

// NewPred constructs an atomic predicate (left op right).
func NewPred[F field.Element[F]](op RelOp, left Expr[F], right Expr[F]) *Pred[F] {
	return &Pred[F]{
		Op:    op,
		Left:  left,
		Right: right,
	}
}

// NewLt builds (< left right)
func NewLt[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opLt, left, right)
}

// NewGt builds (> left right)
func NewGt[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opGt, left, right)
}

// NewLeq builds (<= left right)
func NewLeq[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opLe, left, right)
}

// NewGeq builds (>= left right)
func NewGeq[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opGe, left, right)
}

// NewEq builds (= left right)
func NewEq[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opEq, left, right)
}

// NewNeq builds (!= left right)
func NewNeq[F field.Element[F]](left Expr[F], right Expr[F]) *Pred[F] {
	return NewPred[F](opNe, left, right)
}

// FoldBinopPred constructs a left-associative Boolean formula by folding xs with op,
// i.e., (((x0 op x1) op x2) ...). It panics if fewer than two formulas are provided.
func FoldBinopPred[F field.Element[F]](op BinopConnective, xs []Formula[F]) Formula[F] {
	if len(xs) < 2 {
		panic(fmt.Sprintf("FoldBinopPred: expects at least two elements. Found %d", len(xs)))
	}

	acc := xs[0]
	for i := 1; i < len(xs); i++ {
		acc = &BinopConnectivePred[F]{Op: op, Left: acc, Right: xs[i]}
	}

	return acc
}

// FoldAnd builds an n-ary conjunction of formulas
func FoldAnd[F field.Element[F]](xs []Formula[F]) Formula[F] {
	return FoldBinopPred(opAnd, xs)
}

// FoldOr builds  an n-ary disjunction of formulas
func FoldOr[F field.Element[F]](xs []Formula[F]) Formula[F] {
	return FoldBinopPred(opOr, xs)
}

// BinopConnective enumerates Boolean binary connectives in PCL.
type BinopConnective int

const (
	opAnd     BinopConnective = iota // &&
	opOr                             // ||
	opIff                            // <=>
	opImplies                        // =>
)

// BinopConnectivePred is a composite Boolean formula combining two sub-formulas
// with a binary connective.
type BinopConnectivePred[F field.Element[F]] struct {
	Op    BinopConnective
	Left  Formula[F]
	Right Formula[F]
}

func (*BinopConnectivePred[F]) isFormula() {}

// String returns the S-expression symbol for the binary connective.
func (op BinopConnective) String() string {
	switch op {
	case opAnd:
		return andSymbol
	case opOr:
		return orSymbol
	case opIff:
		return iffSymbol
	case opImplies:
		return impliesSymbol
	default:
		panic(fmt.Sprintf("Unknown binop connective: %d", op))
	}
}

// Lisp renders the binary connective formula as an S-expression:
// (<op> <left> <right>).
func (b *BinopConnectivePred[F]) Lisp() sexp.SExp {
	return sexp.NewList(
		[]sexp.SExp{
			sexp.NewSymbol(b.Op.String()),
			b.Left.Lisp(),
			b.Right.Lisp(),
		})
}

// Not represents logical negation of a sub-formula.
type Not[F field.Element[F]] struct{ Inner Formula[F] }

func (*Not[F]) isFormula() {}

// Lisp renders the negation as an S-expression: (! <formula>).
func (n *Not[F]) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{sexp.NewSymbol(notSymbol), n.Inner.Lisp()})
}

// NewNot constructs the negation of the given formula.
func NewNot[F field.Element[F]](formula Formula[F]) *Not[F] {
	return &Not[F]{
		Inner: formula,
	}
}
