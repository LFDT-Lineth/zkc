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
package micro

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// Expr represents an expression at the micro level, which is either a register
// access or a constant.
type Expr struct {
	util.Union[io.RegisterId, big.Int]
}

// NewRegister constructs an expression representing a register access
func NewRegister(r io.RegisterId) Expr {
	return Expr{util.Union1[io.RegisterId, big.Int](r)}
}

// NewConstant constructs an expression representing a constant.
func NewConstant(c big.Int) Expr {
	return Expr{util.Union2[io.RegisterId](c)}
}

// NewConstant64 constructs an expression representing a constant.
func NewConstant64(c uint64) Expr {
	var val big.Int
	val.SetUint64(c)
	//
	return Expr{util.Union2[io.RegisterId](val)}
}

// ToVec converts this expression into a vectorizable expression.
func (e Expr) ToVec() VecExpr {
	if e.HasFirst() {
		return NewVecExpr(register.NewVector(e.First()))
	}
	//
	return ConstVecExpr(e.Second())
}

// Bitwidth returns the minimum number of bits required to store any evaluation
// of this expression.
func (e Expr) Bitwidth(fn register.Map) uint {
	if e.HasFirst() {
		return fn.Register(e.First()).Width()
	}
	//
	val := e.Second()
	//
	return uint(val.BitLen())
}

// Eval evaluates a set of zero or more expressions producing a set of zero or
// more values.
func (e Expr) Eval(state io.State) *big.Int {
	if e.HasFirst() {
		return state.Load(e.First())
	}
	//
	val := e.Second()
	//
	return &val
}

func (e Expr) String(fn register.Map) string {
	if e.HasFirst() {
		return fn.Register(e.First()).Name()
	}
	//
	val := e.Second()
	//
	return val.String()
}

// Clone this expression
func (e Expr) Clone() Expr {
	var (
		val1 big.Int
		val2 big.Int
	)
	//
	if e.HasFirst() {
		return e
	}
	// Clone big int
	val1 = e.Second()
	val2.Set(&val1)
	//
	return Expr{util.Union2[io.RegisterId](val2)}
}
