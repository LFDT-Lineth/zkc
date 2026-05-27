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
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Ternary represents a conditional expression: condition ? ifTrue : ifFalse
type Ternary[S symbol.Symbol[S]] struct {
	Cond     Expr[S]
	IfTrue   Expr[S]
	IfFalse  Expr[S]
	datatype data.Type[S]
}

// NewTernary creates a new Ternary expression.
func NewTernary[S symbol.Symbol[S]](cond, ifTrue, ifFalse Expr[S]) Expr[S] {
	return &Ternary[S]{Cond: cond, IfTrue: ifTrue, IfFalse: ifFalse}
}

// ExternUses returns the set of external variables used by the expression.
func (p *Ternary[S]) ExternUses() set.AnySortedSet[S] {
	r := externUses(p.Cond, p.IfTrue, p.IfFalse)
	return r
}

// LocalUses returns the set of local variables used by the expression.
func (p *Ternary[S]) LocalUses() bit.Set {
	return localUses(p.Cond, p.IfTrue, p.IfFalse)
}

func (p *Ternary[S]) String(mapping variable.Map[S]) string {
	return p.Cond.String(mapping) + " ? " +
		p.IfTrue.String(mapping) + " : " + p.IfFalse.String(mapping)
}

// SetType sets the data type of the expression, which is determined during type checking.
func (p *Ternary[S]) SetType(t data.Type[S]) { p.datatype = t }

// Type returns the data type of the expression, which is determined during type checking.
func (p *Ternary[S]) Type() data.Type[S] { return p.datatype }
