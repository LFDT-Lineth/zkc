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

// Cast represents a type cast expression of the form "e as T", which truncates
// the value of e to fit within the target type T.
type Cast[S symbol.Symbol[S]] struct {
	Expr     Expr[S]
	CastType data.Type[S]
	datatype data.Type[S]
}

// NewCast constructs a cast expression.
func NewCast[S symbol.Symbol[S]](e Expr[S], castType data.Type[S]) Expr[S] {
	return &Cast[S]{Expr: e, CastType: castType}
}

// ExternUses implementation for the Expr interface.
func (p *Cast[S]) ExternUses() set.AnySortedSet[S] {
	return p.Expr.ExternUses()
}

// LocalUses implementation for the Expr interface.
func (p *Cast[S]) LocalUses() bit.Set {
	return p.Expr.LocalUses()
}

// String implementation for the Expr interface.
func (p *Cast[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for the Expr interface.
func (p *Cast[S]) SetType(t data.Type[S]) {
	p.datatype = t
}

// Type implementation for the Expr interface.
func (p *Cast[S]) Type() data.Type[S] {
	return p.datatype
}
