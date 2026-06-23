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
package ast

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// Environment provides access to information about the enclosing program.
type Environment interface {
	data.ResolvedEnvironment
	// ConstOf resolves an external identifier as a constant expression.
	ConstOf(symbol.Resolved) expr.Resolved
}

// NewEnvironment constructs a new typing environment which wraps a "mapper"
// function which maps type indices to types.
func NewEnvironment(decls ...decl.Resolved) Environment {
	return &environment{decls}
}

// base implementation for Environment interface
type environment struct {
	declarations []decl.Resolved
}

// ConstOf implementation for Environment interface
func (p *environment) ConstOf(id symbol.Resolved) expr.Resolved {
	return p.declarations[id.Index].(*decl.ResolvedConstant).ConstExpr
}

// TypeOf implementation for data.Environment interface
func (p *environment) TypeOf(id symbol.Resolved) data.ResolvedType {
	return p.declarations[id.Index].(*decl.ResolvedTypeAlias).DataType
}

// WellFormed implementation for data.Environment interface
func (p *environment) WellFormed(t data.ResolvedType) bool {
	//
	switch t := t.(type) {
	case nil:
		return false
	case *data.UnsignedInt[symbol.Resolved]:
		return true
	case *data.Alias[symbol.Resolved]:
		return p.WellFormed(p.TypeOf(t.Name))
	case *data.FixedArray[symbol.Resolved]:
		return p.WellFormed(t.DataType)
	case *data.Tuple[symbol.Resolved]:
		for i := range t.Width() {
			if !p.WellFormed(t.Ith(i)) {
				return false
			}
		}
		//
		return true
	case *data.FieldElement[symbol.Resolved]:
		return true
	}
	//
	panic(fmt.Sprintf("unknown type encountered (%s)", t.String(p)))
}
