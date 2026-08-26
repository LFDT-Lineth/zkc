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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
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
	// Restore this machine from a given checkpoint.  This does not continue
	// executiong, but simply initialises the machine state to match the
	// checkpoint.
	Restore(CheckPoint[W])
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

// BootAndExecute boots the given machine from the inputs, executes it in chunks
// of n steps at a time, and extracts any outputs arising.  Two kinds of error
// can arise during this: (i) a recognised machine failure which is expected
// under the right circumstances (e.g. executing a fail instruction); (ii) an
// internal machine failure which is not expected (and signals some kind of bug
// somewhere).  The traceable flag holds when the given execution can be traced
// (i.e. when no errors in the latter category arise).
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

// BootAndCheckpoint boots a given program from a given set of inputs and
// checkpoints it according to a given strategy.  This produces some number of
// checkpoints, the final outputs, an indication as to whether this execution can
// be traced, and any errors arising.
func BootAndCheckpoint[W Word[W]](pr Program[W], in map[string][]byte, strategy ShardingStrategy,
) (checkpoints []CheckPoint[W], outputs map[string][]byte, traceable bool, errors []error) {
	var (
		err   error
		steps uint
		stats = util.NewPerfStats()
		// specify how many steps each shard will be
		clk = util.NewCounter(strategy.shardSteps)
		//
		inputs map[string][]W
	)
	// Locate the function whose calls are to be checkpointed.
	fid, ok := pr.HasModule(strategy.shardFunction)
	if !ok {
		return nil, nil, false, []error{
			fmt.Errorf("unknown function \"%s\"", strategy.shardFunction),
		}
	}
	// Register a breakpoint at fn's entry and build an interpreter for the
	// result, so the breakpointer fires each time fn is entered.
	bci := NewBytecodeInterpreter(pr.BreakPoint(fid, ProgramPoint{Macro: 0, Micro: 0}))
	// Write a checkpoint as a hex string, one per line.  The counter governs how
	// frequently this actually fires: it triggers every interval entries of fn.
	bci.BreakPointer(func(_ uint32) bool {
		// Only record once every interval-th invocation of fn.
		if clk.Tick() {
			// Append next checkoint
			checkpoints = append(checkpoints, bci.CheckPoint())
		}
		// Don't interrupt
		return false
	})
	// Decode inputs and boot machine
	if inputs, errors = DecodeInputs(bci, in); len(errors) != 0 {
		return nil, nil, false, errors
	} else if err := bci.Boot("main", inputs); err != nil {
		return nil, nil, false, append(errors, err)
	}
	// Set initial checkpoint to from beginning of execution until the first
	// time the target function is executed.
	checkpoints = append(checkpoints, bci.CheckPoint())
	// Execute machine to generate checkpoints
	if steps, err = ExecuteAll(bci, math.MaxUint); err != nil {
		errors = append(errors, err)
		// Recognised failures don't prevent tracing.
		_, traceable = err.(*Failure)
	} else {
		// Encode outputs
		outputs = EncodeOutputs(bci)
		traceable = true
	}
	// Log stats
	stats.Log(fmt.Sprintf("Machine execution (%d steps, %d checkpoints)", steps, len(checkpoints)))
	// Done
	return checkpoints, outputs, traceable, errors
}

// BootAndTrace boots a machine from the given inputs and traces according to
// the given strategy.  Two kinds of error can arise during this: (i) a
// recognised machine failure which is expected under the right circumstances
// (e.g. executing a fail instruction); (ii) an internal machine failure which
// is not expected (and signals some kind of bug somewhere).  The traceable flag
// holds when the given execution can be traced (i.e. when no errors in the
// latter category arise).
func BootAndTrace[W Word[W], F Element[F], T Tracer[W, F, T]](pr Program[W], input map[string][]byte,
) (trace Trace[F], output map[string][]byte, errs []error) {
	//
	var (
		// constracter tracer
		tracer T
		//
		traceable bool
	)
	// Initialise the tracer
	tracer = tracer.Init(pr)
	// construct tracing interpreter
	bci := constructTracingInterpreter(pr, tracer)
	// Execute machine in chunks of 1K steps
	if output, traceable, errs = BootAndExecute(bci, input, math.MaxUint); !traceable {
		return nil, nil, errs
	}
	//
	var stats = util.NewPerfStats()
	// Apply post processing
	array.Apply(bci.ExtractMemory(), func(_ uint, p util.Pair[uint16, RuntimeMemory[W]]) {
		tracer.TraceMemory(p.Left, p.Right, pr.Field())
	})
	// Done
	stats.Log("Trace processing")
	// Success, build trace
	return tracer.Build(), output, errs
}

// RestoreAndTraceFor restores a machine from the given checkpoint and traces
// according to the given strategy.  Two kinds of error can arise during this:
// (i) a recognised machine failure which is expected under the right
// circumstances (e.g. executing a fail instruction); (ii) an internal machine
// failure which is not expected (and signals some kind of bug somewhere).  The
// traceable flag holds when the given execution can be traced (i.e. when no
// errors in the latter category arise).
func RestoreAndTraceFor[W Word[W], F Element[F], T Tracer[W, F, T]](pr Program[W], cp CheckPoint[W],
	fn string, nsteps uint64) (trace Trace[F], errs []error) {
	//
	var (
		// constracter tracer
		tracer T
		//
		traceable bool
	)
	// Initialise the tracer
	tracer = tracer.Init(pr)
	// construct tracing interpreter
	bci := constructTraceForInterpreter(pr, fn, nsteps, tracer)
	// Sanity check error arising construct the interpreter.
	if bci == nil {
		return nil, []error{
			fmt.Errorf("unknown function \"%s\"", fn),
		}
	}
	// Execute the given machine
	if traceable, errs = RestoreAndExecute(bci, cp, math.MaxUint); !traceable {
		return nil, errs
	}
	//
	var stats = util.NewPerfStats()
	// Apply post processing
	array.Apply(bci.ExtractMemory(), func(_ uint, p util.Pair[uint16, RuntimeMemory[W]]) {
		tracer.TraceMemory(p.Left, p.Right, pr.Field())
	})
	// Done
	stats.Log("Trace processing")
	//
	return tracer.Build(), errs
}

// RestoreAndExecute restores the given machine from a checkpoint, and continues
// executing in chunks of n steps at a time.  Two kinds of error can arise
// during this: (i) a recognised machine failure which is expected under the
// right circumstances (e.g. executing a fail instruction); (ii) an internal
// machine failure which is not expected (and signals some kind of bug
// somewhere).  The traceable flag holds when the given execution can be traced
// (i.e. when no errors in the latter category arise).
func RestoreAndExecute[W Word[W], M Core[W]](m M, cp CheckPoint[W], n uint) (traceable bool, errs []error) {
	//
	var (
		steps uint
		err   error
		stats = util.NewPerfStats()
	)
	// Restore machine state from checkpoint
	m.Restore(cp)
	// Execute until termination or interruption.
	if steps, err = ExecuteAll(m, n); err != nil {
		errs = append(errs, err)
		// Recognised failures don't prevent tracing.
		_, traceable = err.(*Failure)
	} else {
		// Success
		traceable = true
	}
	// Log stats
	stats.Log(fmt.Sprintf("Machine resumed execution (%d steps)", steps))
	//
	return traceable, errs
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
func FilterInputs[W Word[W], T any](p Program[W], input map[string][]T) (map[string][]T, []string) {
	var (
		inputs  = make(map[string][]T)
		ignores []string
	)
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
			ignores = append(ignores, k)
		}
	}
	//
	return inputs, ignores
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

// ======================================================================================
// Helpers
// ======================================================================================

func constructTracingInterpreter[W Word[W], F Element[F], T Tracer[W, F, T]](pr Program[W], tracer T,
) *interpreter.Interpreter[W] {
	//
	var bci = interpreter.New(pr, true).WithAccessLog()
	// Register breakpoint handler to record all states generated during
	// tracing.
	bci = bci.BreakPointer(func(opcode uint32) bool {
		// Extract state from the interpreter
		fid, pc, frame := interpreter.ExtractExecutingState(bci)
		// NOTE: don't clone the frame here (for now) since it is always
		// converted into a slice of field elements F.
		tracer.TraceFunctionLine(State[W]{fid, pc, isTerminalOpcode(opcode), frame})
		// don't interrupt
		return false
	})
	//
	return bci
}

func constructTraceForInterpreter[W Word[W], F Element[F], T Tracer[W, F, T]](pr Program[W], fn string, nsteps uint64,
	tracer T) *interpreter.Interpreter[W] {
	var (
		funId uint16
		ok    bool
		clk   = util.NewCounter(nsteps)
	)
	// Locate function to be sharded.
	if funId, ok = pr.HasModule(fn); !ok {
		return nil
	}
	// Register breakpoint at start of target function.
	pr = pr.BreakPoint(funId, ProgramPoint{Macro: 0, Micro: 0})
	// Construct interpreter with breakpoint
	var (
		// Construct tracing interpreter
		bci = interpreter.New(pr, true).WithAccessLog()
		// Identify pc of break point in binary encoding
		bp, bp_ok = bci.Binary().AddressOf(funId)
	)
	// Sanity check
	if !bp_ok {
		panic(fmt.Sprintf("missing breakpoint address for %s", fn))
	}
	// Register breakpoint handler to record all states generated during
	// tracing.
	bci = bci.BreakPointer(func(opcode uint32) bool {
		// Check whether break point for step count (or not).
		if bci.ProgramCounter() == bp.Offset {
			// interrupt when counter finished
			return clk.Tick()
		}
		// Extract state from the interpreter
		fid, pc, frame := interpreter.ExtractExecutingState(bci)
		// NOTE: don't clone the frame here (for now) since it is always
		// converted into a slice of field elements F.
		tracer.TraceFunctionLine(State[W]{fid, pc, isTerminalOpcode(opcode), frame})
		// don't interrupt
		return false
	})
	//
	return bci
}

// isTerminalOpcode determines whether the given (encoded) opcode ends the
// enclosing function's frame, meaning the state recorded for it is a terminal
// state (i.e. $ret is set on its trace row).  This holds for returns and dones,
// and also for tail calls since these reuse the frame rather than returning to
// it.
func isTerminalOpcode(opcode uint32) bool {
	switch opcode {
	case encoding.RET, encoding.DONE,
		encoding.TAILCALL_2, encoding.TAILCALL_n,
		encoding.WIDE | encoding.WIDE_RET<<8,
		encoding.WIDE | encoding.WIDE_TAILCALL_n<<8:
		return true
	default:
		return false
	}
}
