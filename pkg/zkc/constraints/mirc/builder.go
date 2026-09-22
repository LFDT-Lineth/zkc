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
package mirc

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// RegisterReader is a simplified view of a translator which is suitable for
// reading registers only.
type RegisterReader[F field.Element[F]] interface {
	// Register returns information about a given register
	Register(register.Id) register.Register
	// RegisterWidths returns the bitwidth of a given set of registers.
	RegisterWidths(reg ...register.Id) []uint
	// ReadRegister constructs a suitable accessor for referring to a given register.
	// This applies forwarding as appropriate.
	ReadRegister(reg register.Id, forwarding bool) Expr[F]
}

// BigNumber constructs a constant expression from a big integer.
func BigNumber[F field.Element[F]](c *big.Int) Expr[F] {
	var (
		empty Expr[F]
		val   big.Int
	)
	// Clone big integer
	val.Set(c)
	//
	return empty.BigInt(val)
}

// False constructs an expression which never holds.
func False[F field.Element[F]]() Expr[F] {
	var empty Expr[F]
	//
	return empty.Bool(false)
}

// If constructs an if-then expression.
func If[F field.Element[F]](condition Expr[F], trueBranch Expr[F]) Expr[F] {
	return condition.Then(trueBranch)
}

// IfElse constructs an if-then-else expression.
func IfElse[F field.Element[F]](condition Expr[F], trueBranch Expr[F], falseBranch Expr[F]) Expr[F] {
	return condition.ThenElse(trueBranch, falseBranch)
}

// Number constructs a constant expression from an unsigned integer.
func Number[F field.Element[F]](c uint) Expr[F] {
	return BigNumber[F](big.NewInt(int64(c)))
}

// Or constructs a disjunction.
func Or[F field.Element[F]](first Expr[F], rest ...Expr[F]) Expr[F] {
	return first.Or(rest...)
}

// Sum constructs a sum over one or more expressions.
func Sum[F field.Element[F]](exprs []Expr[F]) Expr[F] {
	if len(exprs) == 0 {
		return Number[F](0)
	}
	//
	return exprs[0].Add(exprs[1:]...)
}

// Product constructs a product over one or more expressions.
func Product[F field.Element[F]](exprs ...Expr[F]) Expr[F] {
	if len(exprs) == 0 {
		return Number[F](0)
	}
	//
	return exprs[0].Multiply(exprs[1:]...)
}

// True constructs an expression which always holds.
func True[F field.Element[F]]() Expr[F] {
	var empty Expr[F]
	//
	return empty.Bool(true)
}

// Variable is just a convenient wrapper for creating abstract expressions
// representing variable accesses.
func Variable[F field.Element[F]](id register.Id, bitwidth uint, shift int) Expr[F] {
	var empty Expr[F]
	//
	return empty.Variable(id, bitwidth, shift)
}

// ToRegisterIds converts a slice of bytecode register identifiers into schema
// register identifiers.
func ToRegisterIds(ids []vm.RegisterId) []register.Id {
	regs := make([]register.Id, len(ids))
	//
	for i, id := range ids {
		regs[i] = register.NewId(uint(id))
	}
	//
	return regs
}
