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
package schema

import (
	"fmt"
	"runtime"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	log "github.com/sirupsen/logrus"
)

// Accepts determines whether this schema will accept a given trace.  That is,
// whether or not the given trace adheres to the schema constraints.  A trace
// can fail to adhere to the schema for a variety of reasons, such as having a
// constraint which does not hold.
//
//nolint:revive
func Accepts[F field.Element[F], C Constraint[F]](parallel bool, schema Schema[F, C],
	trace trace.Trace[F]) (failures []Failure[F]) {
	//
	if parallel {
		return parallelAccepts(schema.Constraints(), trace, schema)
	}
	// sequential
	return sequentialAccepts(schema.Constraints(), trace, schema)
}

func sequentialAccepts[F field.Element[F], C Constraint[F]](iter iter.Iterator[C], trace trace.Trace[F],
	schema Schema[F, C]) []Failure[F] {
	//
	var (
		context = SeqBuildContext(trace, Any(schema))
		errors  = make([]Failure[F], 0)
	)
	//
	for iter.HasNext() {
		var (
			ith = iter.Next()
			//
			errs = ith.Accepts(trace, Any(schema), context)
		)
		//
		errors = append(errors, errs...)
	}
	//
	return errors
}

func parallelAccepts[F field.Element[F], C Constraint[F]](iter iter.Iterator[C], trace trace.Trace[F],
	schema Schema[F, C]) (errors []Failure[F]) {
	var (
		context = ParBuildContext(trace, Any(schema))
		// Collect all constraints into a slice so we can use ParallelMap.
		constraints = iter.Collect()
	)
	// Process all constraints in parallel using a worker pool.
	errs := array.ParallelMap(constraints, func(i uint, constraint C) []Failure[F] {
		if i%1000 == 0 {
			var percent float64 = float64(100*i) / float64(len(constraints))
			log.Debug(fmt.Sprintf("Checking constraints [%0.1f%%]", percent))
		}
		//
		return processConstraint(constraint, trace, schema, context)
	})
	// Flattern any generated errors
	return array.FlatMap(errs, func(fs []Failure[F]) []Failure[F] { return fs })
}

// processConstraint checks a given constraint against the trace, intercepting any
// panic and converting it into a PanicFailure.
func processConstraint[F field.Element[F], C Constraint[F]](ith C, trace trace.Trace[F],
	schema Schema[F, C], ctx Context[F]) (res []Failure[F]) {
	// Setup panic intercept
	defer func() {
		var err = recover()
		//
		if err != nil {
			var (
				buf [2048]byte
				n   = runtime.Stack(buf[:], false)
			)
			// override return
			res = []Failure[F]{
				&PanicFailure[F]{ith.Name(), fmt.Sprintf("%v", err), buf[:n]},
			}
		}
	}()
	// Check and send outcome back
	return ith.Accepts(trace, Any(schema), ctx)
}

// PanicFailure indicates that a panic arose during constraint checking, rather
// than an actual constraint failure.  The purpose of this is to allow the
// testing framework to distinguish panics from actual constraint failures.
type PanicFailure[F any] struct {
	handle     string
	message    string
	stackTrace []byte
}

// Handle implementation for schema.Failure interface.
func (p *PanicFailure[F]) Handle() string {
	return p.handle
}

// Message returns the message associated with this panic.
func (p *PanicFailure[F]) Message() string {
	return p.String()
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *PanicFailure[F]) RequiredCells(_ trace.Trace[F]) set.AnySortedSet[trace.ShardedCellRef] {
	return nil
}

func (p *PanicFailure[F]) String() string {
	return fmt.Sprintf("%s\n\n%s", p.message, string(p.stackTrace))
}
