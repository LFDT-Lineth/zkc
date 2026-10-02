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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
	log "github.com/sirupsen/logrus"
)

// Environment provides a convenient alias
type Environment[W word.Word[W]] = bytecode.Environment[W]

// RegisterId provides a convenient alias
type RegisterId = bytecode.RegisterId

// Vector provides a covenient alias
type Vector[W word.Word[W]] = bytecode.Vector[W]

// Bytecode provides a covenient alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// ProgramPoint provides a convenient alias
type ProgramPoint = descriptor.ProgramPoint

// Word provides a convenient alias
type Word[W any] = word.Word[W]

// Function validates the body of a function against a given environment,
// returning errors if it is no well-formed.
func Function[W Word[W]](env Environment[W], f *descriptor.Function[W]) (errs []error) {
	var (
		n    = uint(len(f.Vectors()))
		safe = true
	)
	// Perform internal validations first
	for _, vec := range f.Vectors() {
		var es, s = validateVector(env, vec, n)
		//
		errs = append(errs, es...)
		safe = safe && s
	}
	// Validate inter-vector control flow (if it is safe to do so).
	if safe {
		var es, s = validateReachability(f)
		//
		errs = append(errs, es...)
		safe = s
	}
	// valididate register bitwidths (if it is safe to do so).
	if safe {
		for _, warning := range validateRegisterBitwidth(f, env) {
			log.Warn(warning)
		}
	}
	//
	return errs
}

// validateReachability checks that every vector of a given function is
// reachable from its entry.  This is implemented as a straightforward
// depth-first traversal of the jumps in each vector.  This assumes every jump
// target is in bounds, and every bytecode in each vector is reachable within
// that vector, such that any jump it contains may be taken.
func validateReachability[W Word[W]](f *descriptor.Function[W]) ([]error, bool) {
	var (
		worklist stack.Worklist
		vectors  = f.Vectors()
	)
	// A function without any vectors has nothing to reach.
	if len(vectors) == 0 {
		return nil, true
	}
	// Initialise worklist with the entry vector
	worklist.Push(0)
	// Continue until all paths explored
	for worklist.Size() > 0 {
		for _, bc := range vectors[worklist.Pop()].Bytecodes {
			if jmp, ok := bc.(*bytecode.Jmp[W]); ok {
				worklist.Push(uint(jmp.Target))
			}
		}
	}
	// Sanity check that there are no unreachable vectors.
	for i := range uint(len(vectors)) {
		if !worklist.Visited(i) {
			// Only report one error, as likely there will be several vectors
			// grouped together.
			return []error{fmt.Errorf("function has unreachable vector (%d)", i)}, false
		}
	}
	//
	return nil, true
}
