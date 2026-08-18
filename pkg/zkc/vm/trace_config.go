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
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// ShardingStrategy is used to configure parallel tracing.
type ShardingStrategy struct {
	// Function around which sharding pivots.  That is, the function for which
	// checkpoints are made.
	shardFunction string
	// Maximum executions of the given function to execute in each shard.
	shardSteps uint64
}

// NewShardingStrategy constructs a simple strategy which splits the trace into
// some number of shards, where each shard represents at most n executions of
// the given function.
func NewShardingStrategy(fun string, n uint64) ShardingStrategy {
	return ShardingStrategy{fun, n}
}

// TraceConfig provides the necessary configuration for the trace generation.
type TraceConfig struct {
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
	// Determines how much front padding is added to the generated trace (see
	// ir.TraceBuilder.WithPadding).
	paddingStrategy ir.PaddingStrategy
	// Specifies whether or not to use a sharding strategy.
	shardingStrategy util.Option[ShardingStrategy]
}

// DEFAULT_TRACE_CONFIG defines a default configuration for tracing.
var DEFAULT_TRACE_CONFIG = TraceConfig{
	parallel: true, batchSize: 1024, paddingStrategy: ir.NextPowerOfTwoPadding}

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
