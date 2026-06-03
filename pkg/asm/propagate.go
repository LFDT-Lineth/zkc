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
package asm

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/hir"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// RawModule provides a convenient alias
type RawModule = lt.Module[word.BigEndian]

// PropagateAll propagates secondary (i.e. derivative) function instances
// throughout one or more traces.  NOTES:
//
// Parallelism?
// Validation?
// Batch size?
// Recursion limit (to prevent infinite loops)
func PropagateAll[T io.Instruction, M sc.Module[word.BigEndian]](p MixedProgram[word.BigEndian, T, M],
	ts []lt.TraceFile) ([]lt.TraceFile, []error) {
	//
	var (
		errors  []error
		ntraces = make([]lt.TraceFile, len(ts))
	)
	//
	for i, trace := range ts {
		var errs []error
		// NOTE: its possible to get empty traces which arise from comment lines
		// in a trace batch.  Whilst it is kind of awkward, we want to preserve
		// the empty traces as this helps error reporting with respect to line
		// numbers.
		if trace.RawModules() != nil {
			ntraces[i], errs = Propagate(p, trace)
			errors = append(errors, errs...)
		}
	}
	//
	return ntraces, errors
}

// Propagate secondary (i.e. derivative) function instances throughout a trace.
// For example, suppose two functions "f(x)" and "g(y)", where f(x) calls g(y).
// Then, consider a trace which contains exactly one instance "f(a)=b". Since
// f(x) calls g(y) we may (depending on the exact implementation of f(x) and the
// parameter given) require a secondary instance, say "g(y)=c", to be added to
// make the trace complete with respect to the original instance. Trace
// propagation is about figuring out what secondary instances are required, and
// adding them to the trace.
//
// NOTES:
//
// Parallelism?
// Validation?
// Batch size?
// Recursion limit (to prevent infinite loops)
func Propagate[T io.Instruction, M sc.Module[word.BigEndian]](p MixedProgram[word.BigEndian, T, M],
	trace lt.TraceFile) (lt.TraceFile, []error) {
	// Construct suitable executior for the given program
	var (
		errors []error
		n      = uint(len(p.program.Components()))
		//
		executor  = io.NewExecutor(p.program)
		trModules []lt.Module[word.BigEndian]
		// Clone heap in trace file, since will mutate this.
		heap = trace.Heap()
	)
	// Clone heap
	heap = heap.Clone()
	// Perform trace alignment
	trModules, errors = ir.AlignTrace(p.Modules().Collect(), trace.RawModules(), true)
	// Sanity check for errors
	if len(errors) > 0 {
		return lt.TraceFile{}, errors
	}
	// Write seed instances
	errors = writeInstances(p, n, trModules, executor)
	// Read out generated instances
	modules := readInstances(&heap, p.program, executor)
	// Append external modules (which are unaffected by propagation).
	modules = append(modules, trModules[n:]...)
	// Done
	return lt.NewTraceFile(trace.Header().MetaData, heap, modules), errors
}

// WriteInstances writes all of the instances defined in the given trace columns
// into the executor which, in turn, forces it to execute the relevant
// functions, and functions they call, etc.
func writeInstances[T io.Instruction, M sc.Module[word.BigEndian]](p MixedProgram[word.BigEndian, T, M], n uint,
	trace []lt.Module[word.BigEndian], executor *io.Executor[T]) []error {
	//
	var errors []error
	// Write all from assembly modules
	for i, m := range trace[:n] {
		errs := writeFunctionInstances(uint(i), p.program, m, executor)
		errors = append(errors, errs...)
	}
	// Write all from non-assembly modules
	for i, m := range trace[n:] {
		var extern = p.externs[i]
		// Write instances from any external calls
		for _, call := range extractExternalCalls(extern) {
			errs := writeExternCall(call, p.program, m, executor)
			errors = append(errors, errs...)
		}
	}
	//
	return errors
}

func writeFunctionInstances[T io.Instruction](fid uint, p io.Program[T], mod RawModule,
	executor *io.Executor[T]) []error {
	//
	var (
		height  = mod.Height()
		fn      = p.Component(fid)
		inputs  = make([]big.Int, fn.NumInputs())
		outputs = make([]big.Int, fn.NumOutputs())
		errors  []error
	)
	// Invoke padding instance
	extractFunctionPadding(fn.Registers(), inputs, outputs)
	// Execute function call to produce outputs
	errors = executeAndCheck(fid, fn.Name(), inputs, outputs, executor)
	// Invoke each user-defined instance in turn
	for i := range height {
		// Extract function inputs
		extractFunctionColumns(i, mod, inputs, outputs)
		// Execute function call to produce outputs
		errs := executeAndCheck(fid, fn.Name(), inputs, outputs, executor)
		errors = append(errors, errs...)
	}
	//
	return errors
}

// Extract any external function calls found within the given module, returning
// them as an array.
func extractExternalCalls[M sc.Module[word.BigEndian]](extern M) []hir.FunctionCall {
	var calls []hir.FunctionCall
	//
	for iter := extern.Constraints(); iter.HasNext(); {
		c := iter.Next()
		// This should always hold
		if hc, ok := c.(hir.Constraint); ok {
			// Check whether its a call or not
			if call, ok := hc.Unwrap().(hir.FunctionCall); ok {
				// Yes, so record it
				calls = append(calls, call)
			}
		}
	}
	//
	return calls
}

// Write any function instances arising from the given call.
func writeExternCall[T io.Instruction](call hir.FunctionCall, p io.Program[T], mod RawModule,
	executor *io.Executor[T]) []error {
	//
	var (
		trMod   = &mod
		height  = mod.Height()
		fn      = p.Component(call.Callee)
		inputs  = make([]big.Int, fn.NumInputs())
		outputs = make([]big.Int, fn.NumOutputs())
		errors  []error
	)
	//
	if call.Selector.HasValue() {
		var selector = call.Selector.Unwrap()
		// Invoke each user-defined instance in turn
		for i := range height {
			// execute if selector enabled
			if enabled, _, err := selector.TestAt(int(i), trMod, nil); enabled {
				// Extract external columns
				extractExternColumns(int(i), call, trMod, inputs, outputs)
				// Execute function call to produce outputs
				errs := executeAndCheck(call.Callee, fn.Name(), inputs, outputs, executor)
				errors = append(errors, errs...)
			} else if err != nil {
				errors = append(errors, err)
			}
		}
	} else {
		// Invoke each user-defined instance in turn
		for i := range height {
			// Extract external columns
			extractExternColumns(int(i), call, trMod, inputs, outputs)
			// Execute function call to produce outputs
			errs := executeAndCheck(call.Callee, fn.Name(), inputs, outputs, executor)
			errors = append(errors, errs...)
		}
	}
	//
	return errors
}

func executeAndCheck[T io.Instruction](fid uint, name module.Name, inputs, outputs []big.Int,
	executor *io.Executor[T]) []error {
	var (
		errors []error
		// Determine number of expected outputs
		nOutputs = uint(len(outputs))
		// Execute function call to produce actual outputs
		actual = executor.Read(fid, inputs, nOutputs)
	)
	// Sanity actual outputs match expected outputs
	for i := range len(outputs) {
		given := outputs[i]
		computed := actual[i]
		// Check input value
		if given.Cmp(&computed) != 0 {
			ins := toArgumentString(inputs)
			outs := toArgumentString(outputs)
			acts := toArgumentString(actual)
			errors = append(errors, fmt.Errorf("inconsistent instance %s(%s)=%s in trace (expected %s(%s)=%s)",
				name.String(), ins, outs, name, ins, acts))
		}
	}
	//
	return errors
}

func extractFunctionColumns(row uint, mod RawModule, inputs, outputs []big.Int) {
	var (
		numInputs  = uint(len(inputs))
		numOutputs = uint(len(outputs))
	)
	//
	for i := range numInputs {
		var (
			col   = mod.Columns[i]
			input big.Int
		)
		// Assign value
		input.SetBytes(col.Data().Get(row).Bytes())
		//
		inputs[i] = input
	}
	//
	for i := range numOutputs {
		var (
			col    = mod.Columns[i+numInputs]
			output big.Int
		)
		// Assign value
		output.SetBytes(col.Data().Get(row).Bytes())
		//
		outputs[i] = output
	}
}

func extractExternColumns(row int, call hir.FunctionCall, mod trace.Module[word.BigEndian],
	inputs, outputs []big.Int) []error {
	//
	// Extract function arguments
	errs1 := extractExternTerms(row, call.Arguments, mod, inputs)
	// Extract function returns
	errs2 := extractExternTerms(row, call.Returns, mod, outputs)
	//
	return append(errs1, errs2...)
}

func extractExternTerms(row int, terms []hir.Term, mod trace.Module[word.BigEndian], values []big.Int) []error {
	var errors []error
	//
	for i, arg := range terms {
		var (
			ith      big.Int
			val, err = arg.EvalAt(row, mod, nil)
		)
		ith.SetBytes(val.Bytes())
		values[i] = ith
		//
		errors = append(errors, err)
	}
	//
	return errors
}

func extractFunctionPadding(registers []register.Register, inputs, outputs []big.Int) {
	var numInputs = len(inputs)
	//
	for i := range len(inputs) {
		inputs[i] = *registers[i].Padding()
	}

	for i := range len(outputs) {
		outputs[i] = *registers[i+numInputs].Padding()
	}
}

// ReadInstances simply traverses all internal states generated within the
// executor and converts them back into raw columns.
func readInstances[T io.Instruction](heap *lt.WordHeap, p io.Program[T], executor *io.Executor[T],
) []lt.Module[word.BigEndian] {
	var (
		modules = make([]lt.Module[word.BigEndian], len(p.Components()))
		builder = array.NewDynamicBuilder(heap)
	)
	//
	for i := range p.Components() {
		fn := p.Component(uint(i))
		instances := executor.Instances(uint(i))
		modules[i] = readFunctionInstances[T](fn, instances, &builder)
	}
	//
	return modules
}

func readFunctionInstances[T io.Instruction](fn io.Component[T], instances []io.ComponentInstance,
	builder array.Builder[word.BigEndian]) lt.Module[word.BigEndian] {
	var (
		registers = fn.Registers()
		columns   = make([]lt.Column[word.BigEndian], fn.NumInputs()+fn.NumOutputs())
	)
	//
	for i := range columns {
		data := readFunctionInputOutputs(uint(i), registers, instances, builder)
		//
		columns[i] = lt.NewColumn[word.BigEndian](registers[i].Name(), data)
	}
	//
	return lt.NewModule[word.BigEndian](fn.Name(), columns)
}

func readFunctionInputOutputs(arg uint, registers []io.Register, instances []io.ComponentInstance,
	builder array.Builder[word.BigEndian]) array.MutArray[word.BigEndian] {
	var (
		height = uint(len(instances))
		arr    = builder.NewArray(height, registers[arg].Width())
	)
	//
	for i, instance := range instances {
		var (
			ith big.Int = instance.Get(arg)
			w   word.BigEndian
		)
		//
		arr = arr.Set(uint(i), w.SetBytes(ith.Bytes()))
	}
	//
	return arr
}

func toArgumentString(args []big.Int) string {
	var builder strings.Builder
	//
	for i, arg := range args {
		if i != 0 {
			builder.WriteString(", ")
		}

		builder.WriteString(arg.String())
	}
	//
	return builder.String()
}
