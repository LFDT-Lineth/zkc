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
package vm

import (
	"math"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
)

// BootAndDebug boots the given program on the given inputs and executes it to
// completion using the bytecode interpreter, invoking observer once for each
// executed trace line (bytecode vector) with the machine State at that point
// (see State, which carries the executing function id, program counter and
// register values).  This is the execution primitive underlying the `zkc debug`
// command: it uses the interpreter's breakpoint facility (enabled by running in
// tracing mode) so that a breakpoint fires as each vector completes.
func BootAndDebug[W Word[W]](program Program[W], in map[string][]byte, observer func(State[W])) []error {
	// Construct a tracing interpreter, which registers a breakpoint at the
	// terminal of every bytecode vector.
	bci := interpreter.New(program, true)
	// Register a breakpoint handler which reports the executing state to the
	// observer.
	bci = bci.BreakPointer(func(opcode uint32) {
		fid, pc, frame := interpreter.ExtractExecutingState(bci)
		// Clone the frame so the observer sees a stable snapshot which cannot be
		// mutated by subsequent execution.
		observer(State[W]{fid, pc, opcode == encoding.RET, slices.Clone(frame)})
	})
	// Boot and execute to completion.
	_, errs := BootAndExecute(bci, in, math.MaxUint)
	//
	return errs
}
