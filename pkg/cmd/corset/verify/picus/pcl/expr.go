// ====================
// Arithmetic Expr AST
// ====================
package pcl

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Expr is the interface for arithmetic expressions over field elements.
type Expr[F field.Element[F]] interface {
	isExpr()
	// Lisp renders the expression as an S-expression.
	Lisp() sexp.SExp
}

// Const is a constant field element literal.
type Const[F field.Element[F]] struct {
	Val F
}

func (*Const[F]) isExpr() {}

// Lisp renders the constant as a non-negative big-endian integer symbol.
func (c *Const[F]) Lisp() sexp.SExp {
	var bi big.Int
	// Interpret field element bytes as big-endian integer for S-expression printing.
	return sexp.NewSymbol(bi.SetBytes(c.Val.Bytes()).String())
}

// Var is a named variable (independent of field type).
type Var struct {
	Name string
}

func (*Var) isExpr() {}

// Lisp renders a var symbol
func (v *Var) Lisp() sexp.SExp {
	return sexp.NewSymbol(v.Name)
}

// Neg is a unary arithmetic negation expression.
type Neg[F field.Element[F]] struct {
	Inner Expr[F]
}

func (*Neg[F]) isExpr() {}

// Lisp renders the negation as (- <expr>).
func (u *Neg[F]) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(negSymbol),
		u.Inner.Lisp(),
	})
}

// BinaryOp enumerates binary arithmetic operators.
type BinaryOp int

// Defines the different binary ops +, -, *
const (
	Add BinaryOp = iota
	Sub
	Mul
)

// String returns the S-expression symbol for the binary operator.
func (op BinaryOp) String() string {
	switch op {
	case Add:
		return "+"
	case Sub:
		return "-"
	case Mul:
		return "*"
	default:
		panic(fmt.Sprintf("Unknown op %d", op))
	}
}

// Binary represents binary expressions i.e, Add, Sub, Mul.
type Binary[F field.Element[F]] struct {
	Op BinaryOp
	X  Expr[F]
	Y  Expr[F]
}

func (*Binary[F]) isExpr() {}

// Lisp produces the sexp (op x y)
func (b *Binary[F]) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(b.Op.String()),
		b.X.Lisp(),
		b.Y.Lisp(),
	})
}

// C constructs a Picus Constant
func C[F field.Element[F]](v F) Expr[F] { return &Const[F]{Val: v} }

// V constructs a Picus Variable
func V[F field.Element[F]](name string) Expr[F] {
	return &Var{Name: name}
}

// AddE adds two expressions
func AddE[F field.Element[F]](x, y Expr[F]) Expr[F] { return &Binary[F]{Op: Add, X: x, Y: y} }

// SubE subtracts y from x
func SubE[F field.Element[F]](x, y Expr[F]) Expr[F] { return &Binary[F]{Op: Sub, X: x, Y: y} }

// MulE multiplies two expressions
func MulE[F field.Element[F]](x, y Expr[F]) Expr[F] { return &Binary[F]{Op: Mul, X: x, Y: y} }

// NegE negates an expression
func NegE[F field.Element[F]](x Expr[F]) Expr[F] { return &Neg[F]{Inner: x} }

// FoldBinaryE left-folds xs with the given op:
//
//	Add: (((x0 + x1) + x2) + ...)
//	Mul: (((x0 * x1) * x2) * ...)
//	Sub: (((x0 - x1) - x2) - ...)
func FoldBinaryE[F field.Element[F]](op BinaryOp, xs []Expr[F]) Expr[F] {
	if len(xs) < 2 {
		panic(fmt.Sprintf("FoldBinaryE: expects at least two elements. Found %d", len(xs)))
	}

	acc := xs[0]
	for i := 1; i < len(xs); i++ {
		acc = &Binary[F]{Op: op, X: acc, Y: xs[i]}
	}

	return acc
}

// FoldBinaryManyE is a variadic convenience.
func FoldBinaryManyE[F field.Element[F]](op BinaryOp, xs ...Expr[F]) Expr[F] {
	return FoldBinaryE(op, xs)
}

// Zero gets the Picus constant for 0.
func Zero[F field.Element[F]]() *Const[F] {
	var val F

	return &Const[F]{
		Val: val.SetUint64(0),
	}
}
