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
package instruction

import (
	"encoding/gob"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/base"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
)

// FormattedChunk is a convenient alias
type FormattedChunk = base.FormattedChunk

// OpCode is a convenient alias
type OpCode = opcode.OpCode

// Module is a convenient alias
type Module = base.Module

// SystemMap is a convenient alias
type SystemMap = base.SystemMap

// Instruction characterises the kinds of instructions which can be
// vectorized.  They key is that, whilst many instructions are also micro
// instructions, this is not always the case.  Specifically, there are
// instructions which are not valid micro-instructions and, likewise,
// micro-instructions which are not valid instructions.
type Instruction interface {
	// OpCode returns the opcode for this instruction.
	OpCode() OpCode
	// Uses returns the set of variables used (i.e. read) by this instruction.
	Uses() []register.Id
	// Definitions returns the set of variables registers defined (i.e. written)
	// by this instruction.
	Definitions() []register.Id
	// Validate that this micro-instruction is well-formed.  For example, that
	// it is balanced, that there are no conflicting writes, that all
	// temporaries have been allocated, etc.
	MicroValidate(width uint, field field.Config, mapping SystemMap) []error
	// Provide human readable form of instruction
	String(SystemMap) string
}

// ============================================================================
// Base Instructions
// ============================================================================

// Debug is a convenient alias
type Debug = base.Debug

// NewDebug constructs a new debug instruction.
func NewDebug(chunks ...FormattedChunk) *Debug {
	return &Debug{Chunks: chunks}
}

// ============================================================================

// Call invokes the function module identified by Id, passing the values of
// the argument registers as inputs and writing the function's outputs into
// the return registers of the caller's frame.  Execution of the calling
// frame is suspended until the callee returns; on return, control resumes
// at the instruction following the Call.
type Call struct{ base.OpIo }

// NewCall constructs a new function call instruction.
func NewCall(id uint, arguments []register.Id, returns []register.Id) *Call {
	return &Call{base.OpIo{Op: opcode.CALL, Id: id, Arguments: arguments, Returns: returns}}
}

// ============================================================================

// Fail is a convenient alias
type Fail = base.Fail

// NewFail constructs a fresh fail instruction with the given (possibly empty)
// formatted error message.
func NewFail(chunks ...FormattedChunk) *Fail {
	return &Fail{Chunks: chunks}
}

// ============================================================================

// Jump performs an unconditional branch to the instruction identified by the
// immediate operand.  The immediate is interpreted as the target program
// counter within the enclosing function, so executing a Jump simply transfers
// control to that PC.  Jump is one of the three control-flow terminators (along
// with Return and Fail) recognised by the vectoriser as ending a basic block.
type Jump struct{ base.OpImm }

// NewJump constructs a fresh unconditional jump instruction to the given PC
// location.
func NewJump(target uint) *Jump {
	return &Jump{base.OpImm{Op: opcode.JUMP, Immediate: target}}
}

// ============================================================================

// MemRead reads from the memory module identified by Id at the address
// formed from the argument registers, depositing the resulting data words
// into the return registers.  The target module must be a Random Access
// Memory (RAM) or a Read-Only Memory (ROM).
type MemRead struct{ base.OpIo }

// NewMemRead constructs a new instruction which reads the value from either a
// Random Access Memory (RAM) or a Read-Only Memory (ROM).
func NewMemRead(id uint, address []register.Id, data []register.Id) *MemRead {
	return &MemRead{base.OpIo{Op: opcode.MEMORY_READ, Id: id, Arguments: address, Returns: data}}
}

// ============================================================================

// MemWrite writes to the memory module identified by Id, using the argument
// registers as the data words and the return registers as the address.  The
// target module must be a Random Access Memory (RAM) or a Write-Once Memory
// (WOM).  Despite the name, the "return" registers here identify the
// destination address rather than receiving any output — a MemWrite defines
// no registers in the surrounding frame.
type MemWrite struct{ base.OpIo }

// NewMemWrite constructs a new instruction which writes data values to either
// a Random Access Memory (RAM) or a Write-Once Memory (WOM).
func NewMemWrite(id uint, address []register.Id, data []register.Id) *MemWrite {
	return &MemWrite{base.OpIo{Op: opcode.MEMORY_WRITE, Id: id, Arguments: address, Returns: data}}
}

// ============================================================================

// Return leaves the current stack frame, copying the function's return
// registers into the caller's frame and resuming execution at the instruction
// following the originating Call.  When executed at the outermost (boot)
// frame, Return halts the machine.  The immediate operand is unused.  Return
// is one of the three control-flow terminators (along with Jmp and Fail)
// recognised by the vectoriser as ending a basic block.
type Return struct{ base.OpImm }

// NewReturn constructs a fresh return instruction.
func NewReturn() *Return {
	return &Return{base.OpImm{Op: opcode.RETURN, Immediate: 0}}
}

// ============================================================================

// Skip microcode performs an unconditional skip over a given number of codes.
type Skip = base.Skip

// ============================================================================

// SkipIf microcode performs a conditional skip over a given number of codes. The
// condition is either that two registers are equal, or that they are not equal.
// This has two variants: register-register; and, register-constant.  The latter
// is indiciated when the right register is marked as UNUSED.
type SkipIf = base.SkipIf

// NewSkipIf constructs a fresh conditional skip instruction.
func NewSkipIf(condition opcode.Condition, left, right register.Id, skip uint) *SkipIf {
	return NewSkipIfVec(condition, register.NewVector(left), register.NewVector(right), skip)
}

// NewSkipIfVec constructs a fresh (vectored) conditional skip instruction.
func NewSkipIfVec(condition opcode.Condition, left, right register.Vector, skip uint) *SkipIf {
	return &SkipIf{Cond: condition, Left: left, Right: right, Skip: skip}
}

// ============================================================================
// Helpers
// ============================================================================

// NewSystemMap constructs a new system map
func NewSystemMap(regs register.Map, modules []Module) SystemMap {
	return &systemMap{regs, modules}
}

type systemMap struct {
	regs    register.Map
	modules []Module
}

func (p *systemMap) Module(id uint) Module {
	return p.modules[id]
}

func (p *systemMap) Name() trace.ModuleName {
	return p.regs.Name()
}

func (p *systemMap) HasRegister(name string) (register.Id, bool) {
	return p.regs.HasRegister(name)
}

func (p *systemMap) Register(id register.Id) register.Register {
	return p.regs.Register(id)
}

func (p *systemMap) Registers() []register.Register {
	return p.regs.Registers()
}

func (p *systemMap) String() string {
	return p.regs.String()
}

func init() {
	gob.Register(Instruction(&Call{}))
	gob.Register(Instruction(&Debug{}))
	gob.Register(Instruction(&Fail{}))
	gob.Register(Instruction(&FieldHint{}))
	gob.Register(Instruction(&Jump{}))
	gob.Register(Instruction(&MemRead{}))
	gob.Register(Instruction(&MemWrite{}))
	gob.Register(Instruction(&Return{}))
	gob.Register(Instruction(&Skip{}))
	gob.Register(Instruction(&SkipIf{}))
}
