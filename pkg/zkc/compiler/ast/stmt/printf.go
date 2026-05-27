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
	"github.com/consensys/go-corset/pkg/zkc/util"
)

// FormattedChunk represents a chunk of a printf string which consists of some
// text, followed by an optional argument format.
type FormattedChunk struct {
	Text   string
	Format util.Format
}

// Printf provides a simple mechanism for debugging ZkC programs.  It allows one
// to print out the contents of variables at specific points, using a fairly
// standard syntax.
type Printf[S symbol.Symbol[S]] struct {
	Chunks    []FormattedChunk
	Arguments []expr.Expr[S]
}

// Uses implementation for Instruction interface.
func (p *Printf[S]) Uses() []variable.Id {
	return expr.Uses(p.Arguments...)
}

// Definitions implementation for Instruction interface.
func (p *Printf[S]) Definitions() []variable.Id {
	return nil
}

func (p *Printf[S]) String(env variable.Map[S]) string {
	var builder strings.Builder
	builder.WriteString("printf \"")
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
	//
	for _, e := range p.Arguments {
		builder.WriteString(",")
		builder.WriteString(e.String(env))
	}
	//
	return builder.String()
}
