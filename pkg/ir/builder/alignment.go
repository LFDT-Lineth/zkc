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
package builder

import (
	"fmt"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// PaddingStrategy captures the notion of an algorithm that determines how much front padding is added to each module
// when expanding a trace (see TraceBuilder.WithPadding).
type PaddingStrategy func(height, multiplier uint) uint

// Config provides some configuration parameters for the various top-level
// builder functions.
type Config struct {
	// Parallel indicates whether or not to use parallelisation when possible.
	Parallel bool
	// BatchSize indicates size of assignment batches to use when parallelising.
	BatchSize uint
	// Expanding indicates whether or not traces are being actively expanded (or
	// not).
	Expanding bool
	// Padding indicates the padding strategy to use.
	Padding PaddingStrategy
}

// AlignAndPad performs "trace alignment" on a given trace file.  That is, it
// ensures: firstly, the order in which modules occur in the trace file matches
// (i.e. aligns with) those in the given schema; secondly, it ensures that the
// columns within each module match (i.e. align with) those of the corresponding
// schema module.  If any columns or modules are missing, then one or more
// errors will be reported.
//
// NOTE: alignment is impacted by whether or not the trace is being expanded or
// not. Specifically, expanding traces don't need to include data for computed
// columns, since these will be added during expansion.
func AlignAndPad[F field.Element[F]](config Config, schema sc.AnySchema[F], tr trace.Shard[F],
) (trace.Shard[F], []error) {
	//
	var (
		errors  []error
		modules = make([]trace.Module[F], schema.Width())
		modmap  = make(map[string]uint)
		seen    = make([]bool, tr.Width())
	)
	// Initialise module map
	for i := range tr.Width() {
		modmap[tr.Module(i).Name()] = i
	}
	// Align modules one-by-one
	for i := range schema.Width() {
		var (
			errs  []error
			trMod trace.Module[F]
			scMod = schema.Module(i)
		)
		if index, ok := modmap[scMod.Name()]; ok {
			trMod = tr.Module(index)
			// Mark module as seen
			seen[index] = true
		} else {
			// Module is entirely absent from the trace.  This is not
			// necessarily an error: e.g. a module with no input/output
			// registers (or, for an expanding trace, one whose registers are
			// all computed) is legitimately allowed to have no presence in
			// the trace at all.  Any genuinely missing data is detected
			// below, on a column-by-column basis.
			trMod = trace.NewModule[F](trace.NewModuleDescriptor(scMod.Name(), nil))
		}
		// Align trace
		modules[i], errs = alignModule(config, scMod, trMod)
		// Append any errors arising.
		errors = append(errors, errs...)
	}
	// Sanity check for any modules seen in the trace, but not in the schema.
	for i, b := range seen {
		var ith = tr.Module(uint(i))
		//
		if !b {
			errors = append(errors, fmt.Errorf("unknown module '%s' in trace", ith.Name()))
		}
	}
	// Pad modules as required
	padded, errs := padModules(config, schema, modules)
	modules = padded
	//
	errors = append(errors, errs...)
	// Done
	return trace.NewShard(modules), errors
}

func alignModule[F field.Element[F]](config Config, scMod sc.Module[F], trMod trace.Module[F],
) (trace.Module[F], []error) {
	var (
		errors []error
		width  = uint(len(scMod.Registers()))
		// Height is used to sanity check the height of all columns in this
		// modules to ensure they are consistent.
		height      uint
		descriptors = make([]trace.ColumnDescriptor, width)
		columns     = make([]array.Array[F], width)
		regmap      = make(map[string]uint)
		seen        = make([]bool, trMod.Width())
	)
	// Initialise column map and descriptors.  NOTE: every register of the schema
	// gets a descriptor, regardless of whether the corresponding column is
	// actually present in the trace.  This matters for those which are not
	// (e.g. a computed column, or a column of a static reference table), since
	// they are otherwise left nameless.
	for i := range width {
		var r = scMod.Register(register.NewId(i))
		//
		regmap[r.Name()] = i
		//
		if r.IsNative() {
			descriptors[i] = trace.NewColumnDescriptor(r.Name(), util.None[uint]())
		} else {
			descriptors[i] = trace.NewColumnDescriptor(r.Name(), util.Some(r.Width()))
		}
	}
	// Align columns one-by-one
	for i := range trMod.Width() {
		var (
			errs []error
			ith  = trMod.Descriptor().Columns[i]
			// Lookup column in regmap
			cid, ok = regmap[ith.Name]
		)
		// More sanity checks
		if !ok {
			errs = append(errs, fmt.Errorf("unknown column '%s' in module '%s' of trace", ith.Name, trMod.Name()))
		} else if ok := seen[cid]; ok {
			errs = append(errs, fmt.Errorf("duplicate column '%s' in module '%s' of trace", ith.Name, trMod.Name()))
		} else {
			// Clone underlying data
			columns[cid] = trMod.Column(i)
			// Mark column as seen
			seen[cid] = true
			// Update maximum height
			height = max(height, columns[cid].Len())
		}
		// Append any errors arising.
		errors = append(errors, errs...)
	}
	// Sanity check everything we expected was assigned
	for i := range width {
		var (
			reg = scMod.Register(register.NewId(i))
			col = columns[i]
		)
		//
		if reg.IsInputOutput() && col == nil && height != 0 {
			errors = append(errors,
				fmt.Errorf("missing input/output column '%s' from module '%s' of trace", reg.Name(), scMod.Name()))
		} else if !config.Expanding && col == nil {
			errors = append(errors,
				fmt.Errorf("missing computed column '%s' from module '%s' of expanded trace", reg.Name(), scMod.Name()))
		}
	}
	// Done
	return trace.NewModule(trace.NewModuleDescriptor(scMod.Name(), descriptors), columns...), errors
}
