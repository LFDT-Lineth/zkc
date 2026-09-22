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
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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

// CheckStrict provides slightly simpler interface when all shards of a given
// trace are already computed.
func (p Checker[F]) CheckStrict(trace trace.Trace[F]) (failures []Failure[F], errors []error) {
	var (
		mapper = func(id uint, shard Shard[F]) Result[F] {
			return p.processShard(id, shard)
		}
		//
		result Result[F]
	)
	//
	if p.parallel {
		result = array.ParallelMapReduce(trace, mapper, resultReducer)
	} else {
		result = array.MapReduce(trace, mapper, resultReducer)
	}
	// Finalise global constraints
	return append(result.failures, p.finaliseGlobalConstraints(result)...), result.errors
}

// CheckLazy checks the given trace against the schema associated with this
// checker, returning any constraint failures encountered.  This is done lazily
// to ensure that shards are processed one-at-a-time.
func (p Checker[F]) CheckLazy(tr trace.LazyTrace[F]) (failures []Failure[F], errors []error) {
	var (
		result  Result[F]
		results = make([]Result[F], tr.Len())
	)
	// Lazy map
	errors = tr.Apply(func(id uint, shard trace.Shard[F]) {
		results[id] = p.processShard(id, shard)
	})
	// Lazy reduce
	if p.parallel {
		result = array.ParallelReduce(results, resultReducer)
	} else {
		result = array.Reduce(results, resultReducer)
	}
	// Finalise global constraints
	failures = append(result.failures, p.finaliseGlobalConstraints(result)...)
	// Combine all errors together
	return failures, append(errors, result.errors...)
}

func (p Checker[F]) finaliseGlobalConstraints(result Result[F]) (failures []Failure[F]) {
	if result.state != nil {
		// Check for bus constraints (as these are currently the only global constraints)
		for i, c := range p.constraints {
			if c, ok := c.(*bus.Constraint[F]); ok {
				failures = append(failures, finaliseBusConstraint(c, result.state[i])...)
			}
		}
	}
	//
	return failures
}

func (p Checker[F]) processShard(id uint, shard Shard[F]) (res Result[F]) {
	var (
		stats = util.NewPerfStats()
		// Build the shard context
		context = p.buildShardContext(shard)
		// Construct constraint processor
		processor = ConstraintProcessor[F]{
			p, context, id, shard,
		}
	)
	//
	res.state = make([]State[F], len(p.constraints))
	//
	for i, c := range p.constraints {
		var (
			fails []Failure[F]
			err   error
		)
		// Process ith constraint
		res.state[i], fails, err = processor.processConstraint(c)
		// Append any failures arising
		res.failures = append(res.failures, fails...)
		// Append error if arising
		if err != nil {
			res.errors = append(res.errors, err)
		}
	}
	// Log stats
	stats.Log(fmt.Sprintf("[SHARD %d] checked with %d failures", id, len(res.failures)))
	// Done
	return res
}

func (p Checker[F]) buildShardContext(shard Shard[F]) Context[F] {
	if p.parallel {
		return parBuildContext(shard, p.schema)
	}
	//
	return seqBuildContext(shard, p.schema)
}

// Result represents the result from processing a given shard.
type Result[F field.Element[F]] struct {
	state    []State[F]
	failures []Failure[F]
	errors   []error
}

func resultReducer[F field.Element[F]](lhs, rhs Result[F]) Result[F] {
	var (
		// Combine all failures
		failures = append(lhs.failures, rhs.failures...)
		// Combine all errors
		errors = append(lhs.errors, rhs.errors...)

		state []State[F]
	)
	// Check for error cases
	if lhs.state == nil {
		state = rhs.state
	} else if rhs.state == nil {
		state = lhs.state
	} else {
		// Initialise state
		state = lhs.state
		// Reduce states
		for i := range state {
			state[i] = joinStates(lhs.state[i], rhs.state[i])
		}
	}
	//
	return Result[F]{state, failures, errors}
}

func joinStates[F field.Element[F]](l, r State[F]) State[F] {
	if l.data == nil {
		return r
	} else if r.data == nil {
		return l
	}
	// Insert all items from left into right
	for iter := r.data.KeyValues(); iter.HasNext(); {
		var (
			kv    = iter.Next()
			lv, _ = l.data.Get(kv.Left)
		)
		//
		l.data.Insert(kv.Left, lv+kv.Right)
	}
	//
	return l
}

// ConstraintProcessor encapsulates all data required for processing a given
// constraint on a given shard.
type ConstraintProcessor[F field.Element[F]] struct {
	parent Checker[F]
	// Generated context (for this shard)
	context Context[F]
	// Shard Index
	shardId uint
	// Shard data
	shard trace.Shard[F]
}

func (cp ConstraintProcessor[F]) processConstraint(c Constraint[F]) (st State[F], fails []Failure[F], err error) {
	// Setup panic intercept
	defer func() {
		var e = recover()
		//
		if e != nil {
			var (
				buf [2048]byte
				n   = runtime.Stack(buf[:], false)
			)
			// override return
			err = &Panic{c.Name(), fmt.Sprintf("%v", e), buf[:n]}
		}
	}()
	//
	switch c := c.(type) {
	case *bus.Constraint[F]:
		st, fails = processBusConstraint(cp, c)
	case *lookup.Constraint[F]:
		st, fails = processLookupConstraint(cp, c)
	case *ranged.Constraint[F]:
		st, fails = processRangeConstraint(cp, c)
	case *vanishing.Constraint[F, air.LogicalTerm[F]]:
		st, fails = processVanishingConstraint(cp, c)
	case *vanishing.Constraint[F, mir.LogicalTerm[F]]:
		st, fails = processVanishingConstraint(cp, c)
	default:
		panic(fmt.Sprintf("unknown constraint type for \"%s\"", c.Name()))
	}
	//
	return st, fails, err
}

// Panic indicates that a panic arose during constraint checking.
type Panic struct {
	handle     string
	message    string
	stackTrace []byte
}

func (p *Panic) Error() string {
	return fmt.Sprintf("%s:%s\n\n%s", p.handle, p.message, string(p.stackTrace))
}
