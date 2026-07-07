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
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/trace"
)

// Observer is a generic interface for extract information before and after an
// execution step of the VM.  For example, to generate debugging information.
type Observer[W MachineWord[W], I Instruction, M Machine[W, I]] = trace.Observer[W, I, M]

// BaseObserver is an observer for a base machin
type BaseObserver[W Word[W]] = trace.Observer[W, WordInstruction, *WordMachine[W]]

// EmptyBaseObserver is an empty observer for a base machine.
type EmptyBaseObserver = trace.EmptyObserver[Uint, WordInstruction, *WordMachine[Uint]]

// TraceObserver is an observer which can be used to extract a full trace.
type TraceObserver[W Word[W], I Instruction, M Machine[W, I]] = trace.FullObserver[W, I, M]

// TraceProcessor captures the notion of a "post-processing" function on the
// data recorded during execution for a given module.  The goal of this is to
// decouple post-processing logic from the trace observer itself.
type TraceProcessor[W any, F any] = trace.PostProcessor[W, F]

// State captures information recorded during tracing for a given module.  Such
// state typically needs to be "post processed" before it can form part of the
// final trace.
type State[W any] = trace.State[W]

// Trace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func Trace[W Word[W], F field.Element[F]](
	program Program[W], in map[string][]W, processor TraceProcessor[W, F],
) (rtrace.Trace[F], []error) {
	//
	var (
		wm       = BytecodeProgramToWord(program)
		observer TraceObserver[W, WordInstruction, *WordMachine[W]]
		errs     []error
		tr       rtrace.Trace[F]
	)
	// Execute machine
	if err := wm.Boot("main", in); err != nil {
		return nil, append(errs, err)
	}
	//
	if _, err := ExecuteAndObserve(wm, 1, &observer); err != nil {
		errs = append(errs, err)
	} else {
		// Build the trace (finally) using the supplied post-processor
		tr = trace.Trace(&observer, wm, processor)
	}
	// Done
	return tr, errs
}
