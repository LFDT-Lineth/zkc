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
	"math"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// StackFrame represents a single activation record on the VM call stack.  Each
// frame captures the execution state of one in-progress function invocation:
// the function being executed, the values currently held in its registers, and
// the program counter identifying the next instruction to run.  When a function
// calls another, a new frame is pushed; when it returns, its frame is popped.
type StackFrame[W BaseWord[W], I Instruction] struct {
	// module identifier of function
	id uint
	// pc identifies the next instruction to execute within fn's code.
	pc ProgramCounter
	// values holds the current contents of this frame's registers, indexed by
	// register id.  This includes the function's input arguments (occupying the
	// lowest indices), along with any local and output registers.
	values []W
	// fn is the function being executed in this frame.  It provides the code,
	// register declarations and signature information used to interpret pc and
	// values.
	fn *function.Function[I]
}

// FunctionId returns the module Id for the enclosing function.
func (p *StackFrame[W, I]) FunctionId() uint {
	return p.id
}

// BitwidthOf returns the bitwidth of the given register in this frame.
func (p *StackFrame[W, I]) BitwidthOf(v register.Id) uint {
	var reg = p.fn.Register(v)
	//
	if reg.IsNative() {
		return math.MaxUint
	}
	//
	return reg.Width()
}

// Vector returns the vector instruction at a given pc within the code
// associated with this stack frame.
func (p *StackFrame[W, I]) Vector(macro uint) instruction.Vector[I] {
	return p.fn.CodeAt(macro)
}

// Instruction returns the instruction at a given pc within the code associated
// with this stack frame.
func (p *StackFrame[W, I]) Instruction(pc ProgramCounter) I {
	return p.Vector(pc.macroCounter).Codes[pc.microCounter]
}

// Function returns the function associated with this frame.
func (p *StackFrame[W, I]) Function() *function.Function[I] {
	return p.fn
}

// Load the contents of a given register from this stack frame.
func (p *StackFrame[W, I]) Load(v register.Id) W {
	return p.values[v.Unwrap()]
}

// PC returns the program counter for this frame.
func (p *StackFrame[W, I]) PC() ProgramCounter {
	return p.pc
}

// Store a given value into the given register.  Observe that this can fail with
// an error if said valud exceeds the bounds of the target register.
func (p *StackFrame[W, I]) Store(reg register.Id, value W) error {
	var bitwidth = p.BitwidthOf(reg)
	// cast check
	if !value.FitsWithin(bitwidth) {
		return fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
	}
	//
	p.values[reg.Unwrap()] = value
	//
	return nil
}

// Signature returns a simple string representation of the enclosing function
// for the given frame, including its arguments and PC position.
func (p *StackFrame[W, I]) Signature() string {
	var (
		builder strings.Builder
	)
	//
	builder.WriteString(p.fn.Name())
	builder.WriteString("(")

	for i := 0; i != int(p.fn.NumInputs()); i++ {
		if i != 0 {
			builder.WriteString(",")
		}

		builder.WriteString("0x")
		builder.WriteString(p.values[i].Text(16))
	}

	builder.WriteString(")")
	//
	return builder.String()
}

// StoreAcross a given value across a given register vector.  That means the
// least significant bits are assigned to the lowest register in the vector, and
// so on.
func StoreAcross[W word.Word[W], I Instruction](frame StackFrame[W, I], vec register.Vector, value W) error {
	if vec.Len() == 1 {
		return frame.Store(vec.AsRegister(), value)
	} else {
		var bitwidth uint
		//
		for _, rid := range vec.Registers() {
			var (
				id    = rid.Unwrap()
				width = frame.BitwidthOf(rid)
			)
			// Raw write
			frame.values[id] = value.Slice(width)
			value = value.Shr64(uint64(width))
			bitwidth += width
		}
		//
		if value.Cmp64(0) != 0 {
			return fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
		}
		//
		return nil
	}
}
