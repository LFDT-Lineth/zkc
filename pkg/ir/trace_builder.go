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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/ir/builder"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceBuilder provides a mechanical means of constructing a trace from a given
// schema and set of input columns.  The goal is to encapsulate all of the logic
// around building a trace.
type TraceBuilder[F field.Element[F]] struct {
	// Indicates whether or not to perform defensive padding.  This is where
	// padding rows are appended and/or prepended to ensure no constraint in the
	// active region of the trace is clipped.  Whilst not strictly necessary,
	// this can be helpful for identifying invalid constraints which are only
	// exposed with a given amount of padding.
	defensive bool
	// Indicates whether or not to perform trace expansion.  The default should
	// be to apply trace expansion.  However, for testing purposes, it can be
	// useful to provide an already expanded trace to ensure a set of
	// constraints correctly rejects it.
	expand bool
	// Indicates whether or not to validate all column types.  That is, check
	// that the values supplied for all columns (both input and computed) are
	// within their declared type.
	validate bool
	// Indicates whether or not to apply other sanity checks, such as ensuring
	// the number of lines actually added to a trace matches the expected
	// amount.
	checks bool
	// Determines the amount of padding to apply to each module in the trace.
	// At the moment, this is applied uniformly across all modules.  This is
	// somewhat cumbersome, and it would make sense to support different
	// protocols.  For example, one obvious protocol is to expand a module's
	// length upto a power-of-two.
	padding uint
	// Determines whether or not trace expansion should be performed in
	// parallel.  This should be the default, but a sequential option is
	// retained for debugging purposes.
	parallel bool
	// Specify the maximum size of any dispatched batch.
	batchSize uint
	// Mapping specifies whether or not columns in the trace need to be split to
	// match the given field configuration.
	mapping module.LimbsMap
}

// NewTraceBuilder constructs a default trace builder.  The idea is that this
// could then be customized as needed following the builder pattern.
func NewTraceBuilder[F field.Element[F]]() TraceBuilder[F] {
	return TraceBuilder[F]{true, true, true, true, 0, true, math.MaxUint, nil}
}

// WithDefensivePadding updates a given builder configuration to apply defensive padding
// (or not).
func (tb TraceBuilder[F]) WithDefensivePadding(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.defensive = flag
	//
	return ntb
}

// WithExpansionChecks enables runtime safety checks on the expanded trace.
func (tb TraceBuilder[F]) WithExpansionChecks(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.checks = flag
	//
	return ntb
}

// WithExpansion updates a given builder configuration to perform trace expansion (or
// not).
func (tb TraceBuilder[F]) WithExpansion(flag bool) TraceBuilder[F] {
	ntb := tb
	ntb.expand = flag
	//
	return ntb
}

// WithRegisterMapping updates a given builder configuration to split the trace
// according to a given mapping of registers.
func (tb TraceBuilder[F]) WithRegisterMapping(mapping module.LimbsMap) TraceBuilder[F] {
	ntb := tb
	ntb.mapping = mapping
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

// WithPadding updates a given builder configuration to use a given amount of padding
func (tb TraceBuilder[F]) WithPadding(padding uint) TraceBuilder[F] {
	ntb := tb
	ntb.padding = padding
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

// Mapping returns the mapping from registers to limbs used with this builder.
func (tb TraceBuilder[F]) Mapping() module.LimbsMap {
	return tb.mapping
}

// Build attempts to construct a trace for a given schema, producing errors if
// there are inconsistencies (e.g. missing columns, duplicate columns, etc).
func (tb TraceBuilder[F]) Build(schema sc.AnySchema[F], tf lt.TraceFile) (trace.Trace[F], []error) {
	var (
		arrBuilder array.Builder[F]
		modules    []lt.Module[F]
		errors     []error
	)
	// If expansion is enabled, then we must split the trace according to the
	// given mapping; otherwise, we simply lower the trace as is.
	if tb.mapping != nil && tb.expand {
		// Split raw columns, and handle any errors arising.
		arrBuilder, modules, errors = builder.TraceSplitting[F](tb.parallel, tf, tb.mapping)
		// Sanity check for errors
		if len(errors) > 0 {
			return nil, errors
		}
	} else {
		// Lower raw columns
		arrBuilder, modules = builder.TraceLowering[F](tb.parallel, tf)
	}
	// Apply trace alignment to after lowering.
	if modules, errors = AlignTrace(schema.Modules().Collect(), modules, tb.expand); len(errors) > 0 {
		return nil, errors
	}
	// Initialise the actual trace object
	tr := initialiseTrace(schema, arrBuilder, modules)
	//
	if tb.expand {
		// Save original line counts
		moduleHeights := determineModuleHeights(tr)
		// Apply spillage
		addSpillageAndDefensivePadding(tb.defensive, tr, schema)
		// Sanity checks
		if tb.checks {
			if err := checkModuleHeights(moduleHeights, tb.defensive, tr, schema); err != nil {
				return nil, append(errors, err)
			}
		}
		// Expand trace
		if err := builder.TraceExpansion(tb.parallel, tb.batchSize, schema, tr); err != nil {
			return nil, append(errors, err)
		}
		// Validate expanded trace
		if tb.validate {
			// Run (parallel) trace validation
			if errs := builder.TraceValidation(tb.parallel, schema, tr); len(errs) > 0 {
				return nil, errs
			}
		}
	}
	// Padding
	if tb.padding > 0 {
		padColumns(tr, schema, tb.padding)
	}
	//
	return tr, errors
}

func initialiseTrace[F field.Element[F]](schema sc.AnySchema[F], pool array.Builder[F], rawTrace []lt.Module[F],
) *trace.ArrayTrace[F] {
	//
	var modules = make([]trace.ArrayModule[F], schema.Width())
	//
	for i := uint(0); i != schema.Width(); i++ {
		var mod = schema.Module(i)
		//
		modules[i] = fillTraceModule(mod, rawTrace[i])
	}
	// Done
	return trace.NewArrayTrace(pool, modules)
}

func fillTraceModule[F field.Element[F]](mod sc.Module[F], rawModule lt.Module[F]) trace.ArrayModule[F] {
	var (
		traceColumns = make([]trace.ArrayColumn[F], mod.Width())
	)
	//
	for i := range traceColumns {
		var (
			data    array.MutArray[F]
			reg     = mod.Register(register.NewId(uint(i)))
			padding F
		)
		//
		if i < len(rawModule.Columns) {
			data = rawModule.Columns[i].MutData()
		}
		// Set padding for this column
		padding = padding.SetBytes(reg.Padding().Bytes())
		//
		traceColumns[i] = trace.NewArrayColumn(reg.Name(), data, padding)
	}
	//
	return trace.NewArrayModule(mod.Name(), mod.Keys(), traceColumns)
}

// pad each module with its given level of spillage and (optionally) ensure a
// given level of defensive padding.
func addSpillageAndDefensivePadding[F field.Element[F]](defensive bool, tr *trace.ArrayTrace[F],
	schema sc.AnySchema[F]) {
	//
	n := tr.Modules().Count()
	// Iterate over modules
	for i := uint(0); i < n; i++ {
		// Compute extra padding rows required
		padding := sc.RequiredPaddingRows(i, defensive, schema)
		// Don't pad unless we have to
		if padding > 0 {
			// Pad extract rows with 0
			tr.Pad(i, padding, 0)
		}
	}
}

// determineModuleHeights returns the height for each module in the trace.
func determineModuleHeights[F field.Element[F]](tr *trace.ArrayTrace[F]) []uint {
	n := tr.Modules().Count()
	mid := 0
	heights := make([]uint, n)
	// Iterate over modules
	for iter := tr.Modules(); iter.HasNext(); {
		ith := iter.Next()
		heights[mid] = ith.Height()
		mid++
	}
	//
	return heights
}

// checkModuleHeights checks the expanded heights match exactly what was
// expected.
func checkModuleHeights[F field.Element[F]](original []uint, defensive bool, tr *trace.ArrayTrace[F],
	schema sc.AnySchema[F]) error {
	//
	expanded := determineModuleHeights(tr)
	//
	for mid := uint(0); mid < uint(len(expanded)); mid++ {
		spillage := sc.RequiredPaddingRows(mid, defensive, schema)
		expected := original[mid] + spillage
		// Perform the check
		if expected != expanded[mid] {
			name := schema.Module(mid).Name()
			//
			return fmt.Errorf(
				"inconsistent expanded trace height for %s (was %d but expected %d)", name, expanded[mid], expected)
		}
	}
	//
	return nil
}

// PadColumns pads every column in a given trace with a given amount of (front)
// padding. Observe that this applies on top of any spillage and/or defensive
// padding already applied.
func padColumns[F field.Element[F]](tr *trace.ArrayTrace[F], schema sc.AnySchema[F], padding uint) {
	n := tr.Modules().Count()
	// Iterate over modules
	for i := uint(0); i < n; i++ {
		multiplier := schema.Module(i).Name().Multiplier
		tr.Pad(i, padding*multiplier, 0)
	}
}
