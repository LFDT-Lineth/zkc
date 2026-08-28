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
package ir

import (
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/ir/builder"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceBuilder provides a mechanical means of constructing a trace from a given
// schema and set of input columns.  The goal is to encapsulate all of the logic
// around building a trace.
type TraceBuilder[F field.Element[F]] struct {
	// Indicates whether or not to perform trace expansion.  The default should
	// be to apply trace expansion.  However, for testing purposes, it can be
	// useful to provide an already expanded trace to ensure a set of
	// constraints correctly rejects it.
	expand bool
	// Indicates whether or not to validate all column types.  That is, check
	// that the values supplied for all columns (both input and computed) are
	// within their declared type.
	validate bool
	// Determines how much front padding is applied to each module in the
	// trace.  See PaddingStrategy for the available strategies.
	paddingStrategy PaddingStrategy
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
}

// NewTraceBuilder constructs a default trace builder.  The idea is that this
// could then be customized as needed following the builder pattern.
func NewTraceBuilder[F field.Element[F]]() TraceBuilder[F] {
	return TraceBuilder[F]{true, true, NextPowerOfTwoPadding, true, math.MaxUint}
}

// WithExpansion updates a given builder configuration to perform trace expansion (or
// not).
func (tb TraceBuilder[F]) WithExpansion(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.expand = flag
	//
	return ntb
}

// WithValidation updates a given builder configuration to perform trace validation (or
// not).
func (tb TraceBuilder[F]) WithValidation(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.validate = flag
	//
	return ntb
}

// WithPadding updates a given builder configuration to control padding.
func (tb TraceBuilder[F]) WithPadding(strategy PaddingStrategy) TraceBuilder[F] {
	ntb := tb
	ntb.paddingStrategy = strategy
	//
	return ntb
}

// WithParallelism updates a given builder configuration to allow trace expansion to be
// performed concurrently (or not).
func (tb TraceBuilder[F]) WithParallelism(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.parallel = flag
	//
	return ntb
}

// Parallelism checks whether parallelism is enabled for this builder.
func (tb TraceBuilder[F]) Parallelism() bool {
	return tb.parallel
}

// WithBatchSize sets the maximum number of batches to run in parallel during trace
// expansion.
func (tb TraceBuilder[F]) WithBatchSize(batchSize uint) TraceBuilder[F] {
	ntb := tb
	ntb.batchSize = batchSize
	//
	return ntb
}

// Expanding indicates whether or not this builder will expand the trace.
func (tb TraceBuilder[F]) Expanding() bool {
	return tb.expand
}

// BatchSize returns the configured batch size for this builder.
func (tb TraceBuilder[F]) BatchSize() uint {
	return tb.batchSize
}

// Build attempts to construct a trace for a given schema, producing errors if
// there are inconsistencies (e.g. missing columns, duplicate columns, etc).
func (tb TraceBuilder[F]) Build(schema sc.AnySchema[F], tf trace.Trace[F]) (tr trace.Trace[F], errs []error) {
	var (
		shards = make([]trace.Shard[F], len(tf))
		errors = make([][]error, len(tf))
		// Trace Expander function
		expandFn = func(i uint, shard trace.Shard[F]) {
			shards[i], errors[i] = tb.buildShard(schema, i, shard)
		}
	)
	// Build the trace (using parallelism if requested).
	if tb.parallel {
		array.ParallelApply(tf, expandFn)
	} else {
		array.Apply(tf, expandFn)
	}
	// Flattern errors
	return shards, array.FlatMap(errors, func(es []error) []error { return es })
}

func (tb TraceBuilder[F]) buildShard(schema sc.AnySchema[F], shard uint, tf trace.Shard[F],
) (tr trace.Shard[F], errs []error) {
	//
	var (
		atr builder.ArrayTrace[F]
		//
		config = builder.Config{
			Parallel:  false,
			BatchSize: tb.batchSize,
			Expanding: tb.expand,
			Padding:   tb.paddingStrategy,
		}
	)
	// Apply trace alignment and padding
	if atr, errs = builder.AlignAndPad(config, schema, tf); len(errs) > 0 {
		return nil, errs
	}
	// Apply trace expansion (if requested)
	if tb.expand {
		// Expand trace
		if err := builder.TraceExpansion(config, schema, atr); err != nil {
			return nil, append(errs, err)
		}
		// Validate expanded trace
		if tb.validate {
			// Run (parallel) trace validation
			if errs := builder.TraceValidation(config, schema, atr); len(errs) > 0 {
				return nil, errs
			}
		}
	}
	//
	return atr, errs
}
