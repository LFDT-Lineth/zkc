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

// TraceConfig provides the necessary configuration for the trace generation.
type TraceConfig struct {
	// Indicates whether or not to validate all column types.  That is, check
	// that the values supplied for all columns (both input and computed) are
	// within their declared type.
	validate bool
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
}

// DEFAULT_TRACE_CONFIG defines a default configuration for tracing.
var DEFAULT_TRACE_CONFIG = TraceConfig{true, true, 1024}

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
