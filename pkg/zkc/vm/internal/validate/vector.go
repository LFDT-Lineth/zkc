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
	"errors"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// validateVector checks that a given vector instruction is well-formed: every
// constituent bytecode must itself be well-formed, and there must be no
// conflicting reads or writes on any execution path.
//
// A write conflict arises when a register is written which _may_ already have
// been written on the same path; a read conflict arises when a register is read
// which _may_ (but not _definitely_) have been written.
func validateVector[W word.Word[W]](field field.Config, env Environment[W], vec Vector[W], nVecs uint) ([]error, bool) {
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
		return errors, false
	}

	return append(errors, validateReadWriteConflicts(env, vec)...), true
}

// validateStructure checks every index which an environment lookup or jump
// would dereference. The returned boolean indicates whether environment-
// dependent bytecode validation and write-map construction are safe.
func validateStructure[W word.Word[W]](env Environment[W], vec Vector[W], nVecs uint) ([]error, bool) {
	var (
		errors []error
		safe   = true
	)

	for _, code := range vec.Bytecodes {
		for _, registers := range [][]RegisterId{code.Uses(), code.Definitions()} {
			for _, id := range registers {
				if uint(id) >= env.RegisterCount() {
					errors = append(errors, fmt.Errorf("bytecode uses invalid register (%d)", id))
					safe = false
				}
			}
		}

		if jump, ok := code.(*bytecode.Jmp[W]); ok && uint(jump.Target) >= nVecs {
			errors = append(errors, fmt.Errorf("bytecode has invalid jump target (%d)", jump.Target))
			safe = false
		}
	}

	return errors, safe
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
				var reg = env.Register(r)
				//
				if !reg.IsZeroWidth() && ithState.MaybeAssigned(r) && !ithState.DefinitelyAssigned(r) {
					errors = append(errors,
						fmt.Errorf("conflicting read on register \"%s\" in \"%s\"",
							bytecode.RegisterToString(r, env), ith.String(env)))
				}
			}
		}
		// Sanity check for conflicting writes.
		for _, r := range ith.Definitions() {
			var reg = env.Register(r)
			//
			if !reg.IsZeroWidth() && ithState.MaybeAssigned(r) {
				errors = append(errors,
					fmt.Errorf("conflicting write on register \"%s\" in \"%s\"",
						bytecode.RegisterToString(r, env), ith.String(env)))
			}
		}
	}
	//
	return errors
}

// validateControlFlow checks the intra-vector control-flow graph.  Every skip
// destination must exist, including destinations in unreachable code, and every
// reachable path must end in a terminal bytecode.  This is implemented as
// straightforward depth-first traversal of the vector's bytecodes.
func validateControlFlow[W word.Word[W]](vec Vector[W]) ([]error, bool) {
	var (
		worklist stack.Worklist
		errs     []error
		safe     = true
	)
	// Initialise worklist
	worklist.Push(0)
	// Continue until all paths explored
	for worklist.Size() > 0 {
		var pc = worklist.Pop()
		// Sanity check valid position
		if pc >= vec.Len() {
			errs = append(errs, errors.New("vector has unterminated path"))
			safe = false
			// Terminate path
			continue
		}
		//
		switch bc := vec.Bytecodes[pc].(type) {
		case *bytecode.Fail[W], *bytecode.Ret[W], *bytecode.Jmp[W]:
			// Terminate path:
		case *bytecode.Call[W]:
			// Never calls are terminators
			if !bc.Never {
				worklist.Push(pc + 1)
			}
		case *bytecode.Skip[W]:
			worklist.Push(pc + 1 + uint(bc.Skip))
		case *bytecode.SkipIf[W]:
			// Add skip target
			worklist.Push(pc + 1 + uint(bc.Skip))
			// Add fall-thru target
			worklist.Push(pc + 1)
		case *bytecode.Switch[W]:
			for _, c := range bc.Cases {
				worklist.Push(pc + 1 + uint(c.Skip))
			}
			// Add default target
			worklist.Push(pc + 1)
		case *bytecode.Dispatch[W]:
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
		if !worklist.Visited(i) {
			errs = append(errs, errors.New("vector has unreachable code"))
			// Only report one error, as likely there will be several
			// instructions grouped together.
			break
		}
	}
	//
	return errs, safe
}

func isUnsafeCall[W word.Word[W]](code Bytecode[W], env Environment[W]) bool {
	call, ok := code.(*bytecode.Call[W])
	if !ok {
		return false
	}

	module := env.Module(call.Target)
	if module.IsEmpty() {
		return false
	}

	callee := module.Unwrap()

	return callee.IsFunction() && callee.HasUnsafeArgs()
}
