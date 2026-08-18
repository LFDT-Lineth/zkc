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
package vanishing

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
)

// Failure provides structural information about a failing vanishing constraint.
type Failure[F any] struct {
	// Handle of the failing constraint
	Handle string
	// Constraint expression
	Constraint term.Testable[F]
	// Module where constraint failed
	Context schema.ModuleId
	// Row on which the constraint failed
	Row uint
}

// Message provides a suitable error message
func (p *Failure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("constraint \"%s\" does not hold (row %d)", p.Handle, p.Row)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *Failure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	return p.Constraint.RequiredCells(int(p.Row), p.Context)
}

func (p *Failure[F]) String() string {
	return p.Message()
}
