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

	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// While represents a loop construct of the form:
//
//	while (cond) { body }
type While[S symbol.Symbol[S]] struct {
	// Cond is the loop condition (must be Cmp, LogicalAnd, LogicalOr, or LogicalNot)
	Cond expr.Expr[S]
	// Body holds the statements executed each iteration
	Body []Stmt[S]
}

// Uses implementation for Stmt interface.
func (p *While[S]) Uses() []variable.Id {
	var reads []variable.Id

	bits := p.Cond.LocalUses()
	for iter := bits.Iter(); iter.HasNext(); {
		reads = append(reads, iter.Next())
	}

	for _, s := range p.Body {
		reads = append(reads, s.Uses()...)
	}

	return reads
}

// Definitions implementation for Stmt interface.
func (p *While[S]) Definitions() []variable.Id {
	var writes []variable.Id
	for _, s := range p.Body {
		writes = append(writes, s.Definitions()...)
	}

	return writes
}

func (p *While[S]) String(env variable.Map[S]) string {
	var b strings.Builder
	b.WriteString("while (")
	b.WriteString(p.Cond.String(env))
	b.WriteString(") { ... }")

	return b.String()
}
