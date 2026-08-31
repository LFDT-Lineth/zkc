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
package lval

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// Discard represents the wildcard "_" appearing as an assignment target, which
// discards the corresponding return value of a function call (or static memory
// read).  A discarded return has no backing variable: codegen simply omits it
// from the call's return bindings, so no register (and hence no trace column)
// is ever allocated for it.
type Discard[S symbol.Symbol[S]] struct {
	// NOTE: this padding field makes Discard non-zero-sized, so that distinct
	// nodes have distinct addresses.  Zero-sized allocations all share a
	// single address, which breaks source maps (keyed on pointer identity).
	_ byte
}

// NewDiscard constructs an lval discarding the corresponding return value.
func NewDiscard[S symbol.Symbol[S]]() LVal[S] {
	return &Discard[S]{}
}

// ExternUses implementation for the LVal interface.
func (p *Discard[S]) ExternUses() set.AnySortedSet[S] {
	return nil
}

// LocalUses implementation for the LVal interface.
func (p *Discard[S]) LocalUses() bit.Set {
	return bit.Set{}
}

// LocalDefs implementation for the LVal interface.
func (p *Discard[S]) LocalDefs() bit.Set {
	return bit.Set{}
}

func (p *Discard[S]) String(mapping variable.Map[S]) string {
	return "_"
}
