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
package trace

import (
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// PostProcessor captures the notion of a "post-processing" function on the
// data recorded during execution for a given module.  The goal of this is to
// decouple post-processing logic from the trace observer itself.
type PostProcessor[W any, F any] func(m machine.Module, states []State[W]) rtrace.ArrayModule[F]

// FullObserver is an observer which can be used to extract a trace.
type FullObserver[W word.Word[W], I instruction.Instruction, M machine.Machine[W, I]] struct {
	// Contains complete frames for the trace data being constructed during
	// execution.
	trace [][]State[W]
	// Callstack contains partial data
	callstack stack.Stack[StackFrame[W]]
}

// Initialise implementation for Observer interface
func (p *FullObserver[W, I, M]) Initialise(machine M) {
	// initialise data structures
	p.trace = make([][]State[W], len(machine.Modules()))
	p.callstack = stack.Stack[StackFrame[W]]{}
}

// PreExecution implementation for Observer interface
func (p *FullObserver[W, I, M]) PreExecution(machine M) {
	var depth = p.callstack.Len()
	//
	if machine.Depth() > depth {
		p.enterFunction(machine)
		return
	} else if machine.Depth() < depth {
		p.leaveFunction(machine)
		// NOTE: control has now returned to the caller, which may itself be about
		// to execute a terminal instruction (e.g. a "return" immediately following
		// the call, as in a recursive helper).  Fall through to record that
		// caller's terminal state, otherwise its row would be lost.
	}
	// Record the terminal state of the (now) enclosing frame, if any.
	if p.callstack.Len() != 0 {
		p.recordTerminalState(machine)
	}
}

func (p *FullObserver[W, I, M]) enterFunction(machine M) {
	var (
		depth = p.callstack.Len()
		// Extract machine frame
		frame = machine.StackFrame(0)
	)
	// initialise empty stack frame
	p.callstack.Push(StackFrame[W]{id: frame.FunctionId()})
	// sanity check
	if depth+1 != machine.Depth() {
		panic("incorrect machine depth")
	}
}

func (p *FullObserver[W, I, M]) leaveFunction(machine M) {
	// Pop executing stack frame
	frame := p.callstack.Pop()
	// Append all rows to the given trace
	p.trace[frame.id] = append(p.trace[frame.id], frame.states...)
	// sanity check
	if p.callstack.Len() != machine.Depth() {
		panic("incorrect machine depth")
	}
}

// recordTerminalState records a row for the currently-executing frame when it
// is about to execute a terminal instruction (i.e. one which either terminates
// the enclosing function, or moves the program counter to the next vector
// instruction).
func (p *FullObserver[W, I, M]) recordTerminalState(machine M) {
	// Extract enclosing frame
	var frame = machine.StackFrame(0)
	//
	if next, end := isVectorTerminal(frame); next || end {
		var (
			width    = frame.Function().Width()
			contents = loadWords(0, width, frame)
			state    = NewState(frame.PC().Macro(), end, width, contents)
		)
		// Record state
		sf := p.callstack.Pop()
		sf.states = append(sf.states, state)
		p.callstack.Push(sf)
	}
}

// PostExecution implementation for Observer interface
func (p *FullObserver[W, I, M]) PostExecution(machine M) {
	// do nout
}

// Trace performs post-processing on the recorded state information, and
// constructs a suitable (row-major) trace.
func Trace[W word.Word[W], I instruction.Instruction, M machine.Machine[W, I], F any](
	p *FullObserver[W, I, M], machine M, processor PostProcessor[W, F]) rtrace.Trace[F] {
	//
	var modules = make([]rtrace.ArrayModule[F], len(machine.Modules()))
	//
	for i, t := range p.trace {
		modules[i] = processor(machine.Module(uint(i)), t)
	}
	// Construct trace file
	return rtrace.NewArray(modules)
}

func loadWords[W word.Word[W], I instruction.Instruction](start, end uint, frame machine.StackFrame[W, I]) []W {
	var (
		n     = end - start
		words = make([]W, n)
	)
	// Read words
	for i := range n {
		// construct register ID
		var rid = register.NewId(i + start)
		// Read ith word
		words[i] = frame.Load(rid)
	}
	// Done
	return words
}

// Check whether the next instruction to execute will terminate the enclosing
// vector instruction.  There are two ways a vector instruction can terminate.
// Either it returns entirely from the enclosing function, or its jumps to the
// next instruction.
func isVectorTerminal[W machine.BaseWord[W], I instruction.Instruction](frame machine.StackFrame[W, I],
) (next, end bool) {
	var (
		pc = frame.PC()
		// Determine enclosing function
		fun = frame.Function()
		// Determine enclosing vector
		vector = fun.CodeAt(pc.Macro())
		// Determine specific (micro) instruction
		insn any = vector.Codes[pc.Micro()]
	)
	// See what we've got.
	switch insn.(type) {
	case *instruction.Return,
		*instruction.Fail:
		return false, true
	case *instruction.Jump:
		return true, false
	default:
		return false, false
	}
}
