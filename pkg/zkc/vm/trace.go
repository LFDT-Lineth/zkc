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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
)

// Trace defines the type of a general trace
type Trace[F field.Element[F]] = rtrace.Trace[F]

// Element defines the type of field elements
type Element[F any] = field.Element[F]

// Tracer defines a generic mechanism for building a trace in real time as data
// is generated during tracing itself.
type Tracer[W Word[W], F any] interface {
	// Append a line for the given function to the trace.
	TraceFunctionLine(line State[W])
	// Construct the trace for a given memory of some kind
	TraceMemory(mid uint16, m RuntimeMemory[W])
	// Build the final trace
	Build() rtrace.Trace[F]
}

// BootAndTrace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  This can return one (or more) errors if, for example,
// the input is malformed (e.g. is missing expected fields and/or contains
// unexpected fields).
func BootAndTrace[W Word[W], F Element[F]](p Program[W], in map[string][]byte, n uint, tracer Tracer[W, F],
) (Trace[F], map[string][]byte, []error) {
	//
	var (
		tr   rtrace.Trace[F]
		errs []error
		bci  = interpreter.New(p, true)
		out  map[string][]byte
	)
	// Register breakpoint handler to record all states generated during
	// tracing.
	bci = bci.BreakPointer(func(opcode uint32) {
		// Extract state from the interpreter
		fid, pc, frame := interpreter.ExtractExecutingState(bci)
		// Check whether terminating state
		terminal := opcode == encoding.RET
		// NOTE: don't clone the frame here (for now) since it is always
		// converted into a slice of field elements F.
		tracer.TraceFunctionLine(State[W]{fid, pc, terminal, frame})
	})
	// TODO: reinstate memory log #2067
	// Install a recording access log on each read-write memory so its reads and
	// writes are captured during execution (consumed at post-processing by
	// ProcessReadWriteMemory).  Trace-only: fast execution keeps the no-op log.
	// for i := range bci.Binary().Modules() {
	// 	if _, isFn := bci.Binary().Module(uint16(i)).(*Function[W]); isFn {
	// 		continue
	// 	}
	// 	//
	// 	switch mem := bci.Memory(uint16(i)).(type) {
	// 	case *interpreter.RandomAccess[W]:
	// 		mem.SetLog(&interpreter.TraceableMemoryLog[W]{})
	// 	case *interpreter.PagedRandomAccess[W]:
	// 		mem.SetLog(&interpreter.TraceableMemoryLog[W]{})
	// 	}
	// }
	// Execute the interpreter with appropriate breakpoints
	if _, errs = BootAndExecute(bci, in, n); len(errs) != 0 {
		return tr, out, errs
	}
	// Finally, process memory
	var stats = util.NewPerfStats()
	// trace memory contents
	for i, m := range p.Modules() {
		// Decide what we've got
		if _, ok := m.(*Memory[W]); ok {
			tracer.TraceMemory(uint16(i), bci.Memory(uint16(i)))
		}
	}
	// Log processing costs
	stats.Log("Trace processing")
	// Encode outputs
	out = EncodeOutputs(bci)
	//
	return tracer.Build(), out, nil
}

// State collects together information recorded when executing a single vector
// instruction.
type State[W any] struct {
	// Fid identifies the executing function (module) which this state belongs to.
	fid uint16
	// Program Counter position.
	pc uint32
	// Terminal indicates this is a terminating state (i.e. whether or not the
	// next instruction to execute was a return).
	terminal bool
	// Values for each register in this state excluding the program counter
	// (since this is held above).
	frame []W
}

// Fid returns the identifier of the function (module) this state belongs to.
func (p State[W]) Fid() uint16 {
	return p.fid
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
