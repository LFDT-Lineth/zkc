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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// NeverCall represents a call to a "no return" function.  This is called a
// "never call" because such a function has the "never type" (following Rust
// terminology).
type NeverCall[S symbol.Symbol[S]] struct {
	Name S
	Args []expr.Expr[S]
}

// Uses implementation for Stmt interface.
func (p *NeverCall[S]) Uses() []variable.Id {
	return expr.Uses(p.Args...)
}

// Definitions implementation for Stmt interface.
func (p *NeverCall[S]) Definitions() []variable.Id {
	return nil
}

func (p *NeverCall[S]) String(mapping variable.Map[S]) string {
	var builder strings.Builder
	//
	builder.WriteString(p.Name.String())
	builder.WriteString("!(")
	//
	for i, arg := range p.Args {
		if i != 0 {
			builder.WriteString(",")
		}

		builder.WriteString(expr.String(arg, mapping))
	}
	//
	builder.WriteString(")")
	//
	return builder.String()
}
