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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// Cost attaches a compile-time cost label to a statement. It does not change
// the statement semantics.
type Cost[S symbol.Symbol[S]] struct {
	Label string
	Body  Stmt[S]
}

// Uses implementation for Stmt interface.
func (p *Cost[S]) Uses() []variable.Id {
	return p.Body.Uses()
}

// Definitions implementation for Stmt interface.
func (p *Cost[S]) Definitions() []variable.Id {
	return p.Body.Definitions()
}

func (p *Cost[S]) String(env variable.Map[S]) string {
	return fmt.Sprintf("#[cost:%s] %s", p.Label, p.Body.String(env))
}

// UnwrapCost returns the body of a cost annotation, or the statement itself.
func UnwrapCost[S symbol.Symbol[S]](s Stmt[S]) Stmt[S] {
	if c, ok := s.(*Cost[S]); ok {
		return c.Body
	}

	return s
}
