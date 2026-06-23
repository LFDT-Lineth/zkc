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
package macro

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
)

// Expr represents an arbitrary expression used within an instruction.
type Expr = expr.Expr

// AtomicExpr represents either a register access or a constant.
type AtomicExpr = expr.AtomicExpr

// Sum constructs an expression representing the sum of one or more values.
func Sum(exprs ...Expr) Expr {
	if len(exprs) == 0 {
		panic("one or more subexpressions required")
	}
	//
	return &expr.Add{Exprs: exprs}
}

// Constant constructs an expression representing a constant value, along with a
// base (which is used for pretty printing, etc).
func Constant(constant big.Int, base uint) AtomicExpr {
	return &expr.Const{Constant: constant, Base: base}
}

// ConstantAccess constructs an expression representing an access of a labelled
// constant value.
func ConstantAccess(constant string) AtomicExpr {
	return &expr.Const{Label: constant}
}

// RegisterAccess constructs an expression representing a register access.
func RegisterAccess(reg io.RegisterId) AtomicExpr {
	return &expr.RegAccess{Register: reg}
}

// Product constructs an expression representing the product of one or more
// values.
func Product(exprs ...Expr) Expr {
	if len(exprs) == 0 {
		panic("one or more subexpressions required")
	}
	//
	return &expr.Mul{Exprs: exprs}
}

// Subtract constructs an expression representing the subtraction of one or more
// values.
func Subtract(exprs ...Expr) Expr {
	if len(exprs) == 0 {
		panic("one or more subexpressions required")
	}
	//
	return &expr.Sub{Exprs: exprs}
}
