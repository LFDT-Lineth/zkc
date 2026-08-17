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

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// keyedRegisterMap extends register.Map with the number of key columns
// associated with a module, so that alignment can preserve Module.Keys()
// through to the resulting trace module.
type keyedRegisterMap interface {
	register.Map
	// Keys returns the number of key columns in this module.
	Keys() uint
}

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
func AlignTrace[F field.Element[F], M keyedRegisterMap](schema []M, tr trace.Trace[F], expanding bool,
) (*trace.ArrayTrace[F], []error) {
	//
	var (
		errors   []error
		errs     []error
		omodules []trace.Module[F]
		modules  = make([]trace.ArrayModule[F], len(schema))
		builder  = array.NewStaticBuilder[F]()
	)
	// First, align modules
	if omodules, errors = alignModules(schema, tr, expanding); len(errors) > 0 {
		return nil, errors
	}
	// Second, align columns within modules
	for i, m := range schema {
		modules[i], errs = alignColumns(m, omodules[i], expanding)
		errors = append(errors, errs...)
	}
	// Done
	return trace.NewArrayTrace(builder, modules), errors
}

func alignModules[F field.Element[F], M keyedRegisterMap](schema []M, tr trace.Trace[F], expanding bool,
) ([]trace.Module[F], []error) {
	//
	var (
		width  = uint(len(schema))
		modmap = make(map[module.Name]uint)
		nmods  = make([]trace.Module[F], width)
		errs   []error
	)
	// Initialise module mapping
	for i := range width {
		ith := schema[i].Name()
		nmods[i] = trace.NewArrayModule[F](ith, schema[i].Keys(), nil)
		modmap[ith] = i
	}
	// Rearrange layout
	for _, m := range tr.Modules().Collect() {
		if index, ok := modmap[m.Name()]; ok {
			nmods[index] = m
		} else if expanding {
			errs = append(errs, fmt.Errorf("unknown module '%s' in trace", m.Name()))
		}
	}
	//
	return nmods, errs
}

func alignColumns[F field.Element[F]](mapping keyedRegisterMap, module trace.Module[F], expanding bool,
) (trace.ArrayModule[F], []error) {
	var (
		// Errs contains the set of filling errors which are accumulated
		errs  []error
		width = uint(len(mapping.Registers()))
		// Height is used to sanity check the height of all columns in this
		// modules to ensure they are consistent.
		height uint
		// isEmpty is used to determine whether or not this is an "empty
		// module".  This is one which did not actually feature in the trace.
		isEmpty bool = true
		//
		colmap = make(map[string]uint, width)
		seen   = make([]bool, width)
		//
		ncols = make([]trace.ArrayColumn[F], width)
	)
	// Initialise column map
	for i := range width {
		var (
			ith     = mapping.Register(register.NewId(i))
			padding F
		)

		padding = padding.SetBytes(ith.Padding().Bytes())
		ncols[i] = trace.NewArrayColumn(ith.Name(), nil, padding)
		colmap[ith.Name()] = i
	}
	// Assign data for each column given
	for i := range module.Width() {
		var (
			col  = module.Column(i)
			data = col.Data()
			// Determine enclosiong module height
			cid, ok = colmap[col.Name()]
		)
		// More sanity checks
		if !ok {
			errs = append(errs, fmt.Errorf("unknown column '%s' in trace", tr.QualifiedColumnName(mapping.Name(), col.Name())))
		} else if ok := seen[cid]; ok {
			errs = append(errs, fmt.Errorf("duplicate column '%s' in trace", tr.QualifiedColumnName(mapping.Name(), col.Name())))
		} else {
			seen[cid] = true
			// NOTE: preserve the padding value already established above
			// (from the register's declared padding), rather than the raw
			// column's own padding (which is always zero for freshly parsed
			// input traces).
			ncols[cid] = trace.NewArrayColumn(col.Name(), col.Data().Clone(), ncols[cid].Padding())
			// Update height
			if isEmpty && data != nil {
				height = data.Len()
				isEmpty = false
			} else if data != nil && data.Len() != height {
				name := tr.QualifiedColumnName(mapping.Name(), col.Name())
				errs = append(errs,
					fmt.Errorf("inconsistent height for column '%s' in trace (was %d vs %d)", name, data.Len(), height))
			}
		}
	}
	// Sanity check everything we expected was assigned
	for i := range width {
		var (
			reg = mapping.Register(register.NewId(i))
			col = ncols[i]
		)
		//
		if reg.IsInputOutput() && col.Data() == nil && !isEmpty {
			name := tr.QualifiedColumnName(mapping.Name(), reg.Name())
			errs = append(errs, fmt.Errorf("missing input/output column '%s' from trace", name))
		} else if !expanding && col.Data() == nil {
			name := tr.QualifiedColumnName(mapping.Name(), reg.Name())
			errs = append(errs, fmt.Errorf("missing computed column '%s' from expanded trace", name))
		}
	}
	//
	return trace.NewArrayModule(module.Name(), mapping.Keys(), ncols), errs
}
