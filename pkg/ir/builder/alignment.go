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

	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/narray"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ArrayModule provides a convenient alias.
type ArrayModule[F field.Element[F]] = *rtrace.CompactModule[F]

// ArrayTrace provides a convenient alias.
type ArrayTrace[F field.Element[F]] = *rtrace.Array[F, ArrayModule[F]]

// AlignTrace performs "trace alignment" on a given trace file.  That is, it
// ensures: firstly, the order in which modules occur in the trace file matches
// (i.e. aligns with) those in the given schema; secondly, it ensures that the
// columns within each module match (i.e. align with) those of the corresponding
// schema module.  If any columns or modules are missing, then one or more
// errors will be reported.
//
// NOTE: alignment is impacted by whether or not the trace is being expanded or
// not. Specifically, expanding traces don't need to include data for computed
// columns, since these will be added during expansion.
func AlignTrace[F field.Element[F], M register.Map](schema []M, tr rtrace.Trace[F], expanding bool,
) (ArrayTrace[F], []error) {
	//
	var (
		errors  []error
		modules = make([]ArrayModule[F], len(schema))
		modmap  = make(map[string]uint)
		seen    = make([]bool, tr.Width())
	)
	// Initialise module map
	for i := range tr.Width() {
		modmap[tr.Module(i).Name()] = i
	}
	// Align modules one-by-one
	for i, m := range schema {
		var errs []error
		if index, ok := modmap[m.Name()]; ok {
			modules[i], errs = alignModule(m, tr.Module(index), expanding)
			// Mark module as seen
			seen[index] = true
		} else if expanding {
			errs = []error{fmt.Errorf("missing module '%s' in trace", m.Name())}
		}
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
	// Done
	return rtrace.NewArray(modules), errors
}

func alignModule[F field.Element[F], M register.Map](scMod M, trMod rtrace.Module[F], expanding bool,
) (ArrayModule[F], []error) {
	var (
		errors []error
		width  = uint(len(scMod.Registers()))
		// Height is used to sanity check the height of all columns in this
		// modules to ensure they are consistent.
		height      uint
		descriptors = make([]rtrace.ColumnDescriptor, width)
		columns     = make([]narray.MutArray[F], width)
		regmap      = make(map[string]uint)
		seen        = make([]bool, trMod.Width())
	)
	// Initialise column map
	for i := range width {
		var rid = register.NewId(i)
		//
		regmap[scMod.Register(rid).Name()] = i
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
			var r = scMod.Register(register.NewId(cid))
			//
			if r.IsNative() {
				descriptors[i] = rtrace.NewColumnDescriptor(r.Name(), util.None[uint]())
			} else {
				descriptors[i] = rtrace.NewColumnDescriptor(r.Name(), util.Some(r.Width()))
			}
			// Clone underlying data
			columns[cid] = trMod.Column(i).Clone()
			// Mark column as seen
			seen[cid] = true
			// Update maximum height
			height = max(height, columns[i].Len())
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
		} else if !expanding && col == nil {
			errors = append(errors,
				fmt.Errorf("missing computed column '%s' from module '%s' of expanded trace", reg.Name(), scMod.Name()))
		}
	}
	// Done
	return rtrace.NewCompactModule(rtrace.NewModuleDescriptor(scMod.Name(), descriptors), columns...), errors
}
