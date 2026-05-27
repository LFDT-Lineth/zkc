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
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	zkc_util "github.com/consensys/go-corset/pkg/zkc/util"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
)

// Instruction is a convenient alias
type Instruction instruction.Instruction

// BaseWord captures the minimal set of requirements for a word used in the base
// machine.
type BaseWord[W any] interface {
	util.Comparable[W]
	util.Uinter64
	zkc_util.Formattable
	// Check whether this value fits within the given bitwidth.
	FitsWithin(uint) bool
}

// ============================================================================
// Base Machine
// ============================================================================

// Base provides a fundamental implementation of a machine.  The intention is
// that other machine variations build off this by providing executors specific
// to their instruction set.
type Base[W BaseWord[W], I Instruction, T Executor[W, I]] struct {
	modules   []Module
	callstack []Frame[W]
	executor  T
}

// NewBase constructs a new empty base machine
func NewBase[W BaseWord[W], I Instruction, T Executor[W, I]](executor T, modules ...Module) *Base[W, I, T] {
	//
	return &Base[W, I, T]{
		modules:   modules,
		callstack: nil,
		executor:  executor,
	}
}

// Boot this machine by starting the given function with the given inputs.  This
// function assumes the given inputs are correctly formed, and will: (1) ingore
// unknown inputs; (2) initialise empty memories when no input is given for
// them.  Thus, it is recommended to perform sanity checking on input prior to
// calling this function.
func (p *Base[W, I, T]) Boot(fun string, input map[string][]W) error {
	// Look for function with the machine name
	for i, m := range p.modules {
		if _, ok := m.(*function.Function[I]); ok {
			if m.Name() == fun {
				// Initialise memory
				p.initialise(input)
				// Boot the frame
				return p.Enter(uint(i), nil, nil, nil)
			}
		}
	}
	// No function found
	return fmt.Errorf("missing boot function \"%s\"", fun)
}

// Execute the machine for the given number of steps, returning the actual
// number of steps executed and an error (if execution failed).
func (p *Base[W, I, T]) Execute(steps uint) (uint, error) {
	var (
		nsteps uint
		err    error
	)
	//
	for len(p.callstack) > 0 && nsteps < steps {
		if err = p.execute(); err != nil {
			return nsteps, err
		}
		//
		nsteps++
	}
	//
	return nsteps, nil
}

// Enter implementation for the machine.Core interface.
func (p *Base[W, I, T]) Enter(id uint, frame []W, args []register.Id, returns []register.Id) error {
	var (
		fn       = p.modules[id].(*function.Function[I])
		newFrame = NewFrame[W](id, fn.Width(), uint(len(args)), returns)
	)
	// Initialise arguments
	for i, arg := range args {
		var (
			val    = frame[arg.Unwrap()]
			target = fn.Register(register.NewId(uint(i)))
		)
		//
		if !target.IsNative() && !val.FitsWithin(target.Width()) {
			return fmt.Errorf("bit overflow (0x%s not u%d)", val.Text(16), target.Width())
		}
		//
		newFrame.Store(uint(i), frame[arg.Unwrap()])
	}
	// Push frame onto call stack
	p.callstack = append(p.callstack, newFrame)
	//
	return nil
}

// Leave implementation for the machine.Core interface.
func (p *Base[W, I, T]) Leave() (bool, error) {
	var (
		n           = len(p.callstack) - 1
		calleeFrame = p.callstack[n]
	)
	// pop call stack
	p.callstack = p.callstack[:n]
	// write returns (if applicable)
	if n >= 1 {
		// identify caller function
		var (
			callerFrame = p.callstack[n-1]
			caller      = p.modules[callerFrame.functionId].(*function.Function[I])
		)
		//
		for i, r := range calleeFrame.returns {
			var (
				val    = calleeFrame.Return(uint(i))
				target = caller.Register(r)
			)
			//
			if !target.IsNative() && !val.FitsWithin(target.Width()) {
				return true, errors.New("bit overflow")
			}
			//
			callerFrame.Store(r.Unwrap(), val)
		}
	}
	//
	return n == 0, nil
}

// Executor returns the executor for this machine.  This is primarily useful
// for transformations which need to inspect machine-specific executor state
// (e.g. a word machine's prime modulus) when constructing a derived machine.
func (p *Base[W, I, T]) Executor() T {
	return p.executor
}

// Module implementation for the machine.Core interface.
func (p *Base[W, I, T]) Module(id uint) Module {
	return p.modules[id]
}

// Modules implementation for the machine.Core interface.
func (p *Base[W, I, T]) Modules() []Module {
	return p.modules
}

// Depth returns the depth of the call stack.
func (p *Base[W, I, T]) Depth() uint {
	return uint(len(p.callstack))
}

// StackFrame returns the nth stack frame, where n==0 returns the root frame.
func (p *Base[W, I, T]) StackFrame(n uint) Frame[W] {
	return p.callstack[n]
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *Base[W, I, T]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.modules); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.executor); err != nil {
		return nil, err
	}
	// Callstack is execution state and is not persisted.
	return buffer.Bytes(), nil
}

// nolint
func (p *Base[W, I, T]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.modules); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.executor); err != nil {
		return err
	}
	// Callstack starts empty; populated only by Boot/Enter at execution time.
	p.callstack = nil
	//
	return nil
}

// ========================================================
// Helpers
// =======================================================

func (p *Base[W, I, T]) initialise(input map[string][]W) {
	// Initialise stack input memories
	for _, m := range p.modules {
		// Check module is a memory
		mem, ok := m.(memory.Memory[W])
		if !ok {
			continue
		}
		// Initialise with provided contents, or reset to empty if not supplied.
		mem.Initialise(input[m.Name()])
	}
}

// Execute implementation for the Executor interface
func (p *Base[W, I, T]) execute() error {
	// Decode
	var uInsn, width, regs, err = p.decode()
	//
	if err == nil {
		// Execute
		err = p.executeInstruction(uInsn, width, regs)
	}
	//
	if err != nil {
		err = p.extendError(err)
	}
	//
	return err
}

// nolint
func (p *Base[W, I, T]) decode() (I, uint, []register.Register, error) {
	var (
		n = len(p.callstack) - 1
		// Extract executing frame
		frame = p.callstack[n]
		// Identify enclosing function
		fn = p.modules[frame.Function()].(*function.Function[I])
		// Determine current PC position
		pc = frame.PC()
		// Lookup instruction to execute
		insn = fn.CodeAt(pc.Macro())
		//
		uInsn I
		//
		width = uint(1)
	)
	// Decode vector instruction
	uInsn = insn.Codes[pc.Micro()]
	width = uint(len(insn.Codes))
	// Done
	return uInsn, width, fn.Registers(), nil
}

func (p *Base[W, I, T]) extendError(err error) (exterr error) {
	var builder strings.Builder
	//
	for i, frame := range p.callstack {
		if i != 0 {
			builder.WriteString(" > ")
		}
		//
		builder.WriteString(SignatureOf[W, I](frame, p.modules))
		builder.WriteString("@")
		builder.WriteString(frame.pc.String())
	}
	//
	return fmt.Errorf("%w [%s]", err, builder.String())
}

func (p *Base[W, I, T]) executeInstruction(insn I, width uint, regs []register.Register,
) (err error) {
	//
	var (
		fp                                 = len(p.callstack) - 1
		stackframe                         = p.callstack[fp]
		frame                              = stackframe.registers
		pc                                 = stackframe.PC()
		binsn      instruction.Instruction = insn
	)
	//
	//nolint
	switch insn.OpCode() {
	// ==============================================================
	// Control-Flow Instructions
	// ==============================================================
	case opcode.CALL:
		insn := binsn.(*instruction.Call)
		// Enter callee stack frame
		return p.Enter(insn.Id, frame, insn.Arguments, insn.Returns)
	case opcode.FAIL:
		var (
			insn = binsn.(*instruction.Fail)
			//
			msg = executeFormattedChunks(insn.Chunks, regs, frame)
		)
		// check whether to include msg or not
		if len(insn.Chunks) == 0 {
			return errors.New("machine panic")
		}
		// include msg in error
		return fmt.Errorf("machine panic: %s", msg)
	case opcode.JUMP:
		insn := binsn.(*instruction.Jump)
		// Goto target instruction in current frame
		p.callstack[fp].Goto(pc.Goto(uint(insn.Immediate)))
		return nil
	case opcode.RETURN:
		if done, err := p.Leave(); done {
			return err
		}
		// adjust frame pointer
		fp = fp - 1
		// redecode instruction to update width
		_, width, _, err = p.decode()
		// reload pc
		pc = p.callstack[fp].PC()
		// fall thru

	// ==============================================================
	// Memory Instructions
	// ==============================================================
	case opcode.MEMORY_READ:
		var (
			insn = binsn.(*instruction.MemRead)
			rom  = p.modules[insn.Id].(memory.Memory[W])
		)
		// Read data words from tiven address
		err = rom.Read(frame, insn.Address(), insn.Data())
		// Fall thru
	case opcode.MEMORY_WRITE:
		var (
			insn = binsn.(*instruction.MemWrite)
			rom  = p.modules[insn.Id].(memory.Memory[W])
		)
		// Read data words from tiven address
		err = rom.Write(frame, insn.Address(), insn.Data())
		// Fall thru

	// ==============================================================
	// Misc Instructions
	// ==============================================================

	case opcode.SKIP:
		insn := binsn.(*instruction.Skip)
		// Skip some micro-instructions
		pc = pc.Skip(insn.Skip)
		// Fall thru
	case opcode.SKIP_IF:
		insn := binsn.(*instruction.SkipIf)
		// Skip (conditionally) micro-instructions
		if executeCondition(frame, insn.Cond, insn.Left, insn.Right) {
			pc = pc.Skip(insn.Skip)
		}
		// Fall thru
	case opcode.DEBUG:
		insn := binsn.(*instruction.Debug)
		fmt.Print(executeFormattedChunks[W](insn.Chunks, regs, frame))
	default:
		// Call provided executor
		err = p.executor.Execute(insn, frame, regs)
	}
	// Fall through to next instruction if no error.
	if err == nil {
		p.callstack[fp].Goto(pc.Next(width))
	}
	// Done
	return err
}

func executeFormattedChunks[W BaseWord[W]](chunks []base.FormattedChunk, regs []register.Register, frame []W) string {
	var builder strings.Builder
	//
	for _, chunk := range chunks {
		builder.WriteString(chunk.Text)
		//
		if chunk.Format.HasFormat() {
			words := loadAll(frame, chunk.Argument)
			regs := loadAll(regs, chunk.Argument)
			builder.WriteString(zkc_util.FormatWord(chunk.Format, regs, words...))
		}
	}
	//
	return builder.String()
}

// ==============================================================
// Conditions
// ==============================================================
func executeCondition[T util.Comparable[T]](frame []T, cond opcode.Condition, left, right register.Vector) bool {
	var (
		lhs = loadAll(frame, left)
		rhs = loadAll(frame, right)
	)
	// TODO: for now, we make this assumption.  However, it can be releaxed in
	// order to allow comparisons involving registers of different width.  It
	// should be fairly easy to support this though.
	if left.Len() != right.Len() {
		panic("support non-uniform vector comparisons")
	}
	//
	switch cond {
	case opcode.EQ:
		return cmp(lhs, rhs) == 0
	case opcode.NEQ:
		return cmp(lhs, rhs) != 0
	case opcode.LT:
		return cmp(lhs, rhs) < 0
	case opcode.LTEQ:
		return cmp(lhs, rhs) <= 0
	case opcode.GT:
		return cmp(lhs, rhs) > 0
	case opcode.GTEQ:
		return cmp(lhs, rhs) >= 0
	default:
		panic("unreachable")
	}
}

// LoadAll values value of a set of registers from this stack frame.
func loadAll[W any](frame []W, regs register.Vector) []W {
	var words = make([]W, regs.Len())
	//
	for i, reg := range regs.Registers() {
		words[i] = frame[reg.Unwrap()]
	}
	//
	return words
}

// Perform lexicographic comparison of two (equally sized) arrays.  In each
// array, the least significant word is at index 0.
func cmp[T util.Comparable[T]](lhs []T, rhs []T) int {
	for i := len(lhs); i > 0; i-- {
		c := lhs[i-1].Cmp(rhs[i-1])
		//
		if c != 0 {
			return c
		}
	}
	// lhs == rhs
	return 0
}
