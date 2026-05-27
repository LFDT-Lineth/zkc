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
package compiler

import (
	"fmt"
	"math/big"

	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/asm/io/micro"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
)

// Element provides a convenient shorthand.
type Element[F any] = field.Element[F]

// MicroFunction is a function composed entirely of micro instructions.
type MicroFunction = io.Function[micro.Instruction]

// MicroProgram is a program made up from micro- (and external) functions.
type MicroProgram = io.Program[micro.Instruction]

// MicroComponent is a component whose instructions (if applicable) are
// themselves micro instructions.  A micro function represents the lowest
// representation of a function, where each instruction is made up of
// microcodes.
type MicroComponent = io.Component[micro.Instruction]

// FunctionMapping provides information regarding the mapping of a
// assembly-level component (e.g. a function) to the corresponding columns in
// the underlying constraint system.
type FunctionMapping[T any] struct {
	// Name of the Bus
	name module.Name
	// Atomic tells us whether this is a one-line function or not.
	atomic bool
	// Registers
	registers []io.Register
	// Underlying column ids for registers
	columns []T
	// With of program counter / return line
	pcWidth, retWidth uint
}

// ColumnsOf returns the underlying column identifiers for a given set of zero
// or more registers.
func (p *FunctionMapping[T]) ColumnsOf(registers ...io.RegisterId) []T {
	columns := make([]T, len(registers))
	//
	for i, r := range registers {
		columns[i] = p.columns[r.Unwrap()]
	}
	//
	return columns
}

// Bus returns the set of input/output columns which represent the "Bus" for
// this component.
func (p *FunctionMapping[T]) Bus() []T {
	var columns []T
	//
	for i, r := range p.registers {
		if r.IsInput() || r.IsOutput() {
			columns = append(columns, p.columns[i])
		}
	}
	//
	return columns
}

// ProgramCounter returns the corresponding column
func (p *FunctionMapping[T]) ProgramCounter() T {
	var n = len(p.registers)
	//
	if p.atomic {
		panic("no program counter for atomic function")
	}
	//
	return p.columns[n]
}

// ReturnLine returns the corresponding column
func (p *FunctionMapping[T]) ReturnLine() T {
	var n = len(p.registers)
	//
	if p.atomic {
		panic("no return line for atomic function")
	}
	//
	return p.columns[n+1]
}

// Compiler packages up everything needed to compile a given assembly down into
// an HIR schema.  Observe that the compiler may fail if the assembly files are
// malformed in some way (e.g. fail type checking).
type Compiler[F Element[F], T any, E Expr[T, E], M Module[F, T, E, M]] struct {
	modules []M
	// maxInstances determines the maximum number of instances permitted for any
	// given function.
	maxInstances uint
	// Bus records
	buses []FunctionMapping[T]
	// Mapping  of Bus names to Bus records.
	busMap map[module.Name]uint
	// types & reftables
	// sourcemap
}

// NewCompiler constructs a new compiler
func NewCompiler[F Element[F], T any, E Expr[T, E],
	M Module[F, T, E, M]]() *Compiler[F, T, E, M] {
	//
	return &Compiler[F, T, E, M]{
		modules:      nil,
		maxInstances: 32,
		buses:        nil,
		busMap:       make(map[module.Name]uint),
	}
}

// Modules returns the abstract modules constructed during compilation.
func (p *Compiler[F, T, E, M]) Modules() []M {
	return p.modules
}

// Compile a given set of micro functions
func (p *Compiler[F, T, E, M]) Compile(program MicroProgram) {
	var fns = program.Components()
	//
	p.modules = make([]M, len(fns))
	p.buses = make([]FunctionMapping[T], len(fns))
	// Initialise buses
	for i, f := range fns {
		p.initModule(uint(i), f)
	}
	// Compiler functions
	for _, fn := range fns {
		p.compileComponent(fn)
	}
}

func (p *Compiler[F, T, E, M]) compileComponent(unit MicroComponent) {
	switch unit := unit.(type) {
	case *MicroFunction:
		p.compileFunction(*unit)
	default:
		panic("unknown component")
	}
}

// Compile a function with the given name, registers and micro-instructions into
// constraints.
func (p *Compiler[F, T, E, M]) compileFunction(fn MicroFunction) {
	var (
		busId = p.busMap[fn.Name()]
		// Extract module
		module = p.modules[busId]
		// Setup framing columns / constraints
		framing = p.initFunctionFraming(busId, fn)
		// Initialise buses required for this code sequence
		ioLines = p.initBuses(busId, fn)
	)
	// Construct appropriate mapping
	mapping := Translator[F, T, E, M]{
		Framing:   framing,
		Registers: fn.Registers(),
		ioLines:   ioLines,
		Columns:   p.buses[busId].columns,
	}
	// Compile each instruction in turn
	for pc, inst := range fn.Code() {
		// Core translation
		constraint := mapping.Translate(uint(pc), inst)
		// Add constraint
		module.NewConstraint(fmt.Sprintf("pc%d", pc), util.None[int](), constraint)
	}
}

// Create columns in the respective module for all registers associated with a
// given Bus component (e.g. function).
func (p *Compiler[F, T, E, M]) initModule(busId uint, fn MicroComponent) {
	var (
		module M
		bus    FunctionMapping[T]
		// padding defaults to zero
		padding big.Int
	)
	// Initialise module correctly
	module = module.Initialise(busId, fn)
	p.modules[busId] = module
	//
	bus.name = fn.Name()
	bus.registers = fn.Registers()
	bus.atomic = fn.IsAtomic()
	bus.columns = make([]T, len(fn.Registers()))
	//
	for i, reg := range fn.Registers() {
		bus.columns[i] = module.NewColumn(reg.Kind(), reg.Name(), reg.Width(), *reg.Padding())
	}
	//
	switch fn := fn.(type) {
	case *MicroFunction:
		var (
			// determine suitable width of PC register
			pcWidth = bit.Width(uint(1 + len(fn.Code())))
		)
		//
		if !bus.atomic {
			// Create program counter
			bus.columns = append(bus.columns,
				module.NewColumn(register.COMPUTED_REGISTER, io.PC_NAME, pcWidth, padding))
			// Create return line
			bus.columns = append(bus.columns,
				module.NewColumn(register.COMPUTED_REGISTER, io.RET_NAME, 1, padding))
			// Record widths for reference
			bus.pcWidth = pcWidth
			bus.retWidth = 1
		}
	default:
		panic("unknown component")
	}

	//
	p.buses[busId] = bus
	p.busMap[bus.name] = busId
}

func (p *Compiler[F, T, E, M]) initFunctionFraming(busId uint, fn MicroFunction) Framing[T, E] {
	// One line (i.e. atomic functions doen't require any framing.  They don't
	// even require a program counter!!
	if fn.IsAtomic() {
		return NewAtomicFraming[T, E]()
	}
	// Multi-line functions require proper framing.
	return p.initMultLineFunctionFraming(busId, fn)
}

func (p *Compiler[F, T, E, M]) initMultLineFunctionFraming(busId uint, fn MicroFunction) Framing[T, E] {
	var (
		module = p.modules[busId]
		bus    = p.buses[busId]
		// allocate PC register
		pc  = bus.ProgramCounter()
		ret = bus.ReturnLine()
	)
	// NOTE: a key requirement for the following constraints is that they don't
	// need an inverse computation for a shifted row (i.e. no spillage is
	// required).  In fact, this is only true because of shift normalisation.
	pc_i := Variable[T, E](pc, bus.pcWidth, 0)
	pc_im1 := Variable[T, E](pc, bus.pcWidth, -1)
	ret_i := Variable[T, E](ret, bus.retWidth, 0)
	ret_im1 := Variable[T, E](ret, bus.retWidth, -1)
	zero := Number[T, E](0)
	one := Number[T, E](1)
	// PC[i]==0 ==> RET[i]==0 (prevents lookup in padding)
	module.NewConstraint("padding", util.None[int](),
		If(pc_i.Equals(zero), ret_i.Equals(zero)))
	// PC[i-1]==0 && PC[i]!=0 ==> PC[i]==1
	module.NewConstraint("init", util.None[int](),
		If(pc_im1.Equals(zero), If(pc_i.NotEquals(zero), pc_i.Equals(one))))
	// RET[i-1]!=0 ==> PC[i]==1
	module.NewConstraint("reset", util.None[int](),
		If(ret_im1.NotEquals(zero), pc_i.Equals(one)))
	// PC[0] != 0 ==> PC[0] == 1
	module.NewConstraint("first", util.Some(0),
		If(pc_i.NotEquals(zero), pc_i.Equals(one)))
	// Add constancies for all input registers (if applicable)
	p.addInputConstancies(pc, busId, fn)
	//
	return NewMultiLineFraming[T, E](pc, bus.pcWidth, ret, bus.retWidth)
}

// Add input constancies for the given function.  That is, constraints which
// ensure the inputs don't change within a given frame.  Observe that this only
// applies for multi-line functions, as one-line functions don't have internal
// states.
func (p *Compiler[F, T, E, M]) addInputConstancies(pc T, busId uint, fn MicroFunction) {
	var (
		bus    = p.buses[busId]
		module = p.modules[busId]
		pc_i   = Variable[T, E](pc, bus.pcWidth, 0)
		zero   = Number[T, E](0)
		one    = Number[T, E](1)
	)
	// Constancies not required in padding region or for first instruction.
	for i, r := range fn.Registers() {
		if r.IsInput() {
			name := fmt.Sprintf("const_%s", r.Name())
			reg_i := Variable[T, E](bus.columns[i], r.Width(), 0)
			reg_im1 := Variable[T, E](bus.columns[i], r.Width(), -1)
			//
			module.NewConstraint(name, util.None[int](),
				If(pc_i.NotEquals(zero), If(pc_i.NotEquals(one), reg_im1.Equals(reg_i))))
		}
	}
}

// Initialise the buses linked in a given function.
func (p *Compiler[F, T, E, M]) initBuses(caller uint, fn MicroFunction) bit.Set {
	var (
		module      = p.modules[caller]
		ioRegisters bit.Set
	)
	//
	for _, bus := range fn.Buses() {
		// Callee represents the function being called by this Bus.
		var (
			name          = fmt.Sprintf("%s=>%s", fn.Name(), bus.Name)
			callerAddress = p.buses[caller].ColumnsOf(bus.Address()...)
			callerData    = p.buses[caller].ColumnsOf(bus.Data()...)
			callerLines   []T
			calleeBus     = p.buses[bus.BusId].Bus()
			calleeEnable  util.Option[T]
		)
		// Initialise caller address/data lines
		callerLines = append(callerLines, callerAddress...)
		callerLines = append(callerLines, callerData...)
		//
		if b := p.buses[bus.BusId]; !b.atomic {
			calleeEnable = util.Some(b.ReturnLine())
		}
		// Add lookup constraint
		module.NewLookup(name, callerLines, p.modules[bus.BusId], calleeBus, calleeEnable)
		// Mark caller address / data lines as io registers
		for _, r := range bus.Address() {
			ioRegisters.Insert(r.Unwrap())
		}
		// Mark caller data lines as io registers
		for _, r := range bus.Data() {
			ioRegisters.Insert(r.Unwrap())
		}
	}
	//
	return ioRegisters
}
