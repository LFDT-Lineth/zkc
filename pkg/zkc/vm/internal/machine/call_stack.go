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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/stack"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
)

// CallStack encapsulates the notion of a machine's call stack which, at an
// abstract level, represents a stack of "stack frames".  Each stack frame
// represents an executing function.  A key design goal for the call stack is to
// minimise memory allocations.  Thus, as the stack grows initially, memory is
// allocated accordingly.  However, when it retracts in size this capacity is
// reserved such that subsequent rises can occur without allocation.
type CallStack[W BaseWord[W], I Instruction] struct {
	// heap of all values currently on the stack.
	values heap.Heap[W]
	// stack frames which form the actual call stack.
	frames stack.Stack[FrameRecord[I]]
}

// Depth returns the number of frames currently on the stack.
func (p *CallStack[W, I]) Depth() uint {
	return p.frames.Len()
}

// Boot the call stack by allocating an initial frame on the stack.  Since the
// boot function never accepts arguments nothing is copied and, hence, no
// failure can arise (i.e. contrasting with Enter).
func (p *CallStack[W, I]) Boot(id uint, fn *function.Function[I]) {
	var pc = ProgramCounter{}
	// Boot stack
	p.Reset()
	// Allocate space for entry function
	p.values.Alloc(fn.Width())
	// Push record
	p.frames.Push(FrameRecord[I]{0, pc, id, fn})
}

// Enter a given function by allocating a new frame on the stack.  The caller's
// program counter (PC) is included so it can be saved when this happens.  The
// input registers for this frame are initialised from the caller's frame using
// the argument registers (extracted from the call instruction itself).  This
// will produce an error if input values do not fit the parameter widths.
func (p *CallStack[W, I]) Enter(id uint, fn *function.Function[I]) error {
	var (
		fp         = p.values.Size()
		pc         = ProgramCounter{}
		caller     = p.Frame(0)
		insn   any = caller.Instruction(caller.PC())
		call       = insn.(*instruction.Call)
	)
	// Allocate space for function
	p.values.Alloc(fn.Width())
	// Push record
	p.frames.Push(FrameRecord[I]{fp, pc, id, fn})
	// extract callee frame
	callee := p.Frame(0)
	// Copy and check
	return frameCopyInto(caller.values, callee.values, call.Arguments, fn.Inputs())
}

// Goto sets the Program Counter position for the active (i.e. topmost) frame.
// If there is no active frame, this is a no-op.
func (p *CallStack[W, I]) Goto(pc ProgramCounter) {
	if p.frames.Len() > 0 {
		p.frames.Top().pc = pc
	}
}

// Frame returns the nth stack frame from the top.  So, Frame(0) returns the
// active (i.e. currently executing) frame, and Frame(1) returns the caller of
// that frame, etc.
func (p *CallStack[W, I]) Frame(m uint) StackFrame[W, I] {
	var (
		record = p.frames.Peek(m)
		first  = record.fp
		last   = first + record.fn.Width()
	)
	//
	return StackFrame[W, I]{
		pc:     record.pc,
		id:     record.fid,
		values: p.values.Slice(first, last),
		fn:     record.fn,
	}
}

// Leave the current function by freeing the corresponding stack frame.  The
// output registers from this frame are written back into the caller's frame
// using the return registers (again, extracted from the call instruction
// itself).  This will produce an error if the output values do not fit the
// return registers.
func (p *CallStack[W, I]) Leave() (err error) {
	var (
		callee = p.Frame(0)
		// Narrow down on return values
		calleeReturns = callee.values[callee.fn.NumInputs():]
	)
	// Pop stack frame record
	p.frames.Pop()
	// Deallocate frame values
	p.values.Free(callee.fn.Width())
	//
	if p.Depth() > 0 {
		// Copy & check returns
		var (
			caller     = p.Frame(0)
			insn   any = caller.Instruction(caller.PC())
			call       = insn.(*instruction.Call)
		)
		// Copy and check
		return frameCopyFrom(caller.values, calleeReturns, call.Returns, caller.fn.Registers())
	}
	//
	return nil
}

// IsEmpty determines whether or not this stack is empty.
func (p *CallStack[W, I]) IsEmpty() bool {
	return p.frames.Len() == 0
}

// Reset the call stack to an empty state.  Observe that this will not free any
// capacity previously allocated for this stack.
func (p *CallStack[W, I]) Reset() {
	p.frames.Clear()
	p.values.Clear()
}

// ============================================================================
// FrameRecord
// ============================================================================

// FrameRecord is an internal data structure used within a CallStack
type FrameRecord[I Instruction] struct {
	// Frame pointer identifies the heap offset in the call stack where values
	// for this frame begin.
	fp uint
	// Program counter identifies next instruction to execute in this frame or,
	// if this frame has called another function then it identifies the call
	// instruction itself.
	pc ProgramCounter
	// Module ID for enclosing function.
	fid uint
	// instructions being executed in this frame
	fn *function.Function[I]
}

// ============================================================================
// Helpers
// ============================================================================

// Copy arguments from the caller frame up into the callee frame, whilst
// additionally checking those arguments are type safe (i.e. fit within) the
// parameters of the callee.
func frameCopyInto[W BaseWord[W]](caller []W, callee []W, args []register.Id, calleeRegs []register.Register) error {
	//
	for i, arg := range args {
		var (
			val    = caller[arg.Unwrap()]
			target = calleeRegs[i]
		)
		// sanity check value being written fits within the parameter it is
		// being assigned to.
		if !target.IsNative() && !val.FitsWithin(target.Width()) {
			return fmt.Errorf("bit overflow (0x%s not u%d)", val.Text(16), target.Width())
		}
		// copy from caller to callee
		callee[i] = caller[arg.Unwrap()]
	}
	//
	return nil
}

// Copy returns from the callee frame down into target registers within the
// caller frame, whilst additionally checking those returns are type safe (i.e.
// fit within) the target registers of the caller.
func frameCopyFrom[W BaseWord[W]](caller []W, calleeReturns []W, returns []register.Id,
	callerRegs []register.Register) error {
	//
	for i, arg := range returns {
		var (
			val    = calleeReturns[i]
			target = callerRegs[arg.Unwrap()]
		)
		// sanity check value being written fits within the parameter it is
		// being assigned to.
		if !target.IsNative() && !val.FitsWithin(target.Width()) {
			return fmt.Errorf("bit overflow (0x%s not u%d)", val.Text(16), target.Width())
		}
		// copy from caller to callee
		caller[arg.Unwrap()] = calleeReturns[i]
	}
	//
	return nil
}
