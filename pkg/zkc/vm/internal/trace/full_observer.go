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
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/trace/lt"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/pool"
	"github.com/consensys/go-corset/pkg/util/collection/stack"
	"github.com/consensys/go-corset/pkg/util/field"
	util_word "github.com/consensys/go-corset/pkg/util/word"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// FullObserver is an observer which can be used to extract a trace.
type FullObserver[W word.Word[W], M machine.Core[W]] struct {
	// Contains complete frames for the trace data being constructed during
	// execution.
	trace [][]State[W]
	// Callstack contains partial data
	callstack stack.Stack[StackFrame[W]]
}

// Initialise implementation for Observer interface
func (p *FullObserver[W, M]) Initialise(machine M) {
	// initialise data structures
	p.trace = make([][]State[W], len(machine.Modules()))
	p.callstack = stack.Stack[StackFrame[W]]{}
	// initialise input ROMs
	for i, m := range machine.Modules() {
		// Check whether this is a (non-static) read-only memory
		if m, ok := m.(*memory.ReadOnly[W]); ok {
			p.trace[i] = initialiseROM(m)
		}
	}
}

// PreExecution implementation for Observer interface
func (p *FullObserver[W, M]) PreExecution(machine M) {
	// var depth = p.callstack.Len()
	// //
	// if machine.Depth() > depth {
	// 	p.enterFunction(machine)
	// } else if machine.Depth() < depth {
	// 	p.leaveFunction(machine)
	// } else if depth != 0 {
	// 	// Extract enclosing frame
	// 	var frame = machine.StackFrame(depth - 1)
	// 	// Check whether enclosing vector is finishing (i.e. about to execute a
	// 	// terminal instruction which either terminates the enclosing function, or
	// 	// moves the program counter to the next vector instruction).
	// 	if next, end := isVectorTerminal(frame, machine); next || end {
	// 		var (
	// 			contents = loadWords(0, frame.Width(), frame)
	// 			state    = NewState(frame.PC().Macro(), end, frame.Width(), contents)
	// 		)
	// 		// Record state
	// 		sf := p.callstack.Pop()
	// 		sf.states = append(sf.states, state)
	// 		p.callstack.Push(sf)
	// 	}
	// }
	panic("todo")
}

// PostExecution implementation for Observer interface
func (p *FullObserver[W, M]) PostExecution(machine M) {
}

// Trace returns an lt.TraceFile representing the given trace.
func (p *FullObserver[W, M]) Trace(machine M) lt.TraceFile {
	var (
		heap    = pool.NewLocalHeap[util_word.BigEndian]()
		builder = array.NewDynamicBuilder(heap)
		modules = make([]lt.Module[util_word.BigEndian], len(machine.Modules()))
	)
	//
	for i, t := range p.trace {
		var m = machine.Module(uint(i))
		//
		modules[i] = p.traceModule(m, t, &builder)
	}
	// Construct trace file
	return lt.NewTraceFile(nil, *heap, modules)
}

func (p *FullObserver[W, M]) traceModule(m machine.Module, states []State[W],
	builder array.Builder[util_word.BigEndian]) lt.Module[util_word.BigEndian] {
	//
	var (
		name      = trace.ModuleName{Name: m.Name(), Multiplier: 1}
		cols      []array.MutArray[util_word.BigEndian]
		nrows     = uint(len(states))
		multiLine = isMultiLineFunction(m)
	)
	// Initialise columns
	if multiLine {
		// include space for two additional control registers
		cols = make([]array.MutArray[util_word.BigEndian], m.Width()+2)
	} else {
		cols = make([]array.MutArray[util_word.BigEndian], m.Width())
	}
	// Initialise register columns
	for i, r := range m.Registers() {
		cols[i] = builder.NewArray(nrows, r.Width())
	}
	// Initialise control columns (if applicable)
	// transcribe values
	for row, st := range states {
		for i := range m.Registers() {
			var (
				val  util_word.BigEndian
				word = st.state[i]
			)
			// Copy over data
			val = val.SetBytes(word.BigInt().Bytes())
			//
			cols[i] = cols[i].Set(uint(row), val)
		}
	}
	// Set control registers for multi-line functions
	if multiLine {
		// Extract function
		f := m.(*function.Function[instruction.Word])
		// Add control registers
		p.assignControlRegisters(f, cols, states, builder)
	}
	// Done
	return lt.NewModule(name, traceColumns(m.Registers(), cols))
}

func (p *FullObserver[W, M]) assignControlRegisters(m *function.Function[instruction.Word],
	cols []array.MutArray[util_word.BigEndian], states []State[W], builder array.Builder[util_word.BigEndian]) {
	//
	var (
		zero  = field.Zero[util_word.BigEndian]()
		one   = field.One[util_word.BigEndian]()
		nrows = uint(len(states))
		pc    = uint(len(m.Registers()))
		ret   = pc + 1
		// Calculate minimum size of PC; NOTE: +1 because PC==0 is reserved for padding.
		pcWidth = bit.Width(uint(len(m.Code()) + 1))
	)
	// Initialise columns
	cols[pc] = builder.NewArray(nrows, pcWidth)
	cols[ret] = builder.NewArray(nrows, 1)
	// Assign values
	for row, st := range states {
		npc := field.Uint64[util_word.BigEndian](uint64(st.pc + 1))
		// NOTE: +1 because PC==0 reserved for padding.
		cols[pc] = cols[pc].Set(uint(row), npc)
		// Check whether this is a terminating state, or not.
		if st.terminal {
			cols[ret] = cols[ret].Set(uint(row), one)
		} else {
			cols[ret] = cols[ret].Set(uint(row), zero)
		}
	}
}

func (p *FullObserver[W, M]) enterFunction(machine M) {
	// var (
	// 	depth = p.callstack.Len()
	// 	// Extract machine frame
	// 	frame = machine.StackFrame(depth)
	// )
	// // initialise empty stack frame
	// p.callstack.Push(StackFrame[W]{id: frame.Function()})
	// // sanity check
	// if depth+1 != machine.Depth() {
	// 	panic("incorrect machine depth")
	// }
	panic("todo")
}

func (p *FullObserver[W, M]) leaveFunction(machine M) {
	// Pop executing stack frame
	frame := p.callstack.Pop()
	// Append all rows to the given trace
	p.trace[frame.id] = append(p.trace[frame.id], frame.states...)
	// sanity check
	if p.callstack.Len() != machine.Depth() {
		panic("incorrect machine depth")
	}
}

// StackFrame contains all the state related to a given function invocation
// which is currently executing.
type StackFrame[W any] struct {
	// id of function being called
	id uint
	//
	states []State[W]
}

// State collects together local state necessary for executing a given
// instruction.
type State[W any] struct {
	// Program Counter position.
	pc uint
	// Terminal indicates this is a terminating state
	terminal bool
	// Values for each register in this state excluding the program counter
	// (since this is held above).  Thus, this array has one less item than
	// registers.
	state []W
}

// NewState constructs an initial state at the given PC value for an
// invocation with the given arguments.
func NewState[W any](pc uint, terminal bool, width uint, values []W) State[W] {
	var state = make([]W, width)
	// copy over initial argument values
	copy(state, values)
	// Construct state
	return State[W]{pc, terminal, state}
}

// ============================================================================
// Helpers
// ============================================================================

func isMultiLineFunction(m machine.Module) bool {
	if f, ok := m.(*function.Function[instruction.Word]); ok {
		return !f.IsAtomic()
	}
	//
	return false
}

// Check whether the next instruction to execute will terminate the enclosing
// vector instruction.  There are two ways a vector instruction can terminate.
// Either it returns entirely from the enclosing function, or its jumps to the
// next instruction.
//
// func isVectorTerminal[W machine.BaseWord[W], I instruction.Instruction](frame machine.StackFrame[W, I],
// m machine.Core[W]) (next, end bool) {
// 	var (
// 		pc = frame.PC()
// 		// Determine enclosing function
// 		fun = m.Module(frame.Function()).(*function.Function[instruction.Word])
// 		// Determine enclosing vector
// 		vector = fun.CodeAt(pc.Macro())
// 		// Determine specific (micro) instruction
// 		insn = vector.Codes[pc.Micro()]
// 	)
// 	// See what we've got.
// 	switch insn.(type) {
// 	case *instruction.Return,
// 		*instruction.Fail:
// 		return false, true
// 	case *instruction.Jump:
// 		return true, false
// 	default:
// 		return false, false
// 	}
// }

func traceColumns[W any](regs []register.Register, cols []array.MutArray[W]) []lt.Column[W] {
	var ltcols = make([]lt.Column[W], len(cols))
	//
	for i, c := range cols {
		var name string
		// Determine name
		if i < len(regs) {
			name = regs[i].Name()
		} else if i == len(regs) {
			name = "$pc"
		} else {
			name = "$ret"
		}
		//
		ltcols[i] = lt.NewColumn(name, c)
	}
	//
	return ltcols
}

func initialiseROM[W word.Word[W]](rom *memory.ReadOnly[W]) []State[W] {
	var (
		states       []State[W]
		addressWidth = int(rom.Geometry().AddressLines())
		dataWidth    = int(rom.Geometry().DataLines())
		contents     = rom.Contents()
	)
	// sanity check (for now)
	if rom.Geometry().AddressLines() > 1 {
		panic("support ROM with multiple address lines")
	}
	//
	for i := 0; i < len(contents); i += dataWidth {
		var (
			address W
			data    = contents[i : i+dataWidth]
			words   = make([]W, rom.Width())
		)
		// Configure address line
		words[0] = address.SetUint64(uint64(i / dataWidth))
		//
		copy(words[addressWidth:], data)
		//
		states = append(states, NewState(0, false, rom.Width(), words))
	}
	//
	return states
}
