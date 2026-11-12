// Copyright Consensys Incorporated
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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

// ShardingStrategy is used to configure parallel tracing.
type ShardingStrategy = descriptor.ShardingStrategy

// NewShardingStrategy constructs a simple strategy which splits the trace into
// some number of shards, where each shard represents at most n executions of
// the given function.
func NewShardingStrategy(fun string, n uint64) ShardingStrategy {
	return ShardingStrategy{ShardFunction: fun, ShardSteps: n}
}

// CheckPointer extends a given sharding strategy with a given function
// to be called on each checkpoint.
type CheckPointer struct {
	ShardingStrategy
	//
	checkpoint func(checkpoint.CheckPoint)
}

// NewCheckPointer associates a checkpointing function with a given sharding strategy.
func NewCheckPointer(strategy ShardingStrategy, handler func(checkpoint.CheckPoint)) CheckPointer {
	return CheckPointer{strategy, handler}
}

// ============================================================================
// ExecutionConfig
// ============================================================================

// ExecutionConfig provides necessary configuration options for fast-mode execution.
type ExecutionConfig struct {
	// Specified whether or not to use gogen
	gogen bool
	// Specifies whether or not to use a sharding strategy.
	strategy util.Option[CheckPointer]
}

// DEFAULT_EXECUTION_CONFIG defines a default configuration for fast-mode execution.
var DEFAULT_EXECUTION_CONFIG = ExecutionConfig{
	gogen: false,
}

// Checkpointing determines whether a given checkpoint function should be
// applied (or not).
func (tb ExecutionConfig) Checkpointing() util.Option[CheckPointer] {
	return tb.strategy
}

// GoGen determines whether (or not) gogen should be used for fast-mode
// execution.
func (tb ExecutionConfig) GoGen() bool {
	return tb.gogen
}

// WithGoGen updates the configuration given to enable (or disable) gogen for
// fast-mode execution.
func (tb ExecutionConfig) WithGoGen(flag bool) ExecutionConfig {
	tb.gogen = flag
	//
	return tb
}

// WithCheckpointing updates the configuration to employ a specific checking
// strategy.  Otherwise, by default, no sharding is performed.
func (tb ExecutionConfig) WithCheckpointing(strategy CheckPointer) ExecutionConfig {
	tb.strategy = util.Some(strategy)
	//
	return tb
}

// ============================================================================
// TraceConfig
// ============================================================================

// TraceConfig provides the necessary configuration for the trace generation.
type TraceConfig struct {
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
	// Specified whether or not to use gogen
	gogen bool
	// Determines how much front padding is added to the generated trace (see
	// ir.TraceBuilder.WithPadding).
	paddingStrategy ir.PaddingStrategy
	// Specifies whether or not to use a sharding strategy.
	shardingStrategy util.Option[ShardingStrategy]
}

// DEFAULT_TRACE_CONFIG defines a default configuration for tracing.
var DEFAULT_TRACE_CONFIG = TraceConfig{
	parallel: true, batchSize: 1024, gogen: false, paddingStrategy: ir.NextPowerOfTwoPadding}

// WithParallelism updates a given builder configuration to allow trace expansion to be
// performed concurrently (or not).
func (tb TraceConfig) WithParallelism(flag bool) TraceConfig {
	tb.parallel = flag
	//
	return tb
}

// WithBatchSize sets the maximum number of batches to run in parallel during trace
// expansion.
func (tb TraceConfig) WithBatchSize(batchSize uint) TraceConfig {
	tb.batchSize = batchSize
	//
	return tb
}

// GoGen determines whether (or not) gogen should be used for fast-mode
// execution.
func (tb TraceConfig) GoGen() bool {
	return tb.gogen
}

// WithGoGen updates the configuration given to enable (or disable) gogen for
// fast-mode execution.
func (tb TraceConfig) WithGoGen(flag bool) TraceConfig {
	tb.gogen = flag
	//
	return tb
}

// WithPadding updates the trace configuration to control how much front padding
// is added to the generated trace (see ir.PaddingStrategy).
func (tb TraceConfig) WithPadding(strategy ir.PaddingStrategy) TraceConfig {
	tb.paddingStrategy = strategy
	//
	return tb
}

// WithSharding updates the trace configuration to employ a specific sharding
// strategy.  Otherwise, by default, no sharding is performed.
func (tb TraceConfig) WithSharding(strategy ShardingStrategy) TraceConfig {
	tb.shardingStrategy = util.Some(strategy)
	//
	return tb
}

// PaddingStrategy returns the padding strategy configured for this builder.
func (tb TraceConfig) PaddingStrategy() ir.PaddingStrategy {
	return tb.paddingStrategy
}

// Parallelism checks whether parallelism is enabled for this builder.
func (tb TraceConfig) Parallelism() bool {
	return tb.parallel
}

// BatchSize returns the configured batch size for this builder.
func (tb TraceConfig) BatchSize() uint {
	return tb.batchSize
}
