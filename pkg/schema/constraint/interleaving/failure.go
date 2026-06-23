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
package interleaving

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
)

// Failure provides structural information about a failing lookup constraint.
type Failure[F any] struct {
	// Handle of the failing constraint
	Handle string
	// Relevant context for target expressions.
	TargetContext schema.ModuleId
	// Target expression involved
	Target term.Evaluable[F]
	// Relevant context for source expressions.
	SourceContext schema.ModuleId
	// Source expression which were missing
	Source term.Evaluable[F]
	// Target row on which constraint
	Row uint
}

// Message provides a suitable error message
func (p *Failure[F]) Message() string {
	return fmt.Sprintf("interleaving \"%s\" failed (row %d)", p.Handle, p.Row)
}

func (p *Failure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *Failure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	var res = set.NewAnySortedSet[trace.CellRef]()
	//
	res.InsertSorted(p.Source.RequiredCells(int(p.Row), p.SourceContext))
	res.InsertSorted(p.Target.RequiredCells(int(p.Row), p.TargetContext))
	//
	return res
}
