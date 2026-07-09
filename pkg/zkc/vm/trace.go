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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
)

// TraceProcessor captures the notion of a "post-processing" function on the
// data recorded during execution for a given module.  The goal of this is to
// decouple post-processing logic from the trace observer itself.
type TraceProcessor[W Word[W], F any] interface {
	// Post-process the trace for a given function.
	TraceFunction(Function[W], []State[W]) rtrace.ArrayModule[F]
	// Post-process the trace for a memory of some kind
	TraceMemory(m Memory[W]) rtrace.ArrayModule[F]
}

// BootAndTrace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func BootAndTrace[W Word[W], F field.Element[F]](
	program Program[W], in map[string][]byte, n uint, processor TraceProcessor[W, F],
) (rtrace.Trace[F], []error) {
	//
	var (
		states = make([][]State[W], len(program.Modules()))
		tr     rtrace.Trace[F]
		errs   []error
		bci    = interpreter.New(program, true)
	)
	// Register breakpoint handler to record all states generated during
	// tracing.
	bci = bci.BreakPointer(func(opcode uint32) {
		// Extract state from the interpreter
		fid, pc, frame := interpreter.ExtractExecutingState(bci)
		// Check whether terminating state
		terminal := opcode == encoding.RET
		// Clone the frame to ensure it is preserved until execution has
		// completed.
		frame = slices.Clone(frame)
		// Append state
		states[fid] = append(states[fid], State[W]{pc, terminal, frame})
	})
	// Execute the interpreter with appropriate breakpoints
	if _, errs = BootAndExecute(bci, in, n); len(errs) == 0 {
		// Post process trace states
		tr = postProcess(bci, states, processor)
	}
	//
	return tr, errs
}

func postProcess[W Word[W], F field.Element[F]](bci *Interpreter[W], states [][]State[W],
	processor TraceProcessor[W, F]) rtrace.Trace[F] {
	//
	var (
		binary  = bci.Binary()
		modules = make([]rtrace.ArrayModule[F], len(binary.Modules()))
	)
	// Post process trace states
	for i := range states {
		var m = binary.Module(uint16(i))
		// Decide what we've got
		if f, ok := m.(*Function[W]); ok {
			modules[i] = processor.TraceFunction(*f, states[i])
		} else {
			modules[i] = processor.TraceMemory(bci.Memory(uint16(i)))
		}
	}
	// Construct trace file
	return rtrace.NewArray(modules)
}

// State collects together information recorded when executing a single vector
// instruction.
type State[W any] struct {
	// Program Counter position.
	pc uint32
	// Terminal indicates this is a terminating state (i.e. whether or not the
	// next instruction to execute was a return).
	terminal bool
	// Values for each register in this state excluding the program counter
	// (since this is held above).
	frame []W
}

// Frame returns frame data stored in this state
func (p State[W]) Frame() []W {
	return p.frame
}

// Width returns with the width of this state.
func (p State[W]) Width() uint {
	return uint(len(p.frame))
}

// PC returns the value of program counter for this state.
func (p State[W]) PC() uint {
	return uint(p.pc)
}

// IsTerminal indicates whether or not this is a "terminal state" for the
// enclosing function.
func (p State[W]) IsTerminal() bool {
	return p.terminal
}
