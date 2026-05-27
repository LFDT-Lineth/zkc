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

// Switch represents a switch statement of the form:
//
//	switch (discr) {
//		case a, b, ..., z: { branch_az }	// 1st case
//		case A, B, ..., Z: { branch_AZ }	// 2nd case, etc ...
//		default: { branch_default }		// optional default branch
//	}
//
// the discriminant is allowed to be any expression, there may be no branches
// at all, the default branch in particular is optional.
type Switch[S symbol.Symbol[S]] struct {
	// Discriminant dictates the case
	Discriminant expr.Expr[S]
	// Branches contains all branches of the body, including any default branches
	Branches []SwitchBranch[S]
}

// SwitchBranch represents a branch in a switch statement.
//
// Note: the Labels slice should only contain constants or numerical values
type SwitchBranch[S symbol.Symbol[S]] struct {
	IsDefault bool
	Labels    []expr.Expr[S]
	Body      []Stmt[S]
}

// LogicalOrOfCases takes a branch of a switch statement, say
//
//	switch (discr) {
//		...
//		case a, b, ..., z: { ... }	// sample branch
//		...
//	}
//
// and returns the logical disjunction
//
//	logicalOrOfCases  ≡  (discr == a) ∨ … ∨ (discr == z)
//
// This function is used to build an equivalent if-then-else statement
func (s *SwitchBranch[S]) LogicalOrOfCases(discriminant expr.Expr[S]) (logicalOrOfCases expr.LogicalOr[S]) {
	var labelComparisons = make([]expr.Expr[S], len(s.Labels))

	for i, label := range s.Labels {
		labelComparisons[i] = expr.NewCmp(expr.EQ, discriminant, label)
	}

	return expr.LogicalOr[S]{Exprs: labelComparisons}
}

// DefaultCaseCount returns the number of default case declarations in a switch statement
// a valid switch statement should contain 0 or 1 default cases
func (p *Switch[S]) DefaultCaseCount() (nDefaultCases uint) {
	for _, branch := range p.Branches {
		if branch.IsDefault {
			nDefaultCases++
		}
	}

	return
}

// Uses implementation for Stmt interface.
func (p *Switch[S]) Uses() []variable.Id {
	var reads []variable.Id
	// Collect variables read by the argument
	bits := p.Discriminant.LocalUses()
	for iter := bits.Iter(); iter.HasNext(); {
		reads = append(reads, iter.Next())
	}

	// Collect variables from non default branches
	for _, branch := range p.Branches {
		for _, statement := range branch.Body {
			reads = append(reads, statement.Uses()...)
		}
	}

	return reads
}

// Definitions implementation for Stmt interface.
func (p *Switch[S]) Definitions() []variable.Id {
	var writes []variable.Id

	for _, branch := range p.Branches {
		for _, statement := range branch.Body {
			writes = append(writes, statement.Definitions()...)
		}
	}

	return writes
}

func (p *Switch[S]) String(env variable.Map[S]) string {
	var b strings.Builder
	b.WriteString("switch (")
	b.WriteString(p.Discriminant.String(env))
	b.WriteString(") {\n\t case _: { ... }\n\t default: { ... } }")

	return b.String()
}
