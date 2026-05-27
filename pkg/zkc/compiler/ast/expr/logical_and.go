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

// LogicalAnd represents a bitwise-or of one or more terms.
type LogicalAnd[S symbol.Symbol[S]] struct {
	Exprs []Expr[S]
}

// NewLogicalAnd constructs an expression representing the bitwise-or of one or
// more values.
func NewLogicalAnd[S symbol.Symbol[S]](exprs ...Expr[S]) Expr[S] {
	if len(exprs) == 0 {
		panic("one or more subexpressions required")
	}
	//
	return &LogicalAnd[S]{Exprs: exprs}
}

// ExternUses implementation for the Expr interface.
func (p *LogicalAnd[S]) ExternUses() set.AnySortedSet[S] {
	return externUses(p.Exprs...)
}

// LocalUses implementation for the Expr interface.
func (p *LogicalAnd[S]) LocalUses() bit.Set {
	return localUses(p.Exprs...)
}

func (p *LogicalAnd[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for Expr interface
func (p *LogicalAnd[S]) SetType(t data.Type[S]) {
	panic("unreachable")
}

// Type implementation for Expr interface
func (p *LogicalAnd[S]) Type() data.Type[S] {
	panic("unreachable")
}
