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
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
)

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
func AlignTrace[F any, M register.Map](schema []M, trace []lt.Module[F], expanding bool,
) ([]lt.Module[F], []error) {
	//
	var errors []error
	// First, align modules
	if trace, errors = alignModules(schema, trace, expanding); len(errors) > 0 {
		return nil, errors
	}
	// Second, align columns within modules
	for i, m := range schema {
		cols, errs := alignColumns(m, trace[i].Columns, expanding)
		errors = append(errors, errs...)
		trace[i].Columns = cols
	}
	// Done
	return trace, errors
}

func alignModules[F any, M register.Map](schema []M, mods []lt.Module[F], expanding bool) ([]lt.Module[F], []error) {
	//
	var (
		width  = uint(len(schema))
		modmap = make(map[module.Name]uint)
		nmods  = make([]lt.Module[F], width)
		errs   []error
	)
	// Initialise module mapping
	for i := range width {
		ith := schema[i].Name()
		nmods[i] = lt.NewModule[F](ith, nil)
		modmap[ith] = i
	}
	// Rearrange layout
	for _, m := range mods {
		if index, ok := modmap[m.Name()]; ok {
			nmods[index] = m
		} else if expanding {
			errs = append(errs, fmt.Errorf("unknown module '%s' in trace", m.Name()))
		}
	}
	//
	return nmods, errs
}

func alignColumns[F any](mapping register.Map, columns []lt.Column[F], expanding bool) ([]lt.Column[F], []error) {
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
		ncols = make([]lt.Column[F], width)
	)
	// Initialise column map
	for i := range width {
		ith := mapping.Register(register.NewId(i))
		ncols[i] = lt.NewColumn[F](ith.Name(), nil)
		colmap[ith.Name()] = i
	}
	// Assign data for each column given
	for _, col := range columns {
		var (
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
			ncols[cid] = col
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
	return ncols, errs
}
