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
package stmt

import (
	"strings"

	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// VarDecl represents a variable declaration of the form:
//
//	var x:T [= e]
//
// or, for multi-variable declarations:
//
//	var x:T, y:U
//
// Variables holds the IDs of all declared variables; their names and types are
// accessible via the enclosing function's variable map.
// Init holds the optional initialiser expression (util.None when absent).
// Invariant: Init.HasValue() => len(Variables) == 1.
type VarDecl[S symbol.Symbol[S]] struct {
	// Variables contains the IDs of all declared variables.
	Variables []variable.Id
	// Init is the optional initialiser expression.
	Init util.Option[expr.Expr[S]]
}

// Uses implementation for Stmt interface.
func (p *VarDecl[S]) Uses() []variable.Id {
	if p.Init.HasValue() {
		return expr.Uses[S](p.Init.Unwrap())
	}

	return nil
}

// Definitions implementation for Stmt interface.
func (p *VarDecl[S]) Definitions() []variable.Id {
	return p.Variables
}

// String implementation for Stmt interface.
func (p *VarDecl[S]) String(env variable.Map[S]) string {
	var builder strings.Builder

	builder.WriteString("var ")

	for i, id := range p.Variables {
		if i != 0 {
			builder.WriteString(", ")
		}

		v := env.Variable(id)
		builder.WriteString(v.Name)
		builder.WriteString(":")
		builder.WriteString(v.DataType.String(nil))
	}

	if p.Init.HasValue() {
		builder.WriteString(" = ")
		builder.WriteString(p.Init.Unwrap().String(env))
	}

	return builder.String()
}
