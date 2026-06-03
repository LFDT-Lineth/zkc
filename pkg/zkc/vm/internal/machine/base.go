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
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Instruction is a convenient alias
type Instruction = instruction.Instruction

// BaseWord captures the minimal set of requirements for a word used in the base
// machine.
type BaseWord[W any] = word.Base[W]

// ============================================================================
// Base Machine
// ============================================================================

// Base provides a fundamental implementation of a machine.  The intention is
// that other machine variations build off this by providing executors specific
// to their instruction set.
type Base[W BaseWord[W], I Instruction, T Executor[W, I]] struct {
	modules   []Module
	callstack CallStack[W, I]
	executor  T
}

// NewBase constructs a new empty base machine
func NewBase[W BaseWord[W], I Instruction, T Executor[W, I]](executor T, modules ...Module) *Base[W, I, T] {
	//
	return &Base[W, I, T]{
		modules:  modules,
		executor: executor,
	}
}

// Boot this machine by starting the given function with the given inputs.  This
// function assumes the given inputs are correctly formed, and will: (1) ingore
// unknown inputs; (2) initialise empty memories when no input is given for
// them.  Thus, it is recommended to perform sanity checking on input prior to
// calling this function.
func (p *Base[W, I, T]) Boot(fun string, input map[string][]W) error {
	// Reset call stack
	p.callstack.Reset()
	// Look for function with the machine name
	for i, m := range p.modules {
		if _, ok := m.(*function.Function[I]); ok {
			if m.Name() == fun {
				fid := uint(i)
				// Initialise memory
				p.initialise(input)
				// Boot the call stack
				p.callstack.Boot(fid, p.Function(fid))
				//
				return nil
			}
		}
	}
	// No function found
	return fmt.Errorf("missing boot function \"%s\"", fun)
}

// Function returns the function with the corresponding ID (or panics if the ID
// does not correspond to a function).
func (p *Base[W, I, T]) Function(id uint) *function.Function[I] {
	return p.modules[id].(*function.Function[I])
}

// Execute the machine for the given number of steps, returning the actual
// number of steps executed and an error (if execution failed).
func (p *Base[W, I, T]) Execute(steps uint) (uint, error) {
	var (
		nsteps = steps
		err    error
	)
	//
	for !p.callstack.IsEmpty() && nsteps > 0 && err == nil {
		nsteps, err = p.execute(nsteps)
	}
	//
	return (steps - nsteps), err
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
	return p.callstack.Depth()
}

// StackFrame returns the nth stack frame, where n==0 returns the active frame.
func (p *Base[W, I, T]) StackFrame(n uint) StackFrame[W, I] {
	return p.callstack.Frame(n)
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
	p.callstack.Reset()
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

// Execute at most n steps of the machine, returning the number of steps
// actually remaining along with an optional error.
func (p *Base[W, I, T]) execute(n uint) (uint, error) {
	var (
		depth = p.callstack.Depth()
		frame = p.callstack.Frame(0)
		codes = frame.Vector(frame.pc.Macro()).Codes
	)
	// continue executing until either all requested instructions are completed,
	// the program is completed, or an error is hit.  This is a hot loop, and
	// the goal is to minimise unnecessary accesses to the call stack.
	for n > 0 && depth > 0 {
		var (
			odepth = depth
			insn   = codes[frame.pc.Micro()]
		)
		// Execute the current instruction
		npc, nonseq, err := p.executeInstruction(insn, frame)
		//
		if err != nil {
			// machine panic
			return n, err
		} else if nonseq {
			// Reload depth
			depth = p.callstack.Depth()
			// Decide what happened
			switch {
			case depth == 0:
				// termination
				return n - 1, nil
			case depth > odepth:
				// enter
				frame = p.callstack.Frame(0)
				codes = frame.Vector(frame.pc.Macro()).Codes
			case depth < odepth:
				// return
				frame = p.callstack.Frame(0)
				codes = frame.Vector(frame.pc.Macro()).Codes
				// Fall through
				frame.pc = frame.PC().Next(uint(len(codes)))
			default:
				// jump
				frame.pc = npc
				codes = frame.Vector(npc.Macro()).Codes
			}
		} else {
			// sequential instruction
			frame.pc = npc.Next(uint(len(codes)))
		}
		//
		n = n - 1
	}
	//
	p.callstack.Goto(frame.pc)
	//
	return n, nil
}
func (p *Base[W, I, T]) executeInstruction(insn I, frame StackFrame[W, I],
) (npc ProgramCounter, jmp bool, err error) {
	//nolint
	switch insn.OpCode() {
	// ==============================================================
	// Control-Flow Instructions
	// ==============================================================
	case opcode.CALL:
		var binsn any = insn
		return p.executeCall(binsn.(*instruction.Call), frame)
	case opcode.FAIL:
		var binsn any = insn
		return p.executeFail(binsn.(*instruction.Fail), frame)
	case opcode.JUMP:
		var binsn any = insn
		insn := binsn.(*instruction.Jump)
		// Goto target instruction in current frame
		return frame.pc.Goto(uint(insn.Immediate)), true, nil
	case opcode.RETURN:
		err := p.callstack.Leave()
		return PC_UNUSED, true, err

	// ==============================================================
	// Memory Instructions
	// ==============================================================
	case opcode.MEMORY_READ:
		var binsn any = insn
		err = p.executeMemRead(binsn.(*instruction.MemRead), frame)
		// Fall thru
	case opcode.MEMORY_WRITE:
		var binsn any = insn
		err = p.executeMemWrite(binsn.(*instruction.MemWrite), frame)
		// Fall thru
	// ==============================================================
	// Misc Instructions
	// ==============================================================

	case opcode.SKIP:
		var binsn any = insn
		insn := binsn.(*instruction.Skip)
		// Skip some micro-instructions
		frame.pc = frame.pc.Skip(insn.Skip)
		// Fall thru
	case opcode.SKIP_IF:
		var binsn any = insn
		insn := binsn.(*instruction.SkipIf)
		// Skip (conditionally) micro-instructions
		if executeCondition(frame, insn.Cond, insn.Left, insn.Right) {
			frame.pc = frame.pc.Skip(insn.Skip)
		}
		// Fall thru
	case opcode.DEBUG:
		var binsn any = insn
		insn := binsn.(*instruction.Debug)
		fmt.Print(executeFormattedChunks(insn.Chunks, frame))
	default:
		// Call provided executor
		err = p.executor.Execute(insn, frame)
	}
	// Fall through to next instruction if no error.
	return frame.pc, false, err
}

func (p *Base[W, I, T]) executeCall(insn *instruction.Call, frame StackFrame[W, I]) (ProgramCounter, bool, error) {
	// Save caller PC
	p.callstack.Goto(frame.pc)
	// Enter callee stack frame
	return PC_UNUSED, true, p.callstack.Enter(insn.Id, p.Function(insn.Id))
}

func (p *Base[W, I, T]) executeFail(insn *instruction.Fail, frame StackFrame[W, I]) (ProgramCounter, bool, error) {
	var msg = executeFormattedChunks(insn.Chunks, frame)
	// check whether to include msg or not
	if len(insn.Chunks) == 0 {
		return frame.pc, false, errors.New("machine panic")
	}
	// include msg in error
	return PC_UNUSED, false, fmt.Errorf("machine panic: %s", msg)
}

func (p *Base[W, I, T]) executeMemRead(insn *instruction.MemRead, frame StackFrame[W, I]) (err error) {
	var (
		mem        = p.modules[insn.Id].(memory.Memory[W])
		address, _ = mem.Geometry().Decode(frame.values, insn.Address())
		targets    = insn.Data()
		val        W
	)
	// Read data words from tiven address
	for i := 0; i < len(targets) && err == nil; i++ {
		val, err = mem.Read(address)
		//
		if err == nil {
			err = frame.Store(targets[i], val)
		}
		//
		address++
	}
	//
	return err
}

func (p *Base[W, I, T]) executeMemWrite(insn *instruction.MemWrite, frame StackFrame[W, I]) (err error) {
	var (
		mem        = p.modules[insn.Id].(memory.Memory[W])
		address, _ = mem.Geometry().Decode(frame.values, insn.Address())
		targetRegs = mem.Geometry().DataRegisters()
		sourceRegs = insn.Data()
	)
	// Write data words to the given address range
	for i := 0; i < len(sourceRegs) && err == nil; i++ {
		var (
			ith = targetRegs[i]
			val = frame.Load(sourceRegs[i])
		)
		// bitwidth check
		if ith.IsNative() || val.FitsWithin(ith.Width()) {
			err = mem.Write(address, val)
		} else {
			// failed
			err = fmt.Errorf("bit overflow (0x%s not u%d)", val.Text(16), ith.Width())
		}
		//
		address++
	}
	// Fall thru
	return err
}

func executeFormattedChunks[W BaseWord[W], I Instruction](chunks []base.FormattedChunk, frame StackFrame[W, I]) string {
	var builder strings.Builder
	//
	for _, chunk := range chunks {
		builder.WriteString(chunk.Text)
		//
		if chunk.Format.HasFormat() {
			builder.WriteString(formatWord(chunk.Format, chunk.Argument, frame))
		}
	}
	//
	return builder.String()
}

func executeCondition[W BaseWord[W], I Instruction](frame StackFrame[W, I], cond opcode.Condition,
	lhs, rhs register.Vector) bool {
	// TODO: for now, we make this assumption.  However, it can be releaxed in
	// order to allow comparisons involving registers of different width.  It
	// should be fairly easy to support this though.
	if lhs.Len() != rhs.Len() {
		panic("support non-uniform vector comparisons")
	}
	//
	switch cond {
	case opcode.EQ:
		return cmp(lhs, rhs, frame) == 0
	case opcode.NEQ:
		return cmp(lhs, rhs, frame) != 0
	case opcode.LT:
		return cmp(lhs, rhs, frame) < 0
	case opcode.LTEQ:
		return cmp(lhs, rhs, frame) <= 0
	case opcode.GT:
		return cmp(lhs, rhs, frame) > 0
	case opcode.GTEQ:
		return cmp(lhs, rhs, frame) >= 0
	default:
		panic("unreachable")
	}
}

// ==============================================================
// Helpers
// ==============================================================

// FormatWord applies a given format to a given word to generate a formatted string.
func formatWord[W BaseWord[W], I Instruction](fmt zkc_util.Format, vec register.Vector, frame StackFrame[W, I]) string {
	var (
		digits string
		value  big.Int
		regs   = vec.Registers()
	)
	// Loop from most-significant word to least significant.
	for i := vec.Len(); i > 0; i-- {
		var reg = regs[i-1]
		// Shift left
		value.Lsh(&value, frame.BitwidthOf(reg))
		// Add next word
		value.Add(&value, frame.Load(reg).BigInt())
	}
	//
	switch fmt.Code {
	case zkc_util.FORMAT_DEC:
		digits = value.Text(10)
	case zkc_util.FORMAT_HEX:
		digits = value.Text(16)
	case zkc_util.FORMAT_BIN:
		digits = value.Text(2)
	case zkc_util.FORMAT_CHR:
		// Render the value as a single ASCII character.  Type-checking
		// (in the zkc compiler) enforces that the argument is a concrete
		// u8, so the value fits in a single byte; nonetheless we mask
		// the low 8 bits defensively in case this is called outside
		// that path (e.g. by future Unicode work, or by tests that
		// bypass the type checker).
		if w, ok := any(value).(interface{ BigInt() *big.Int }); ok {
			return string([]byte{byte(w.BigInt().Uint64() & 0xff)})
		}
		//
		var v big.Int
		v.SetString(value.Text(10), 10)
		//
		return string([]byte{byte(v.Uint64() & 0xff)})
	default:
		panic("invalid format")
	}
	// Apply any padding to the digit portion.
	if uint(len(digits)) < fmt.Width {
		padding := int(fmt.Width) - len(digits)
		//
		if fmt.ZeroPad {
			digits = strings.Repeat("0", padding) + digits
		} else {
			digits = strings.Repeat(" ", padding) + digits
		}
	}
	//
	return digits
}

// Perform lexicographic comparison of two (equally sized) arrays.  In each
// array, the least significant word is at index 0.
func cmp[W BaseWord[W], I Instruction](left, right register.Vector, frame StackFrame[W, I]) int {
	var (
		lhs = left.Registers()
		rhs = right.Registers()
	)
	for i := len(lhs); i > 0; i-- {
		var (
			l = frame.Load(lhs[i-1])
			r = frame.Load(rhs[i-1])
		)

		c := l.Cmp(r)
		//
		if c != 0 {
			return c
		}
	}
	// lhs == rhs
	return 0
}
