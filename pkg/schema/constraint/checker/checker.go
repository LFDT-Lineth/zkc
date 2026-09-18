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
	"fmt"
	"runtime"

	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Constraint provides a convenient alias
type Constraint[F field.Element[F]] = schema.Constraint[F]

// Failure provides a convenient alias
type Failure[F field.Element[F]] = schema.Failure[F]

// Shard provides a convenient alias
type Shard[F field.Element[F]] = trace.Shard[F]

// State is used to hold state which persists across the entire trace for a
// given constraint.
type State[F field.Element[F]] struct {
	data *bus.Tally[F]
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
func New[F field.Element[F]](schema schema.Schema[F]) Checker[F] {
	var (
		// Identify full set of constraints
		constraints = schema.Constraints().Collect()
	)
	//
	return Checker[F]{true, schema, constraints}
}

// WithParallelism returns a new checker with parallelism either enabled or
// disabled.
func (p Checker[F]) WithParallelism(enable bool) Checker[F] {
	p.parallel = enable
	//
	return p
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
	for id, shard := range trace {
		var stats = util.NewPerfStats()
		// process shard
		fs := p.processShard(state, uint(id), shard)
		//
		failures = append(failures, fs...)
		//
		stats.Log(fmt.Sprintf("[SHARD %d] checked with %d failures", id, len(fs)))
	}
	// Finalise global constraints
	return append(failures, p.finaliseGlobalConstraints(state)...)
}

func (p Checker[F]) finaliseGlobalConstraints(state []State[F]) (failures []Failure[F]) {
	// Check for bus constraints (as these are currently the only global constraints)
	for i, c := range p.constraints {
		if c, ok := c.(*bus.Constraint[F]); ok {
			failures = append(failures, finaliseBusConstraint(c, state[i])...)
		}
	}
	//
	return failures
}

func (p Checker[F]) processShard(state []State[F], shardId uint, shard Shard[F]) []Failure[F] {
	var (
		// Build the shard context
		context = p.buildShardContext(shard)
		// Construct constraint processor
		processor = func(i uint, c Constraint[F]) []Failure[F] {
			// Construct constraint processor
			var cp = ConstraintProcessor[F]{
				p, context, state[i], shardId, shard,
			}
			// Process constrant
			return cp.processConstraint(c)
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
	var state State[F]
	// Check for bus constraints (as these are currently the only global constraints)
	if _, ok := c.(*bus.Constraint[F]); ok {
		// Initialise the tally
		state.data = hash.NewMap[hash.Array[F], int](32)
	}
	//
	return state
}

// ConstraintProcessor encapsulates all data required for processing a given
// constraint on a given shard.
type ConstraintProcessor[F field.Element[F]] struct {
	parent Checker[F]
	// Generated context (for this shard)
	context Context[F]
	// Global state (for this constraint)
	state State[F]
	// Shard Index
	shardId uint
	// Shard data
	shard trace.Shard[F]
}

func (cp ConstraintProcessor[F]) processConstraint(c Constraint[F]) (res []Failure[F]) {
	// Setup panic intercept
	defer func() {
		var err = recover()
		//
		if err != nil {
			var (
				buf [2048]byte
				n   = runtime.Stack(buf[:], false)
			)
			// override return
			res = []Failure[F]{
				constraint.NewPanicFailure[F](c.Name(), fmt.Sprintf("%v", err), buf[:n]),
			}
		}
	}()
	//
	switch c := c.(type) {
	case *bus.Constraint[F]:
		return processBusConstraint(cp, c)
	case *lookup.Constraint[F]:
		return processLookupConstraint(cp, c)
	case *ranged.Constraint[F]:
		return processRangeConstraint(cp, c)
	case *vanishing.Constraint[F, air.LogicalTerm[F]]:
		return processVanishingConstraint(cp, c)
	case *vanishing.Constraint[F, mir.LogicalTerm[F]]:
		return processVanishingConstraint(cp, c)
	default:
		panic(fmt.Sprintf("unknown constraint type for \"%s\"", c.Name()))
	}
}
