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
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Variable represents a register access within an expresion.
type Variable[S symbol.Symbol[S]] struct {
	Ids []variable.Id
}

// NewVariable constructs an expression representing a register access.
func NewVariable[S symbol.Symbol[S]](variable ...variable.Id) LVal[S] {
	return &Variable[S]{Ids: variable}
}

// ExternUses implementation for the LVal interface.
func (p *Variable[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the LVal interface.
func (p *Variable[S]) LocalUses() bit.Set {
	return bit.Set{}
}

// LocalDefs implementation for the LVal interface.
func (p *Variable[S]) LocalDefs() bit.Set {
	var read bit.Set
	//
	for _, id := range p.Ids {
		read.Insert(id)
	}
	//
	return read
}

func (p *Variable[S]) String(mapping variable.Map[S]) string {
	var builder strings.Builder
	//
	for i, id := range p.Ids {
		if i != 0 {
			builder.WriteString("::")
		}
		//
		builder.WriteString(mapping.Variable(id).Name)
	}
	//
	return builder.String()
}
