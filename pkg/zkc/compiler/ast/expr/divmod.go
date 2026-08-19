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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// DivMod represents the combined division/remainder ("/%") expression, which
// computes both the quotient and the remainder of its two operands.  Unlike
// the other arithmetic operators it produces two values, so its type is a
// tuple and it can only appear as the full source of a two-target assignment
// (e.g. "q, r = a /% b").
type DivMod[S symbol.Symbol[S]] struct {
	Exprs    []Expr[S]
	datatype data.Type[S]
}

// NewDivMod constructs an expression computing both the quotient and the
// remainder of the given dividend and divisor.
func NewDivMod[S symbol.Symbol[S]](dividend, divisor Expr[S]) Expr[S] {
	return &DivMod[S]{Exprs: []Expr[S]{dividend, divisor}}
}

// ExternUses implementation for the Expr interface.
func (p *DivMod[S]) ExternUses() set.AnySortedSet[S] {
	return externUses(p.Exprs...)
}

// LocalUses implementation for the Expr interface.
func (p *DivMod[S]) LocalUses() bit.Set {
	return localUses(p.Exprs...)
}

func (p *DivMod[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for Expr interface
func (p *DivMod[S]) SetType(t data.Type[S]) {
	p.datatype = t
}

// Type implementation for Expr interface
func (p *DivMod[S]) Type() data.Type[S] {
	return p.datatype
}
