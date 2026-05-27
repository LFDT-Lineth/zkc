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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/lval"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Assign represents a generic assignment of the following form:
//
// tn, .., t0 := e
//
// Here, t0 .. tn are the *target registers*, of which tn is the *most
// significant*.  These must be disjoint as we cannot assign simultaneously to
// the same register.  Likewise, e is the source expression.  For example,
// consider this case:
//
// c, r0 := r1 + 1
//
// Suppose that r0 and r1 are 16bit registers, whilst c is a 1bit register. The
// result of r1 + 1 occupies 17bits, of which the first 16 are written to r0
// with the most significant (i.e. 16th) bit written to c.  Thus, in this
// particular example, c represents a carry flag.
type Assign[S symbol.Symbol[S]] struct {
	// Target registers for assignment
	Targets []lval.LVal[S]
	// Source expresion for assignment
	Source expr.Expr[S]
}

// Uses implementation for Instruction interface.
func (p *Assign[S]) Uses() []variable.Id {
	return expr.Uses[S](p.Source)
}

// Definitions implementation for Instruction interface.
func (p *Assign[S]) Definitions() []variable.Id {
	return lval.Definitions(p.Targets...)
}

func (p *Assign[S]) String(env variable.Map[S]) string {
	var builder strings.Builder
	//
	if len(p.Targets) > 0 {
		builder.WriteString(lvalsToString[S](env, p.Targets...))
		builder.WriteString(" = ")
	}
	//
	builder.WriteString(p.Source.String(env))
	//
	return builder.String()
}

// lvalsToString returns a string representation for zero or more registers
// separated by a comma.
func lvalsToString[S symbol.Symbol[S]](env variable.Map[S], lvals ...lval.LVal[S]) string {
	var builder strings.Builder
	//
	for i := 0; i < len(lvals); i++ {
		var lv = lvals[i]
		//
		if i != 0 {
			builder.WriteString(", ")
		}
		//
		builder.WriteString(lv.String(env))
	}
	//
	return builder.String()
}
