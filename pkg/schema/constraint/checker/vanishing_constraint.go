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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// VanishingFailure provides structural information about a failing vanishing constraint.
type VanishingFailure[F field.Element[F]] struct {
	rowFailure[F]
	// Constraint expression
	constraint term.Testable[F]
	// Module where constraint failed
	context schema.ModuleId
}

// Message provides a suitable error message
func (p *VanishingFailure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("constraint \"%s\" does not hold (row %d, shard %d)", p.handle, p.row, p.shardId)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *VanishingFailure[F]) RequiredCells() set.AnySortedSet[trace.CellRef] {
	return *p.constraint.RequiredCells(int(p.row), p.context)
}

func (p *VanishingFailure[F]) String() string {
	return p.Message()
}

func processVanishingConstraint[F field.Element[F], T term.Testable[F]](
	cp ConstraintProcessor[F], c *vanishing.Constraint[F, T]) (State[F], []Failure[F]) {
	//
	var (
		state State[F]
		// Handle is used for error reporting.
		handle = constraint.DetermineHandle(c.Handle, c.Context, cp.shard)
	)
	//
	if c.Domain.IsEmpty() {
		// Global Constraint
		return state, holdsGlobally(cp, handle, c.Context, c.Constraint)
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
	return state, holdsLocally(cp, start, handle, c.Context, c.Constraint)
}

// holdsGlobally checks whether a given expression vanishes (i.e. evaluates to
// zero) for all rows of a trace.  If not, report an appropriate error.
func holdsGlobally[F field.Element[F], T term.Testable[F]](cp ConstraintProcessor[F], handle string,
	ctx schema.ModuleId, constraint T) []Failure[F] {
	//
	var (
		// Determine height of enclosing module
		height = cp.shard.Module(ctx).Height()
		// Determine well-definedness bounds for this constraint
		bounds = constraint.Bounds()
	)
	// Sanity check enough rows
	if bounds.End < height {
		// Check all in-bounds values
		for k := bounds.Start; k < (height - bounds.End); k++ {
			if errs := holdsLocally(cp, k, handle, ctx, constraint); len(errs) > 0 {
				return errs
			}
		}
	}
	// Success
	return nil
}

// holdsLocally checks whether a given constraint holds (e.g. vanishes) on a
// specific row of a trace. If not, report an appropriate error.
func holdsLocally[F field.Element[F], T term.Testable[F]](cp ConstraintProcessor[F], k uint, handle string,
	ctx schema.ModuleId, term T) []Failure[F] {
	//
	var (
		trMod = cp.shard.Module(ctx)
		scMod = cp.parent.schema.Module(ctx)
	)
	//
	ok, err := term.TestAt(k, trMod, scMod)
	// Check for errors
	if err != nil {
		return []Failure[F]{constraint.NewInternalFailure[F](handle, ctx, k, err.Error())}
	} else if !ok {
		// Evaluation failure
		return []Failure[F]{&VanishingFailure[F]{newRowFailure(handle, k, cp), term, ctx}}
	}
	// Success
	return nil
}
