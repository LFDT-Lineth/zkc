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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
)

// Observer is a generic interface for extract information before and after an
// execution step of the VM.  For example, to generate debugging information.
type Observer[W machine.BaseWord[W], I instruction.Instruction, M machine.Machine[W, I]] interface {
	Initialise(machine M)
	// PreExecution is called directly before each instruction is executed
	PreExecution(machine M)
	// PostExecution is called directly after each instruction is executed.
	PostExecution(machine M)
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

// Get the ith value in this state
func (p State[W]) Get(idx uint) W {
	return p.state[idx]
}

// Width returns with the width of this state.
func (p State[W]) Width() uint {
	return uint(len(p.state))
}

// PC returns the value of program counter for this state.
func (p State[W]) PC() uint {
	return p.pc
}

// IsTerminal indicates whether or not this is a "terminal state" for the
// enclosing function.
func (p State[W]) IsTerminal() bool {
	return p.terminal
}
