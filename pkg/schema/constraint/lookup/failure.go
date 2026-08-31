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
package lookup

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
)

// Failure provides structural information about a failing lookup constraint.
type Failure[F any] struct {
	// Handle of the failing constraint
	LookupHandle string
	// SourceId gives the set identifier of the source
	SourceId schema.SetId
	// Row on which the constraint failed
	Row uint
	// Shard on which the constraint failed
	Shard uint
}

// Handle implementation of schema.Failure interface
func (p *Failure[F]) Handle() string {
	return p.LookupHandle
}

// Message provides a suitable error message
func (p *Failure[F]) Message() string {
	return fmt.Sprintf("lookup \"%s\" failed (row %d, shard %d)", p.Handle(), p.Row, p.Shard)
}

func (p *Failure[F]) String() string {
	return p.Message()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *Failure[F]) RequiredCells(_ trace.Trace[F]) set.AnySortedSet[trace.ShardedCellRef] {
	res := set.NewAnySortedSet[trace.ShardedCellRef]()
	// Handle registers
	for i := range p.SourceId.Width() {
		var rid = p.SourceId.Ith(i)
		//
		ref := trace.NewColumnRef(p.SourceId.Module(), rid)
		res.Insert(trace.NewShardedCellRef(p.Shard, ref, int(p.Row)))
	}
	//
	return *res
}
