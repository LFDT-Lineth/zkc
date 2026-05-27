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
package validate

import (
	"fmt"
	"reflect"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/stmt"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// ControlFlow checks for issues related to the control-flow of a function.  For
// example, where a register is not definitely assigned before being used.  Or,
// a control-flow path exists which is not terminated with ret.  This is
// implemented using a straightforward dataflow analysis.  One aspect worth
// noting is that the dataflow sets hold true for registers which are undefined,
// and false for registers which are defined.
func ControlFlow(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, d := range program.Components() {
		switch d := d.(type) {
		case *decl.ResolvedConstant:
			// ignore
		case *decl.ResolvedFunction:
			errors = append(errors, validateFunctionFlow(*d, srcmaps)...)
		case *decl.ResolvedInclude:
			// ignore
		case *decl.ResolvedMemory:
			// ignore
		case *decl.ResolvedTypeAlias:
			// ignore
		default:
			panic(fmt.Sprintf("unknown component: %s", reflect.TypeOf(d).String()))
		}
	}
	//
	return errors
}

func validateFunctionFlow(fn decl.ResolvedFunction, srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		n          = uint(len(fn.Code))
		errors     []source.SyntaxError
		entryState bit.Set
	)
	// Initialise entry state (since these are assigned on entry)
	for i, r := range fn.Variables {
		if !r.IsParameter() {
			entryState.Insert(uint(i))
		}
	}
	// Construct the worklist which is the heart of this algorithm.
	worklist := NewWorklist(n, 0, entryState)
	// Continue until all reachable instructions visited
	for !worklist.Empty() {
		// Abstract execute instruction
		errs := applyInstructionSemantics(&worklist, fn, srcmaps)
		// Collect any errors
		errors = append(errors, errs...)
	}
	// Sanity check all instructions reachable.
	for pc, stmt := range fn.Code {
		if !worklist.Visited(uint(pc)) {
			if stmt == nil {
				continue
			}

			errors = append(errors, *srcmaps.SyntaxError(stmt, "unreachable"))
		}
	}
	//
	return errors
}

// Abstractly execute a given vector instruction with respect to a given state
// at the beginning of the instruction.
func applyInstructionSemantics(worklist *Worklist, fn decl.ResolvedFunction, srcmaps source.Maps[any],
) []source.SyntaxError {
	//
	var errors []source.SyntaxError
	// Pop the next item from the stack
	pc, state := worklist.Pop()
	insn := fn.Code[pc]
	// Apply effect of instruction on state
	state, errors = applyInstructionFlow(insn, state, fn, srcmaps)
	// Propagate state along branches
	switch insn := insn.(type) {
	case *stmt.Goto[symbol.Resolved]:
		// Unconditional jump target
		worklist.Join(insn.Target, state)
	case *stmt.IfGoto[symbol.Resolved]:
		// Conditional jump target
		worklist.Join(insn.Target, state)
		// Fall thru
		worklist.Join(pc+1, state)
	case *stmt.Return[symbol.Resolved]:
		// Check all outputs are assigned
		errs := checkOutputsAssigned(insn, state, fn, srcmaps)
		errors = append(errors, errs...)
	case *stmt.Fail[symbol.Resolved]:
		// Nothing to do here
	default:
		// fall through cases
		worklist.Join(pc+1, state)
	}
	//
	return errors
}

// Apply the dataflow transfer function (i.e. the effects of given instruction
// on the record of which registesr are definitely assigned).
func applyInstructionFlow(stmt stmt.Resolved, state bit.Set, fn decl.ResolvedFunction,
	srcmaps source.Maps[any]) (bit.Set, []source.SyntaxError) {
	if stmt == nil {
		return state, nil
	}
	//
	var errors []source.SyntaxError
	// Ensure every register read has been defined.
	for _, r := range stmt.Uses() {
		if state.Contains(r) {
			msg := fmt.Sprintf("variable %s possibly undefined", fn.Variables[r].Name)
			errors = append(errors, *srcmaps.SyntaxError(stmt, msg))
			// mark as defined to avoid follow on errors
			state.Remove(r)
		}
	}
	// Mark all target registers as written.
	for _, r := range stmt.Definitions() {
		state.Remove(r)
	}
	// Done
	return state, errors
}

// Check that all output registers have been definitely assigned at the point of
// a return.
func checkOutputsAssigned(stmt stmt.Resolved, state bit.Set, fn decl.ResolvedFunction,
	srcmaps source.Maps[any]) []source.SyntaxError {
	//
	var errors []source.SyntaxError
	//
	for i, r := range fn.Variables {
		if r.IsReturn() && state.Contains(uint(i)) {
			msg := fmt.Sprintf("return %s possibly undefined", r.Name)
			errors = append(errors, *srcmaps.SyntaxError(stmt, msg))
		}
	}
	//
	return errors
}

// Worklist encapsulates the notion of a worklist, along with the necessary
// dataflow sets for a dataflow analysis algorithm.
type Worklist struct {
	// Visited is used to determine which instruct
	visited bit.Set
	states  []bit.Set
	stack   []uint
}

// NewWorklist constructs a new worklist of a given capacity.
func NewWorklist(nstates uint, start uint, init bit.Set) Worklist {
	var (
		visited bit.Set
		states  = make([]bit.Set, nstates)
	)
	//
	states[start] = init
	// mark start visited
	visited.Insert(start)
	//
	return Worklist{
		visited,
		states,
		[]uint{start},
	}
}

// Empty determines whether or not this worklist is empty.
func (p *Worklist) Empty() bool {
	return len(p.stack) == 0
}

// Visited checks whether a given program point was reached during the analysis.
func (p *Worklist) Visited(pc uint) bool {
	return p.visited.Contains(pc)
}

// Pop removes the next item from the stack, and also returns the relevant
// dataflow state.
func (p *Worklist) Pop() (uint, bit.Set) {
	n := len(p.stack) - 1
	pc := p.stack[n]
	bs := p.states[pc]
	p.stack = p.stack[:n]
	//
	return pc, bs.Clone()
}

// Join joins a given state into the state recorded for a given pc location.
func (p *Worklist) Join(pc uint, state bit.Set) {
	pcState := &p.states[pc]
	// Visit state if it hasn't been visited before, or there is an update to
	// its dataflow state.
	if pcState.Union(state) || !p.visited.Contains(pc) {
		// mark item as visited
		p.visited.Insert(pc)
		// push item on stack
		p.stack = append(p.stack, pc)
	}
}
