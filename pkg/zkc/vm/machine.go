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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	log "github.com/sirupsen/logrus"
)

// Core provides a minimal interface for booting and executing a machine with a
// given set of inputs, and collecting the outputs afterwards.
type Core[W Word[W]] interface {
	// Boot this machine by starting the given function with the given inputs.  This
	// function assumes the given inputs are correctly formed, and will: (1) ingore
	// unknown inputs; (2) initialise empty memories when no input is given for
	// them.  Thus, it is recommended to perform sanity checking on input prior to
	// calling this function.
	Boot(fun string, input map[string][]W) error
	// Execute the machine for the given number of steps, returning the actual
	// number of steps executed and an error (if execution failed).
	Execute(steps uint) (uint, error)
	// Return array of (non-static) input memories
	Inputs() iter.Iterator[interpreter.InputOutput[W]]
	// Return array of output memories
	Outputs() iter.Iterator[interpreter.InputOutput[W]]
}

// ProgramPoint identifies a specific bytecode instruction within a given
// bytecode program.  A key aspect is that it two dimensional to account for
// bytecode vectors: (1) it identifies the enclosing bytecode vector (macro)
// being executed; (2) it identifies the exact bytecode instruction (micro)
// within that being executed.
type ProgramPoint = descriptor.ProgramPoint

// ============================================================================
// Constructors
// ============================================================================

// BootAndExecute executes the program embodied by these constraints in chunks
// of n steps at a time, producing any outputs arising.  Execution is faster
// than trace because it does not record any internal information about the
// trace --- it simply extracts the outputs at the end.
func BootAndExecute[W Word[W], M Core[W]](m M, input map[string][]byte, n uint,
) (output map[string][]byte, traceable bool, errs []error) {
	//
	var (
		steps  uint
		inputs map[string][]W
		stats  = util.NewPerfStats()
	)
	// Execute machine in chunks of 1K steps
	if inputs, errs = DecodeInputs(m, input); len(errs) != 0 {
		return nil, false, errs
	}
	// Boot & execute
	if err := m.Boot("main", inputs); err != nil {
		errs = append(errs, err)
	} else if steps, err = ExecuteAll(m, n); err != nil {
		errs = append(errs, err)
		// Recognised failures don't prevent tracing.
		_, traceable = err.(*Failure)
	} else {
		output = EncodeOutputs(m)
		traceable = true
	}
	// Log stats
	stats.Log(fmt.Sprintf("Machine execution (%d steps)", steps))
	//
	return output, traceable, errs
}

// ExecuteAll executes a given machine to completion in chunks of n steps,
// returning the number of steps executed and/or any error arising.
func ExecuteAll[W Word[W], M Core[W]](machine M, n uint) (uint, error) {
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

// FilterInputs restricts the given set of (parsed) inputs to the program's
// declared input memories.
func FilterInputs[W Word[W], T any](p Program[W], input map[string][]T) map[string][]T {
	inputs := make(map[string][]T)
	//
	for it := p.Inputs(); it.HasNext(); {
		in := it.Next()
		if bytes, ok := input[in.Name()]; ok {
			inputs[in.Name()] = bytes
		}
	}
	// Sanity check what was actually filtered out
	for k := range input {
		if _, ok := inputs[k]; !ok {
			log.Warn("ignoring input/output \"", k, "\"")
		}
	}
	//
	return inputs
}

// DecodeInputsOutputs decodes  given set of input and output bytes
// appropriately for the given machine.  If there are unknown or conflicting
// inputs, then errors are returned.
func DecodeInputsOutputs[W Word[W]](program descriptor.Program[W], data map[string][]byte,
) (inputs, outputs map[string][]W, errs []error) {
	//
	var visited = make(map[string]bool)
	//
	inputs = make(map[string][]W)
	outputs = make(map[string][]W)
	// scan input modules
	for iter := program.Inputs(); iter.HasNext(); {
		var input = iter.Next()
		// Record visited information
		visited[input.Name()] = true
		//
		if bytes, ok := data[input.Name()]; ok {
			inputs[input.Name()] = DecodeBytes(bytes, input)
		} else {
			errs = append(errs, fmt.Errorf("missing input \"%s\"", input.Name()))
		}
	}
	// scan output modules
	for iter := program.Outputs(); iter.HasNext(); {
		var output = iter.Next()
		// Record visited information
		visited[output.Name()] = true
		//
		if bytes, ok := data[output.Name()]; ok {
			outputs[output.Name()] = DecodeBytes(bytes, output)
		} else {
			errs = append(errs, fmt.Errorf("missing input/output \"%s\"", output.Name()))
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
	// scan input modules
	for iter := m.Inputs(); iter.HasNext(); {
		var (
			c    = iter.Next()
			name = c.Descriptor().Name()
		)
		// Record visited information
		visited[name] = true
		//
		if bytes, ok := input[name]; ok {
			inputs[name] = DecodeBytes(bytes, *c.Descriptor())
		} else {
			errs = append(errs, fmt.Errorf("missing input \"%s\"", name))
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
	var outputs = make(map[string][]byte)
	// scan modules
	// scan output modules
	for iter := m.Outputs(); iter.HasNext(); {
		var (
			output = iter.Next()
			name   = output.Descriptor().Name()
		)
		//
		outputs[name] = EncodeBytes(output.Contents(), *output.Descriptor())
	}
	//
	return outputs
}
