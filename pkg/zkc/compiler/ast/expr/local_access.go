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

// LocalAccess represents a register access within an expression.
type LocalAccess[S symbol.Symbol[S]] struct {
	Variable variable.Id
	datatype data.Type[S]
}

// NewLocalAccess constructs an expression representing a register access.
func NewLocalAccess[S symbol.Symbol[S]](variable variable.Id) Expr[S] {
	return &LocalAccess[S]{Variable: variable}
}

// ExternUses implementation for the Expr interface.
func (p *LocalAccess[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the Expr interface.
func (p *LocalAccess[S]) LocalUses() bit.Set {
	var read bit.Set
	read.Insert(p.Variable)
	//
	return read
}

func (p *LocalAccess[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for Expr interface
func (p *LocalAccess[S]) SetType(t data.Type[S]) {
	p.datatype = t
}

// Type implementation for Expr interface
func (p *LocalAccess[S]) Type() data.Type[S] {
	return p.datatype
}
