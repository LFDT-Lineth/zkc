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
package io

import (
	"math"
	"math/big"
	"sync"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
)

// Executor provides a mechanism for executing a program efficiently and
// generating a suitable top-level trace.  Executor implements the io.Map
// interface.
type Executor[T Instruction] struct {
	functions []*ComponentTrace[T]
}

// NewExecutor constructs a new executor.
func NewExecutor[T Instruction](program Program[T]) *Executor[T] {
	// Initialise executor traces
	traces := make([]*ComponentTrace[T], len(program.Components()))
	//
	for i := range traces {
		traces[i] = NewFunctionTrace[T](program.functions[i])
	}
	// Construct new executor
	return &Executor[T]{traces}
}

// Instance returns a valid instance of the given bus.
func (p *Executor[T]) Instance(bus uint) ComponentInstance {
	var (
		fn     = p.functions[bus].fn
		inputs = make([]big.Int, fn.NumInputs())
	)
	// Intialise inputs values
	for i := range fn.NumInputs() {
		var (
			ith big.Int
			reg = fn.Register(register.NewId(i))
		)
		// Initialise input from padding value
		inputs[i] = *ith.Set(reg.Padding())
	}
	// Compute function instance
	return p.functions[bus].Call(inputs, p)
}

// Read implementation for the io.Map interface.
func (p *Executor[T]) Read(bus uint, address []big.Int, _ uint) []big.Int {
	return p.functions[bus].Call(address, p).Outputs()
}

// Instances returns accrued function instances for the given bus.
func (p *Executor[T]) Instances(bus uint) []ComponentInstance {
	return p.functions[bus].instances
}

// Count the total number of instances currently recorded in this executor.
func (p *Executor[T]) Count() uint {
	var count uint
	//
	for _, fn := range p.functions {
		count += fn.Count()
	}
	//
	return count
}

// Write implementation for the io.Map interface.
func (p *Executor[T]) Write(bus uint, address []big.Int, values []big.Int) {
	// At this stage, there no components use this functionality.
	panic("unsupported operation")
}

// ============================================================================
// FunctionTrace
// ============================================================================

// ComponentTrace captures all instances for a given component, and provides a
// (thread-safe) API for calling to compute its output for a given set of
// inputs.
type ComponentTrace[T Instruction] struct {
	// Function whose instances are captured here
	fn Component[T]
	// Cached instances of the given function
	instances set.AnySortedSet[ComponentInstance]
	// mutex required to ensure thread safety.
	mux sync.RWMutex
}

// NewFunctionTrace constructs an empty trace for a given function.
func NewFunctionTrace[T Instruction](fn Component[T]) *ComponentTrace[T] {
	instances := set.NewAnySortedSet[ComponentInstance]()
	//
	return &ComponentTrace[T]{
		fn:        fn,
		instances: *instances,
	}
}

// Count the number of instances recorded as part of this function's trace.
func (p *ComponentTrace[T]) Count() uint {
	p.mux.RLock()
	count := uint(len(p.instances))
	p.mux.RUnlock()
	//
	return count
}

// Call this function to determine its outputs for a given set of inputs.  If
// this instance has been seen before, it will simply return that.  Otherwise,
// it will execute the function to determine the correct outputs.
func (p *ComponentTrace[T]) Call(inputs []big.Int, iomap Map) ComponentInstance {
	var iostate = ComponentInstance{uint(len(inputs)), inputs}
	// Obtain read lock
	p.mux.RLock()
	// Look for cached instance
	index := p.instances.Find(iostate)
	// Check for cache hit.
	if index != math.MaxUint {
		// Yes, therefore return precomputed outputs
		instance := p.instances[index]
		// Release read lock
		p.mux.RUnlock()
		//
		return instance
	}
	// Release read lock
	p.mux.RUnlock()
	// Execute function to determine new outputs.
	return p.executeCall(inputs, iomap)
}

// Execute this function for a given set of inputs to determine its outputs and
// produce a given instance.  The created instance is recorded within the trace
// so it can be reused rather than recomputed in the future.  This function is
// thread-safe, and will acquire the write lock on the cached instances
// momentarily to insert the new instance.
//
// NOTE: this does not attempt any form of thread blocking (e.g. when a desired
// instance if being computed by another thread). Instead, it eagerly computes
// instances --- even if that means, occasionally, an instance is computed more
// than once.  This is safe since instances are always deterministic (i.e. same
// output for a given input).
func (p *ComponentTrace[T]) executeCall(inputs []big.Int, iomap Map) ComponentInstance {
	switch p.fn.(type) {
	case *Function[T]:
		return p.executeFunctionCall(inputs, iomap)
	default:
		panic("unknown component")
	}
}

// Execute this function for a given set of inputs to determine its outputs and
// produce a given instance.  The created instance is recorded within the trace
// so it can be reused rather than recomputed in the future.  This function is
// thread-safe, and will acquire the write lock on the cached instances
// momentarily to insert the new instance.
//
// NOTE: this does not attempt any form of thread blocking (e.g. when a desired
// instance if being computed by another thread). Instead, it eagerly computes
// instances --- even if that means, occasionally, an instance is computed more
// than once.  This is safe since instances are always deterministic (i.e. same
// output for a given input).
func (p *ComponentTrace[T]) executeFunctionCall(inputs []big.Int, iomap Map) ComponentInstance {
	var (
		fn = p.fn.(*Function[T])
		// Determine how many I/O registers
		nio = fn.NumInputs() + fn.NumOutputs()
		//
		pc = uint(0)
		//
		state = InitialState(inputs, fn.Registers(), fn.Buses(), iomap)
	)
	// Keep executing until we're done.
	for pc != RETURN && pc != FAIL {
		insn := fn.CodeAt(pc)
		// execute given instruction
		pc = insn.Execute(state)
		// update state pc
		state.Goto(pc)
	}
	// Cache I/O instance
	instance := ComponentInstance{fn.NumInputs(), state.state[:nio]}
	// Obtain  write lock
	p.mux.Lock()
	// Insert new instance
	p.instances.Insert(instance)
	// Release write lock
	p.mux.Unlock()
	// Done
	return instance
}

// ============================================================================
// FunctionInstance
// ============================================================================

// ComponentInstance captures the mapping from inputs (i.e. parameters) to
// outputs (i.e. returns) for a particular instance of a given function.
type ComponentInstance struct {
	ninputs uint
	state   []big.Int
}

// Cmp comparator for the I/O registers of a particular function instance.
// Observe that, since functions are always deterministic, this only considers
// the inputs (as the outputs follow directly from this).
func (p ComponentInstance) Cmp(other ComponentInstance) int {
	// NOTE: since limbs are split such that the least significant comes first,
	// we have to sort from right-to-left rather than left-to-right to ensure
	// instances remain sorted after register splitting.  This is important for
	// ArrayTrace.FindLast().
	for i := p.ninputs; i > 0; {
		i--
		if c := p.state[i].Cmp(&other.state[i]); c != 0 {
			return c
		}
	}
	//
	return 0
}

// Outputs returns the output values for this function instance.
func (p ComponentInstance) Outputs() []big.Int {
	return p.state[p.ninputs:]
}

// Get value of given input or output argument for this instance.
func (p ComponentInstance) Get(arg uint) big.Int {
	return p.state[arg]
}
