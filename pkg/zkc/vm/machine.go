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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
)

// Core provides a minimal interface for booting and executing a machine with a
// given set of inputs, and collecting the outputs afterwards.
type Core[W MachineWord[W]] = machine.Core[W]

// Machine represents the state of an executing "machine", including all
// registers, memories and functions.  A machine may be executing or terminated.
// Machines are abstracted over a given type of word W, and instruction I.  For
// example, a machine could be operating over 16bit words or 8bit words, or over
// field elements directly.  Furthermore, a machine may be operating over
// instructions compiled into bytes (for efficient execution), or instructions
// represented at a higher level (e.g. for analysis or compilation).
type Machine[W MachineWord[W], I Instruction] = machine.Machine[W, I]

// Executor captures the notion of an instruction-specific executor.  That is,
// an executor designed for executing certain instructions over a given type of
// machine word (e.g. a Word or a field.Element, etc).  A key aspect of the
// executor is that its really only intended for straight-line instructions, and
// other control-flow instructions (e.g. skipping, calling, etc) are handled by
// the base machine (since they are common to all machines).
type Executor[W MachineWord[W], I Instruction] = machine.Executor[W, I]

// StackFrame captures the state of an executing function on the call stack.
// Specifically, it contains the state of all registers at the current point of
// execution.
type StackFrame[W MachineWord[W], I Instruction] = machine.StackFrame[W, I]

// ProgramCounter abstracts the notion of a program counter in a machine.  A key
// aspect is that it two dimensional to account for so-called "vector"
// instructions: (1) it identifies the (macro) instruction being executed; (2)
// it identifies the (micro) instruction within that being executed.
type ProgramCounter = machine.ProgramCounter

// MachineWord captures the minimal set of requirements for a word used in the base
// machine.
type MachineWord[W any] = machine.BaseWord[W]

// BaseMachine provides a fundamental machine implementation.  The intention is
// that other machine variations build off this by providing executors specific
// to their instruction set.
type BaseMachine[W MachineWord[W], I Instruction, E Executor[W, I]] = machine.Base[W, I, E]

// ============================================================================
// Word Machine
// ============================================================================

// WordMachine is a machine which operates over standard machine words.
type WordMachine[W Word[W]] = machine.Word[W]

// WordFunction is a function made up of word instructions.
type WordFunction = Function[instruction.Word]

// WordInstruction is an instruction which operates over standard machine words.
type WordInstruction = instruction.Word

// ============================================================================
// Field Machine
// ============================================================================

// FieldFunction is a function made up of field instructions.
type FieldFunction = Function[instruction.Field]

// FieldMachine is a machine which operates over field elements only.
type FieldMachine[F field.Element[F]] = machine.Field[F]

// FieldInstruction is an instruction which operates over field elements only.
type FieldInstruction = instruction.Field

// ============================================================================
// Constructors
// ============================================================================

// NewFunction constructs a new function with the given components.  The native
// flag indicates whether this function is backed by a native circuit (i.e.
// declared with the @native annotation) rather than by code.
func NewFunction[I Instruction](name string, native bool, registers []register.Register,
	code []instruction.Vector[I]) *Function[I] {
	return function.New(name, native, registers, code)
}

// NewWordMachine constructs a new word machine with a given set of modules and
// a given field configuration (for native field instructions).
func NewWordMachine[W Word[W]](field field.Config, modules ...Module) *WordMachine[W] {
	return machine.NewWord[W](field, modules...)
}

// BootAndExecute executes the program embodied by these constraints in chunks
// of n steps at a time, producing any outputs arising.  Execution is faster
// than trace because it does not record any internal information about the
// trace --- it simply extracts the outputs at the end.
func BootAndExecute[W Word[W], M Core[W]](m M, input map[string][]byte, n uint,
) (output map[string][]byte, errs []error) {
	//
	var (
		steps  uint
		inputs map[string][]W
		stats  = util.NewPerfStats()
	)
	// Execute machine in chunks of 1K steps
	if inputs, errs = DecodeInputs(m, input); len(errs) != 0 {
		return nil, errs
	}
	// Initialise inputs
	for _, input := range m.Inputs() {
		input.Initialise(inputs[input.Name()])
	}
	// Boot & execute
	if err := m.Boot("main"); err != nil {
		errs = append(errs, err)
	} else if steps, err = ExecuteAll(m, n); err != nil {
		errs = append(errs, err)
	} else {
		output = EncodeOutputs(m)
	}
	// Log stats
	stats.Log(fmt.Sprintf("Machine execution (%d steps)", steps))
	//
	return output, errs
}

// ============================================================================
// Utils
// ============================================================================

// ExecuteAll executes a given machine to completion in chunks of n steps,
// returning the number of steps executed and/or any error arising.
func ExecuteAll[W MachineWord[W], M Core[W]](machine M, n uint) (uint, error) {
	var nsteps uint
	//
	for {
		// Execute upto n steps
		m, err := machine.Execute(n)
		// update the tally
		nsteps += m
		// check for termination
		if err != nil || m < n {
			//
			return nsteps, err
		}
	}
}

// ExecuteAndObserve executes a given machine for n steps with a supplied
// observer.  The purpose of this is that it provides a way to extract
// information (as desired) from an executing machine.
func ExecuteAndObserve[W Word[W], I Instruction, M Machine[W, I], V Observer[W, I, M]](machine M, n uint, observer V,
) (uint, error) {
	var (
		nsteps uint
	)
	//
	observer.Initialise(machine)
	//
	for {
		// observe pre execution
		observer.PreExecution(machine)
		// Execute upto n steps
		m, err := machine.Execute(n)
		// observe pre execution
		observer.PostExecution(machine)
		// update the tally
		nsteps += m
		// check for termination
		if err != nil || m < n {
			return nsteps, err
		}
	}
}

// DecodeInputsOutputs decodes  given set of input and output bytes
// appropriately for the given machine.  If there are unknown or conflicting
// inputs, then errors are returned.
func DecodeInputsOutputs[W Word[W], M Core[W]](m M, data map[string][]byte,
) (inputs, outputs map[string][]W, errs []error) {
	//
	var visited = make(map[string]bool)
	//
	inputs = make(map[string][]W)
	outputs = make(map[string][]W)
	// scan input modules
	for _, c := range m.Inputs() {
		// Record visited information
		visited[c.Name()] = true
		//
		if bytes, ok := data[c.Name()]; ok {
			inputs[c.Name()] = DecodeBytes(bytes, c.Geometry())
		} else {
			errs = append(errs, fmt.Errorf("missing input \"%s\"", c.Name()))
		}
	}
	// scan output modules
	for _, c := range m.Outputs() {
		// Record visited information
		visited[c.Name()] = true
		//
		if bytes, ok := data[c.Name()]; ok {
			outputs[c.Name()] = DecodeBytes(bytes, c.Geometry())
		} else {
			errs = append(errs, fmt.Errorf("missing input/output \"%s\"", c.Name()))
		}
	}
	// sanity check for extraneous inputs
	for k := range data {
		if _, ok := visited[k]; !ok {
			errs = append(errs, fmt.Errorf("unknown input \"%s\"", k))
		}
	}
	//
	return inputs, outputs, errs
}

// DecodeInputs configures a given set of input bytes appropriately for the
// given machine.  If there are unknown or conflicting inputs, then errors are
// returned.
func DecodeInputs[W Word[W], C Core[W]](m C, input map[string][]byte) (map[string][]W, []error) {
	var (
		visited = make(map[string]bool)
		inputs  = make(map[string][]W)
		errs    []error
	)
	// scan modules
	for _, c := range m.Inputs() {
		// Record visited information
		visited[c.Name()] = true
		//
		if bytes, ok := input[c.Name()]; ok {
			inputs[c.Name()] = DecodeBytes(bytes, c.Geometry())
		} else {
			errs = append(errs, fmt.Errorf("missing input \"%s\"", c.Name()))
		}
	}
	// sanity check for extraneous inputs
	for k := range input {
		if _, ok := visited[k]; !ok {
			errs = append(errs, fmt.Errorf("unknown input \"%s\"", k))
		}
	}
	//
	return inputs, errs
}

// EncodeOutputs extract the output from a given machine and encodes it into
// byte arrays.
func EncodeOutputs[W Word[W], M Core[W]](m M) map[string][]byte {
	var output = make(map[string][]byte)
	// scan modules
	for _, c := range m.Outputs() {
		//
		output[c.Name()] = EncodeBytes(c.Contents(), c.Geometry())
	}
	//
	return output
}
