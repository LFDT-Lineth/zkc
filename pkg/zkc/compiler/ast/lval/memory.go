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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// MemAccess represents a memory write within an assignment.
type MemAccess[S symbol.Symbol[S]] struct {
	// name of the memory being written
	Name S
	// identifies location being written
	Args []expr.Expr[S]
}

// NewMemAccess constructs an expression representing a register access.
func NewMemAccess[S symbol.Symbol[S]](name S, args []expr.Expr[S]) LVal[S] {
	return &MemAccess[S]{name, args}
}

// ExternUses implementation for the LVal interface.
func (p *MemAccess[S]) ExternUses() set.AnySortedSet[S] {
	var res set.AnySortedSet[S]
	//
	for _, e := range p.Args {
		ith := e.ExternUses()
		res.InsertSorted(&ith)
	}
	//
	return res
}

// LocalUses implementation for the LVal interface.
func (p *MemAccess[S]) LocalUses() bit.Set {
	var reads bit.Set
	//
	for _, e := range p.Args {
		reads.Union(e.LocalUses())
	}
	//
	return reads
}

// LocalDefs implementation for the LVal interface.
func (p *MemAccess[S]) LocalDefs() bit.Set {
	return bit.Set{}
}

func (p *MemAccess[S]) String(mapping variable.Map[S]) string {
	return String[S](p, mapping)
}
