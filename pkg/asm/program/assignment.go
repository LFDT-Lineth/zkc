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
package program

import (
	"fmt"
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Assignment represents a wrapper around an instruction in order for it to
// conform to the schema.Assignment interface.
type Assignment[F field.Element[F], T io.Instruction] struct {
	id sc.ModuleId
	fn *io.Function[T]
}

// NewAssignment constructs a new assignment capable of trace filling for a
// given function.
func NewAssignment[F field.Element[F], T io.Instruction](id sc.ModuleId, fn *io.Function[T],
) *Assignment[F, T] {
	//
	return &Assignment[F, T]{id, fn}
}

// Bounds implementation for schema.Assignment interface.
func (p Assignment[F, T]) Bounds(module uint) util.Bounds {
	return util.EMPTY_BOUND
}

// Compute implementation for schema.Assignment interface.
func (p Assignment[F, T]) Compute(trace tr.Trace[F], schema sc.AnySchema[F]) ([]array.MutArray[F], error) {
	//
	var (
		trModule   = trace.Module(p.id)
		states     []io.State
		iomap      = NewTraceIoMap(trace)
		numInputs  = p.fn.NumInputs()
		numOutputs = p.fn.NumOutputs()
	)
	// Trace given rows
	for i := range trModule.Height() {
		inputs := extractValues(i, trModule, 0, p.fn.NumInputs())
		outputs := extractValues(i, trModule, numInputs, numInputs+numOutputs)
		sts := p.trace(inputs, outputs, iomap)
		states = append(states, sts...)
	}
	//
	return p.states2columns(trModule.Width(), states, trace.Builder()), nil
}

// Consistent implementation for schema.Assignment interface.
func (p Assignment[F, T]) Consistent(sc.AnySchema[F]) []error {
	return nil
}

// Lisp implementation for schema.Assignment interface.
func (p Assignment[F, T]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	//
	var cols []sexp.SExp
	//
	for _, r := range p.fn.Registers() {
		cols = append(cols, sexp.NewSymbol(r.Name()))
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("compute"),
		sexp.NewSymbol(p.fn.Name().String()),
		sexp.NewList(cols),
	})
}

// RegistersExpanded implementation for schema.Assignment interface.
func (p Assignment[F, T]) RegistersExpanded() []register.Ref {
	return p.RegistersRead()
}

// RegistersRead implementation for schema.Assignment interface.
func (p Assignment[F, T]) RegistersRead() []register.Ref {
	var regs []register.Ref
	//
	for i, reg := range p.fn.Registers() {
		if reg.IsInputOutput() {
			rid := register.NewId(uint(i))
			regs = append(regs, register.NewRef(p.id, rid))
		}
	}
	//
	return regs
}

// RegistersWritten implementation for schema.Assignment interface.
func (p Assignment[F, T]) RegistersWritten() []register.Ref {
	var (
		regs       []register.Ref
		nRegisters = len(p.fn.Registers())
		multiLine  = len(p.fn.Code()) > 1
	)
	// Include control registers for multi-line functions.
	if multiLine {
		nRegisters += 2
	}
	// Trace expansion writes to all registers, including input/outputs.
	// This is because it may expand the I/O registers.
	for i := range nRegisters {
		rid := register.NewId(uint(i))
		regs = append(regs, register.NewRef(p.id, rid))
	}
	//
	return regs
}

// Substitute implementation for schema.Assignment interface.
func (p Assignment[F, T]) Substitute(map[string]F) {
	// Do nothing since assembly instructions do not (at the time of writing)
	// employ labelled constants.
}

// Trace a given function with the given arguments in a given I/O environment to
// produce a given set of output values, along with the complete set of internal
// traces.  The expected outputs are not strictly necessary here, but are
// included so they can be checked against the internally generated outputs to
// ensure internal consistency.
func (p Assignment[F, T]) trace(inputs, outputs []big.Int, iomap io.Map) []io.State {
	var (
		code   = p.fn.Code()
		states []io.State
		// Construct local state
		state = io.InitialState(inputs, p.fn.Registers(), p.fn.Buses(), iomap)
		// Program counter position
		pc uint = 0
	)
	// Keep executing until we're done.
	for pc != io.RETURN && pc != io.FAIL {
		insn := code[pc]
		// execute given instruction
		pc = insn.Execute(state)
		// record internal state
		states = append(states, finaliseState(pc == io.RETURN, state, outputs))
		// update state pc
		state.Goto(pc)
	}
	// Done
	return states
}

// Convert a given set of states into a corresponding set of array columns.
func (p Assignment[F, T]) states2columns(width uint, states []io.State, builder array.Builder[F]) []array.MutArray[F] {
	var (
		cols      = make([]array.MutArray[F], width)
		nrows     = uint(len(states))
		multiLine = len(p.fn.Code()) > 1
	)
	// Initialise register columns
	for i, r := range p.fn.Registers() {
		cols[i] = builder.NewArray(nrows, r.Width())
	}
	// Initialise control columns (if applicable)
	// transcribe values
	for row, st := range states {
		for i := range p.fn.Registers() {
			var (
				val F
				rid = register.NewId(uint(i))
			)
			//
			val = val.SetBytes(st.Load(rid).Bytes())
			//
			cols[i] = cols[i].Set(uint(row), val)
		}
	}
	// Set control registers for multi-line functions
	if multiLine {
		p.assignControlRegisters(cols, states, builder)
	}
	// Done
	return cols
}

func (p Assignment[F, T]) assignControlRegisters(cols []array.MutArray[F], states []io.State,
	builder array.Builder[F]) {
	//
	var (
		zero  = field.Zero[F]()
		one   = field.One[F]()
		nrows = uint(len(states))
		pc    = uint(len(p.fn.Registers()))
		ret   = pc + 1
		// Calculate minimum size of PC; NOTE: +1 because PC==0 is reserved for padding.
		pcWidth = bit.Width(uint(len(p.fn.Code()) + 1))
	)
	// Initialise columns
	cols[pc] = builder.NewArray(nrows, pcWidth)
	cols[ret] = builder.NewArray(nrows, 1)
	// Assign values
	for row, st := range states {
		npc := field.Uint64[F](uint64(st.Pc() + 1))
		// NOTE: +1 because PC==0 reserved for padding.
		cols[pc] = cols[pc].Set(uint(row), npc)
		// Check whether this is a terminating state, or not.
		if st.IsTerminal() {
			cols[ret] = cols[ret].Set(uint(row), one)
		} else {
			cols[ret] = cols[ret].Set(uint(row), zero)
		}
	}
}

// Finalising a given state does two things: firstly, it clones the state;
// secondly, if the state has terminated, it makes sure the outputs match the
// original trace.
func finaliseState(terminated bool, state io.State, outputs []big.Int) io.State {
	// Clone state
	var nstate = state.Clone()
	// Cheeck whether terminal state
	if terminated {
		// NOTE: the following
		checkConsistentOutputs(nstate, outputs)
		// Mark state as terminated
		nstate.Terminate()
	}
	//
	return nstate
}

// Internal consistency check.  This is useful for detecting mis-translations
// from the ASM level down to the UASM level.  Specifically, inconsistent traces
// are detected at the ASM level.  Thus, a consistent trace at the ASM level can
// be transformed into an inconsistent one (e.g. due to a bug somewhere) and
// this goes unnoticed.
func checkConsistentOutputs(state io.State, outputs []big.Int) {
	var index = 0
	//
	for i, reg := range state.Registers() {
		if reg.IsOutput() {
			var (
				rid = register.NewId(uint(i))
				// Read out actual output value
				actual = state.Load(rid)
				// Get expected value
				expected = outputs[index]
			)
			// Check actual output matches expected output
			if actual.Cmp(&expected) != 0 {
				// Following should be unreachable unless there is a bug
				// somewhere (e.g. when translating from ASM to UASM).
				panic(fmt.Sprintf("computed output for register \"%s\" does not match expected output (0x%s vs 0x%s)",
					reg.Name(), actual.Text(16), expected.Text(16)))
			}
			//
			index++
		}
	}
}

// TraceIoMap adapts a trace to look like an instance of io.Map.  This assumes
// all function instances are already recorded in the trace (i.e. trace
// propagation has already occurred).
type TraceIoMap[F field.Element[F]] struct {
	trace tr.Trace[F]
}

// NewTraceIoMap constructs a new instance of TraceIoMap[F].
func NewTraceIoMap[F field.Element[F]](trace tr.Trace[F]) io.Map {
	return TraceIoMap[F]{trace}
}

// Read implementation of the io.Map interface.
func (p TraceIoMap[F]) Read(bus uint, address []big.Int, nOutputs uint) []big.Int {
	var (
		nInputs  = uint(len(address))
		mod      = p.trace.Module(bus)
		fAddress = field.BigInts[F](address)
		row      = mod.FindLast(fAddress...)
	)
	//
	if row == math.MaxUint {
		panic(fmt.Sprintf("read failure for %s%s", mod.Name().String(), array.ToString(fAddress)))
	}
	//
	return extractValues(row, mod, nInputs, nInputs+nOutputs)
}

// Write implementation of the io.Map interface.
func (p TraceIoMap[F]) Write(bus uint, address []big.Int, values []big.Int) {
	panic("unsupported operation")
}

func extractValues[F field.Element[F]](row uint, mod tr.Module[F], start, end uint) []big.Int {
	var (
		n      = end - start
		values = make([]big.Int, n)
	)
	// Initialise arguments
	for i := start; i < end; i++ {
		var (
			val = mod.Column(i).Data().Get(row)
			ith big.Int
		)
		// Clone big int.
		ith.SetBytes(val.Bytes())
		// Assign to ith register
		values[i-start] = ith
	}
	//
	return values
}
