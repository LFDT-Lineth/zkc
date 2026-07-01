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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_word "github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

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
	// initialise input ROMs and static reference tables
	for i, m := range machine.Modules() {
		switch m := m.(type) {
		case *memory.StaticReadOnly[W]:
			// Static reference tables (e.g. the $range_un range-check tables) are
			// not populated by execution, so their full contents must be seeded
			// here; otherwise lookups into them would see an empty target.
			p.trace[i] = initializeMemoryAddressesAndContents(&m.ReadOnly)
		case *memory.ReadOnly[W]:
			// (non-static) read-only memory, i.e. an input ROM.
			p.trace[i] = initializeMemoryAddressesAndContents(m)
		case *memory.WriteOnce[W]:
			// write-once output memory.
			p.trace[i] = initializeMemoryAddressesAndContents(m)
		}
	}
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
}

// Trace returns an lt.TraceFile representing the given trace.
func (p *FullObserver[W, I, M]) Trace(machine M) lt.TraceFile {
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

func (p *FullObserver[W, I, M]) traceModule(m machine.Module, states []State[W],
	builder array.Builder[util_word.BigEndian]) lt.Module[util_word.BigEndian] {
	//
	var (
		name                = trace.ModuleName{Name: m.Name(), Multiplier: 1}
		cols                []array.MutArray[util_word.BigEndian]
		nrows               = uint(len(states))
		isMultiLineFunction = isMultiLineFunction(m)
		isAccessOnceMemory  = isAccessOnceMemory[W](m)
		extra               = extraColumnsForAccessOnceMemory[W](m)
	)
	// Initialise columns
	switch {
	case isMultiLineFunction:
		// include space for the program counter, return line and one selector
		// per instruction.
		nSelectors := uint(len(m.(*function.Function[instruction.Word]).Code()))
		cols = make([]array.MutArray[util_word.BigEndian], m.Width()+2+nSelectors)
	case isAccessOnceMemory:
		cols = make([]array.MutArray[util_word.BigEndian], m.Width()+extra)
	default:
		cols = make([]array.MutArray[util_word.BigEndian], m.Width())
	}
	// Initialise register columns
	for i, r := range m.Registers() {
		//
		if r.IsNative() {
			cols[i] = builder.NewArray(nrows, math.MaxUint)
		} else {
			cols[i] = builder.NewArray(nrows, r.Width())
		}
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
	// Names for any trailing (computed/control) columns beyond the registers.
	// These must match the corresponding schema register names, since the trace
	// is aligned to the schema by column name.
	var auxNames []string
	// Set control registers for multi-line functions
	if isMultiLineFunction {
		// Extract function
		f := m.(*function.Function[instruction.Word])
		// Add control registers
		p.assignControlRegisters(f, cols, states, builder)
		// PC, RET, then one selector per code line.
		auxNames = append(auxNames, io.PC_NAME, io.RET_NAME)
		for c := range f.Code() {
			auxNames = append(auxNames, io.SelectorName(uint(c)))
		}
	}
	//
	if isAccessOnceMemory {
		mem, ok := m.(memory.Memory[W])
		if !ok {
			panic("expected memory module")
		}

		p.assignRomWomRegisters(mem, cols, states, builder)
		// access bit, then (for multi-limb addresses) one at_flag per limb.
		auxNames = append(auxNames, io.ACCESS_BIT_NAME)

		if mem.Geometry().IsMultiLineAddress() {
			for k := uint(0); k < mem.Geometry().AddressLines(); k++ {
				auxNames = append(auxNames, io.AtFlagName(k))
			}
		}
	}

	return lt.NewModule(name, traceColumns(m.Registers(), auxNames, cols))
}

func (p *FullObserver[W, I, M]) assignRomWomRegisters(
	mem memory.Memory[W],
	cols []array.MutArray[util_word.BigEndian],
	states []State[W],
	builder array.Builder[util_word.BigEndian],
) {
	var (
		one                = field.One[util_word.BigEndian]()
		extra              = extraColumnsForAccessOnceMemory[W](mem)
		accessOffset       = uint(len(mem.Registers()))
		atFlagOffset       = accessOffset + 1
		nRows              = uint(len(states))
		nLines             = mem.Geometry().AddressLines()
		isMultiLineAddress = mem.Geometry().IsMultiLineAddress()
	)

	// Initialise columns
	cols[accessOffset] = builder.NewArray(nRows, 1)
	for i := uint(1); i < extra; i++ {
		cols[accessOffset+i] = builder.NewArray(nRows, 1)
	}

	// Assign values
	for row, st := range states {
		cols[accessOffset] = cols[accessOffset].Set(uint(row), one)
		if isMultiLineAddress && row != 0 {
			var k uint = nLines - 1
			for k > 0 && st.state[k].Cmp64(0) == 0 {
				k--
			}

			cols[atFlagOffset+k] = cols[atFlagOffset+k].Set(uint(row), one)
		}
	}
}

func (p *FullObserver[W, I, M]) assignControlRegisters(m *function.Function[instruction.Word],
	cols []array.MutArray[util_word.BigEndian], states []State[W], builder array.Builder[util_word.BigEndian]) {
	//
	var (
		zero  = field.Zero[util_word.BigEndian]()
		one   = field.One[util_word.BigEndian]()
		nrows = uint(len(states))
		pc    = uint(len(m.Registers()))
		ret   = pc + 1
		// First selector column; one selector per instruction follows.
		sel = ret + 1
		// Calculate minimum size of PC; NOTE: +1 because PC==0 is reserved for padding.
		pcWidth = bit.Width(uint(len(m.Code()) + 1))
	)
	// Initialise columns
	cols[pc] = builder.NewArray(nrows, pcWidth)
	cols[ret] = builder.NewArray(nrows, 1)
	// Initialise is_PC_* selector columns (one per instruction).
	for c := range m.Code() {
		cols[sel+uint(c)] = builder.NewArray(nrows, 1)
	}
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
		// Set is_PC_i to 1 when PC == i
		cols[sel+st.pc] = cols[sel+st.pc].Set(uint(row), one)
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

func isMultiLineFunction(m machine.Module) bool {
	if f, ok := m.(*function.Function[instruction.Word]); ok {
		return !f.IsAtomic()
	}
	//
	return false
}

// isAccessOnceMemory detects WriteOnce and ReadOnce memories.
func isAccessOnceMemory[W word.Word[W]](m machine.Module) bool {
	if mem, ok := m.(memory.Memory[W]); ok {
		switch {
		case mem.IsStatic() || mem.IsReadWrite():
			return false
		case mem.IsWriteOnly() || mem.IsReadOnly():
			return true
		default:
			panic("undefined memory type")
		}
	}

	return false
}

// extraColumnsForAccessOnceMemory computes the number of extra columns
// that a ROM/WOM module requires:
//   - one comes from the ACCESS_BIT column
//   - whenever L > 1, where L is the number of address lines, there
//     are also the at_flags_k, k = 0..L, columns
func extraColumnsForAccessOnceMemory[W word.Word[W]](m machine.Module) uint {
	if mem, ok := m.(memory.Memory[W]); ok {
		switch {
		case mem.IsStatic() || mem.IsReadWrite():
			return uint(0)
		case mem.IsWriteOnly() || mem.IsReadOnly():
			extra := uint(1)
			if mem.Geometry().IsMultiLineAddress() {
				extra += mem.Geometry().AddressLines()
			}

			return extra
		default:
			panic("undefined memory type")
		}
	}

	return uint(0)
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

func traceColumns[W any](regs []register.Register, auxNames []string, cols []array.MutArray[W]) []lt.Column[W] {
	var ltcols = make([]lt.Column[W], len(cols))
	//
	for i, c := range cols {
		var name string
		// Register columns are named directly; trailing (computed/control)
		// columns take their names from auxNames, supplied by the caller.
		if i < len(regs) {
			name = regs[i].Name()
		} else {
			name = auxNames[i-len(regs)]
		}
		//
		ltcols[i] = lt.NewColumn(name, c)
	}
	//
	return ltcols
}

// initializeMemoryAddressesAndContents materializes the trace rows for a
// static, read-only (ROM) or write-once (WOM) memory: one row per cell with
// consecutive addresses (single- or multi-limb, via splitAddress) plus the
// cell's data.  It only seeds the address and data register columns; the
// access-once control columns (ACCESS bit, at_flags) are added separately in
// traceModule and only for ROM/WOM, not static memory.
func initializeMemoryAddressesAndContents[W word.Word[W]](m memory.Memory[W]) []State[W] {
	var (
		states       []State[W]
		addressWidth = int(m.Geometry().AddressLines())
		dataWidth    = int(m.Geometry().DataLines())
		contents     = m.Contents()
	)
	// sanity check (for now)
	var isMultiLineAddressWom = m.Geometry().IsMultiLineAddress()

	switch {
	case !isMultiLineAddressWom:
		for i := 0; i < len(contents); i += dataWidth {
			var (
				address W
				data    = contents[i : i+dataWidth]
				words   = make([]W, m.Width())
			)
			// Configure address line
			words[0] = address.SetUint64(uint64(i / dataWidth))
			//
			copy(words[addressWidth:], data)
			//
			states = append(states, NewState(0, false, m.Width(), words))
		}
	case isMultiLineAddressWom:
		var (
			masks  = make([]uint64, addressWidth)
			widths = make([]uint, addressWidth)
		)

		for i := range addressWidth {
			// TODO: is the usage of Uint64() problematic ? Are registers short ?
			masks[i] = m.Registers()[i].MaxValue().Uint64()
			widths[i] = m.Registers()[i].Width()
		}

		var addressUint64 = uint64(0)

		for i := 0; i < len(contents); i += dataWidth {
			var (
				data  = contents[i : i+dataWidth]
				words = make([]W, m.Width())
			)
			// Configure address line
			copy(words[:addressWidth], splitAddress[W](masks, widths, addressUint64))
			//
			copy(words[addressWidth:], data)
			//
			states = append(states, NewState(0, false, m.Width(), words))
			addressUint64++
		}
	}
	//
	return states
}

// split address takes an address uint64 and returns this address split according
// to a slice of bit widths
//
// **Note.** splitAddress expects an address that fits into a uint64. This must be the
// case for ROM's and WOM's, where the address space is expected to be read beginning to
// end and to contain no gaps. General multi-line RAM's can be of arbitrary size and have
// sparsely allocated memory.
func splitAddress[W word.Word[W]](masks []uint64, widths []uint, address uint64) []W {
	addressWidth := len(masks)

	var split = make([]W, addressWidth)
	for i := addressWidth - 1; i >= 0; i-- {
		split[i] = split[i].SetUint64(address & masks[i])
		address = address >> widths[i]
	}

	return split
}
