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

// TraceValidation validates that values held in trace columns match the
// expected type.  This is really a sanity check that the trace is not
// malformed.
func TraceValidation[F field.Element[F]](config Config, schema sc.AnySchema[F], tr trace.Trace[F]) []error {
	var (
		errors []error
		// Start timer
		stats = util.NewPerfStats()
		// Flatten all columns
		columns, errs = flattenTrace(schema, tr)
		// Mapping function
		mapfn = func(_ uint, c trace.ColumnRef) error {
			var (
				smod = schema.Module(c.Module())
				tmod = tr.Module(c.Module())
				rid  = c.Column()
			)

			return validateColumnBitWidth(rid, smod, tmod)
		}
	)
	//
	if config.Parallel {
		// Run parallel map
		errors = array.ParallelMap(columns, mapfn)
	} else {
		// Run sequential map
		errors = array.Map(columns, mapfn)
	}
	// Filter our any nil errors
	errors = array.Filter(errors, func(e error) bool { return e != nil })
	// Log stats
	stats.Log("Trace validation")
	// Done
	return append(errs, errors...)
}

func flattenTrace[F field.Element[F]](schema sc.AnySchema[F], tr trace.Trace[F]) ([]trace.ColumnRef, []error) {
	var (
		errors []error
		//
		columns []trace.ColumnRef
	)
	//
	for i := uint(0); i < max(schema.Width(), tr.Width()); i++ {
		// Sanity checks first
		if i >= tr.Width() {
			err := fmt.Errorf("module %s missing from trace", schema.Module(i).Name())
			errors = append(errors, err)
		} else if i >= schema.Width() {
			err := fmt.Errorf("unknown module %s in trace", tr.Module(i).Name())
			errors = append(errors, err)
		} else {
			var cols, errs = flattenColumns(i, schema.Module(i), tr.Module(i))
			//
			columns = append(columns, cols...)
			errors = append(errors, errs...)
		}
	}
	// Done
	return columns, errors
}

func flattenColumns[F field.Element[F]](mid uint, scMod sc.Module[F], trMod trace.Module[F],
) ([]trace.ColumnRef, []error) {
	var (
		errors []error
		// Extract module registers
		registers = scMod.Registers()
		//
		columns []trace.ColumnRef
	)
	// Sanity check
	if scMod.Name() != trMod.Name() {
		err := fmt.Errorf("misaligned module during trace expansion (%s vs %s)", scMod.Name(), trMod.Name())
		errors = append(errors, err)
	} else {
		for i := uint(0); i < max(trMod.Width(), scMod.Width()); i++ {
			// Sanity checks first
			if i >= trMod.Width() {
				err := fmt.Errorf("register %s.%s missing from trace", trMod.Name(), registers[i].Name())
				errors = append(errors, err)
			} else if i >= scMod.Width() {
				err := fmt.Errorf("unknown column %s.%s in trace", trMod.Name(), trMod.Descriptor().Columns[i].Name)
				errors = append(errors, err)
			} else {
				columns = append(columns, trace.NewColumnRef(mid, register.NewId(i)))
			}
		}
	}
	// Done
	return columns, errors
}

// Validate that all elements of a given column fit within a given bitwidth.
func validateColumnBitWidth[F field.Element[F]](rid register.Id, smod sc.Module[F], tmod trace.Module[F]) error {
	var (
		reg = smod.Register(rid)
		col = tmod.Column(rid.Unwrap())
	)
	// Sanity check bitwidth can be checked.
	if reg.IsNative() {
		// This indicates a column which has no fixed bitwidth but, rather, uses
		// the entire field element.  The only situation this arises in practice
		// is for columns holding the multiplicative inverse of some other
		// column.
		return nil
	} else if smod.IsStatic() {
		// Static modules always have empty columns in the trace.
		return nil
	} else if col == nil {
		panic(fmt.Sprintf("column %s is unassigned in module %s", reg.Name(), smod.Name()))
	}
	// FIXME: this will fail for small fields!!!!!
	var bound = field.TwoPowN[F](reg.Width())
	//
	for j := range col.Len() {
		var jth = col.Get(j)
		//
		if jth.Cmp(bound) >= 0 {
			qualColName := trace.QualifiedColumnName(smod.Name(), reg.Name())
			return fmt.Errorf("row %d of column %s is out-of-bounds (%s)", j, qualColName, jth.String())
		}
	}
	// success
	return nil
}
