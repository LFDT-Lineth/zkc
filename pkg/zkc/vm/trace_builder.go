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
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	log "github.com/sirupsen/logrus"
)

// Shard defines a component of a trace.
type Shard[F field.Element[F]] = trace.Shard[F]

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
	Build() trace.Shard[F]
}

// TraceBuilder provides a generic mechanism for tracing a given program, and
// abstracts the myriad different ways this can be done (e.g. sharding,
// parallelism, etc).
type TraceBuilder[W Word[W], F field.Element[F], T Tracer[W, F, T], E Word[E]] struct {
	config      TraceConfig
	builder     ir.TraceBuilder[F]
	execution   Program[E]
	tracing     Program[W]
	constraints air.Schema[F]
}

// NewTraceBuilder constructs a default tracer builder which, most likely,
// should be further configured before use.
func NewTraceBuilder[W Word[W], F Element[F], T Tracer[W, F, T], E Word[E]](config TraceConfig,
	execution Program[E], tracing Program[W], constraints air.Schema[F]) TraceBuilder[W, F, T, E] {
	//
	builder := ir.NewTraceBuilder[F]().
		// NOTE: never use validation, as it hides constraint failures.
		WithValidation(false).
		WithParallelism(false).
		WithExpansion(true).
		WithPadding(config.PaddingStrategy())
		//
	return TraceBuilder[W, F, T, E]{config, builder, execution, tracing, constraints}
}

// BootAndTrace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func (p TraceBuilder[W, F, T, E]) BootAndTrace(inputs map[string][]byte,
) (outputs map[string][]byte, tr util.Option[trace.LazyTrace[F]], errors []error) {
	// Check whether we have a sharding strategy
	if p.config.shardingStrategy.IsEmpty() {
		return p.bootAndTraceUnsharded(inputs)
	}
	// apply sharding strategy
	return p.bootAndTraceShards(inputs)
}

// unsharded BootAndTrace simply traces directly.
func (p TraceBuilder[W, F, T, E]) bootAndTraceUnsharded(inputs map[string][]byte,
) (outputs map[string][]byte, tr util.Option[trace.LazyTrace[F]], errors []error) {
	var (
		shard util.Option[trace.Shard[F]]
	)
	// No strategy, therefore trace sequentially
	shard, outputs, errors = BootAndTrace[W, F, T](p.tracing, inputs)
	// Sanity check whether can continue or not
	if shard.IsEmpty() {
		// No, have an unrecoverable error
		return outputs, util.None[trace.LazyTrace[F]](), errors
	}
	// Convert into a future
	future := func() (util.Option[trace.Shard[F]], []error) {
		// Perform trace expansion
		var shard, errs = p.builder.BuildShard(p.constraints, shard.Unwrap())
		// Repackage
		return util.Some(shard), errs
	}
	// Done
	return outputs, util.Some(trace.NewLazyTrace(future)), errors
}

// Sharded BootAndTrace performs sharding according to the given sharding
// strategy.
func (p TraceBuilder[W, F, T, E]) bootAndTraceShards(inputs map[string][]byte,
) (map[string][]byte, util.Option[trace.LazyTrace[F]], []error) {
	var (
		//
		strategy = p.config.shardingStrategy.Unwrap()
		// fast mode execution to generate checkpoints
		checkpoints, outputs, traceable, errors = BootAndCheckpoint(p.execution, inputs, strategy)
		//
		futures []trace.Future[F]
	)
	// Sanity check
	if !traceable {
		return outputs, util.None[trace.LazyTrace[F]](), errors
	}
	// Perform tracing in parallel (or sequentially)
	futures = p.traceCheckPoints(checkpoints)
	// Done
	return outputs, util.Some(trace.NewLazyTrace(futures...)), errors
}

func (p TraceBuilder[W, F, T, E]) traceCheckPoints(checkpoints []CheckPoint) (jobs []trace.Future[F]) {
	var (
		strategy = p.config.shardingStrategy.Unwrap()
		//
		futures = make([]trace.Future[F], len(checkpoints))
	)
	//
	for i, cp := range checkpoints {
		// Construct ith tracing future
		futures[i] = func() (util.Option[Shard[F]], []error) {
			var steps = strategy.shardSteps
			// Increment steps for all except first shard to account for the
			// fact that restoring at the exact point the breakpoint was
			// triggered will naturally trigger it again.
			if i != 0 {
				steps++
			}
			// Trace ith shard
			steps, oshard, errs1 := RestoreAndTraceFor[W, F, T](p.tracing, cp, strategy.shardFunction, steps)
			// Sanity check what happened
			if oshard.IsEmpty() {
				// Log failure
				log.Error("[SHARD ", i, "] tracing failed (", steps, " steps)")
				return util.None[Shard[F]](), errs1
			}
			// Perform trace expansion
			shard, errs2 := p.builder.BuildShard(p.constraints, oshard.Unwrap())
			// Log stats
			log.Debug("[SHARD ", i, "] traced execution (", steps, " steps)")
			// Done
			return util.Some(shard), append(errs1, errs2...)
		}
	}
	//
	return futures
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
