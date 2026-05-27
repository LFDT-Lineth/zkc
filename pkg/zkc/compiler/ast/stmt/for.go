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

// For represents a loop construct of the form:
//
//	for (init; cond; post) { body }
type For[S symbol.Symbol[S]] struct {
	// Init is the initialiser statement (e.g. a variable assignment)
	Init Stmt[S]
	// Cond is the loop condition (must be Cmp, LogicalAnd, LogicalOr, or LogicalNot)
	Cond expr.Expr[S]
	// Post is the post-iteration statement (e.g. an increment assignment)
	Post Stmt[S]
	// Body holds the statements executed each iteration
	Body []Stmt[S]
}

// Uses implementation for Stmt interface.
func (p *For[S]) Uses() []variable.Id {
	var reads []variable.Id

	reads = append(reads, p.Init.Uses()...)

	bits := p.Cond.LocalUses()
	for iter := bits.Iter(); iter.HasNext(); {
		reads = append(reads, iter.Next())
	}

	reads = append(reads, p.Post.Uses()...)
	for _, s := range p.Body {
		reads = append(reads, s.Uses()...)
	}

	return reads
}

// Definitions implementation for Stmt interface.
func (p *For[S]) Definitions() []variable.Id {
	var writes []variable.Id

	writes = append(writes, p.Init.Definitions()...)

	writes = append(writes, p.Post.Definitions()...)
	for _, s := range p.Body {
		writes = append(writes, s.Definitions()...)
	}

	return writes
}

func (p *For[S]) String(env variable.Map[S]) string {
	var b strings.Builder
	b.WriteString("for (")
	b.WriteString(p.Init.String(env))
	b.WriteString("; ")
	b.WriteString(p.Cond.String(env))
	b.WriteString("; ")
	b.WriteString(p.Post.String(env))
	b.WriteString(") { ... }")

	return b.String()
}
