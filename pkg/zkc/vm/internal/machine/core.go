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
package machine

import (
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
)

// Executor --- see documentation on vm.Executor
type Executor[W any, I any] interface {
	// Execute the given instruction in the given frame with the given register
	// descriptors, possibly returning an error if something goes wrong (e.g. an
	// overflow).
	Execute(insn I, frame []W, regs []register.Register) error
}

// Core represents the state of an executing machine, including the state of
// all registers, memories and functions.  A machine may be executing or
// terminated.  Machines are abstracted over a given type of word W, and
// instruction I.  For example, a machine could be operating over 16bit words or
// 8bit words, etc (i.e. as determined by the underlying field).  Furthermore, a
// machine may be operating over instructions compiled into bytes (for efficient
// execution), or instructions represented at a higher level (e.g. for analysis
// or compilation).
type Core[W any] interface {
	// Execute the machine for the given number of steps, returning the actual
	// number of steps executed and an error (if execution failed).
	Execute(steps uint) (uint, error)
	// Return ith module in this machine (either a function or some form of memory).
	Module(id uint) Module
	// Return set of modules in this machine.
	Modules() []Module
	// Depth returns the depth of the call stack.
	Depth() uint
	// StackFrame returns the nth stack frame, where n==0 returns the root frame.
	StackFrame(n uint) Frame[W]
	// Enter a new function on the call-stack, whilst initialising its arguments
	// with those values in the current frame taken from the given argument
	// registers.  In addition, the return registers are saved for when (if) the
	// function returns.  Specifically, the return registers will be assigned
	// the return values from the callee.
	Enter(id uint, frame []W, args, returns []register.Id) error
	// Leave pops the current stack frame off the stack, whilst ensuring the
	// return values are written into the return registers.  This also returns
	// true if the last frame was popped off the stack (i.e. the machine has
	// terminated).
	Leave() (bool, error)
}

// Module represents an either a function or memory within the machine.
type Module = base.Module

// ============================================================================
// Frame
// ============================================================================

// Frame represents an executing function on the call stack.  Specifically,
// it contains the state of all registers at the current point of execution for
// that function.
type Frame[W any] struct {
	// Function identifier
	functionId uint
	// Program Counter
	pc ProgramCounter
	// Number of inputs (i.e. arguments)
	args uint
	// Registers
	registers []W
	// Returns identifies those registers in the target frame which should be
	// assigned the return values of this call.
	returns []register.Id
}

// NewFrame constructs an initially empty frame for a function with a given
// number of registers.
func NewFrame[W any](fid, width, args uint, returns []register.Id) Frame[W] {
	return Frame[W]{
		functionId: fid,
		pc:         ProgramCounter{0, 0},
		registers:  make([]W, width),
		args:       args,
		returns:    returns,
	}
}

// Function identifies the function to which this stack frame corresponds.
func (p *Frame[W]) Function() uint {
	return p.functionId
}

// PC returns the current Program Counter position.
func (p *Frame[W]) PC() ProgramCounter {
	return p.pc
}

// Goto sets the Program Counter to a given position.
func (p *Frame[W]) Goto(pc ProgramCounter) {
	p.pc = pc
}

// Load the value of the ith register from this stack frame.
func (p *Frame[W]) Load(reg uint) W {
	return p.registers[reg]
}

// Return the value of the ith return (i.e. output) register from this stack
// frame.
func (p *Frame[W]) Return(reg uint) W {
	return p.registers[p.args+reg]
}

// Store a given value into the ith register of this stack frame, overwriting
// its previous contents.
func (p *Frame[W]) Store(reg uint, value W) {
	p.registers[reg] = value
}

// Width returns the number of values in this stack frame.
func (p *Frame[W]) Width() uint {
	return uint(len(p.registers))
}

// SignatureOf returns a simple string representation of the enclosing function
// for the given frame, including its arguments and PC position.
func SignatureOf[W BaseWord[W], I Instruction](frame Frame[W], modules []Module) string {
	var (
		builder strings.Builder
		fn      = modules[frame.Function()].(*function.Function[I])
	)
	//
	builder.WriteString(fn.Name())
	builder.WriteString("(")

	for i := 0; i != int(fn.NumInputs()); i++ {
		if i != 0 {
			builder.WriteString(",")
		}

		builder.WriteString("0x")
		builder.WriteString(frame.registers[i].Text(16))
	}

	builder.WriteString(")")
	//
	return builder.String()
}

// ============================================================================
// Program Counter
// ============================================================================

// ProgramCounter --- see vm.ProgramCounter for documentation.
type ProgramCounter struct {
	// Program Counter (PC) identifies the macro instruction being executed
	macroCounter uint
	// Code Counter (CC) identifies the micro code within the enclosing
	// instruction being executed.
	microCounter uint
}

// Macro returns the macro instruction identfied by this program counter
// position.
func (p ProgramCounter) Macro() uint {
	return p.macroCounter
}

// Micro returns the micro code within the enclosing macro instruction identfied
// by this program position.
func (p ProgramCounter) Micro() uint {
	return p.microCounter
}

// First checks whether this PC value represents location (0,0) i.e. the start
// of a trace.
func (p ProgramCounter) First() bool {
	return p.microCounter == 0 && p.macroCounter == 0
}

// Next shifts the program counter to the next instruction, assuming the current
// instruction has a given width (i.e. number of micro-instructions).
func (p ProgramCounter) Next(width uint) ProgramCounter {
	var ncc = p.microCounter + 1
	//
	if ncc >= width {
		return p.Goto(p.macroCounter + 1)
	}
	//
	return ProgramCounter{p.macroCounter, ncc}
}

// Goto a given (macro) instruction.  This sets the macro counter to a given
// position, and resets the micro counter.  If the enclosing function has too
// few macro instructions, then this will result in a machine failure on the
// next cycle.
func (p ProgramCounter) Goto(pc uint) (q ProgramCounter) {
	q.macroCounter = pc
	q.microCounter = 0
	//
	return q
}

// Skip over some number of (micro) instructions.  If the enclosing
// instruction has too few micro instructions, then this will result in a
// machine failure on the next cycle.
func (p ProgramCounter) Skip(n uint) (q ProgramCounter) {
	q.macroCounter = p.macroCounter
	q.microCounter = p.microCounter + n
	//
	return q
}

func (p ProgramCounter) String() string {
	return fmt.Sprintf("%02d.%02d", p.macroCounter, p.microCounter)
}
