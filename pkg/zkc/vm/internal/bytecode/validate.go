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
package bytecode

import (
	"errors"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Validate the body of a function against a given environment, returning errors
// if it is no well-formed.
func Validate[W word.Word[W]](field field.Config, env Environment[W], body []Vector[W]) (errs []error) {
	var n = uint(len(body))
	// Perform internal validations first
	for _, vec := range body {
		errs = append(errs, validateVector(field, env, vec, n)...)
	}
	//
	return errs
}

// Validate checks that this vector instruction is well-formed: every
// constituent bytecode must itself be well-formed, and there must be no
// conflicting reads or writes on any execution path.  This mirrors
// instruction.Vector.Validate.
//
// A write conflict arises when a register is written which _may_ already have
// been written on the same path; a read conflict arises when a register is read
// which _may_ (but not _definitely_) have been written.
func validateVector[W word.Word[W]](field FieldConfig, env Environment[W], vec Vector[W], nVecs uint) []error {
	var (
		errors, structureSafe      = validateStructure(env, vec, nVecs)
		controlErrors, controlSafe = validateControlFlow(vec)
	)
	//
	errors = append(errors, controlErrors...)
	// Validate individual bytecodes. Each bytecode checks its operands before
	// performing any environment lookup.
	for _, b := range vec.Bytecodes {
		errors = append(errors, b.Validate(field, env)...)
	}
	// WriteMap assumes that every control-flow destination is in bounds and
	// every bytecode is non-nil.
	if !structureSafe || !controlSafe {
		return errors
	}

	return append(errors, validateReadWriteConflicts(env, vec)...)
}

// validateStructure checks every index which an environment lookup or jump
// would dereference. The returned boolean indicates whether environment-
// dependent bytecode validation and write-map construction are safe.
func validateStructure[W word.Word[W]](env Environment[W], vec Vector[W], nVecs uint) ([]error, bool) {
	var (
		errors []error
		safe   = true
	)

	for i, code := range vec.Bytecodes {
		for _, registers := range [][]RegisterId{code.Uses(), code.Definitions()} {
			for _, id := range registers {
				if uint(id) >= env.RegisterCount() {
					safe = false
				}
			}
		}

		if jump, ok := code.(*Jmp[W]); ok && uint(jump.Target) >= nVecs {
			errors = append(errors, fmt.Errorf("bytecode %d: jump target %d does not exist", i, jump.Target))
			safe = false
		}
	}

	return errors, safe
}

// validateControlFlow checks the intra-vector control-flow graph.  Every skip
// destination must exist, including destinations in unreachable code, and every
// reachable path must end in a terminal bytecode.  This is implemented as
// straightforward depth-first traversal of the vector's bytecodes.
func validateControlFlow[W word.Word[W]](vec Vector[W]) ([]error, bool) {
	var (
		worklist Worklist
		errs     []error
		safe     = true
	)
	// Initialise worklist
	worklist.Push(0)
	// Continue until all paths explored
	for worklist.Size() > 0 {
		var pc = worklist.Pop()
		// Sanity check valid position
		if pc > vec.Len() {
			errs = append(errs, errors.New("vector has unterminated path"))
			safe = false
			// Terminate path
			continue
		}
		//
		switch bc := vec.Bytecodes[pc].(type) {
		case *Fail[W], *Ret[W]:
			// Terminate path
		case *Skip[W]:
			worklist.Push(pc + 1 + uint(bc.Skip))
		case *SkipIf[W]:
			// Add skip target
			worklist.Push(pc + 1 + uint(bc.Skip))
			// Add fall-thru target
			worklist.Push(pc + 1)
		case *Switch[W]:
			for _, c := range bc.Cases {
				worklist.Push(pc + 1 + uint(c.Skip))
			}
			// Add default target
			worklist.Push(pc + 1)
		case *Dispatch[W]:
			for _, c := range bc.Cases {
				worklist.Push(pc + 1 + uint(c.Skip))
			}
			// Add default target
			worklist.Push(pc + 1)
		default:
			// Add fall thru target
			worklist.Push(pc + 1)
		}
	}
	// Final sanity check that there is no unreachable code.
	for i := range vec.Len() {
		if !worklist.visited.Contains(i) {
			errs = append(errs, errors.New("vector has unreachable code"))
			// Only report one error, as likely there will be several
			// instructions grouped together.
			break
		}
	}
	//
	return errs, safe
}

// Worklist provides a generic worklist structure which is suitable, for
// example, for implementing a depth-first search, etc.
type Worklist struct {
	stacl   stack.Stack[uint]
	visited bit.Set
}

// Size returns number of items remaining
func (p *Worklist) Size() uint {
	return p.stacl.Len()
}

// Pop pops an item of the stack
func (p *Worklist) Pop() uint {
	return p.stacl.Pop()
}

// Push pusts an item on the stack, provided that it has not been seen before.
func (p *Worklist) Push(n uint) {
	if !p.visited.Contains(n) {
		p.visited.Insert(n)
		p.stacl.Push(n)
	}
}

// validateReadWriteConflicts checks for ambiguous reads and writes along every
// execution path through this vector.
func validateReadWriteConflicts[W word.Word[W]](env Environment[W], vec Vector[W]) []error {
	var (
		errors   []error
		writeMap = vec.WriteMap()
	)
	for i := range uint(len(vec.Bytecodes)) {
		var (
			ithState = writeMap.StateOf(i)
			ith      = vec.Bytecodes[i]
		)
		// Sanity check for conflicting reads.
		if !isUnsafeCall(ith, env) {
			for _, r := range ith.Uses() {
				if !IsZeroWidth(env.Register(r)) && ithState.MaybeAssigned(r) && !ithState.DefinitelyAssigned(r) {
					errors = append(errors,
						fmt.Errorf("conflicting read on register \"%s\" in \"%s\"", RegisterToString(r, env), ith.String(env)))
				}
			}
		}
		// Sanity check for conflicting writes.
		for _, r := range ith.Definitions() {
			if !IsZeroWidth(env.Register(r)) && ithState.MaybeAssigned(r) {
				errors = append(errors,
					fmt.Errorf("conflicting write on register \"%s\" in \"%s\"", RegisterToString(r, env), ith.String(env)))
			}
		}
	}
	//
	return errors
}
