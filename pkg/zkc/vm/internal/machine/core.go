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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
)

// Executor --- see documentation on vm.Executor
type Executor[W BaseWord[W], I Instruction] interface {
	// Execute the given instruction in the given frame with the given register
	// descriptors, possibly returning an error if something goes wrong (e.g. an
	// overflow).
	Execute(insn I, frame StackFrame[W, I]) error
}

// Core provides a minimal interface for booting and executing a machine with a
// given set of inputs, and collecting the outputs afterwards.
type Core[W BaseWord[W]] interface {
	// Boot this machine by starting the given function from the current state.
	// This function assumes the given inputs are correctly formed.
	// Furthermore, it is recommended to perform sanity checking on input prior
	// to calling this function.
	Boot(fun string) error
	// Execute the machine for the given number of steps, returning the actual
	// number of steps executed and an error (if execution failed).
	Execute(steps uint) (uint, error)
	// Return array of (non-static) input memories
	Inputs() []memory.InputOutput[W]
	// Return array of output memories
	Outputs() []memory.InputOutput[W]
}

// Machine represents the state of an executing machine, including the state of
// all registers, memories and functions.  A machine may be executing or
// terminated.  Machines are abstracted over a given type of word W, and
// instruction I.  For example, a machine could be operating over 16bit words or
// 8bit words, etc (i.e. as determined by the underlying field).  Furthermore, a
// machine may be operating over instructions compiled into bytes (for efficient
// execution), or instructions represented at a higher level (e.g. for analysis
// or compilation).
type Machine[W BaseWord[W], I Instruction] interface {
	Core[W]
	// Return ith module in this machine (either a function or some form of memory).
	Module(id uint) Module
	// Return set of modules in this machine.
	Modules() []Module
	// Depth returns the depth of the call stack.
	Depth() uint
	// StackFrame returns the nth stack frame, where n==0 returns the root frame.
	StackFrame(n uint) StackFrame[W, I]
}

// Module represents an either a function or memory within the machine.
type Module = base.Module

// ============================================================================
// Program Counter
// ============================================================================

// PC_UNUSED represents the location 0,0.  It is intended to clarify situations
// where the given PC value is not actually used.
var PC_UNUSED = ProgramCounter{}

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
