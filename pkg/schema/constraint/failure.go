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
package constraint

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
)

// InternalFailure is a generic mechanism for reporting failures, particularly
// as arising from evaluation of a given expression.
type InternalFailure[F any] struct {
	// Handle of the failing constraint
	handle string
	// Module in which constraint failed.
	context schema.ModuleId
	// Row on which the constraint failed
	row uint
	// Cells involved (if any)
	term term.Contextual
	// Error message
	error string
}

// NewInternalFailure constructs a new internal failure object.
func NewInternalFailure[F any](handle string, ctx schema.ModuleId, row uint, term term.Contextual,
	err string) *InternalFailure[F] {
	//
	return &InternalFailure[F]{handle, ctx, row, term, err}
}

// Error provides a suitable error message
func (p *InternalFailure[F]) Error() string {
	return p.Message()
}

// Message provides a suitable error message
func (p *InternalFailure[F]) Message() string {
	return fmt.Sprintf("%s (row %d) - %s", p.handle, p.row, p.error)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *InternalFailure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	if p.term != nil {
		return p.term.RequiredCells(int(p.row), p.context)
	}
	// Empty set
	return set.NewAnySortedSet[trace.CellRef]()
}
