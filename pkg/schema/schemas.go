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

	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// RequiredPaddingRows determines the number of additional (spillage / padding)
// rows that will be added during trace expansion.  The exact value depends on
// whether defensive padding is enabled or not.
func RequiredPaddingRows[F any](module uint, defensive bool, schema AnySchema[F]) uint {
	var (
		multiplier = schema.Module(module).Name().Multiplier
		padding    = requiredSpillage(module, schema)
	)
	//
	if defensive {
		// determine minimum levels of defensive padding required.
		padding = max(padding, defensivePadding(module, schema))
	}
	// Technically, we could avoid multiplying by the multiplier here, but in
	// practice it shouldn't matter.  That's because of the very limited ways in
	// which interleaved columns are used in practice.
	return padding * multiplier
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
func Accepts[F field.Element[F], C Constraint[F]](parallel bool, batchsize uint, schema Schema[F, C],
	trace tr.Trace[F]) []Failure {
	//
	return accepts(parallel, batchsize, schema.Constraints(), trace, schema, "Constraint")
}

//nolint:revive
func accepts[F field.Element[F], C Constraint[F]](parallel bool, batchsize uint, iter iter.Iterator[C],
	trace tr.Trace[F], schema Schema[F, C], kind string) []Failure {
	//
	if parallel {
		return parallelAccepts(batchsize, iter, trace, schema, kind)
	}
	// sequential
	return sequentialAccepts(iter, trace, schema)
}

func sequentialAccepts[F field.Element[F], C Constraint[F]](iter iter.Iterator[C], trace tr.Trace[F],
	schema Schema[F, C]) []Failure {
	//
	errors := make([]Failure, 0)
	//
	for iter.HasNext() {
		ith := iter.Next()
		//
		_, err := ith.Accepts(trace, Any(schema))
		if err != nil {
			errors = append(errors, err)
		}
	}
	//
	return errors
}

func parallelAccepts[F field.Element[F], C Constraint[F]](batchsize uint, iter iter.Iterator[C], trace tr.Trace[F],
	schema Schema[F, C], kind string) []Failure {
	//
	errors := make([]Failure, 0)
	// Initialise batch number (for debugging purposes)
	batch := uint(0)
	// Process constraints in batches
	for iter.HasNext() {
		errs := processConstraintBatch(kind, batch, batchsize, iter, trace, schema)
		errors = append(errors, errs...)
		// Increment batch number
		batch++
	}
	// Success
	return errors
}

// Process a given set of constraints in a single batch whilst recording all constraint failures.
func processConstraintBatch[F field.Element[F], C Constraint[F]](logtitle string, batch uint, batchsize uint,
	iter iter.Iterator[C], trace tr.Trace[F], schema Schema[F, C]) []Failure {
	//
	n := uint(0)
	c := make(chan batchOutcome, batchsize)
	errors := make([]Failure, 0)
	stats := util.NewPerfStats()
	// Launch at most 100 go-routines.
	for ; n < batchsize && iter.HasNext(); n++ {
		// Get ith constraint
		ith := iter.Next()
		// Launch checker for constraint
		go func() {
			var (
				context = ith.Contexts()[0]
				name    = ith.Name()
				cov     bit.Set
			)
			// Setup panic intercept
			defer func() {
				var err = recover()
				//
				if err != nil {
					var (
						buf [2048]byte
						n   = runtime.Stack(buf[:], false)
					)
					c <- batchOutcome{context, name, cov, &panicFailure{
						fmt.Sprintf("%v", err), buf[:n],
					}}
				}
			}()
			// Check and send outcome back
			cov, err := ith.Accepts(trace, Any(schema))
			//
			c <- batchOutcome{context, name, cov, err}
		}()
	}
	//
	for i := uint(0); i < n; i++ {
		p := <-c
		// Read from channel
		if p.error != nil {
			errors = append(errors, p.error)
		}
	}
	// Log stats about this batch
	stats.Log(fmt.Sprintf("%s batch %d (%d items)", logtitle, batch, n))
	//
	return errors
}

type batchOutcome struct {
	module uint
	handle string
	data   bit.Set
	error  Failure
}

type panicFailure struct {
	message    string
	stackTrace []byte
}

func (p *panicFailure) Message() string {
	return p.String()
}

func (p *panicFailure) String() string {
	return fmt.Sprintf("%s\n\n%s", p.message, string(p.stackTrace))
}
