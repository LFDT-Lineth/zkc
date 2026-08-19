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
package vm

import (
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Trace defines the type of a general trace
type Trace[F field.Element[F]] = trace.Trace[F]

// Element defines the type of field elements
type Element[F any] = field.Element[F]

// Tracer defines a generic mechanism for building a trace in real time as data
// is generated during tracing itself.
type Tracer[W Word[W], F Element[F], T any] interface {
	// Initialise tracer for the given program.
	Init(Program[W]) T
	// Append a line for the given function to the trace.
	TraceFunctionLine(line State[W])
	// Construct the trace for a given memory of some kind
	TraceMemory(mid uint16, m RuntimeMemory[W], cfg field.Config)
	// Build the final trace
	Build() trace.Trace[F]
}

// TraceBuilder provides a generic mechanism for tracing a given program, and
// abstracts the myriad different ways this can be done (e.g. sharding,
// parallelism, etc).
type TraceBuilder[W Word[W], F field.Element[F], T Tracer[W, F, T]] struct {
	config  TraceConfig
	tracing Program[W]
}

// NewTraceBuilder constructs a default tracer builder which, most likely,
// should be further configured before use.
func NewTraceBuilder[W Word[W], F Element[F], T Tracer[W, F, T]](config TraceConfig,
	tracing Program[W]) TraceBuilder[W, F, T] {
	//
	return TraceBuilder[W, F, T]{config, tracing}
}

// BootAndTrace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func (p TraceBuilder[W, F, T]) BootAndTrace(inputs map[string][]byte,
) (tr trace.Trace[F], outputs map[string][]byte, errors []error) {
	// Check whether we have a sharding strategy
	if p.config.shardingStrategy.IsEmpty() {
		// no strategy, therefore trace sequentially
		return BootAndTrace[W, F, T](p.tracing, inputs)
	}
	// apply sharding strategy
	shards, outputs, errors := p.checkpointAndTrace(inputs)
	// Perform trace reduction
	var stats = util.NewPerfStats()
	// Recombine shards
	if p.config.parallel {
		// Parallel
		tr = trace.ParallelReduce(shards)
		//
		stats.Log("Trace reduction (parallel)")
	} else {
		// Sequential
		tr = trace.Reduce(shards)
		//
		stats.Log("Trace reduction (sequential)")
	}
	//
	return tr, outputs, errors
}

// BootAndTraceShards shards generates a given number of shards from a given
// program.
func (p TraceBuilder[W, F, T]) BootAndTraceShards(inputs map[string][]byte,
) (traces []Trace[F], outputs map[string][]byte, errors []error) {
	panic("todo")
}

// Parallel BootAndTrace performs sharding according to the given sharding
// strategy.
func (p TraceBuilder[W, F, T]) checkpointAndTrace(inputs map[string][]byte,
) ([]Trace[F], map[string][]byte, []error) {
	var (
		strategy = p.config.shardingStrategy.Unwrap()
		// fast mode execution to generate checkpoints
		checkpoints, outputs, traceable, errors = BootAndCheckpoint(p.tracing, inputs, strategy)
		//
		traces = make([]Trace[F], len(checkpoints))
	)
	// Sanity check
	if traceable {
		// Perform tracing in parallel (or sequentially)
		results := p.traceCheckPoints(checkpoints)
		// Collapse results and check traceability
		for i, ith := range results {
			errors = append(errors, ith.errors...)
			// Record trace
			traces[i] = ith.trace
			// Record overall traceability
			traceable = traceable && traces[i] != nil
		}
	}
	//
	if !traceable {
		return nil, outputs, errors
	}
	//
	return traces, outputs, errors
}

func (p TraceBuilder[W, F, T]) traceCheckPoints(checkpoints []CheckPoint[W]) (jobs []traceJob[F]) {
	var (
		strategy = p.config.shardingStrategy.Unwrap()
		// Construct tracing function
		traceFn = func(i uint, cp CheckPoint[W]) traceJob[F] {
			var steps = strategy.shardSteps
			// Increment steps for all except first shard to account for the
			// fact that restoring at the exact point the breakpoint was
			// triggered will naturally trigger it again.
			if i != 0 {
				steps++
			}
			// Trace ith shard
			var trace, errs = RestoreAndTraceFor[W, F, T](p.tracing, cp, strategy.shardFunction, steps)
			// Done
			return traceJob[F]{trace, errs}
		}
	)
	//
	if p.config.parallel {
		// Perform tracing in parallel
		return array.ParallelMap(checkpoints, traceFn)
	}
	// Perform tracing sequentially
	return array.Map(checkpoints, traceFn)
}

type traceJob[F Element[F]] struct {
	trace  Trace[F]
	errors []error
}

// ============================================================================
// Helpers
// ============================================================================

// State collects together information recorded when executing a single vector
// instruction.
type State[W any] struct {
	// Fid identifies the executing function (module) which this state belongs to.
	fid uint16
	// Program Counter position.
	pc uint32
	// Terminal indicates this is a terminating state (i.e. whether or not the
	// next instruction to execute was a return).
	terminal bool
	// Values for each register in this state excluding the program counter
	// (since this is held above).
	frame []W
}

// Fid returns the identifier of the function (module) this state belongs to.
func (p State[W]) Fid() uint16 {
	return p.fid
}

// Frame returns frame data stored in this state
func (p State[W]) Frame() []W {
	return p.frame
}

// Width returns with the width of this state.
func (p State[W]) Width() uint {
	return uint(len(p.frame))
}

// PC returns the value of program counter for this state.
func (p State[W]) PC() uint {
	return uint(p.pc)
}

// IsTerminal indicates whether or not this is a "terminal state" for the
// enclosing function.
func (p State[W]) IsTerminal() bool {
	return p.terminal
}
