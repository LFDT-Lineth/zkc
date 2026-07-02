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
package interpreter

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Interpreter is a fast, register-based interpreter for the bytecode form of a
// compiled program.  It implements the machine.Core interface and exists to
// execute programs as efficiently as possible (e.g. for testing and benchmark
// purposes), in contrast to the higher-level instruction representations which
// are better suited to analysis and compilation.
//
// Execution proceeds by decoding and dispatching one bytecode at a time (see
// Execute) within the body of the currently executing function.  The
// interpreter maintains a small amount of machine state directly:
//
//   - A program counter (pc) identifying the next bytecode to execute, as an
//     offset into the program's flat bytecode array.
//   - A frame pointer (fp) identifying the base of the current function's
//     activation record within the data stack.  Registers are addressed
//     relative to this frame (i.e. stack[fp+r]).
//   - A data stack holding the registers of all active activation records, and
//     a call stack holding return addresses.
//
// All memory accessible to the program is held externally to the data/call
// stacks, organised by access discipline.  The memories are extracted from the
// program's modules up front (see NewInterpreter) and indexed by kind so that
// read/write bytecodes can address them directly by identifier.
//
// The interpreter is parameterised over the underlying word type W (e.g. 8-bit
// or 16-bit words, as determined by the target field).
type Interpreter[W word.Word[W]] struct {
	program encoding.Binary[W]
	// Prime modulus of the surrounding field, needed to simulate native field
	// instructions (ADDMOD_P, SUBMOD_P and MULMOD_P).
	modulus W
	// Current function module identifier.
	fid uint16
	// Program counter: offset into program.bytecodes of the next bytecode to
	// decode and execute.
	pc uint32
	// Frame pointer: base offset within dataStack of the current function's
	// activation record.  Registers are addressed relative to this.
	fp uint32
	// return pointer / width (used for function call returns).
	rp, rw uint32
	// Data stack holding the activation records (registers) of active function
	// calls.  The current frame begins at fp.
	dataStack heap.Heap[W]
	// Call stack holding caller state for nested calls.
	callStack heap.Heap[StackFrame]
	// Static read-only memories: read-only memories whose contents are fixed and
	// not provided as inputs.
	sroms []memory.StaticReadOnly[W]
	// Read-only memories.  Non-static read-only memories form the program's
	// inputs (see Inputs).
	roms []memory.ReadOnly[W]
	// Write-once memories.  These form the program's outputs (see Outputs).
	woms []memory.WriteOnce[W]
	// (Small) random-access memories which may be freely read and written.
	rams []memory.RandomAccess[W]
	// (Large) paged random-access memories which may be freely read and
	// written.
	prams []memory.PagedRandomAccess[W]
	// Optional callback invoked when a CHECKPOINT bytecode is executed, passed a
	// snapshot of the current machine state (see CheckPoint).  Configured via
	// the CheckPointer builder method; nil if no checkpointer has been set.
	checkpointer func(checkpoint.CheckPoint[W])
	// Counter governing how frequently the checkpointer fires.  Configured
	// alongside the checkpointer via the CheckPointer builder method.
	counter util.Counter
}

// StackFrame captures relevant information about all functions currently
// executing a CALL.  Such functions are "paused" whilst the active function is
// being executed.  The purpose of a stack frame is to record the Frame Pointer
// (FP) and Program Counter (PC) of the relevant function so that these can be
// restored when it becomes the active function.
type StackFrame = checkpoint.StackFrame

// New constructs a new bytecode interpreter for the given program.
// The program's memory modules are partitioned by access discipline (static
// read-only, read-only, write-once, random-access and paged random-access)
// so that read/write bytecodes can locate them directly by identifier during
// execution.  The modulus is the prime characteristic of the surrounding field,
// used when executing native field instructions.  The interpreter is created in
// an unbooted state; Boot must be called to select an entry point and supply
// inputs before calling Execute.
func New[W word.Word[W]](program descriptor.Program[W], modulus W) *Interpreter[W] {
	var (
		sroms    []memory.StaticReadOnly[W]
		roms     []memory.ReadOnly[W]
		woms     []memory.WriteOnce[W]
		rams     []memory.RandomAccess[W]
		prams    []memory.PagedRandomAccess[W]
		compiled = CompileProgram(program)
	)
	// Initialise memories
	for _, m := range program.Modules() {
		//
		if m, ok := m.(*descriptor.Memory[W]); ok {
			switch {
			case m.IsStatic():
				var mem = memory.NewStatic(m.Name(), m.IsPublic(), m.Geometry(), m.StaticContents()...)
				//
				sroms = append(sroms, *mem)
			case m.IsReadOnly():
				var mem = memory.NewReadOnly(m.Name(), m.IsPublic(), m.Geometry())
				//
				roms = append(roms, *mem)
			case m.IsWriteOnly():
				var mem = memory.NewWriteOnce(m.Name(), m.IsPublic(), m.Geometry())
				//
				woms = append(woms, *mem)
			case m.IsPaged():
				var mem = memory.NewPagedRandomAccess(m.Name(), m.Geometry())
				//
				prams = append(prams, *mem)
			case m.IsReadWrite():
				var mem = memory.NewRandomAccess(m.Name(), m.Geometry())
				//
				rams = append(rams, *mem)
			default:
				panic(fmt.Sprintf("unknown memory \"%s\" encountered", m.Name()))
			}
		}
	}
	//
	return &Interpreter[W]{
		program: compiled,
		modulus: modulus,
		pc:      0,
		fp:      0,
		rp:      0,
		rw:      0,
		sroms:   sroms,
		roms:    roms,
		woms:    woms,
		rams:    rams,
		prams:   prams,
		// Default checkpointer: panics until a real one is configured via
		// CheckPointer, since a CHECKPOINT bytecode is meaningless without one.
		checkpointer: func(checkpoint.CheckPoint[W]) {
			panic("no checkpointer configured")
		},
		// Default counter fires on every CHECKPOINT, so an unconfigured
		// interpreter reaches the panicking default checkpointer above.
		counter: util.NewCounter(1),
	}
}

// CheckPointer configures the callback invoked whenever a CHECKPOINT bytecode
// is executed, passing it a snapshot of the current machine state (see
// CheckPoint).  The given counter initialises the interpreter's checkpoint
// counter, governing how frequently the checkpointer fires.  It returns the
// interpreter to allow method chaining.
func (p *Interpreter[W]) CheckPointer(counter util.Counter,
	checkpointer func(checkpoint.CheckPoint[W])) *Interpreter[W] {
	//
	p.counter = counter
	p.checkpointer = checkpointer
	//
	return p
}

// Boot implementation of Core interface.  This locates the named function,
// points the program counter at its entry address, allocates an activation
// record for it on the data stack, and initialises all memories (loading the
// provided inputs into the input memories and resetting the rest).
func (p *Interpreter[W]) Boot(fun string, input map[string][]W) (err error) {
	// lookup function identifier
	fid, ok := p.program.HasModule(fun)
	//
	if !ok {
		return fmt.Errorf("unknown function \"%s\"", fun)
	}
	// find instruction to boot
	if p.pc, ok = p.program.AddressOf(fid); !ok {
		return fmt.Errorf("missing symbol for \"%s\"", fun)
	}
	//
	p.fid = fid
	p.fp = 0
	p.callStack.Clear()
	p.dataStack.Clear()
	// allocate space for the given function
	p.dataStack.Alloc(p.program.Module(fid).Width())
	// initialise memory
	p.initialise(input)
	//
	return err
}

// Inputs implementation of Core interface.  The inputs are the non-static
// read-only memories, i.e. those whose contents are supplied to Boot.
func (p *Interpreter[W]) Inputs() iter.Iterator[memory.InputOutput[W]] {
	var inputs []memory.InputOutput[W]
	//
	for i := range p.roms {
		if !p.roms[i].IsStatic() {
			inputs = append(inputs, &p.roms[i])
		}
	}
	//
	return iter.NewArrayIterator(inputs)
}

// Outputs implementation of Core interface.  The outputs are the write-once
// memories, whose contents are populated as the program executes.
func (p *Interpreter[W]) Outputs() iter.Iterator[memory.InputOutput[W]] {
	var outputs = make([]memory.InputOutput[W], len(p.woms))
	//
	for i := range p.woms {
		outputs[i] = &p.woms[i]
	}
	//
	return iter.NewArrayIterator(outputs)
}

// CheckPoint captures the current state of this interpreter as a checkpoint,
// from which execution can later be resumed (see checkpoint.CheckPoint).  The
// returned checkpoint records:
//
//   - the call stack of paused callers, with the currently executing frame
//     pushed on top so that the full active call chain is self-contained;
//   - the data stack holding the activation records (registers) of all active
//     frames;
//   - a snapshot of every read-only input memory and every mutable memory.
//     Static read-only memories form part of the fixed program and are not
//     captured.
//
// The data and call stacks are copied so that the checkpoint is unaffected by
// any subsequent execution of the interpreter.
func (p *Interpreter[W]) CheckPoint() checkpoint.CheckPoint[W] {
	var (
		callStack = slices.Clone(p.callStack.SliceEnd(0))
		dataStack = slices.Clone(p.dataStack.SliceEnd(0))
		memory    []checkpoint.Memory[W]
	)
	// Record the currently executing frame as the top of the call stack, so the
	// checkpoint fully describes the active call chain.
	callStack = append(callStack, checkpoint.NewStackFrame(p.fid, p.fp, p.pc))
	// Snapshot read-only input memories and mutable memories.
	for i := range p.roms {
		memory = append(memory, p.snapshotMemory(&p.roms[i]))
	}
	//
	for i := range p.woms {
		memory = append(memory, p.snapshotMemory(&p.woms[i]))
	}
	//
	for i := range p.rams {
		memory = append(memory, p.snapshotMemory(&p.rams[i]))
	}
	//
	for i := range p.prams {
		memory = append(memory, p.snapshotPagedMemory(&p.prams[i]))
	}
	//
	return checkpoint.NewCheckPoint(callStack, dataStack, memory)
}

// snapshotMemory captures the full contents of a flat memory as a single
// checkpoint page beginning at address zero.  The memory's module identifier is
// recovered from the program by name.
func (p *Interpreter[W]) snapshotMemory(mem memory.Memory[W]) checkpoint.Memory[W] {
	var (
		moduleId, _ = p.program.HasModule(mem.Name())
		page        = checkpoint.NewPage(0, slices.Clone(mem.Contents()))
	)
	//
	return checkpoint.NewMemory(moduleId, []checkpoint.Page[W]{page})
}

// snapshotPagedMemory captures a paged random-access memory as one checkpoint
// page per allocated memory page, preserving the sparse layout: page i in the
// backing table begins at physical address i*PAGE_SIZE.  Pages which have never
// been written (nil) are omitted.  The memory's module identifier is recovered
// from the program by name.
func (p *Interpreter[W]) snapshotPagedMemory(mem *memory.PagedRandomAccess[W]) checkpoint.Memory[W] {
	var (
		moduleId, _ = p.program.HasModule(mem.Name())
		pages       []checkpoint.Page[W]
	)
	//
	for it := mem.Pages(); it.HasNext(); {
		// Clone each page, as Pages references the live backing slices.
		pages = append(pages, it.Next().Clone())
	}
	//
	return checkpoint.NewMemory(moduleId, pages)
}

// Restore resets this interpreter to the state captured by the given checkpoint
// (see CheckPoint), such that execution can resume from that point.  The call
// stack, data stack and all captured memories are overwritten; the currently
// executing frame (the top of the checkpoint's call stack) is unpacked into the
// active fid/fp/pc.  The checkpoint's slices are copied into the interpreter, so
// the checkpoint remains valid for subsequent restores.
func (p *Interpreter[W]) Restore(cp checkpoint.CheckPoint[W]) {
	var (
		frames = cp.CallStack()
		// The topmost frame records the currently executing function (see
		// CheckPoint); the remainder are the paused callers.
		active = frames[len(frames)-1]
	)
	// Restore the active frame.
	p.fid = active.FunctionId
	p.fp = active.FramePointer
	p.pc = active.ProgramCounter
	// Reset the return pointer/width: these are transient and meaningful only
	// mid-return, so a checkpoint never captures an in-flight value.
	p.rp = 0
	p.rw = 0
	// Restore the paused callers.
	p.callStack.Clear()
	//
	for _, f := range frames[:len(frames)-1] {
		p.callStack.Push(f)
	}
	// Restore the data stack.
	var data = cp.DataStack()

	p.dataStack.Clear()
	p.dataStack.Alloc(uint(len(data)))
	copy(p.dataStack.SliceEnd(0), data)
	// Restore captured memories.
	for _, m := range cp.Memories() {
		p.restoreMemory(m)
	}
}

// restoreMemory overwrites the contents of the live memory identified by the
// snapshot's module identifier.  Read-only input memories are restored by
// replacing their flat contents directly, whilst mutable memories are first
// cleared and then each captured page is written back at its recorded address.
// Writing page-by-page preserves the sparse layout of paged memories (only
// captured pages are materialised).
func (p *Interpreter[W]) restoreMemory(m checkpoint.Memory[W]) {
	var mem = p.findMemory(p.program.Module(m.ModuleId()).Name())
	// Read-only memories cannot be written cell-by-cell; checkpoint snapshots
	// for ROMs are flat pages, so restore them by replacing the contents.
	if mem.IsReadOnly() {
		mem.Initialise(flattenMemory(m.Pages()))
		return
	}
	// Clear any existing contents.
	mem.Initialise(nil)
	// Write back each captured page.
	for _, page := range m.Pages() {
		var address = page.Address()
		//
		for _, w := range page.Data() {
			//nolint
			mem.Write(address, w)
			//
			address++
		}
	}
}

// flattenMemory converts a page-based checkpoint snapshot into flat contents.
// This is used for ROMs, whose checkpoint snapshots are full flat pages.
func flattenMemory[W word.Word[W]](pages []checkpoint.Page[W]) []W {
	var size uint64
	//
	for _, page := range pages {
		size = max(size, page.Address()+uint64(len(page.Data())))
	}
	//
	contents := make([]W, size)
	//
	for _, page := range pages {
		copy(contents[page.Address():], page.Data())
	}
	//
	return contents
}

// findMemory locates the live checkpointable memory with the given module name
// amongst the read-only input, write-once, random-access and paged random-access
// memories.  It panics if no such memory exists, as a checkpoint should only
// ever reference memories belonging to the program being executed.
func (p *Interpreter[W]) findMemory(name string) memory.Memory[W] {
	for i := range p.roms {
		if p.roms[i].Name() == name {
			return &p.roms[i]
		}
	}
	//
	for i := range p.woms {
		if p.woms[i].Name() == name {
			return &p.woms[i]
		}
	}
	//
	for i := range p.rams {
		if p.rams[i].Name() == name {
			return &p.rams[i]
		}
	}
	//
	for i := range p.prams {
		if p.prams[i].Name() == name {
			return &p.prams[i]
		}
	}
	//
	panic(fmt.Sprintf("unknown memory \"%s\"", name))
}

// Execute implementation of Core interface.  This runs the central fetch-decode-
// dispatch loop: each iteration reads the bytecode at the current program
// counter, extracts its opcode, and dispatches to the corresponding executor
// which performs the operation and returns the next program counter.  The loop
// runs for at most steps iterations, stopping early if the program returns from
// its outermost frame (RET with an empty call stack) or an error occurs (e.g.
// arithmetic overflow, or an explicit FAIL).  It returns the number of steps
// actually executed together with any error.
func (p *Interpreter[W]) Execute(steps uint) (uint, error) {
	var (
		nsteps    = uint(0)
		err       error
		frame     []W = p.dataStack.SliceEnd(uint(p.fp))
		bytecodes     = p.program.Bytecodes()
	)
	//
	for nsteps < steps && err == nil {
		// decode instruction
		var opcode = bytecodes[p.pc] & encoding.OPCODE_MASK
		// increase step counter
		nsteps++
		//
		switch opcode & encoding.OPCODE_MASK {
		case encoding.FAIL:
			return nsteps, p.executeFail(p.pc, bytecodes, frame)
		case encoding.CHECKCAST:
			p.pc, err = executeCheckCast(p.pc, bytecodes, frame)
		case encoding.DEBUG:
			p.pc = p.executeDebug(p.pc, bytecodes, frame)
		case encoding.LDC:
			p.pc = executeLdc_1(p.pc, bytecodes, frame)
		case encoding.LDC_w:
			p.pc = executeLdc_w(p.pc, bytecodes, frame)
		case encoding.MOVE:
			p.pc = executeMove_1s1(p.pc, bytecodes, frame)
		case encoding.ENTER_n:
			err = p.executeEnter_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.ENTERCP_n:
			err = p.executeEnterCheckPoint_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.LEAVE_n:
			p.pc = p.executeLeave_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.RET:
			// check for termination
			if p.callStack.Size() == 0 {
				return nsteps, nil
			}
			// normal reutrn
			p.pc, err = p.executeReturn(p.pc, bytecodes)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.JMP:
			p.pc, _ = encoding.DecodeJmp1(p.pc, bytecodes)
		case encoding.SKIP:
			p.pc, _ = encoding.DecodeSkip1(p.pc, bytecodes)
		case encoding.SEQ_rr:
			p.pc = executeSkipIf_rr[W, util.Equal[W]](p.pc, bytecodes, frame)
		case encoding.SNE_rr:
			p.pc = executeSkipIf_rr[W, util.NotEqual[W]](p.pc, bytecodes, frame)
		case encoding.SLT_rr:
			p.pc = executeSkipIf_rr[W, util.LessThan[W]](p.pc, bytecodes, frame)
		case encoding.SGT_rr:
			p.pc = executeSkipIf_rr[W, util.GreaterThan[W]](p.pc, bytecodes, frame)
		case encoding.SLE_rr:
			p.pc = executeSkipIf_rr[W, util.LessThanOrEqual[W]](p.pc, bytecodes, frame)
		case encoding.SGE_rr:
			p.pc = executeSkipIf_rr[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, frame)
		case encoding.SKIP_M:
			p.pc = executeSkipTable(p.pc, bytecodes, frame)
		case encoding.SEQ_rv:
			p.pc = executeSkipIf_rv[W, util.Equal[W]](p.pc, bytecodes, frame)
		case encoding.SNE_rv:
			p.pc = executeSkipIf_rv[W, util.NotEqual[W]](p.pc, bytecodes, frame)
		case encoding.SLT_rv:
			p.pc = executeSkipIf_rv[W, util.LessThan[W]](p.pc, bytecodes, frame)
		case encoding.SGT_rv:
			p.pc = executeSkipIf_rv[W, util.GreaterThan[W]](p.pc, bytecodes, frame)
		case encoding.SLE_rv:
			p.pc = executeSkipIf_rv[W, util.LessThanOrEqual[W]](p.pc, bytecodes, frame)
		case encoding.SGE_rv:
			p.pc = executeSkipIf_rv[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, frame)
			// Input / Output Operations
		case encoding.RD_ROM_nm:
			p.pc = executeReadRom_sn(p.pc, bytecodes, frame, p.roms)
		case encoding.RD_SROM_nm:
			p.pc = executeReadSrom_sn(p.pc, bytecodes, frame, p.sroms)
		case encoding.WR_WOM_nm:
			p.pc = executeWriteWom_sn(p.pc, bytecodes, frame, p.woms)
		case encoding.RD_RAM_nm:
			p.pc = executeReadRam_sn(p.pc, bytecodes, frame, p.rams)
		case encoding.WR_RAM_nm:
			p.pc = executeWriteRam_sn(p.pc, bytecodes, frame, p.rams)
		case encoding.RD_PRAM_nm:
			p.pc = executeReadPagedRam_sn(p.pc, bytecodes, frame, p.prams)
		case encoding.WR_PRAM_nm:
			p.pc = executeWritePagedRam_sn(p.pc, bytecodes, frame, p.prams)
		// Arithmetic Operations
		case encoding.ADD_2n1:
			p.pc, err = executeAdd_2n1(p.pc, bytecodes, frame)
		case encoding.ADDC:
			p.pc, err = executeAdd_1n1c(p.pc, bytecodes, frame)
		case encoding.SUB_2n1:
			p.pc, err = executeSub_2n1(p.pc, bytecodes, frame)
		case encoding.SUBC:
			p.pc, err = executeSub_1n1c(p.pc, bytecodes, frame)
		case encoding.MUL_2n1:
			p.pc, err = executeMul_2n1(p.pc, bytecodes, frame)
		case encoding.MULC:
			p.pc, err = executeMul_1n1c(p.pc, bytecodes, frame)
		case encoding.ADD_nm:
			p.pc, err = p.executeAdd_nm(p.pc, bytecodes, frame)
		case encoding.SUB_nm:
			p.pc, err = p.executeSub_nm(p.pc, bytecodes, frame)
		case encoding.MUL_nm:
			p.pc, err = p.executeMul_nm(p.pc, bytecodes, frame)
		case encoding.DIV:
			p.pc, err = executeDiv(p.pc, bytecodes, frame)
		case encoding.REM:
			p.pc, err = executeRem(p.pc, bytecodes, frame)
		case encoding.HINT:
			p.pc, err = p.executeHint(p.pc, bytecodes, frame)
		case encoding.ADDMOD_P:
			p.pc, err = p.executeFieldAdd(p.pc, bytecodes, frame)
		case encoding.SUBMOD_P:
			p.pc, err = p.executeFieldSub(p.pc, bytecodes, frame)
		case encoding.MULMOD_P:
			p.pc, err = p.executeFieldMul(p.pc, bytecodes, frame)
		case encoding.CAT:
			p.pc, err = p.executeCat(p.pc, bytecodes, frame)
		case encoding.NOT:
			p.pc, err = executeNot(p.pc, bytecodes, frame)
		case encoding.AND:
			p.pc, err = executeAnd(p.pc, bytecodes, frame)
		case encoding.OR:
			p.pc, err = executeOr(p.pc, bytecodes, frame)
		case encoding.XOR:
			p.pc, err = executeXor(p.pc, bytecodes, frame)
		case encoding.SHL:
			p.pc, err = executeShl(p.pc, bytecodes, frame)
		case encoding.SHR:
			p.pc, err = executeShr(p.pc, bytecodes, frame)
		default:
			err = fmt.Errorf("unknown bytecode encountered (0x%x)", opcode)
		}
	}
	//
	return nsteps, err
}

// initialise prepares all memories for a fresh execution.  Input (read-only and
// static read-only) memories are loaded with the values supplied for their name
// in the input map, whilst output and scratch memories (write-once, random-
// access and paged random-access) are reset to empty.
func (p *Interpreter[W]) initialise(input map[string][]W) {
	// initialise roms
	for i, m := range p.roms {
		p.roms[i].Initialise(input[m.Name()])
	}
	// initialise static roms
	for i, m := range p.sroms {
		p.sroms[i].Initialise(input[m.Name()])
	}
	// reset woms
	for i := range p.woms {
		p.woms[i].Initialise(nil)
	}
	// reset (small) rams
	for i := range p.rams {
		p.rams[i].Initialise(nil)
	}
	// reset (big) prams
	for i := range p.prams {
		p.prams[i].Initialise(nil)
	}
}

// ============================================================================
// Executors
// ============================================================================
//
// Each executor implements a single bytecode.  By convention an executor takes
// the current program counter pc, the program's flat bytecode array codes, and
// the current frame's register window stack (i.e. dataStack sliced at the frame
// pointer, so stack[r] is register r).  It decodes its operands from codes at
// pc, performs the operation, and returns the program counter of the following
// bytecode (pc+n, where n is this bytecode's width, or a branch target).
// Executors which may fail additionally return an error.

func (p *Interpreter[W]) executeEnter_n(pc uint32, codes []uint32, stack []W) error {
	var (
		width, target, args, n = encoding.DecodeEnter_n(pc, codes)
		// determine callee frame pointer
		calleeFp = p.fp + uint32(len(stack))
	)
	// allocate callee frame
	p.dataStack.Alloc(uint(width))
	// save function pointer and return address
	p.callStack.Push(checkpoint.NewStackFrame(p.fid, p.fp, p.pc+n))
	// copy arguments into callee frame
	for i := uint(calleeFp); args.HasNext(); i++ {
		p.dataStack.Set(i, stack[args.Next()])
	}
	// FIXME: following to be deprecated
	p.fid = p.program.FunctionAt(target).Unwrap()
	p.fp = calleeFp
	p.pc = target
	//
	return nil
}

func (p *Interpreter[W]) executeEnterCheckPoint_n(pc uint32, codes []uint32, stack []W) error {
	// Enter checkpoint function
	err := p.executeEnter_n(pc, codes, stack)
	// Only fire the checkpointer once every counter period.
	if p.counter.Tick() {
		p.checkpointer(p.CheckPoint())
	}
	//
	return err
}

func (p *Interpreter[W]) executeLeave_n(pc uint32, codes []uint32, stack []W) uint32 {
	var (
		rets, n = encoding.DecodeLeave_n(pc, codes)
	)
	// copy returns from callee frame
	for i := uint(p.rp); rets.HasNext(); i++ {
		stack[rets.Next()] = p.dataStack.Get(i)
	}
	// drop callee frame
	p.dataStack.Free(uint(p.rw))
	//
	return pc + n
}

func (p *Interpreter[W]) executeReturn(pc uint32, codes []uint32) (uint32, error) {
	var (
		frame             = p.callStack.Pop()
		width, roffset, _ = encoding.DecodeRet1(pc, codes)
	)
	//
	p.fid = frame.FunctionId // FIXME: remove
	p.rp = p.fp + uint32(roffset)
	p.rw = uint32(width)
	p.fp = frame.FramePointer
	//
	return frame.ProgramCounter, nil
}

// executeAdd_nm implements ADD_nm: it sums the constant and all sources and
// stores the result across a vector target using the same low-limb-first rule
// as the word machine, reporting an error on overflow.
func (p *Interpreter[W]) executeAdd_nm(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, constant, n = encoding.DecodeArith_nm[W](pc, codes)
		val                           = constant
	)
	//
	for sources.HasNext() {
		var (
			overflow bool
			src      = sources.Next()
		)
		//
		val, overflow = val.Add(stack[src])
		//
		if overflow {
			return pc, errors.New("arithmetic overflow")
		}
	}
	//
	return pc + n, storeAcross(p.program.Module(p.fid), targets, val, stack)
}

// executeSub_nm implements SUB_nm: it seeds the value from the first source,
// subtracts the remaining sources and the constant, and stores the result
// across a vector target using the same low-limb-first rule as the word
// machine, reporting an error on underflow.
func (p *Interpreter[W]) executeSub_nm(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, constant, n = encoding.DecodeArith_nm[W](pc, codes)
		val                           W
		underflow                     bool
	)
	// Seed initial value
	val = stack[sources.Next()]
	// Subtract rest from it
	for sources.HasNext() {
		var src = sources.Next()
		//
		if val, underflow = val.Sub(stack[src]); underflow {
			return pc, errors.New("arithmetic underflow")
		}
	}
	//
	if val, underflow = val.Sub(constant); underflow {
		return pc, errors.New("arithmetic underflow")
	}
	//
	return pc + n, storeAcross(p.program.Module(p.fid), targets, val, stack)
}

// executeMul_nm implements MUL_nm: it multiplies the constant by all sources
// and stores the result across a vector target using the same low-limb-first
// rule as the word machine, reporting an error on overflow.
func (p *Interpreter[W]) executeMul_nm(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, constant, n = encoding.DecodeArith_nm[W](pc, codes)
		val                           = constant
		overflow                      bool
	)
	//
	for sources.HasNext() {
		var (
			hi     W
			source = uint16(sources.Next())
		)
		//
		hi, val = val.Mul(stack[source])
		overflow = overflow || hi.Cmp64(0) != 0
	}
	// A zero result is exact even when an intermediate product overflowed
	// (matches executeMul in the slow word machine).
	if overflow && val.Cmp64(0) != 0 {
		return pc, errors.New("arithmetic overflow")
	}
	//
	return pc + n, storeAcross(p.program.Module(p.fid), targets, val, stack)
}

// executeFieldAdd implements ADDMOD_P: it sums the constant and all sources
// modulo the field's prime characteristic, storing the (reduced) result in the
// single target register.  Matches executeFieldAdd in the slow word machine.
func (p *Interpreter[W]) executeFieldAdd(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands[W](pc, codes)
		val                      = constant
	)
	//
	for sources.HasNext() {
		val = val.AddMod(stack[sources.Next()], p.modulus)
	}
	//
	stack[rd] = val
	//
	return pc + n, nil
}

// executeFieldSub implements SUBMOD_P: it seeds the value from the first source,
// then subtracts the remaining sources and the constant modulo the field's
// prime characteristic, storing the (reduced) result in the single target
// register.  Matches executeFieldSub in the slow word machine.
func (p *Interpreter[W]) executeFieldSub(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands[W](pc, codes)
		val                      W
	)
	//
	for i := 0; sources.HasNext(); i++ {
		ith := stack[sources.Next()]
		//
		if i == 0 {
			val = ith
		} else {
			val = val.SubMod(ith, p.modulus)
		}
	}
	//
	stack[rd] = val.SubMod(constant, p.modulus)
	//
	return pc + n, nil
}

// executeFieldMul implements MULMOD_P: it multiplies the constant by all sources
// modulo the field's prime characteristic, storing the (reduced) result in the
// single target register.  Matches executeFieldMul in the slow word machine.
func (p *Interpreter[W]) executeFieldMul(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands[W](pc, codes)
		val                      = constant
	)
	//
	for sources.HasNext() {
		val = val.MulMod(stack[sources.Next()], p.modulus)
	}
	//
	stack[rd] = val
	//
	return pc + n, nil
}

// executeCat matches executeConcat in the slow word machine.
func (p *Interpreter[W]) executeCat(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, n = encoding.DecodeCatOperands(pc, codes)
		module              = p.program.Module(p.fid)
		val                 W
		width               uint
	)
	//
	for sources.HasNext() {
		var (
			reg = uint16(sources.Next())
		)
		//
		_, lo := stack[reg].Shl64(uint64(width))
		val = val.Or(lo)
		//
		width = width + bitwidthOf(module, reg)
	}
	//
	return pc + n, storeAcross(module, targets, val, stack)
}

// executeDebug implements DEBUG: it reproduces the reference word machine's
// debug/printf handling, writing the formatted message to stdout (matching
// machine.Base's opcode.DEBUG case).  The chunk-set index packed into the
// bytecode word selects this site's specification from the program's side-table.
func (p *Interpreter[W]) executeDebug(pc uint32, codes []uint32, frame []W) uint32 {
	var (
		index, sources, n = encoding.DecodeDebug(pc, codes)
	)
	//
	fmt.Print(p.formatChunks(p.program.Chunks(index), sources, frame))
	//
	return pc + n
}

// executeFail implements FAIL: it reproduces the reference word machine's
// executeFail.  A fail with no message chunks aborts with a bare "machine
// panic"; otherwise the formatted message (looked up in the program's
// side-table by the index packed into the bytecode word) is included.
func (p *Interpreter[W]) executeFail(pc uint32, codes []uint32, frame []W) error {
	var (
		index, sources, _ = encoding.DecodeDebug(pc, codes)
		chunks            = p.program.Chunks(index)
	)
	//
	if len(chunks) == 0 {
		return errors.New("machine panic")
	}
	//
	return fmt.Errorf("machine panic: %s", p.formatChunks(chunks, sources, frame))
}

// formatChunks renders a formatted-message chunk-set against the current frame,
// mirroring executeFormattedChunks in the reference word machine: each chunk's
// literal text is emitted verbatim and each formatted argument is rendered
// against the frame.
func (p *Interpreter[W]) formatChunks(chunks []bytecode.FormattedChunk, sources encoding.Op8Iter, frame []W) string {
	var (
		module  = p.program.Module(p.fid)
		builder strings.Builder
	)
	//
	for _, chunk := range chunks {
		builder.WriteString(chunk.Text)
		//
		if chunk.Format.HasFormat() {
			var (
				base = bytecode.RegisterId(sources.Next())
				len  = uint16(sources.Next())
				vec  = bytecode.RegisterVector{Base: base, Len: len}
			)
			//
			builder.WriteString(p.formatArgument(module, chunk.Format, vec, frame))
		}
	}
	//
	return builder.String()
}

// formatArgument packs a (low-limb-first) register vector into a single integer
// and renders it with the given format, mirroring formatWord in the reference
// word machine: limbs are accumulated most-significant first, shifting by each
// limb's bitwidth, and the shared Format.Render produces the final text.
func (p *Interpreter[W]) formatArgument(module descriptor.Module[W], format zkc_util.Format,
	vec bytecode.RegisterVector, frame []W) string {
	//
	var value big.Int
	// Loop from most-significant limb to least significant.
	for i := uint16(0); i < vec.Len; i++ {
		var reg = vec.Base + i
		// Shift accumulator by this limb's width, then add the limb.
		value.Lsh(&value, bitwidthOf(module, reg))
		value.Add(&value, frame[reg].BigInt())
	}
	//
	return format.Render(&value)
}

// executeAdd_2n1 implements ADD_2n1: stack[rd] = stack[rs0] + stack[rs1],
// returning an error if the addition overflows the word type.
func executeAdd_2n1[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs0, rs1, rd, n = encoding.DecodeArith_2n1(pc, codes)
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
		// Add v0 + v1
		res, overflow = val0.Add(val1)
	)
	// Check for overflow
	if overflow {
		return pc, errors.New("arithmetic overflow")
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeAdd_1n1c implements ADDC: stack[rd] = stack[rs] + constant.
func executeAdd_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c[W](pc, codes)
		val                 = stack[rs]
		res, overflow       = val.Add(constant)
	)
	//
	if overflow {
		return pc, errors.New("arithmetic overflow")
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeAnd implements AND: stack[rd] = stack[lhs] & stack[rhs].
func executeAnd[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, lhs, rhs, _, n = encoding.DecodeBitwise_2n1(pc, codes)
	//
	stack[rd] = stack[lhs].And(stack[rhs])
	//
	return pc + n, nil
}

// executeOr implements OR: stack[rd] = stack[lhs] | stack[rhs].
func executeOr[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, lhs, rhs, _, n = encoding.DecodeBitwise_2n1(pc, codes)
	//
	stack[rd] = stack[lhs].Or(stack[rhs])
	//
	return pc + n, nil
}

// executeXor implements XOR: stack[rd] = stack[lhs] ^ stack[rhs].
func executeXor[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, lhs, rhs, _, n = encoding.DecodeBitwise_2n1(pc, codes)
	//
	stack[rd] = stack[lhs].Xor(stack[rhs])
	//
	return pc + n, nil
}

// executeCheckCast implements CHECKCAST: it checks that the value in register
// rd fits within the given bit-width, returning an error if it does not.  The
// register itself is left unchanged.
func executeCheckCast[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, bitwidth, n = encoding.DecodeCheckCast(pc, codes)
		value           = stack[rd]
	)
	// perform check
	if !value.FitsWithin(uint(bitwidth)) {
		return pc, fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
	}
	//
	return pc + n, nil
}

// executeDiv implements DIV: stack[rd] = stack[dividend] / stack[divisor],
// returning an error if the divisor is zero.
func executeDiv[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, dividend, divisor, n = encoding.DecodeDivRem_2n1(pc, codes)
	//
	if stack[divisor].Cmp64(0) == 0 {
		return pc, errors.New("division by zero")
	}
	//
	stack[rd] = stack[dividend].Div(stack[divisor])
	//
	return pc + n, nil
}

// executeRem implements REM: stack[rd] = stack[dividend] % stack[divisor],
// returning an error if the divisor is zero.
func executeRem[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, dividend, divisor, n = encoding.DecodeDivRem_2n1(pc, codes)
	//
	if stack[divisor].Cmp64(0) == 0 {
		return pc, errors.New("division by zero")
	}
	//
	stack[rd] = stack[dividend].Rem(stack[divisor])
	//
	return pc + n, nil
}

// executeHint implements HINT: it decodes the operation selector and dispatches
// to the corresponding hint.  Currently the only supported operation is
// DIV_HINT.
func (p *Interpreter[W]) executeHint(pc uint32, codes []uint32, stack []W) (uint32, error) {
	op, targets, sources, n := encoding.DecodeHintOperands(pc, codes)
	//
	switch op {
	case bytecode.DIV_HINT:
		return p.executeDivHint(pc, n, targets, sources, stack)
	default:
		return pc, fmt.Errorf("unknown hint operation (%d)", op)
	}
}

// executeDivHint implements the DIV_HINT hint: it reconstructs the dividend and
// divisor arguments from their (possibly multi-limb) register vectors, then
// assigns the quotient, remainder and range witness (divisor - remainder - 1)
// of the division across the corresponding target vectors, returning an error
// if the divisor is zero.  big.Int arithmetic is used so values spanning
// several limbs (i.e. wider than the machine word) are handled correctly.
func (p *Interpreter[W]) executeDivHint(pc, n uint32, targets, sources encoding.Op8Iter,
	stack []W) (uint32, error) {
	var (
		module   = p.program.Module(p.fid)
		dividend = loadHintOperand(module, &sources, stack)
		divisor  = loadHintOperand(module, &sources, stack)
	)
	//
	if divisor.Sign() == 0 {
		return pc, errors.New("division by zero")
	}
	//
	var (
		q = new(big.Int).Quo(dividend, divisor)
		r = new(big.Int).Rem(dividend, divisor)
		w = new(big.Int).Sub(divisor, r)
	)
	//
	w.Sub(w, big.NewInt(1))
	//
	if w.Sign() < 0 {
		return pc, errors.New("arithmetic underflow")
	}
	// Distribute quotient, remainder and witness across their target vectors.
	for _, val := range []*big.Int{q, r, w} {
		if err := storeHintResult(module, &targets, val, stack); err != nil {
			return pc, err
		}
	}
	//
	return pc + n, nil
}

// loadHintOperand reconstructs the value of a single hint operand from the next
// (base, len) register vector in the iterator, with the least-significant limb
// held in the lowest-indexed register (matching storeAcross).
func loadHintOperand[W word.Word[W]](module descriptor.Module[W], iter *encoding.Op8Iter, stack []W) *big.Int {
	var (
		base   = uint16(iter.Next())
		length = uint(iter.Next())
		value  = new(big.Int)
		offset uint
	)
	//
	for i := uint(0); i < length; i++ {
		var (
			reg  = base + uint16(i)
			limb = new(big.Int).Lsh(stack[reg].BigInt(), offset)
		)
		//
		value.Or(value, limb)
		//
		offset += bitwidthOf(module, reg)
	}
	//
	return value
}

// storeHintResult distributes value across the next (base, len) register vector
// in the iterator, writing the least-significant limb into the lowest-indexed
// register (matching storeAcross).  It errors if the value does not fit within
// the vector's total width.
func storeHintResult[W word.Word[W]](module descriptor.Module[W], iter *encoding.Op8Iter,
	value *big.Int, stack []W) error {
	var (
		base   = uint16(iter.Next())
		length = uint(iter.Next())
		acc    = new(big.Int).Set(value)
		total  uint
	)
	//
	for i := uint(0); i < length; i++ {
		var (
			reg   = base + uint16(i)
			width = bitwidthOf(module, reg)
			mask  = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width), big.NewInt(1))
			limb  W
		)
		//
		stack[reg] = limb.SetBigInt(new(big.Int).And(acc, mask))
		acc.Rsh(acc, width)
		total += width
	}
	//
	if acc.Sign() != 0 {
		return fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), total)
	}
	//
	return nil
}

// executeSkipIf_rr implements the conditional register-register forward branch
// bytecodes (SEQ_rr, SNE_rr, SLT_rr, SGT_rr, SLE_rr, SGE_rr).  The comparison
// is selected via the Comparator type parameter F.  If stack[rs0] compares to
// stack[rs1] as required, execution skips forward to the encoded target;
// otherwise it falls through to the following bytecode.
func executeSkipIf_rr[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		c F
		//
		skip, rs0, rs1, _, n = encoding.DecodeSkipIf_rr(pc, codes)
		// Calculate skip target
		target = pc + 1 + skip
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
	)
	//
	if c.Cmp(val0, val1) {
		// true branch
		return target
	}
	// false branch
	return pc + n
}

// executeSkipIf_rv implements the conditional register-register branch bytecodes
// (JEQ_rv, JNE_rv, JLT_rv, JGT_rv, JLE_rv, JGE_rv).  The comparison is selected
// via the Comparator type parameter F.  If stack[rs0] compares to stack[rs1] as
// required, execution jumps to the encoded target; otherwise it falls through
// to the following bytecode.
func executeSkipIf_rv[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		cmp F
		//
		skip, rs0, rs1, _, n = encoding.DecodeSkipIf_rv(pc, codes)
		// Calculate skip target
		target = pc + 1 + skip
	)
	//
	for i := rs0.Len; i > 0; {
		i = i - 1
		// Read rs0
		val0 := stack[rs0.Base+i]
		// Read rs1
		val1 := stack[rs1.Base+i]
		//
		if i != 0 && val0.Cmp(val1) == 0 {
			continue
		} else if cmp.Cmp(val0, val1) {
			// true branch
			return target
		}
		// false branch
		return pc + n
	}
	//
	panic("unreachable")
}

// executeSkipTable implements the SMW dispatch: the source register is
// compared against each case value and, on the first match, control transfers
// to that case's (absolute) target; otherwise control falls through past the
// whole instruction to the following one.
func executeSkipTable[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		word0  = codes[pc]
		count  = (word0 >> 24) & 0xff
		source = (word0 >> 8) & 0xffff
		val    = stack[source]
	)
	//
	for i := range count {
		var (
			base  = pc + 1 + (i * 3)
			value = uint64(codes[base]) | (uint64(codes[base+1]) << 32)
		)
		//
		if val.Cmp64(value) == 0 {
			return codes[base+2]
		}
	}
	// no match: fall through past the whole instruction
	return pc + 1 + (3 * count)
}

// executeLdc_1 implements LDC: it loads a constant value into register rd.
func executeLdc_1[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	val, rd, n := encoding.DecodeLdc_1[W](pc, codes)
	//
	stack[rd] = val
	//
	return pc + n
}

// executeLdc_w implements LDC_w: it loads a wide constant value into register
// rd.
func executeLdc_w[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	val, rd, n := encoding.DecodeLdc_w[W](pc, codes)
	//
	stack[rd] = val
	//
	return pc + n
}

// executeMove_1s1 implements MOVE: it copies the value of register rs into
// register rd.
func executeMove_1s1[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		rs, rd, n = encoding.DecodeMove_1s1(pc, codes)
		// Read rs
		val = stack[rs]
	)
	// Write rd
	stack[rd] = val
	//
	return pc + n
}

// executeMul_2n1 implements MUL_2n1: stack[rd] = stack[rs0] * stack[rs1],
// returning an error if the multiplication overflows the word type.
func executeMul_2n1[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs0, rs1, rd, n = encoding.DecodeArith_2n1(pc, codes)
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
		// Add v0 * v1
		hi, lo = val0.Mul(val1)
	)
	// Check for overflow
	if hi.Cmp64(0) != 0 {
		return pc, errors.New("arithmetic overflow")
	}
	//
	stack[rd] = lo
	//
	return pc + n, nil
}

// executeMul_1n1c implements MULC: stack[rd] = stack[rs] * constant.
func executeMul_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c[W](pc, codes)
		val                 = stack[rs]
		hi, lo              = val.Mul(constant)
	)
	//
	if hi.Cmp64(0) != 0 {
		return pc, errors.New("arithmetic overflow")
	}
	//
	stack[rd] = lo
	//
	return pc + n, nil
}

// executeNot computes a bitwise complement within the encoded mask width.
func executeNot[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, rs, _, bitwidth, n = encoding.DecodeBitwise_2n1(pc, codes)
		val                    = stack[rs].Not(uint(bitwidth))
	)
	//
	stack[rd] = val
	//
	return pc + n, nil
}

// executeReadSrom_sn implements RD_SROM_nm: it reads ndata consecutive words
// from the static read-only memory identified by id, starting at the address
// decoded from the operand registers, into successive destination registers.
func executeReadSrom_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	sroms []memory.StaticReadOnly[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		srom              = &sroms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, srom.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		stack[data.Next()], _ = srom.Read(address)
		//
		address++
	}
	//
	return pc + n
}

// executeReadRom_sn implements RD_ROM_nm: it reads ndata consecutive words from
// the read-only memory identified by id, starting at the address decoded from
// the operand registers, into successive destination registers.
func executeReadRom_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	roms []memory.ReadOnly[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		rom               = &roms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, rom.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		stack[data.Next()], _ = rom.Read(address)
		//
		address++
	}
	//
	return pc + n
}

// executeShl implements SHL: it shifts a value left by the amount held in a
// register, masking the result to the encoded width.
func executeShl[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, rs, amt, bitwidth, n = encoding.DecodeBitwise_2n1(pc, codes)
	//
	stack[rd] = stack[rs].Shl(uint(bitwidth), stack[amt])
	//
	return pc + n, nil
}

// executeShr implements SHR: it shifts a value right by the amount held in a
// register.
func executeShr[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rd, rs, amt, _, n = encoding.DecodeBitwise_2n1(pc, codes)
	//
	stack[rd] = stack[rs].Shr(stack[amt])
	//
	return pc + n, nil
}

// executeSub_1n1c implements SUBC: stack[rd] = stack[rs] - constant.
func executeSub_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c[W](pc, codes)
		val                 = stack[rs]
		res, underflow      = val.Sub(constant)
	)
	//
	if underflow {
		return pc, errors.New("arithmetic underflow")
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeSub_2n1 implements SUB_2n1: stack[rd] = stack[rs0] - stack[rs1],
// returning an error if the subtraction underflows the word type.
func executeSub_2n1[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs0, rs1, rd, n = encoding.DecodeArith_2n1(pc, codes)
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
		// Subtrace v0 - v1
		res, underflow = val0.Sub(val1)
	)
	// Check for overflow
	if underflow {
		return pc, errors.New("arithmetic underflow")
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeWriteWom_sn implements WR_WOM_nm: it writes ndata consecutive words
// from successive source registers into the write-once memory identified by id,
// starting at the address decoded from the operand registers.
func executeWriteWom_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	woms []memory.WriteOnce[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		wom               = &woms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, wom.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		wom.Write(address, stack[data.Next()])
		//
		address++
	}
	//
	return pc + n
}

// executeReadRam_sn implements RD_RAM_nm: it reads ndata consecutive words from
// the random-access memory identified by id, starting at the address decoded
// from the operand registers, into successive destination registers.
func executeReadRam_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	rams []memory.RandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		ram               = &rams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, ram.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		stack[data.Next()], _ = ram.Read(address)
		//
		address++
	}
	//
	return pc + n
}

// executeWriteRam_sn implements WR_RAM_nm: it writes ndata consecutive words
// from successive source registers into the random-access memory identified by
// id, starting at the address decoded from the operand registers.
func executeWriteRam_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	rams []memory.RandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		ram               = &rams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, ram.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		ram.Write(address, stack[data.Next()])
		//
		address++
	}
	//
	return pc + n
}

// executeReadPagedRam_sn implements RD_PRAM_nm: it reads ndata consecutive words
// from the paged random-access memory identified by id, starting at the address
// decoded from the operand registers, into successive destination registers.
func executeReadPagedRam_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	prams []memory.PagedRandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		pram              = &prams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, pram.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		stack[data.Next()], _ = pram.Read(address)
		//
		address++
	}
	//
	return pc + n
}

// executeWritePram_sn implements WR_PRAM_nm: it writes ndata consecutive words
// from successive source registers into the paged random-access memory
// identified by id, starting at the address decoded from the operand registers.
func executeWritePagedRam_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	prams []memory.PagedRandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		pram              = &prams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, pram.Geometry(), stack)
	//
	for data.HasNext() {
		//nolint
		pram.Write(address, stack[data.Next()])
		//
		address++
	}
	//
	return pc + n
}

// ============================================================================
// Helpers
// ============================================================================

// decodeAddress computes a flat memory address from the operand registers,
// according to the given memory's geometry.  It consumes one register per
// address line, packing their values (most-significant first) into a single
// index, then scales that index by the number of data lines so the result
// addresses the first word of the selected memory row.  The advanced register
// iterator is returned so the caller can continue reading the data registers.
func decodeAddress[W word.Word[W]](regs encoding.Op8Iter, geometry memory.Geometry[W], stack []W) uint64 {
	var (
		index      uint64
		registers  = geometry.Registers()
		numInputs  = geometry.AddressLines()
		numOutputs = geometry.DataLines()
	)

	for i := range numInputs {
		var (
			bitwidth = uint64(registers[i].Width())
			val      = stack[regs.Next()]
		)
		//
		index = (index << bitwidth) | val.Uint64()
	}

	return index * uint64(numOutputs)
}

func bitwidthOf[W word.Word[W]](module descriptor.Module[W], reg RegisterId) uint {
	var r = module.Register(reg)
	//
	return r.Bitwidth().UnwrapOr(math.MaxUint)
}

func storeAcross[W word.Word[W]](module descriptor.Module[W], targets encoding.Op8Iter, value W, stack []W) error {
	var bitwidth uint
	//
	for targets.HasNext() {
		var (
			target = uint16(targets.Next())
			width  = bitwidthOf(module, target)
		)
		//
		// Low limbs are written first, matching machine.StoreAcross.
		stack[target] = value.Slice(width)
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
