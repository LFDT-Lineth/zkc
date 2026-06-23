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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
)

// Fail signals an exceptional return from the enclosing function.  An optional
// formatted error message can be supplied using the same chunked
// representation as Printf.
type Fail[S symbol.Symbol[S]] struct {
	Chunks    []FormattedChunk
	Arguments []expr.Expr[S]
}

// Uses implementation for Instruction interface.
func (p *Fail[S]) Uses() []variable.Id {
	return expr.Uses(p.Arguments...)
}

// Definitions implementation for Instruction interface.
func (p *Fail[S]) Definitions() []variable.Id {
	return nil
}

func (p *Fail[S]) String(env variable.Map[S]) string {
	var builder strings.Builder
	builder.WriteString("fail")
	//
	if len(p.Chunks) > 0 {
		builder.WriteString(" \"")
		//
		for _, chunk := range p.Chunks {
			builder.WriteString(util.EscapeFormattedText(chunk.Text))
			//
			if chunk.Format.HasFormat() {
				builder.WriteString(chunk.Format.String())
			}
		}
		//
		builder.WriteString("\"")
	}
	//
	for _, e := range p.Arguments {
		builder.WriteString(",")
		builder.WriteString(e.String(env))
	}
	//
	return builder.String()
}
