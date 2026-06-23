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
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceValidation validates that values held in trace columns match the
// expected type.  This is really a sanity check that the trace is not
// malformed.
func TraceValidation[F field.Element[F]](parallel bool, schema sc.AnySchema[F], tr tr.Trace[F]) []error {
	var (
		errors []error
		// Start timer
		stats = util.NewPerfStats()
	)
	// Validate expanded trace
	if parallel {
		// Run (parallel) trace validation
		errors = ParallelTraceValidation(schema, tr)
	} else {
		// Run (sequential) trace validation
		errors = SequentialTraceValidation(schema, tr)
	}
	// Log stats
	stats.Log("Trace validation")
	//
	return errors
}

// SequentialTraceValidation validates that values held in trace columns match
// the expected type.  This is really a sanity check that the trace is not
// malformed.
func SequentialTraceValidation[F field.Element[F]](schema sc.AnySchema[F], tr trace.Trace[F]) []error {
	var errors []error
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
			var (
				scMod = schema.Module(i)
				trMod = tr.Module(i)
			)
			// Validate module
			errors = append(errors, sequentialModuleValidation(scMod, trMod)...)
		}
	}
	// Done
	return errors
}

// ParallelTraceValidation validates that values held in trace columns match the
// expected type.  This is really a sanity check that the trace is not
// malformed.
func ParallelTraceValidation[F field.Element[F]](schema sc.AnySchema[F], trace tr.Trace[F]) []error {
	var (
		errors []error
		// Construct a communication channel for errors.
		c = make(chan error, tr.NumberOfColumns(trace))
		// Number of columns to validate
		ntodo = uint(0)
	)
	// Check each module in turn
	for mid := uint(0); mid < trace.Width(); mid++ {
		var (
			scMod = schema.Module(mid)
			trMod = trace.Module(mid)
		)
		// Check each column within each module
		for i := uint(0); i < trMod.Width(); i++ {
			rid := register.NewId(i)
			// Check elements
			go func(reg register.Register, data tr.Column[F]) {
				// Send outcome back
				c <- validateColumnBitWidth(reg, data, scMod)
			}(scMod.Register(rid), trMod.Column(i))
			//
			ntodo++
		}
	}
	// Collect up all the results
	for i := uint(0); i < ntodo; i++ {
		// Read from channel
		if e := <-c; e != nil {
			errors = append(errors, e)
		}
	}
	// Done
	return errors
}

func sequentialModuleValidation[F field.Element[F]](scMod sc.Module[F], trMod trace.Module[F]) []error {
	var (
		errors []error
		// Extract module registers
		registers = scMod.Registers()
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
				err := fmt.Errorf("unknown register %s.%s in trace", trMod.Name(), trMod.Column(i).Name())
				errors = append(errors, err)
			} else {
				var (
					rid  = register.NewId(i)
					reg  = scMod.Register(rid)
					data = trMod.Column(i)
				)
				// Sanity check data has expected bitwidth
				if err := validateColumnBitWidth(reg, data, scMod); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}
	// Done
	return errors
}

// Validate that all elements of a given column fit within a given bitwidth.
func validateColumnBitWidth[F field.Element[F]](reg register.Register, col tr.Column[F], mod sc.Module[F]) error {
	// Sanity check bitwidth can be checked.
	if reg.IsNative() {
		// This indicates a column which has no fixed bitwidth but, rather, uses
		// the entire field element.  The only situation this arises in practice
		// is for columns holding the multiplicative inverse of some other
		// column.
		return nil
	} else if col.Data() == nil {
		panic(fmt.Sprintf("column %s is unassigned in module %s", col.Name(), mod.Name()))
	}
	// FIXME: this will fail for small fields!!!!!
	var bound = field.TwoPowN[F](reg.Width())
	//
	for j := 0; j < int(col.Data().Len()); j++ {
		var jth = col.Get(j)
		//
		if jth.Cmp(bound) >= 0 {
			qualColName := trace.QualifiedColumnName(mod.Name(), col.Name())
			return fmt.Errorf("row %d of column %s is out-of-bounds (%s)", j, qualColName, jth.String())
		}
	}
	// success
	return nil
}
