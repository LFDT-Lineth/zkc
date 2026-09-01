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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Failure provides structural information about a failing vanishing constraint.
type Failure[F field.Element[F]] struct {
	// Handle of the failing constraint
	VanishingHandle string
	// Constraint expression
	Constraint term.Testable[F]
	// Module where constraint failed
	Context schema.ModuleId
	// Row on which the constraint failed
	Row uint
	// Shard on which the constraint failed
	Shard uint
}

// Handle implementation of schema.Failure interface
func (p *Failure[F]) Handle() string {
	return p.VanishingHandle
}

// Message provides a suitable error message
func (p *Failure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("constraint \"%s\" does not hold (row %d, shard %d)", p.Handle(), p.Row, p.Shard)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *Failure[F]) RequiredCells(_ trace.Trace[F]) set.AnySortedSet[trace.ShardedCellRef] {
	var cells = p.Constraint.RequiredCells(int(p.Row), p.Context)
	// Convert from cell ref into sharded cell ref
	return array.Map(cells.ToArray(), func(_ uint, r trace.CellRef) trace.ShardedCellRef {
		return trace.NewShardedCellRef(p.Shard, r.Column, r.Row)
	})
}

func (p *Failure[F]) String() string {
	return p.Message()
}
