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

// IfElse represents a conditional block construct of the form:
//
//	if (cond) { trueBranch } else { falseBranch }
//
// FalseBranch is empty when there is no else clause.
type IfElse[S symbol.Symbol[S]] struct {
	// Cond is the branch condition (must be Cmp, LogicalAnd, LogicalOr, or LogicalNot)
	Cond expr.Expr[S]
	// TrueBranch holds the statements executed when the condition is true
	TrueBranch []Stmt[S]
	// FalseBranch holds the statements executed when the condition is false (may be empty)
	FalseBranch []Stmt[S]
}

// Uses implementation for Stmt interface.
func (p *IfElse[S]) Uses() []variable.Id {
	var reads []variable.Id
	// Collect variables read by the condition
	bits := p.Cond.LocalUses()
	for iter := bits.Iter(); iter.HasNext(); {
		reads = append(reads, iter.Next())
	}
	// Collect from both branches
	for _, s := range p.TrueBranch {
		reads = append(reads, s.Uses()...)
	}

	for _, s := range p.FalseBranch {
		reads = append(reads, s.Uses()...)
	}

	return reads
}

// Definitions implementation for Stmt interface.
func (p *IfElse[S]) Definitions() []variable.Id {
	var writes []variable.Id
	for _, s := range p.TrueBranch {
		writes = append(writes, s.Definitions()...)
	}

	for _, s := range p.FalseBranch {
		writes = append(writes, s.Definitions()...)
	}

	return writes
}

func (p *IfElse[S]) String(env variable.Map[S]) string {
	var b strings.Builder
	b.WriteString("if (")
	b.WriteString(p.Cond.String(env))
	b.WriteString(") { ... }")

	if len(p.FalseBranch) > 0 {
		b.WriteString(" else { ... }")
	}

	return b.String()
}
