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
package expr

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// Const represents a constant value within an expresion.
type Const[S symbol.Symbol[S]] struct {
	constant big.Int
	base     uint
	datatype data.Type[S]
}

// NewUntypedConstant constructs an expression representing a constant value,
// along with a base (which is used for pretty printing, etc).  This can be used
// to construct a constant expression prior to typing, but should not be used to
// construct an expression after typing has been performed.
func NewUntypedConstant[S symbol.Symbol[S]](constant big.Int, base uint) Expr[S] {
	return &Const[S]{constant, base, nil}
}

// NewTypedConstant constructs an expression representing a constant value with
// a given bitwidth, along with a base (which is used for pretty printing, etc).
// This can be used to construct a constant expression after typing has been
// performed (i.e. because it includes an appropriate type)
func NewTypedConstant[S symbol.Symbol[S]](constant big.Int, base uint, bitwidth uint) Expr[S] {
	return &Const[S]{constant, base, data.NewUnsignedInt[S](bitwidth, false)}
}

// Base returns the based representation for this constant
func (p *Const[S]) Base() uint {
	return p.base
}

// Constant returns the constant value this represents
func (p *Const[S]) Constant() *big.Int {
	return &p.constant
}

// ExternUses implementation for the Expr interface.
func (p *Const[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the Expr interface.
func (p *Const[S]) LocalUses() bit.Set {
	var empty bit.Set
	return empty
}

func (p *Const[S]) String(mapping variable.Map[S]) string {
	return String(p, mapping)
}

// SetType implementation for Expr interface
func (p *Const[S]) SetType(t data.Type[S]) {
	p.datatype = t
}

// Type implementation for Expr interface
func (p *Const[S]) Type() data.Type[S] {
	return p.datatype
}
