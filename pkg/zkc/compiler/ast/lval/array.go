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
package lval

import (
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Array represents an array write within an assignment.
type Array[S symbol.Symbol[S]] struct {
	// name of the array being written
	Id variable.Id
	// identifies location being written
	Arg expr.Expr[S]
}

// NewArray constructs an expression representing an array access.
func NewArray[S symbol.Symbol[S]](variable variable.Id, arg expr.Expr[S]) LVal[S] {
	return &Array[S]{variable, arg}
}

// ExternUses implementation for the LVal interface.
func (p *Array[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the LVal interface.
func (p *Array[S]) LocalUses() bit.Set {
	return p.Arg.LocalUses()
}

// LocalDefs implementation for the LVal interface.
func (p *Array[S]) LocalDefs() bit.Set {
	var defs bit.Set
	defs.Insert(p.Id)
	//
	return defs
}

func (p *Array[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}
