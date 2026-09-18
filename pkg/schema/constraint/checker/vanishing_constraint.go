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
package checker

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

func processVanishingConstraint[F field.Element[F], T term.Testable[F]](
	cp ConstraintProcessor[F], c *vanishing.Constraint[F, T]) []Failure[F] {
	//
	var (
		// Handle is used for error reporting.
		handle = constraint.DetermineHandle(c.Handle, c.Context, cp.shard)
		// Determine enclosing module
		trModule = cp.shard.Module(c.Context)
		scModule = cp.parent.schema.Module(c.Context)
	)
	//
	if c.Domain.IsEmpty() {
		// Global Constraint
		return HoldsGlobally(handle, c.Context, c.Constraint, cp.shardId, trModule, scModule)
	}
	// Extract domain
	domain := c.Domain.Unwrap()
	// Local constraint
	var start uint
	// Handle negative domains
	if domain < 0 {
		// Determine height of enclosing module
		height := cp.shard.Module(c.Context).Height()
		// Negative rows calculated from end of trace.
		start = height + uint(domain)
	} else {
		start = uint(domain)
	}
	// Check specific row
	return HoldsLocally(start, handle, c.Constraint, c.Context, cp.shardId, trModule, scModule)
}

// HoldsGlobally checks whether a given expression vanishes (i.e. evaluates to
// zero) for all rows of a trace.  If not, report an appropriate error.
func HoldsGlobally[F field.Element[F], T term.Testable[F]](handle string, ctx schema.ModuleId, constraint T,
	shard uint, trMod trace.Module[F], scMod schema.Module[F]) []Failure[F] {
	//
	var (
		// Determine height of enclosing module
		height = trMod.Height()
		// Determine well-definedness bounds for this constraint
		bounds = constraint.Bounds()
	)
	// Sanity check enough rows
	if bounds.End < height {
		// Check all in-bounds values
		for k := bounds.Start; k < (height - bounds.End); k++ {
			if errs := HoldsLocally(k, handle, constraint, ctx, shard, trMod, scMod); len(errs) > 0 {
				return errs
			}
		}
	}
	// Success
	return nil
}

// HoldsLocally checks whether a given constraint holds (e.g. vanishes) on a
// specific row of a trace. If not, report an appropriate error.
func HoldsLocally[F field.Element[F], T term.Testable[F]](k uint, handle string, term T, ctx schema.ModuleId,
	shard uint, trMod trace.Module[F], scMod schema.Module[F]) []Failure[F] {
	//
	ok, _, err := term.TestAt(k, trMod, scMod)
	// Check for errors
	if err != nil {
		return []Failure[F]{constraint.NewInternalFailure[F](handle, ctx, k, err.Error())}
	} else if !ok {
		// Evaluation failure
		return []Failure[F]{&vanishing.Failure[F]{
			VanishingHandle: handle,
			Constraint:      term,
			Context:         ctx,
			Row:             k,
			Shard:           shard}}
	}
	// Success
	return nil
}
