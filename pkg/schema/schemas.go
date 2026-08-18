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
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	log "github.com/sirupsen/logrus"
)

// RequiredPaddingRows determines the number of additional (spillage / padding)
// rows that will be added during trace expansion.  The exact value depends on
// whether defensive padding is enabled or not.
func RequiredPaddingRows[F any](module uint, defensive bool, schema AnySchema[F]) uint {
	var padding = requiredSpillage(module, schema)
	//
	if defensive {
		// determine minimum levels of defensive padding required.
		padding = max(padding, defensivePadding(module, schema))
	}
	//
	return padding
}

// RequiredSpillage returns the minimum amount of spillage required for a given
// module to ensure valid traces are accepted in the presence of arbitrary
// padding.  Spillage can only arise from computations as this is where values
// outside of the user's control are determined.
func requiredSpillage[F any](module uint, schema AnySchema[F]) uint {
	var mod = schema.Module(module)
	// Sanity check whether padding is allowed for this module.
	if !mod.AllowPadding() {
		return 0
	}
	// For modules that allow padding we currently (for legacy reasons) always
	// ensure an initial padding row is present.
	mx := uint(1)
	// Determine if any more spillage required
	for i := mod.Assignments(); i.HasNext(); {
		// Get ith assignment
		ith := i.Next()
		// NOTE: Spillage is only currently considered to be necessary at
		// the front (i.e. start) of a trace.  This is because the prover
		// always inserts padding at the front, never the back.  As such, it
		// is the maximum positive shift which determines how much spillage
		// is required for a comptuation.
		mx = max(mx, ith.Bounds(module).End)
	}
	//
	return mx
}

// DefensivePadding returns the maximum amount of front padding required to
// ensure no constraint operating in the active region is clipped.  Observe that
// only front padding is considered because, for now, we assume the prover will
// only pad at the front.
func defensivePadding[F any](module uint, schema AnySchema[F]) uint {
	var (
		mod   = schema.Module(module)
		front = uint(0)
	)
	// Check whether module supports defensive padding, or not.
	if mod.AllowPadding() {
		// Determine maximum amounts of defensive padding required for constraints.
		for i := schema.Constraints(); i.HasNext(); {
			bounds := i.Next().Bounds(module)
			//
			front = max(front, bounds.Start)
		}
	}
	//
	return front
}

// Accepts determines whether this schema will accept a given trace.  That is,
// whether or not the given trace adheres to the schema constraints.  A trace
// can fail to adhere to the schema for a variety of reasons, such as having a
// constraint which does not hold.
//
//nolint:revive
func Accepts[F field.Element[F], C Constraint[F]](parallel bool, schema Schema[F, C],
	trace trace.Trace[F]) []Failure {
	//
	return accepts(parallel, schema.Constraints(), trace, schema)
}

//nolint:revive
func accepts[F field.Element[F], C Constraint[F]](parallel bool, iter iter.Iterator[C],
	trace trace.Trace[F], schema Schema[F, C]) []Failure {
	//
	if parallel {
		return parallelAccepts(iter, trace, schema)
	}
	// sequential
	return sequentialAccepts(iter, trace, schema)
}

func sequentialAccepts[F field.Element[F], C Constraint[F]](iter iter.Iterator[C], trace trace.Trace[F],
	schema Schema[F, C]) []Failure {
	//
	var (
		context = SeqBuildContext(trace, Any(schema))
		errors  = make([]Failure, 0)
	)

	//
	for iter.HasNext() {
		ith := iter.Next()
		//
		err := ith.Accepts(trace, Any(schema), context)
		if err != nil {
			errors = append(errors, err)
		}
	}
	//
	return errors
}

func parallelAccepts[F field.Element[F], C Constraint[F]](iter iter.Iterator[C], trace trace.Trace[F],
	schema Schema[F, C]) (errors []Failure) {
	var (
		context = ParBuildContext(trace, Any(schema))
		// Collect all constraints into a slice so we can use ParallelMap.
		constraints = iter.Collect()
	)
	// Process all constraints in parallel using a worker pool.
	errors = array.ParallelMap(constraints, func(i uint, constraint C) Failure {
		if i%1000 == 0 {
			var percent float64 = float64(100*i) / float64(len(constraints))
			log.Debug(fmt.Sprintf("Checking constraints [%0.1f%%]", percent))
		}
		//
		return processConstraint(constraint, trace, schema, context)
	})

	//
	return array.Filter(errors, func(f Failure) bool { return f != nil })
}

// processConstraint checks a given constraint against the trace, intercepting any
// panic and converting it into a PanicFailure.
func processConstraint[F field.Element[F], C Constraint[F]](ith C, trace trace.Trace[F], schema Schema[F, C],
	ctx Context[F]) (res Failure) {
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
			res = &PanicFailure{fmt.Sprintf("%v", err), buf[:n]}
		}
	}()
	// Check and send outcome back
	err := ith.Accepts(trace, Any(schema), ctx)
	//
	return err
}

// PanicFailure indicates that a panic arose during constraint checking, rather
// than an actual constraint failure.  The purpose of this is to allow the
// testing framework to distinguish panics from actual constraint failures.
type PanicFailure struct {
	message    string
	stackTrace []byte
}

// Message returns the message associated with this panic.
func (p *PanicFailure) Message() string {
	return p.String()
}

func (p *PanicFailure) String() string {
	return fmt.Sprintf("%s\n\n%s", p.message, string(p.stackTrace))
}
