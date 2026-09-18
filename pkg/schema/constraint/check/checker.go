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
package check

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

type Constraint[F field.Element[F]] = schema.Constraint[F]

type Failure[F field.Element[F]] = schema.Failure[F]

type Shard[F field.Element[F]] = trace.Shard[F]

type State[F field.Element[F]] struct {
	data *hash.Map[hash.Array[F], int]
}

// Checker provides an API for trace checking which abstracts various challenges
// around efficient checking in the present of large, sharded traces.  For
// example, by exploiting parallelism.
type Checker[F field.Element[F]] struct {
	// Enable parallelism
	parallel bool
	// Overarching schema
	schema schema.Schema[F]
	// Collected constraints
	constraints []Constraint[F]
}

// New constructs a new constraint checker with suitable defaults.
func New[F field.Element[F], C Constraint[F]](schema schema.Schema[F]) Checker[F] {
	var (
		// Identify full set of constraints
		constraints = schema.Constraints().Collect()
	)
	//
	return Checker[F]{true, schema, constraints}
}

// Check the given trace against the schema associated with this checker,
// returning any constraint failures encountered.  A key challenge with this
// feature is to check constraints without holding the entire trace in memory at
// once.  Instead, it holds one shard in memory at a time.  Note, however, that
// global (i.e. bus) constraints require state to be shared across all shards.
func (p Checker[F]) Check(trace trace.Trace[F]) (failures []Failure[F]) {
	var (
		// Initialise state for state constraints
		state = array.Map(p.constraints, initialiseGlobalConstraint[F])
	)
	//
	for _, shard := range trace {
		fs := p.processShard(state, shard)
		//
		failures = append(failures, fs...)
	}
	// Finalise global constraints
	return append(failures, p.finaliseGlobalConstraints(state)...)
}

func (p Checker[F]) finaliseGlobalConstraints(state []State[F]) []Failure[F] {
	panic("todo")
}

func (p Checker[F]) processShard(state []State[F], shard Shard[F]) []Failure[F] {
	var (
		// Build the shard context
		context = p.buildShardContext(shard)
		// Construct constraint processor
		processor = func(i uint, c Constraint[F]) []Failure[F] {
			return processConstraint(context, state[i], c, shard)
		}
		//
		errors [][]Failure[F]
	)
	//
	if p.parallel {
		errors = array.ParallelMap(p.constraints, processor)
	} else {
		errors = array.Map(p.constraints, processor)
	}
	//
	return array.FlatMap(errors, func(fs []schema.Failure[F]) []Failure[F] {
		return fs
	})
}

func (p Checker[F]) buildShardContext(shard Shard[F]) Context[F] {
	if p.parallel {
		return parBuildContext(shard, p.schema)
	}
	//
	return seqBuildContext(shard, p.schema)
}

func initialiseGlobalConstraint[F field.Element[F]](_ uint, c Constraint[F]) State[F] {
	//
	panic("todo")
}

func processConstraint[F field.Element[F]](ctx Context[F], state State[F], c Constraint[F], shard Shard[F]) []Failure[F] {
	panic("todo")
}
