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
package checker

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Checker provides an API for trace checking which abstracts various challenges
// around efficient checking in the present of large, sharded traces.  For
// example, by exploiting parallelism.
type Checker[F field.Element[F], C schema.Constraint[F]] struct {
	// Enable parallelism
	parallel bool
	// Overarching schema
	schema schema.Schema[F, C]
	// Collected constraints
	constraints []C
}

type globalState[F field.Element[F]] struct {
	data *hash.Map[hash.Array[F], int]
}

// New constructs a new constraint checker with suitable defaults.
func New[F field.Element[F], C schema.Constraint[F]](schema schema.Schema[F, C]) Checker[F, C] {
	var (
		// Identify full set of constraints
		constraints = schema.Constraints().Collect()
	)
	//
	return Checker[F, C]{true, schema, constraints}
}

// Check the given trace against the schema associated with this checker,
// returning any constraint failures encountered.  A key challenge with this
// feature is to check constraints without holding the entire trace in memory at
// once.  Instead, it holds one shard in memory at a time.  Note, however, that
// global (i.e. bus) constraints require state to be shared across all shards.
func (p Checker[F, C]) Check(trace trace.Trace[F]) (failures []schema.Failure[F]) {
	var (
		// Initialise state for state constraints
		state = p.initialiseGlobalConstraints()
	)
	//
	for _, shard := range trace {
		var fs []schema.Failure[F]
		//
		if p.parallel {
			fs = p.parProcessShard(state, shard)
		} else {
			fs = p.seqProcessShard(state, shard)
		}
		//
		failures = append(failures, fs...)
	}
	// Finalise global constraints
	return append(failures, p.finaliseGlobalConstraints(state)...)
}

func (p Checker[F, C]) initialiseGlobalConstraints() []globalState[F] {
	panic("todo")
}

func (p Checker[F, C]) finaliseGlobalConstraints(state []globalState[F]) []schema.Failure[F] {
	panic("todo")
}

func (p Checker[F, C]) seqProcessShard(state []globalState[F], shard trace.Shard[F]) []schema.Failure[F] {
	panic("todo")
}

func (p Checker[F, C]) parProcessShard(state []globalState[F], shard trace.Shard[F]) []schema.Failure[F] {
	panic("todo")
}
