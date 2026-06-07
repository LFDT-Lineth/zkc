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
package constraints

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// TraceConfig provides the necessary configuration for the trace generation.
type TraceConfig struct {
	// Indicates whether or not to perform defensive padding.  This is where
	// padding rows are appended and/or prepended to ensure no constraint in the
	// active region of the trace is clipped.  Whilst not strictly necessary,
	// this can be helpful for identifying invalid constraints which are only
	// exposed with a given amount of padding.
	defensive bool
	// Indicates whether or not to validate all column types.  That is, check
	// that the values supplied for all columns (both input and computed) are
	// within their declared type.
	validate bool
	// Indicates whether or not to apply other sanity checks, such as ensuring
	// the number of lines actually added to a trace matches the expected
	// amount.
	checks bool
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
}

// DEFAULT_TRACE_CONFIG defines a default configuration for tracing.
var DEFAULT_TRACE_CONFIG = TraceConfig{true, true, true, true, 1024}

// WithDefensivePadding updates a given builder configuration to apply defensive padding
// (or not).
func (tb TraceConfig) WithDefensivePadding(flag bool) TraceConfig {
	ntb := tb
	ntb.defensive = flag
	//
	return ntb
}

// WithExpansionChecks enables runtime safety checks on the expanded trace.
func (tb TraceConfig) WithExpansionChecks(flag bool) TraceConfig {
	ntb := tb
	ntb.checks = flag
	//
	return ntb
}

// WithValidation updates a given builder configuration to perform trace validation (or
// not).
func (tb TraceConfig) WithValidation(flag bool) TraceConfig {
	ntb := tb
	ntb.validate = flag
	//
	return ntb
}

// WithParallelism updates a given builder configuration to allow trace expansion to be
// performed concurrently (or not).
func (tb TraceConfig) WithParallelism(flag bool) TraceConfig {
	ntb := tb
	ntb.parallel = flag
	//
	return ntb
}

// WithBatchSize sets the maximum number of batches to run in parallel during trace
// expansion.
func (tb TraceConfig) WithBatchSize(batchSize uint) TraceConfig {
	ntb := tb
	ntb.batchSize = batchSize
	//
	return ntb
}

// Parallelism checks whether parallelism is enabled for this builder.
func (tb TraceConfig) Parallelism() bool {
	return tb.parallel
}

// BatchSize returns the configured batch size for this builder.
func (tb TraceConfig) BatchSize() uint {
	return tb.batchSize
}

// Trace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func Trace[F field.Element[F]](bf *BinaryFile[F], in map[string][]vm.Uint, cfg TraceConfig) (trace.Trace[F], []error) {
	var (
		observer vm.TraceObserver[vm.Uint, vm.WordInstruction, *vm.WordMachine[vm.Uint]]
		stats    = util.NewPerfStats()
		errs     []error
		tr       trace.Trace[F]
	)
	// Initialise inputs
	for iter := bf.machine.Inputs(); iter.HasNext(); {
		var input = iter.Next()
		input.Initialise(in[input.Name()])
	}
	// Execute machine
	if err := bf.machine.Boot("main"); err != nil {
		errs = append(errs, err)
	} else if _, err := vm.ExecuteAndObserve(&bf.machine, 1, &observer); err != nil {
		errs = append(errs, err)
	} else {
		// Extract AIR constraints
		constraints := bf.AirConstraints()
		// Construct trace builder
		builder := ir.NewTraceBuilder[F]().
			WithValidation(cfg.validate).
			WithDefensivePadding(cfg.defensive).
			WithExpansionChecks(cfg.checks).
			WithExpansion(true).
			WithParallelism(cfg.parallel).
			WithBatchSize(cfg.batchSize)
		// Build the trace (finally)
		tr, errs = builder.Build(constraints, observer.Trace(&bf.machine))
	}
	//
	stats.Log("Trace generation")
	// Done
	return tr, errs
}
