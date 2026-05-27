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
package codegen

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/vm"
)

// ConstantEvaluator provides machinery for evaluating compile-time constant
// expression using the provided declaration list and type environment.  It is
// used during function code generation and when initialising static memory
// contents, and also during typing (for array type size expressions).  As a
// result of the latter, it must be robust against error.  That is, it may be
// called on a malformed expression and, hence, it must handle this gracefully.
type ConstantEvaluator struct {
	field        field.Config
	env          data.ResolvedEnvironment
	declarations []Declaration
}

// NewConstantEvaluator constructs a new constant evaluator.
func NewConstantEvaluator(field field.Config, env data.ResolvedEnvironment, declarations ...Declaration,
) ConstantEvaluator {
	//
	return ConstantEvaluator{field, env, declarations}
}

// Eval attempts to evaluate a given expression to a constant.
func (p ConstantEvaluator) Eval(e Expr, definition bool) (res vm.Uint, errorMessage string) {
	var e_t = e.Type()
	// sanity check whether this is a field operation, or not.
	if !p.env.WellFormed(e_t) {
		return res, ""
	} else if ft := e.Type().AsField(p.env); ft != nil {
		// field expression
		return p.evalFieldConstant(e, definition)
	}
	// uint expression
	return p.evalIntConstant(e, definition)
}

func (p ConstantEvaluator) evalConstants(es []Expr, definition bool) ([]vm.Uint, string) {
	var (
		words        = make([]vm.Uint, len(es))
		errorMessage string
	)

	for i, e := range es {
		var errorMsg string

		words[i], errorMsg = p.Eval(e, definition)

		if errorMsg != "" {
			errorMessage = errorMsg
		}
	}
	//
	return words, errorMessage
}

func (p ConstantEvaluator) evalIntConstant(e Expr, definition bool) (res vm.Uint, err string) {
	var (
		overflow, ok bool
		bitwidth     uint
		args         []vm.Uint
	)
	// NOTE: we must sanity check the bitwidth identified is valid in order to
	// ensure this function is robust against errors.  This is necessary because
	// it is used during typing and, thus, could be called on a malformed
	// expression as a result.
	if bitwidth, ok = data.BitWidthOf(e.Type(), p.env); !ok {
		return res, "invalid constant"
	}
	//
	switch e := e.(type) {
	case *expr.Add[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		res, overflow = Sum(bitwidth, args...)

		if overflow && definition {
			err = "arithmetic overflow"
		}

		return
	case *expr.Sub[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		res, overflow = Subtract(bitwidth, args...)
		//
		if overflow && definition {
			err = "arithmetic underflow"
		}
		//
		return
	case *expr.BitwiseAnd[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		return BitwiseAnd(args...), err
	case *expr.Const[symbol.Resolved]:
		var c vm.Uint
		//
		return c.SetBigInt(e.Constant()), ""
	case *expr.Mul[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		res, overflow = Product(bitwidth, args...)

		if overflow && definition {
			err = "arithmetic overflow"
		}

		return res, err
	case *expr.Div[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		res = Quotient(args...)

		return res, err
	case *expr.Rem[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		res = Remainder(args...)

		return res, err
	case *expr.BitwiseNot[symbol.Resolved]:
		arg, err := p.Eval(e.Expr, definition)
		return arg.Not(bitwidth), err
	case *expr.BitwiseOr[symbol.Resolved]:
		args, err := p.evalConstants(e.Exprs, definition)
		return BitwiseOr(args...), err
	case *expr.Shl[symbol.Resolved]:
		args, err := p.evalConstants(e.Exprs, definition)
		return BitwiseShl(bitwidth, args...), err
	case *expr.Shr[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		return BitwiseShr(args...), err
	case *expr.Xor[symbol.Resolved]:
		args, err = p.evalConstants(e.Exprs, definition)
		return BitwiseXor(args...), err
	case *expr.Cast[symbol.Resolved]:
		inner, err := p.Eval(e.Expr, definition)
		width := e.CastType.AsUint(p.env).BitWidth()
		sliced := inner.Slice(width)

		if inner.Cmp(sliced) != 0 && definition {
			err = "cast overflow"
		}

		return sliced, err
	case *expr.ExternAccess[symbol.Resolved]:
		c, ok := p.declarations[e.Name.Index].(*decl.ResolvedConstant)
		if !ok {
			return res, "not a constant expression"
		}

		res, _ = p.Eval(c.ConstExpr, false)

		return res, ""
	default:
		return res, "not a constant expression"
	}
}

func (p ConstantEvaluator) evalFieldConstant(e Expr, definition bool) (res vm.Uint, errorMessage string) {
	var (
		fmod    = p.field.Modulus()
		modulus vm.Uint
	)
	//
	modulus = modulus.SetBigInt(fmod)
	//
	switch e := e.(type) {
	case *expr.Const[symbol.Resolved]:
		var c vm.Uint
		// sanity check for overflow
		if e.Constant().Cmp(fmod) >= 0 {
			return res, fmt.Sprintf("constant overflows field \"%s\"", p.field.Name)
		}
		//
		return c.SetBigInt(e.Constant()), ""
	case *expr.Add[symbol.Resolved]:
		var (
			val       = vm.Uint64[vm.Uint](0)
			args, err = p.evalConstants(e.Exprs, definition)
		)
		//
		for _, arg := range args {
			val = val.AddMod(arg, modulus)
		}
		//
		return val, err
	case *expr.Sub[symbol.Resolved]:
		var (
			val       vm.Uint
			args, err = p.evalConstants(e.Exprs, definition)
		)
		//
		for i, arg := range args {
			if i == 0 {
				val = arg
			} else {
				val = val.SubMod(arg, modulus)
			}
		}
		//
		return val, err
	case *expr.Mul[symbol.Resolved]:
		var (
			val       = vm.Uint64[vm.Uint](1)
			args, err = p.evalConstants(e.Exprs, definition)
		)
		//
		for _, arg := range args {
			val = val.MulMod(arg, modulus)
		}
		//
		return val, err
	case *expr.ExternAccess[symbol.Resolved]:
		c, ok := p.declarations[e.Name.Index].(*decl.ResolvedConstant)
		if !ok {
			return res, "not a constant expression"
		}

		res, _ = p.Eval(c.ConstExpr, false)

		return res, ""
	default:
		return res, "not a constant expression"
	}
}

// Sum a given set of words together.
func Sum[W vm.Word[W]](bitwidth uint, values ...W) (W, bool) {
	var (
		res      W
		overflow bool
	)
	//
	for i, v := range values {
		var carry bool
		//
		if i == 0 {
			res = v
		} else {
			res, carry = res.Add(v)
			//
			overflow = overflow || carry
		}
	}
	//
	overflow = overflow || !res.FitsWithin(bitwidth)
	//
	return res, overflow
}

// Subtract a given set of words together, producing the difference and an
// underflow indicator.
func Subtract[W vm.Word[W]](bitwidth uint, values ...W) (W, bool) {
	var (
		res       W
		underflow bool
	)
	//
	for i, v := range values {
		var borrow bool
		//
		if i == 0 {
			res = v
		} else {
			res, borrow = res.Sub(v)
			//
			underflow = underflow || borrow
		}
	}
	//
	underflow = underflow || !res.FitsWithin(bitwidth)
	//
	return res, underflow
}

// Product mulitplies a given set of words together.
func Product[W vm.Word[W]](bitwidth uint, values ...W) (W, bool) {
	var (
		res      W
		overflow bool
	)
	//
	for i, v := range values {
		var carry bool

		if i == 0 {
			res = v
		} else {
			res, carry = res.Mul(v)
			//
			overflow = overflow || carry
		}
	}
	//
	overflow = overflow || !res.FitsWithin(bitwidth)
	//
	return res, overflow
}

// Quotient divides a sequence of words left-to-right.
func Quotient[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Div(v)
		}
	}
	//
	return res
}

// Remainder computes the remainder of dividing a sequence of words left-to-right.
func Remainder[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Rem(v)
		}
	}
	//
	return res
}

// BitwiseAnd computes the bitwise AND of a set of words.
func BitwiseAnd[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.And(v)
		}
	}
	//
	return res
}

// BitwiseOr computes the bitwise OR of a set of words.
func BitwiseOr[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Or(v)
		}
	}
	//
	return res
}

// BitwiseXor computes the bitwise XOR of a set of words.
func BitwiseXor[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Xor(v)
		}
	}
	//
	return res
}

// BitwiseShl computes a left-shift chain over a set of words.
func BitwiseShl[W vm.Word[W]](bitwidth uint, values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Shl(bitwidth, v)
		}
	}
	//
	return res
}

// BitwiseShr computes a right-shift chain over a set of words.
func BitwiseShr[W vm.Word[W]](values ...W) W {
	var res W
	//
	for i, v := range values {
		if i == 0 {
			res = v
		} else {
			res = res.Shr(v)
		}
	}
	//
	return res
}
