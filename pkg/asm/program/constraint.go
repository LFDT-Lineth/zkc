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
package program

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// ConstraintFailure provides structural information about a failing vanishing constraint.
type ConstraintFailure[F field.Element[F]] struct {
	// Module where constraint failed
	Context schema.ModuleId
	// Row on which the constraint failed
	Row uint
	// Message
	Msg string
}

// Message provides a suitable error message
func (p *ConstraintFailure[F]) Message() string {
	// Construct useful error message
	return fmt.Sprintf("constraint failure (row %d): %s", p.Row, p.Msg)
}

// RequiredCells identifies the cells required to evaluate the failing constraint at the failing row.
func (p *ConstraintFailure[F]) RequiredCells(tr trace.Trace[F]) *set.AnySortedSet[trace.CellRef] {
	return set.NewAnySortedSet[trace.CellRef]()
}

func (p *ConstraintFailure[F]) String() string {
	return p.Message()
}

// Constraint represents a wrapper around an instruction in order for it to
// conform to the constraint interface.
type Constraint[F field.Element[F], T io.Instruction] struct {
	id        sc.ModuleId
	name      string
	registers []io.Register
	code      []T
}

// Accepts implementation for schema.Constraint interface.
func (p Constraint[F, T]) Accepts(trace tr.Trace[F], _ sc.AnySchema[F],
) (bit.Set, sc.Failure) {
	// Extract relevant part of the trace
	var (
		coverage bit.Set
		trModule = trace.Module(p.id)
		state    = io.EmptyState(io.RETURN, p.registers, nil)
	)
	//
	for i := range trModule.Height() {
		// Initialise or check state
		if state.Terminated() {
			// Extract state at start of function instance.
			state = extractState(int(i), state, trModule)
			// Reset to function start
			state.Goto(0)
		}
		// Execute instruction
		pc := p.code[state.Pc()].Execute(state)
		// Sanity check state
		if err := checkState(int(i), state, p.id, trModule); err != nil {
			return coverage, err
		}
		// Execute instruction
		state.Goto(pc)
	}
	// Sanity check frame is complete.
	if !state.Terminated() {
		msg := fmt.Sprintf("function terminated unexpectedly (pc=%d)", state.Pc())
		return coverage, &ConstraintFailure[F]{p.id, trModule.Height() - 1, msg}
	}
	// Success
	return coverage, nil
}

// Bounds implementation for schema.Constraint interface.
func (p Constraint[F, T]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Consistent implementation for schema.Constraint interface.
func (p Constraint[F, T]) Consistent(sc.AnySchema[F]) []error {
	return nil
}

// Contexts implementation for schema.Constraint interface.
func (p Constraint[F, T]) Contexts() []sc.ModuleId {
	return []sc.ModuleId{p.id}
}

// Name implementation for schema.Constraint interface.
func (p Constraint[F, T]) Name() string {
	return p.name
}

// Lisp implementation for schema.Constraint interface.
func (p Constraint[F, T]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("function"),
		sexp.NewSymbol(p.name),
	})
}

// Substitute implementation for schema.Constraint interface.
func (p Constraint[F, T]) Substitute(map[string]F) {
	// Do nothing since assembly instructions do not (at the time of writing)
	// employ labelled constants.
}

func extractState[F field.Element[F]](row int, state io.State, trace tr.Module[F]) io.State {
	//
	for i := range state.Registers() {
		var (
			rid   = register.NewId(uint(i))
			col   = trace.Column(uint(i))
			frVal = col.Get(row)
			biVal big.Int
		)
		// Convert field element to big int
		biVal.SetBytes(frVal.Bytes())
		// Assign corresponding register
		state.Store(rid, biVal)
	}
	//
	return state
}

func checkState[F field.Element[F]](row int, state io.State, mid sc.ModuleId, trace tr.Module[F]) sc.Failure {
	// Check each regsiter in turn
	for i := range trace.Width() {
		var (
			rid   = register.NewId(i)
			col   = trace.Column(i)
			frVal = col.Get(row)
			biVal big.Int
			stVal = state.Load(rid)
		)
		// Convert field element to big int
		biVal.SetBytes(frVal.Bytes())
		// Sanity check they match
		if biVal.Cmp(stVal) != 0 {
			msg := fmt.Sprintf("invalid register state (%s holds 0x%s, expected 0x%s)",
				state.Registers()[i].Name(), biVal.Text(16), stVal.Text(16))
			//
			return &ConstraintFailure[F]{mid, uint(row), msg}
		}
	}
	// Success
	return nil
}
