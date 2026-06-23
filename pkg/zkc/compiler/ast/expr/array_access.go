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

// ArrayAccess represents an array access within an expression.
type ArrayAccess[S symbol.Symbol[S]] struct {
	Id       variable.Id
	Arg      Expr[S]
	Datatype data.Type[S]
}

// NewArrayAccess constructs an expression representing an array access.
func NewArrayAccess[S symbol.Symbol[S]](id variable.Id, arg Expr[S]) Expr[S] {
	return &ArrayAccess[S]{Id: id, Arg: arg}
}

// ExternUses implementation for the Expr interface.
func (p *ArrayAccess[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the Expr interface.
func (p *ArrayAccess[S]) LocalUses() bit.Set {
	var read bit.Set
	read.Insert(p.Id)
	read.Union(p.Arg.LocalUses())
	//
	return read
}

func (p *ArrayAccess[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for Expr interface
func (p *ArrayAccess[S]) SetType(t data.Type[S]) {
	p.Datatype = t
}

// Type implementation for Expr interface
func (p *ArrayAccess[S]) Type() data.Type[S] {
	return p.Datatype
}
