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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Break exits the innermost enclosing loop.
type Break[S symbol.Symbol[S]] struct {
	// dummy forces heap allocation
	//nolint
	Dummy uint
}

// Uses implementation for Stmt interface.
func (p *Break[S]) Uses() []variable.Id {
	return nil
}

// Definitions implementation for Stmt interface.
func (p *Break[S]) Definitions() []variable.Id {
	return nil
}

func (p *Break[S]) String(_ variable.Map[S]) string {
	return "break"
}
