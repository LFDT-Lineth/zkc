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
package compiler

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// Register encapsulates information about a "register" in the underlying
// constraint system.  The rough analogy is that a Corset column is mapped down
// to an HIR column (a.k.a register).  The distinction between columns at the
// Corset level, and registers at the HIR level is necessary because one corset
// column can expand to several HIR registers (e.g. an array column expands
// into one register per element).
type Register struct {
	// Context (i.e. enclosing module) of this register.
	Context ast.Context
	// Fully qualified (i.e. absolute) path of the source-level column.
	Path file.Path
	// Underlying bitwidth of this register.
	Bitwidth uint
	// Provability requirement for this register.
	MustProve bool
	// Determines whether this is a Computed column.
	Computed bool
	// Common padding value
	Padding big.Int
	// Display modifier
	Display string
	// Cached name
	cached_name *string
}

// IsInput determines whether or not this register represents an input column,
// or not.
func (r *Register) IsInput() bool {
	return !r.Computed
}

// Name returns the given name for this register.
func (r *Register) Name() string {
	if r.cached_name == nil {
		name := r.Path.Tail()
		r.cached_name = &name
	}
	//
	return *r.cached_name
}
