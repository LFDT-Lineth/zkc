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

// ExternAccess represents a reference to an external declaration, such as a
// named constant or memory.
type ExternAccess[S symbol.Symbol[S]] struct {
	Name     S
	Args     []Expr[S]
	datatype data.Type[S]
}

// NewExternAccess constructs an expression representing a non-local access,
// such as for a named constant or memory.
func NewExternAccess[S symbol.Symbol[S]](name S, args ...Expr[S]) Expr[S] {
	return &ExternAccess[S]{Name: name, Args: args}
}

// ExternUses implementation for the Expr interface.
func (p *ExternAccess[S]) ExternUses() set.AnySortedSet[S] {
	var uses = externUses(p.Args...)
	//
	uses.Insert(p.Name)
	//
	return uses
}

// LocalUses implementation for the Expr interface.
func (p *ExternAccess[S]) LocalUses() bit.Set {
	return localUses(p.Args...)
}

func (p *ExternAccess[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}

// SetType implementation for Expr interface
func (p *ExternAccess[S]) SetType(t data.Type[S]) {
	p.datatype = t
}

// Type implementation for Expr interface
func (p *ExternAccess[S]) Type() data.Type[S] {
	return p.datatype
}
