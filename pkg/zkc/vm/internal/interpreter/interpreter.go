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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ExtractExecutingState extracts the currently executing state from the
// bytecode intepreter.  That is, specifically, the current: executing function,
// program counter and register values.
func ExtractExecutingState[W word.Word[W]](p *Interpreter[W]) (fid uint16, pc uint32, st []W) {
	lab := p.program.FunctionAt(p.pc)
	//
	return lab.ModuleId, uint32(lab.Point.Macro), p.dataStack.SliceEnd(uint(p.fp))
}

// Failure indicates a machine failure, such as executing a fail instruction.
// Thus, a machine failure indicates specifically that the machine executed an
// instruction which caused a recognised problem.  This includes executing a
// fail instruction, but also other kinds of failures such as arithmetic
// overflow, etc.  Machine failures are distinct from other forms of error which
// the interpreter can return, the latter are really just internal failures that
// should never arise.
type Failure struct {
	Message string
}

// Error implementation for error interface.
func (p *Failure) Error() string {
	return p.Message
}

// Failure constructs a suitable failure.
func failure(msg string, values ...any) error {
	return &Failure{fmt.Sprintf(msg, values...)}
}

// failure records the current machine state and then constructs a suitable
// machine failure.  Recording is done by triggering the breakpoint handler
// which, outside of tracing, is a no-op (see New).  Routing every recognised
// runtime failure through here ensures the row which triggered the failure is
// committed to the trace being generated, so the constraints can reject it
// (e.g. a u32 addition which overflows leaves a wrapped result in the frame
// that violates the corresponding arithmetic / range constraint).
func (p *Interpreter[W]) failure(msg string, values ...any) error {
	// Snapshot the current (failing) state.  A failing row is never a normal
	// return, so the opcode passed to the handler is irrelevant (0 ≠ RET).
	p.breakpoint(0)
	//
	return failure(msg, values...)
}

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
	// Configuration of the surrounding field, which determines (amongst other
	// things) the encoded width of native registers within a checkpoint (see
	// Pack).
	field word.Config
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
	// Scratch space (i.e. for tail calls)
	scratch heap.Heap[W]
	// Static read-only memories: read-only memories whose contents are fixed and
	// not provided as inputs.
	sroms []StaticReadOnly[W]
	// Read-only memories.  Non-static read-only memories form the program's
	// inputs (see Inputs).
	roms []ReadOnly[W]
	// Write-once memories.  These form the program's outputs (see Outputs).
	woms []WriteOnce[W]
	// (Small) random-access memories which may be freely read and written.
	rams []RandomAccess[W]
	// (Large) paged random-access memories which may be freely read and
	// written.
	prams []PagedRandomAccess[W]
	// Optional callback invoked whenever a breakpoint is reached, i.e. an
	// instruction flagged with the BREAKPOINT modifier bit (see BreakPoint) is
	// about to execute.  The callback typically snapshots the current machine
	// state itself (see CheckPoint) and is responsible for governing how
	// frequently it actually acts.  Configured via the BreakPointer builder
	// method; defaults to a panicking stub until one has been set.  Finally,
	// the breakpoint function may "interrupt" the interpreter's execution by
	// returning true.
	breakpoint func(uint32) (interrupt bool)
}

// StackFrame captures relevant information about all functions currently
// executing a CALL.  Such functions are "paused" whilst the active function is
// being executed.  The purpose of a stack frame is to record the Frame Pointer
// (FP) and Program Counter (PC) of the relevant function so that these can be
// restored when it becomes the active function.
type StackFrame struct {
	// module identifier of the executing function.
	FunctionId uint16
	// frame pointer of the executing function.
	FramePointer uint32
	// program counter identifies next bytecode to execute.
	ProgramCounter uint32
}

// NewStackFrame constructs a stack frame recording the function identifier,
// frame pointer and program counter of a paused function.
func NewStackFrame(fid uint16, fp uint32, pc uint32) StackFrame {
	return StackFrame{fid, fp, pc}
}

// New constructs a new bytecode interpreter for the given program.
// The program's memory modules are partitioned by access discipline (static
// read-only, read-only, write-once, random-access and paged random-access)
// so that read/write bytecodes can locate them directly by identifier during
// execution.  The modulus is the prime characteristic of the surrounding field,
// used when executing native field instructions.  The interpreter is created in
// an unbooted state; Boot must be called to select an entry point and supply
// inputs before calling Execute.
func New[W word.Word[W]](program descriptor.Program[W], tracing bool) *Interpreter[W] {
	var (
		prime    W
		sroms    []StaticReadOnly[W]
		roms     []ReadOnly[W]
		woms     []WriteOnce[W]
		rams     []RandomAccess[W]
		prams    []PagedRandomAccess[W]
		compiled = CompileProgram(program, tracing)
	)
	// sanity check prime fits within target word
	if prime.Bandwidth() < program.Field().BandWidth {
		panic("insufficient bandwidth for prime field")
	}
	// Construct prime field
	prime = prime.SetBigInt(program.Field().Modulus())
	// Initialise memories
	for _, m := range program.Modules() {
		//
		if m, ok := m.(*descriptor.Memory[W]); ok {
			switch {
			case m.IsStatic():
				var mem = NewStatic(*m, m.StaticContents()...)
				//
				sroms = append(sroms, *mem)
			case m.IsReadOnly():
				var mem = NewReadOnly(*m)
				//
				roms = append(roms, *mem)
			case m.IsWriteOnly():
				var mem = NewWriteOnce(*m)
				//
				woms = append(woms, *mem)
			case m.IsPaged():
				var mem = NewPagedRandomAccess(*m)
				//
				prams = append(prams, *mem)
			case m.IsReadWrite():
				var mem = NewRandomAccess(*m)
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
		modulus: prime,
		field:   program.Field(),
		pc:      0,
		fp:      0,
		rp:      0,
		rw:      0,
		sroms:   sroms,
		roms:    roms,
		woms:    woms,
		rams:    rams,
		prams:   prams,
		// Default breakpointer: a no-op.  A real handler is installed via
		// BreakPointer when tracing / debugging (see BootAndTrace, BootAndDebug).
		// Note that failures route through this handler too (see failure), so it
		// must be safe to invoke even when no explicit handler is configured.
		breakpoint: func(uint32) bool { return false },
	}
}

// ProgramCounter returns the current PC for the interpreter.  Observe that this
// indentifies the next encoded instruction to execture, and is unrelated to the
// (two dimensional) program counter of a bytecode function.
func (p *Interpreter[W]) ProgramCounter() uint32 {
	return p.pc
}

// WithAccessLog enables logging of memory accesses on each read-write memory so
// its reads and writes are captured during execution.  This is only intended
// to be used when tracing.
func (p *Interpreter[W]) WithAccessLog() *Interpreter[W] {
	for i := range p.rams {
		p.rams[i].SetLog(&TraceableMemoryLog[W]{})
	}
	//
	for i := range p.prams {
		p.prams[i].SetLog(&TraceableMemoryLog[W]{})
	}
	//
	return p
}

// BreakPointer configures the callback invoked whenever a breakpoint is
// reached, i.e. an instruction flagged with the BREAKPOINT modifier bit (see
// BreakPoint) is about to execute.  The callback typically snapshots the
// current machine state itself (see CheckPoint) and is responsible for
// governing how frequently it actually acts.  It returns the interpreter to
// allow method chaining.
func (p *Interpreter[W]) BreakPointer(breakpointer func(uint32) bool) *Interpreter[W] {
	//
	p.breakpoint = breakpointer
	//
	return p
}

// Boot implementation of Core interface.  This locates the named function,
// points the program counter at its entry address, allocates an activation
// record for it on the data stack, and initialises all memories (loading the
// provided inputs into the input memories and resetting the rest).
func (p *Interpreter[W]) Boot(fun string, input map[string][]W) (err error) {
	var (
		sym encoding.Symbol
		// lookup function identifier
		fid, ok = p.program.HasModule(fun)
	)
	//
	if !ok {
		return fmt.Errorf("unknown function \"%s\"", fun)
	}
	// find instruction to boot
	if sym, ok = p.program.AddressOf(fid); !ok {
		return fmt.Errorf("missing symbol for \"%s\"", fun)
	}
	//
	p.pc = sym.Offset
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
func (p *Interpreter[W]) Inputs() iter.Iterator[InputOutput[W]] {
	var inputs []InputOutput[W]
	//
	for i := range p.roms {
		if !p.roms[i].Descriptor().IsStatic() {
			inputs = append(inputs, &p.roms[i])
		}
	}
	//
	return iter.NewArrayIterator(inputs)
}

// Outputs implementation of Core interface.  The outputs are the write-once
// memories, whose contents are populated as the program executes.
func (p *Interpreter[W]) Outputs() iter.Iterator[InputOutput[W]] {
	var outputs = make([]InputOutput[W], len(p.woms))
	//
	for i := range p.woms {
		outputs[i] = &p.woms[i]
	}
	//
	return iter.NewArrayIterator(outputs)
}

// CheckPoint relevant state of the interpreter, such that execution can be
// resumed from this point.  Observe that any threaded stamp-in inputs (see
// ThreadTimestamps) are excluded from the packed arguments: they are not
// meaningful across machines of different word widths (e.g. fast mode has no
// such registers), and are instead reconstructed from the memory clocks upon
// restore (see Restore).
func (p *Interpreter[W]) CheckPoint() checkpoint.CheckPoint {
	var (
		fun = p.program.Module(p.fid).(*descriptor.Function[W])
		// Determine number of leading stamp-in inputs (to be excluded)
		k = p.numStampInputs(fun)
		// Determine start/end of remaining arguments
		start = uint(p.fp) + k
		end   = uint(p.fp) + fun.NumInputs()
		// Extract arguments
		frame = p.dataStack.Slice(start, end)
		// Pack into bytes
		args     []byte = Pack(p.field, fun.Inputs()[k:], frame)
		memories []checkpoint.Memory
	)
	// Pack memories
	for i, m := range p.program.Modules() {
		var mid = uint16(i)
		// Note that static memories (SROMs) are not checkpointed, since their
		// contents form part of the program itself.  Non-static ROMs (i.e.
		// inputs) are checkpointed, since a restored machine has no other way to
		// recover them.
		if mem, ok := m.(*descriptor.Memory[W]); ok && !mem.IsStatic() {
			var ith = p.Memory(mid)
			//
			memories = append(memories, ith.Checkpoint(mid, p.field))
		}
	}
	//
	return checkpoint.NewCheckPoint(p.fid, args, memories)
}

// Restore resets this interpreter to the state captured by the given checkpoint
// (see CheckPoint), such that execution can resume from that point.  The call
// stack, data stack and all captured memories are overwritten; the currently
// executing frame (the top of the checkpoint's call stack) is unpacked into the
// active fid/fp/pc.  The checkpoint's slices are copied into the interpreter, so
// the checkpoint remains valid for subsequent restores.
func (p *Interpreter[W]) Restore(cp checkpoint.CheckPoint) {
	var (
		memories = cp.Memories()
		sym, ok  = p.Binary().AddressOf(cp.Function())
		fun      = p.program.Module(cp.Function()).(*descriptor.Function[W])
		// Determine number of leading stamp-in inputs (which were excluded from
		// the packed arguments --- see CheckPoint).
		k = p.numStampInputs(fun)
	)
	//
	util.Assert(ok, "cannot restore checkpoint")
	//
	p.fid = cp.Function()
	p.fp = 0
	p.pc = sym.Offset
	// Reset return pointer/width
	p.rp = 0
	p.rw = 0
	// Restore stack frame
	p.callStack.Clear()
	p.dataStack.Clear()
	p.dataStack.Alloc(fun.Width())
	// Restore arguments into the frame's input slots (which Alloc has already
	// reserved, mirroring the ENTER_n call convention).
	for i, val := range Unpack(p.field, fun.Inputs()[k:], cp.ArgumentBytes()) {
		p.dataStack.Set(k+uint(i), val)
	}
	// Restore memories
	for i, m := range p.program.Modules() {
		var mid = uint16(i)
		// Note that static memories (SROMs) are not checkpointed (see CheckPoint)
		if mem, ok := m.(*descriptor.Memory[W]); ok && !mem.IsStatic() {
			var ith = p.Memory(mid)
			// Restore ith memory from checkpoint
			ith.Restore(memories[0], p.field)
			// Pop checkpoint
			memories = memories[1:]
		}
	}
	// Seed the threaded stamp-in inputs (if any) from the restored memory
	// clocks.
	p.seedStampInputs(fun, cp.Memories())
}

// numStampInputs returns the number of leading input registers of the given
// function which are threaded stamp-in registers (or limbs thereof).  The
// ThreadTimestamps transform places one stamp-in register per read-write
// memory effect at the front of a function's inputs (each possibly split into
// several limbs); programs which have not been threaded (e.g. for fast mode
// execution) have none.
func (p *Interpreter[W]) numStampInputs(fun *descriptor.Function[W]) uint {
	var (
		inputs = fun.Inputs()
		n      uint
	)
	//
	for _, e := range fun.Effects() {
		var name = p.program.Module(e).Name() + "$stamp"
		//
		for n < uint(len(inputs)) && isLimbOf(inputs[n].Name(), name) {
			n++
		}
	}
	//
	return n
}

// seedStampInputs initialises the threaded stamp-in inputs (if any) of the
// given function from the given memory clocks.  Since the runtime clock ticks
// before each access and the first access of the restored shard carries
// exactly its stamp-in value (see ThreadTimestamps), each stamp-in is seeded
// with clock+1.  This reconstructs the value the caller would have forwarded,
// which cannot be transferred via the packed arguments (the checkpoint may
// originate from a machine without threaded stamps --- see CheckPoint).
func (p *Interpreter[W]) seedStampInputs(fun *descriptor.Function[W], memories []checkpoint.Memory) {
	var (
		inputs = fun.Inputs()
		clocks = make(map[uint16]uint64, len(memories))
		n      uint
	)
	//
	for _, m := range memories {
		clocks[m.ModuleId()] = m.Clock()
	}
	//
	for _, e := range fun.Effects() {
		var (
			name  = p.program.Module(e).Name() + "$stamp"
			stamp = clocks[e] + 1
			start = n
		)
		// Determine the limb group of this stamp register.
		for n < uint(len(inputs)) && isLimbOf(inputs[n].Name(), name) {
			n++
		}
		// Fan the stamp out across the limbs, which are ordered most
		// significant first.
		for i := n; i > start; i-- {
			var width = inputs[i-1].Bitwidth().Unwrap()
			//
			if width >= 64 {
				p.dataStack.Set(i-1, word.Const64[W](stamp))
				stamp = 0
			} else {
				p.dataStack.Set(i-1, word.Const64[W](stamp&((1<<width)-1)))
				stamp >>= width
			}
		}
	}
}

// isLimbOf checks whether a register with the given name is the register base
// itself, or one of its limbs (named "base'k" after register splitting).
func isLimbOf(name, base string) bool {
	return name == base || strings.HasPrefix(name, base+"'")
}

// Binary provides access to the compiled program binary being executed.
func (p *Interpreter[W]) Binary() encoding.Binary[W] {
	return p.program
}

// ExtractMemory returns a mapping of the runtime memories
func (p *Interpreter[W]) ExtractMemory() (mems []util.Pair[uint16, Memory[W]]) {
	for i, m := range p.program.Modules() {
		if _, ok := m.(*descriptor.Memory[W]); ok {
			var mem = p.Memory(uint16(i))
			//
			mems = append(mems, util.NewPair(uint16(i), mem))
		}
	}
	//
	return mems
}

// Memory provides access to the underlying memory corresponding to a given
// module identifier.
func (p *Interpreter[W]) Memory(mid uint16) Memory[W] {
	var (
		sym, ok = p.program.AddressOf(mid)
	)
	// Sanity check
	if ok {
		//
		switch sym.Kind {
		case encoding.STATIC_MEMORY:
			return &p.sroms[sym.Offset]
		case encoding.READONLY_MEMORY:
			return &p.roms[sym.Offset]
		case encoding.WRITEONCE_MEMORY:
			return &p.woms[sym.Offset]
		case encoding.READWRITE_MEMORY:
			return &p.rams[sym.Offset]
		case encoding.PAGED_READWRITE_MEMORY:
			return &p.prams[sym.Offset]
		}
	}
	//
	panic("internal failure")
}

// Execute implementation of Core interface.  This runs the central fetch-decode-
// dispatch loop: each iteration reads the bytecode at the current program
// counter, extracts its opcode, and dispatches to the corresponding executor
// which performs the operation and returns the next program counter.  The loop
// runs for at most steps iterations, stopping early if the program returns from
// its outermost frame (RET with an empty call stack) or an error occurs (e.g.
// arithmetic overflow, or an explicit FAIL).  It returns the number of steps
// actually executed together with any error.
func (p *Interpreter[W]) Execute(steps uint64) (uint64, error) {
	var (
		nsteps    = uint64(0)
		err       error
		frame     []W = p.dataStack.SliceEnd(uint(p.fp))
		bytecodes     = p.program.Bytecodes()
		pool          = p.program.Constants()
	)
	//
	for nsteps < steps && err == nil {
		// decode instruction
		var (
			opcode     = bytecodes[p.pc] & encoding.OPCODE_MASK
			breakpoint = bytecodes[p.pc]&encoding.BREAKPOINT != 0
		)
		// Check for breakpoint.  NOTE: for wide instructions, the wide opcode
		// (second byte) is passed through alongside the WIDE escape, allowing
		// observers to identify the instruction (e.g. WIDE|WIDE_RET<<8).
		if breakpoint {
			var cbop = opcode
			//
			if opcode == encoding.WIDE {
				cbop |= bytecodes[p.pc] & 0xff00
			}
			// Apply breakpoint and interrupt if requested
			if p.breakpoint(cbop) {
				// fire interrupt
				break
			}
		}
		// increase step counter
		nsteps++
		//
		switch opcode & encoding.OPCODE_MASK {
		case encoding.FAIL:
			return nsteps, p.executeFail(p.pc, bytecodes, frame)
		case encoding.WIDE:
			var halt bool
			//
			p.pc, halt, err = p.executeWide(p.pc, bytecodes, pool, frame)
			//
			if halt {
				return nsteps, err
			}
			// refresh the register window, since wide instructions include
			// those (ENTER/LEAVE/RET) which change the enclosing frame.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.CHECKCAST:
			p.pc, err = p.executeCheckCast(p.pc, bytecodes, frame)
		case encoding.DEBUG:
			p.pc = p.executeDebug(p.pc, bytecodes, frame)
		case encoding.LDC:
			p.pc = executeLdc_1(p.pc, bytecodes, frame)
		case encoding.LDC_w:
			p.pc = executeLdc_w(p.pc, bytecodes, pool, frame)
		case encoding.MOVE:
			p.pc = executeMove_1s1(p.pc, bytecodes, frame)
		case encoding.ENTER_n:
			err = p.executeEnter_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.ENTER_2:
			err = p.executeEnter_2(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.TAILCALL_n:
			err = p.executeTailCall_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.TAILCALL_2:
			err = p.executeTailCall_2(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.LEAVE_n:
			p.pc = p.executeLeave_n(p.pc, bytecodes, frame)
			// refresh the register window.
			frame = p.dataStack.SliceEnd(uint(p.fp))
		case encoding.LEAVE_2:
			p.pc = p.executeLeave_2(p.pc, bytecodes, frame)
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
		case encoding.DONE:
			// Termination
			return nsteps, nil
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
		case encoding.SGE_rr:
			p.pc = executeSkipIf_rr[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, frame)
		case encoding.SKIP_M:
			p.pc = executeSkipTable(p.pc, bytecodes, pool, frame)
		case encoding.SKIP_B:
			p.pc = executeDispatch(p.pc, bytecodes, frame)
		case encoding.SEQ_rv:
			p.pc = executeSkipIf_rv[W, util.Equal[W]](p.pc, bytecodes, frame)
		case encoding.SNE_rv:
			p.pc = executeSkipIf_rv[W, util.NotEqual[W]](p.pc, bytecodes, frame)
		case encoding.SLT_rv:
			p.pc = executeSkipIf_rv[W, util.LessThan[W]](p.pc, bytecodes, frame)
		case encoding.SGE_rv:
			p.pc = executeSkipIf_rv[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, frame)
		case encoding.SEQ_rc:
			p.pc = executeSkipIf_rc[W, util.Equal[W]](p.pc, bytecodes, pool, frame)
		case encoding.SNE_rc:
			p.pc = executeSkipIf_rc[W, util.NotEqual[W]](p.pc, bytecodes, pool, frame)
		case encoding.SLT_rc:
			p.pc = executeSkipIf_rc[W, util.LessThan[W]](p.pc, bytecodes, pool, frame)
		case encoding.SGE_rc:
			p.pc = executeSkipIf_rc[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, pool, frame)
		case encoding.SEQ_rcv:
			p.pc = executeSkipIf_rcv[W, util.Equal[W]](p.pc, bytecodes, pool, frame)
		case encoding.SNE_rcv:
			p.pc = executeSkipIf_rcv[W, util.NotEqual[W]](p.pc, bytecodes, pool, frame)
		case encoding.SLT_rcv:
			p.pc = executeSkipIf_rcv[W, util.LessThan[W]](p.pc, bytecodes, pool, frame)
		case encoding.SGE_rcv:
			p.pc = executeSkipIf_rcv[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, pool, frame)
			// Input / Output Operations
		case encoding.RD_ROM_nm:
			p.pc = executeReadRom_sn(p.pc, bytecodes, frame, p.roms)
		case encoding.RD_SROM_nm:
			p.pc = executeReadSrom_sn(p.pc, bytecodes, frame, p.sroms)
		case encoding.WR_WOM_nm:
			p.pc, err = p.executeWriteWom_sn(p.pc, bytecodes, frame, p.woms)
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
			p.pc, err = p.executeAdd_2n1(p.pc, bytecodes, frame)
		case encoding.ADDC:
			p.pc, err = p.executeAdd_1n1c(p.pc, bytecodes, pool, frame)
		case encoding.SUB_2n1:
			p.pc, err = p.executeSub_2n1(p.pc, bytecodes, frame)
		case encoding.SUBC:
			p.pc, err = p.executeSub_1n1c(p.pc, bytecodes, pool, frame)
		case encoding.MUL_2n1:
			p.pc, err = p.executeMul_2n1(p.pc, bytecodes, frame)
		case encoding.MULC:
			p.pc, err = p.executeMul_1n1c(p.pc, bytecodes, pool, frame)
		case encoding.ADD_nm:
			p.pc, err = p.executeAdd_nm(p.pc, bytecodes, pool, frame)
		case encoding.SUB_nm:
			p.pc, err = p.executeSub_nm(p.pc, bytecodes, pool, frame)
		case encoding.MUL_nm:
			p.pc, err = p.executeMul_nm(p.pc, bytecodes, pool, frame)
		case encoding.DIVMOD:
			p.pc, err = p.executeDivMod(p.pc, bytecodes, frame)
		case encoding.DIVMODC:
			p.pc, err = p.executeDivModConst(p.pc, bytecodes, pool, frame)
		case encoding.INTRINSIC:
			p.pc, err = p.executeIntrinsic(p.pc, bytecodes, pool, frame)
		case encoding.ADDMOD_P:
			p.pc, err = p.executeFieldAdd(p.pc, bytecodes, pool, frame)
		case encoding.SUBMOD_P:
			p.pc, err = p.executeFieldSub(p.pc, bytecodes, pool, frame)
		case encoding.MULMOD_P:
			p.pc, err = p.executeFieldMul(p.pc, bytecodes, pool, frame)
		case encoding.CAT:
			p.pc, err = p.executeCat(p.pc, bytecodes, frame)
		case encoding.CAT_2n1:
			p.pc, err = p.executeCat_2n1(p.pc, bytecodes, frame)
		case encoding.CAT_1n:
			p.pc, err = p.executeCat_1n(p.pc, bytecodes, frame)
		case encoding.CAT_n1:
			p.pc, err = p.executeCat_n1(p.pc, bytecodes, frame)
		case encoding.UINT_TO_FIELD:
			p.pc, err = p.executeUintToField(p.pc, bytecodes, frame)
		case encoding.FIELD_TO_UINT:
			p.pc, err = p.executeFieldToUint(p.pc, bytecodes, frame)
		case encoding.NOT:
			p.pc, err = executeNot(p.pc, bytecodes, frame)
		case encoding.AND:
			p.pc, err = executeAnd(p.pc, bytecodes, frame)
		case encoding.ANDC:
			p.pc, err = executeAnd_1n1c(p.pc, bytecodes, pool, frame)
		case encoding.OR:
			p.pc, err = executeOr(p.pc, bytecodes, frame)
		case encoding.ORC:
			p.pc, err = executeOr_1n1c(p.pc, bytecodes, pool, frame)
		case encoding.XOR:
			p.pc, err = executeXor(p.pc, bytecodes, frame)
		case encoding.XORC:
			p.pc, err = executeXor_1n1c(p.pc, bytecodes, pool, frame)
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

// executeWide dispatches an instruction encoded via the WIDE escape opcode,
// switching on the wide opcode held in the second byte of its first word.  It
// returns the next program counter, together with a halt flag signalling that
// execution has terminated (i.e. an explicit failure, or a return from the
// outermost frame) and any error.
//
//nolint:funlen
func (p *Interpreter[W]) executeWide(pc uint32, codes []uint32, pool []W, stack []W) (uint32, bool, error) {
	var (
		wopcode = (codes[pc] >> 8) & 0xff
		err     error
	)
	//
	switch wopcode {
	case encoding.WIDE_FAIL:
		return pc, true, p.executeFail(pc, codes, stack)
	case encoding.WIDE_CHECKCAST:
		pc, err = p.executeCheckCast(pc, codes, stack)
	case encoding.WIDE_DEBUG:
		pc = p.executeDebug(pc, codes, stack)
	case encoding.WIDE_LDC_w:
		pc = executeLdc_w(pc, codes, pool, stack)
	case encoding.WIDE_MOVE:
		pc = executeMove_1s1(pc, codes, stack)
	case encoding.WIDE_ENTER_n:
		err = p.executeEnter_n(pc, codes, stack)
		pc = p.pc
	case encoding.WIDE_TAILCALL_n:
		err = p.executeTailCall_n(pc, codes, stack)
		pc = p.pc
	case encoding.WIDE_LEAVE_n:
		pc = p.executeLeave_n(pc, codes, stack)
	case encoding.WIDE_RET:
		// check for termination
		if p.callStack.Size() == 0 {
			return pc, true, nil
		}
		// normal return
		pc, err = p.executeReturn(pc, codes)
	case encoding.WIDE_SEQ_rr:
		pc = executeSkipIf_rr[W, util.Equal[W]](pc, codes, stack)
	case encoding.WIDE_SNE_rr:
		pc = executeSkipIf_rr[W, util.NotEqual[W]](pc, codes, stack)
	case encoding.WIDE_SLT_rr:
		pc = executeSkipIf_rr[W, util.LessThan[W]](pc, codes, stack)
	case encoding.WIDE_SGE_rr:
		pc = executeSkipIf_rr[W, util.GreaterThanOrEqual[W]](pc, codes, stack)
	case encoding.WIDE_SEQ_rv:
		pc = executeSkipIf_rv[W, util.Equal[W]](pc, codes, stack)
	case encoding.WIDE_SNE_rv:
		pc = executeSkipIf_rv[W, util.NotEqual[W]](pc, codes, stack)
	case encoding.WIDE_SLT_rv:
		pc = executeSkipIf_rv[W, util.LessThan[W]](pc, codes, stack)
	case encoding.WIDE_SGE_rv:
		pc = executeSkipIf_rv[W, util.GreaterThanOrEqual[W]](pc, codes, stack)
	case encoding.WIDE_SEQ_rcv:
		pc = executeSkipIf_rcv[W, util.Equal[W]](pc, codes, pool, stack)
	case encoding.WIDE_SNE_rcv:
		pc = executeSkipIf_rcv[W, util.NotEqual[W]](pc, codes, pool, stack)
	case encoding.WIDE_SLT_rcv:
		pc = executeSkipIf_rcv[W, util.LessThan[W]](pc, codes, pool, stack)
	case encoding.WIDE_SGE_rcv:
		pc = executeSkipIf_rcv[W, util.GreaterThanOrEqual[W]](pc, codes, pool, stack)
	case encoding.WIDE_RD_ROM_nm:
		pc = executeReadRom_sn(pc, codes, stack, p.roms)
	case encoding.WIDE_RD_SROM_nm:
		pc = executeReadSrom_sn(pc, codes, stack, p.sroms)
	case encoding.WIDE_WR_WOM_nm:
		pc, err = p.executeWriteWom_sn(pc, codes, stack, p.woms)
	case encoding.WIDE_RD_RAM_nm:
		pc = executeReadRam_sn(pc, codes, stack, p.rams)
	case encoding.WIDE_WR_RAM_nm:
		pc = executeWriteRam_sn(pc, codes, stack, p.rams)
	case encoding.WIDE_RD_PRAM_nm:
		pc = executeReadPagedRam_sn(pc, codes, stack, p.prams)
	case encoding.WIDE_WR_PRAM_nm:
		pc = executeWritePagedRam_sn(pc, codes, stack, p.prams)
	case encoding.WIDE_ADD_2n1:
		pc, err = p.executeAdd_2n1(pc, codes, stack)
	case encoding.WIDE_SUB_2n1:
		pc, err = p.executeSub_2n1(pc, codes, stack)
	case encoding.WIDE_MUL_2n1:
		pc, err = p.executeMul_2n1(pc, codes, stack)
	case encoding.WIDE_ADDC:
		pc, err = p.executeAdd_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_SUBC:
		pc, err = p.executeSub_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_MULC:
		pc, err = p.executeMul_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_ADD_nm:
		pc, err = p.executeAdd_nm(pc, codes, pool, stack)
	case encoding.WIDE_SUB_nm:
		pc, err = p.executeSub_nm(pc, codes, pool, stack)
	case encoding.WIDE_MUL_nm:
		pc, err = p.executeMul_nm(pc, codes, pool, stack)
	case encoding.WIDE_DIVMOD:
		pc, err = p.executeDivMod(pc, codes, stack)
	case encoding.WIDE_DIVMODC:
		pc, err = p.executeDivModConst(pc, codes, pool, stack)
	case encoding.WIDE_INTRINSIC:
		pc, err = p.executeIntrinsic(pc, codes, pool, stack)
	case encoding.WIDE_ADDMOD_P:
		pc, err = p.executeFieldAdd(pc, codes, pool, stack)
	case encoding.WIDE_SUBMOD_P:
		pc, err = p.executeFieldSub(pc, codes, pool, stack)
	case encoding.WIDE_MULMOD_P:
		pc, err = p.executeFieldMul(pc, codes, pool, stack)
	case encoding.WIDE_AND:
		pc, err = executeAnd(pc, codes, stack)
	case encoding.WIDE_ANDC:
		pc, err = executeAnd_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_OR:
		pc, err = executeOr(pc, codes, stack)
	case encoding.WIDE_ORC:
		pc, err = executeOr_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_XOR:
		pc, err = executeXor(pc, codes, stack)
	case encoding.WIDE_XORC:
		pc, err = executeXor_1n1c(pc, codes, pool, stack)
	case encoding.WIDE_NOT:
		pc, err = executeNot(pc, codes, stack)
	case encoding.WIDE_SHL:
		pc, err = executeShl(pc, codes, stack)
	case encoding.WIDE_SHR:
		pc, err = executeShr(pc, codes, stack)
	case encoding.WIDE_CAT:
		pc, err = p.executeCat(pc, codes, stack)
	case encoding.WIDE_UINT_TO_FIELD:
		pc, err = p.executeUintToField(pc, codes, stack)
	case encoding.WIDE_FIELD_TO_UINT:
		pc, err = p.executeFieldToUint(pc, codes, stack)
	case encoding.WIDE_SKIP_M:
		pc = executeWideSkipTable(pc, codes, pool, stack)
	default:
		err = fmt.Errorf("unknown wide bytecode encountered (0x%x)", wopcode)
	}
	//
	return pc, false, err
}

// initialise prepares all memories for a fresh execution.  Input (read-only and
// static read-only) memories are loaded with the values supplied for their name
// in the input map, whilst output and scratch memories (write-once, random-
// access and paged random-access) are reset to empty.
func (p *Interpreter[W]) initialise(input map[string][]W) {
	// initialise roms
	for i, m := range p.roms {
		p.roms[i].Initialise(input[m.Descriptor().Name()])
	}
	// initialise static roms
	for i, m := range p.sroms {
		p.sroms[i].Initialise(input[m.Descriptor().Name()])
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
	p.callStack.Push(NewStackFrame(p.fid, p.fp, p.pc+n))
	// copy arguments into callee frame
	for i := uint(calleeFp); args.HasNext(); i++ {
		p.dataStack.Set(i, stack[args.Next()])
	}
	//
	p.fid = p.program.FunctionAt(target).ModuleId
	p.fp = calleeFp
	p.pc = target
	//
	return nil
}

// executeEnter_2 implements ENTER_2: the dedicated single-argument form of
// ENTER_n.  It binds the one argument register directly, without the general
// register-list (Operands) machinery.
func (p *Interpreter[W]) executeEnter_2(pc uint32, codes []uint32, stack []W) error {
	var (
		width, target, arg, n = encoding.DecodeEnter_2(pc, codes)
		// determine callee frame pointer
		calleeFp = p.fp + uint32(len(stack))
	)
	// allocate callee frame
	p.dataStack.Alloc(uint(width))
	// save function pointer and return address
	p.callStack.Push(NewStackFrame(p.fid, p.fp, p.pc+n))
	// copy the one argument into the callee frame
	p.dataStack.Set(uint(calleeFp), stack[arg])
	//
	p.fid = p.program.FunctionAt(target).ModuleId
	p.fp = calleeFp
	p.pc = target
	//
	return nil
}

// executeTailCall_n implements TAILCALL_n: a call to a no-return function,
// which reuses the caller's frame rather than allocating a new one (no
// call-stack record is pushed, since the callee never returns).  The arguments
// are staged through the scratch buffer, since freeing the caller frame
// invalidates them in place.
func (p *Interpreter[W]) executeTailCall_n(pc uint32, codes []uint32, stack []W) error {
	var (
		width, target, args, _ = encoding.DecodeEnter_n(pc, codes)
	)
	// move argument(s) into scratch buffer
	for args.HasNext() {
		p.scratch.Push(stack[args.Next()])
	}
	// resize caller frame, whilst ensuring all zeroed out.
	p.dataStack.Free(uint(len(stack)))
	p.dataStack.Alloc(uint(width))
	// assign from scratch into frame
	for i := uint(0); i < p.scratch.Size(); i++ {
		p.dataStack.Set(i+uint(p.fp), p.scratch.Get(i))
	}
	// clear scratch
	p.scratch.Clear()
	//
	p.fid = p.program.FunctionAt(target).ModuleId
	p.pc = target
	//
	return nil
}

// executeTailCall_2 implements TAILCALL_2: the dedicated single-argument form
// of executeTailCall_n.  The one argument is saved before the frame is freed,
// since freeing invalidates it in place.
func (p *Interpreter[W]) executeTailCall_2(pc uint32, codes []uint32, stack []W) error {
	var (
		width, target, arg, _ = encoding.DecodeEnter_2(pc, codes)
		//
		tmp = stack[arg]
	)
	// resize caller frame, whilst ensuring all zeroed out.
	p.dataStack.Free(uint(len(stack)))
	p.dataStack.Alloc(uint(width))
	// copy the one argument into the callee frame
	p.dataStack.Set(uint(p.fp), tmp)
	//
	p.fid = p.program.FunctionAt(target).ModuleId
	p.pc = target
	//
	return nil
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

// executeLeave_2 implements LEAVE_2: the dedicated single-return form of
// LEAVE_n.  It binds the one return register directly, without the general
// register-list (Operands) machinery.
func (p *Interpreter[W]) executeLeave_2(pc uint32, codes []uint32, stack []W) uint32 {
	var (
		ret, n = encoding.DecodeLeave_2(pc, codes)
	)
	// copy the one return from the callee frame
	stack[ret] = p.dataStack.Get(uint(p.rp))
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
	p.rp = p.fp + roffset
	p.rw = uint32(width)
	p.fp = frame.FramePointer
	//
	return frame.ProgramCounter, nil
}

// executeAdd_nm implements ADD_nm: it sums the constant and all sources and
// stores the result across a vector target using the same low-limb-first rule
// as the word machine, reporting an error on overflow.
func (p *Interpreter[W]) executeAdd_nm(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		targets, sources, constant, _, n = encoding.DecodeArith_nm(pc, codes, pool)
		val                              = constant
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
			return pc, p.failure("arithmetic overflow")
		}
	}
	//
	return pc + n, p.storeAcross(pc, p.program.Module(p.fid).Registers(), targets, val, stack)
}

// executeMul_nm implements MUL_nm: it multiplies the constant by all sources
// and stores the result across a vector target using the same low-limb-first
// rule as the word machine, reporting an error on overflow.
func (p *Interpreter[W]) executeMul_nm(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		targets, sources, constant, _, n = encoding.DecodeArith_nm(pc, codes, pool)
		val                              = constant
		overflow                         bool
	)
	//
	for sources.HasNext() {
		var (
			hi     W
			source = sources.Next()
		)
		//
		hi, val = val.Mul(stack[source])
		overflow = overflow || hi.Cmp64(0) != 0
	}
	// A zero result is exact even when an intermediate product overflowed
	// (matches executeMul in the slow word machine).
	if overflow && val.Cmp64(0) != 0 {
		return pc, p.failure("arithmetic overflow")
	}
	//
	return pc + n, p.storeAcross(pc, p.program.Module(p.fid).Registers(), targets, val, stack)
}

// executeFieldAdd implements ADDMOD_P: it sums the constant and all sources
// modulo the field's prime characteristic, storing the (reduced) result in the
// single target register.  Matches executeFieldAdd in the slow word machine.
func (p *Interpreter[W]) executeFieldAdd(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands(pc, codes, pool)
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
func (p *Interpreter[W]) executeFieldSub(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands(pc, codes, pool)
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
func (p *Interpreter[W]) executeFieldMul(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rd, sources, constant, n = encoding.DecodeFieldArithOperands(pc, codes, pool)
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
		targets, sources, n = encoding.DecodeRegisterLists(pc, codes)
		regs                = p.program.Module(p.fid).Registers()
	)
	//
	val := loadAcross(regs, sources, stack)

	return pc + n, p.storeAcross(pc, regs, targets, val, stack)
}

// executeCat_2n1 implements CAT_2n1: the dedicated two-target, one-source form
// of CAT.  It distributes the single source register's value across the two
// target registers, low limb (t0) first, exactly as the general form's
// storeAcross would -- but without the register-list (Operands) machinery,
// since both targets are fixed operands here.  Unlike storeAcross, no overflow
// check is performed: CAT_2n1 only ever arises from splitting an already
// width-exact concatenation (see split/concat.go's carry-line insertion, and
// InsertCheckCasts's explicit exclusion of concat), so the source is
// guaranteed to fit within the combined target width by construction.
func (p *Interpreter[W]) executeCat_2n1(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, t0, t1, n = encoding.DecodeCat_2n1(pc, codes)
		regs          = p.program.Module(p.fid).Registers()
		value         = stack[rs]
		w0            = bitwidthOf(regs, t0)
	)
	//
	stack[t0] = value.Slice(w0)
	stack[t1] = value.Shr64(uint64(w0)).Slice(bitwidthOf(regs, t1))
	//
	return pc + n, nil
}

// executeCat_1n implements CAT_1n: the dedicated one-source, N-target form of
// CAT.  It distributes the single source register's value across the target
// registers, low limb first, exactly as the general form's storeAcross would
// -- but without the register-list (Operands) machinery on the source side,
// since there is only ever one source here.  As for CAT_2n1, no overflow
// check is performed (see executeCat_2n1's comment for why).
func (p *Interpreter[W]) executeCat_1n(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, targets, n = encoding.DecodeCat_1n(pc, codes)
		regs           = p.program.Module(p.fid).Registers()
		value          = stack[rs]
	)
	//
	for targets.HasNext() {
		var (
			target = targets.Next()
			width  = bitwidthOf(regs, target)
		)
		//
		stack[target] = value.Slice(width)
		value = value.Shr64(uint64(width))
	}
	//
	return pc + n, nil
}

// executeCat_n1 implements CAT_n1: the dedicated N-source, one-target form of
// CAT.  It combines the source registers into a single value (low limb
// first, via loadAcross) and writes it directly to the one target register.
// Unlike the general form's storeAcross, the result is not re-sliced to the
// target's declared width: for the same reason CAT_2n1 needs no overflow
// check, the combined value is already guaranteed to fit, so a truncating
// slice would be a no-op here.
func (p *Interpreter[W]) executeCat_n1(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, sources, n = encoding.DecodeCat_n1(pc, codes)
		regs           = p.program.Module(p.fid).Registers()
	)
	//
	stack[rd] = loadAcross(regs, sources, stack)
	//
	return pc + n, nil
}

// executeUintToField assembles the uint sources and reduces the result modulo P
// into the native target.  The reduction never fails (a value ≥ P wraps).
func (p *Interpreter[W]) executeUintToField(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, n = encoding.DecodeRegisterLists(pc, codes)
		regs                = p.program.Module(p.fid).Registers()
		val                 = loadAcross(regs, sources, stack)
	)
	//
	stack[targets.Next()] = val.Rem(p.modulus)
	//
	return pc + n, nil
}

// executeFieldToUint distributes the native source (already canonical) across the
// uint targets, failing when the value does not fit their combined width.
func (p *Interpreter[W]) executeFieldToUint(pc uint32, codes []uint32, stack []W) (uint32, error) {
	targets, sources, n := encoding.DecodeRegisterLists(pc, codes)
	//
	return pc + n, p.storeAcross(pc, p.program.Module(p.fid).Registers(), targets, stack[sources.Next()], stack)
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
		return p.failure("machine panic")
	}
	//
	return p.failure("%s", p.formatChunks(chunks, sources, frame))
}

// formatChunks renders a formatted-message chunk-set against the current frame,
// mirroring executeFormattedChunks in the reference word machine: each chunk's
// literal text is emitted verbatim and each formatted argument is rendered
// against the frame.
func (p *Interpreter[W]) formatChunks(chunks []bytecode.FormattedChunk, sources encoding.Operands, frame []W) string {
	var (
		regs    = p.program.Module(p.fid).Registers()
		builder strings.Builder
	)
	//
	for _, chunk := range chunks {
		builder.WriteString(chunk.Text)
		//
		if chunk.Format.HasFormat() {
			var (
				base = sources.Next()
				len  = sources.Next()
				vec  = bytecode.RegisterVector{Base: base, Len: len}
			)
			//
			builder.WriteString(p.formatArgument(regs, chunk.Format, vec, frame))
		}
	}
	//
	return builder.String()
}

// formatArgument packs a (low-limb-first) register vector into a single integer
// and renders it with the given format, mirroring formatWord in the reference
// word machine: limbs are accumulated most-significant first, shifting by each
// limb's bitwidth, and the shared Format.Render produces the final text.
func (p *Interpreter[W]) formatArgument(regs []descriptor.Register[W], format zkc_util.Format,
	vec bytecode.RegisterVector, frame []W) string {
	//
	var value big.Int
	// Loop from most-significant limb to least significant.
	for i := uint16(0); i < vec.Len; i++ {
		var reg = vec.Base + i
		// Shift accumulator by this limb's width, then add the limb.
		value.Lsh(&value, bitwidthOf(regs, reg))
		value.Add(&value, frame[reg].BigInt())
	}
	//
	return format.Render(&value)
}

// executeAdd_2n1 implements ADD_2n1: stack[rd] = stack[rs0] + stack[rs1],
// returning an error if the addition overflows the word type.
func (p *Interpreter[W]) executeAdd_2n1(pc uint32, codes []uint32, stack []W) (uint32, error) {
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
		return pc, p.failure("arithmetic overflow")
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeAdd_1n1c implements ADDC: stack[rd] = stack[rs] + constant.
func (p *Interpreter[W]) executeAdd_1n1c(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c(pc, codes, pool)
		val                 = stack[rs]
		res, overflow       = val.Add(constant)
	)
	//
	if overflow {
		return pc, p.failure("arithmetic overflow")
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

// executeAnd_1n1c implements ANDC: stack[rd] = stack[lhs] & constant.
func executeAnd_1n1c[W word.Word[W]](pc uint32, codes []uint32, pool, stack []W) (uint32, error) {
	var rd, lhs, constant, _, n = encoding.DecodeBitwise_1n1c(pc, codes, pool)
	//
	stack[rd] = stack[lhs].And(constant)
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

// executeOr_1n1c implements ORC: stack[rd] = stack[lhs] | constant.
func executeOr_1n1c[W word.Word[W]](pc uint32, codes []uint32, pool, stack []W) (uint32, error) {
	var rd, lhs, constant, _, n = encoding.DecodeBitwise_1n1c(pc, codes, pool)
	//
	stack[rd] = stack[lhs].Or(constant)
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

// executeXor_1n1c implements XORC: stack[rd] = stack[lhs] ^ constant.
func executeXor_1n1c[W word.Word[W]](pc uint32, codes []uint32, pool, stack []W) (uint32, error) {
	var rd, lhs, constant, _, n = encoding.DecodeBitwise_1n1c(pc, codes, pool)
	//
	stack[rd] = stack[lhs].Xor(constant)
	//
	return pc + n, nil
}

// executeCheckCast implements CHECKCAST: it checks that the value in register
// rd fits within the given bit-width, returning an error if it does not.  The
// register itself is left unchanged.
func (p *Interpreter[W]) executeCheckCast(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, bitwidth, n = encoding.DecodeCheckCast(pc, codes)
		value           = stack[rd]
	)
	// perform check
	if !value.FitsWithin(uint(bitwidth)) {
		return pc, p.failure("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
	}
	//
	return pc + n, nil
}

// executeDivMod implements DIVMOD: stack[rq] = stack[dividend] /
// stack[divisor] and stack[rr] = stack[dividend] % stack[divisor], returning
// an error if the divisor is zero.
func (p *Interpreter[W]) executeDivMod(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var rq, rr, dividend, divisor, n = encoding.DecodeDivMod(pc, codes)
	//
	if stack[divisor].Cmp64(0) == 0 {
		return pc, p.failure("division by zero")
	}
	//
	stack[rq] = stack[dividend].Div(stack[divisor])
	stack[rr] = stack[dividend].Rem(stack[divisor])
	//
	return pc + n, nil
}

// executeDivModConst implements DIVMODC: stack[rq] = stack[dividend] /
// constant and stack[rr] = stack[dividend] % constant.  A zero divisor cannot
// be encoded (bytecode validation rejects it), but is guarded regardless.
func (p *Interpreter[W]) executeDivModConst(pc uint32, codes []uint32, pool, stack []W) (uint32, error) {
	var rq, rr, dividend, divisor, n = encoding.DecodeDivMod_C(pc, codes, pool)
	//
	if divisor.Cmp64(0) == 0 {
		return pc, p.failure("division by zero")
	}
	//
	stack[rq] = stack[dividend].Div(divisor)
	stack[rr] = stack[dividend].Rem(divisor)
	//
	return pc + n, nil
}

// executeIntrinsic implements INTRINSIC: it decodes the operation selector and
// dispatches to the corresponding intrinsic (DIV_HINT or WIDE_SHL).
func (p *Interpreter[W]) executeIntrinsic(pc uint32, codes []uint32, pool, stack []W) (uint32, error) {
	op, targets, sources, n := encoding.DecodeIntrinsicOperands(pc, codes)
	//
	switch op {
	case bytecode.DIV_HINT:
		return p.executeDivHint(pc, n, targets, sources, pool, stack)
	case bytecode.WIDE_SHL:
		return p.executeWideShlHint(pc, n, targets, sources, pool, stack)
	case bytecode.WIDE_SHR:
		return p.executeWideShrHint(pc, n, targets, sources, pool, stack)
	case bytecode.WIDE_DIVMOD:
		return p.executeWideDivModHint(pc, n, targets, sources, pool, stack)
	default:
		return pc, fmt.Errorf("unknown intrinsic operation (%d)", op)
	}
}

// executeDivHint implements the DIV_HINT hint: it reconstructs the dividend and
// divisor arguments from their (possibly multi-limb) register vectors, then
// assigns the quotient, remainder and range witness (divisor - remainder - 1)
// of the division across the corresponding target vectors, returning an error
// if the divisor is zero.  big.Int arithmetic is used so values spanning
// several limbs (i.e. wider than the machine word) are handled correctly.
func (p *Interpreter[W]) executeDivHint(pc, n uint32, targets, sources encoding.Operands,
	pool, stack []W) (uint32, error) {
	var (
		regs     = p.program.Module(p.fid).Registers()
		dividend = loadIntrinsicOperand(regs, &sources, pool, stack)
		divisor  = loadIntrinsicOperand(regs, &sources, pool, stack)
	)
	//
	if divisor.Sign() == 0 {
		return pc, p.failure("division by zero")
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
		return pc, p.failure("arithmetic underflow")
	}
	// Distribute quotient, remainder and witness across their target vectors.
	for _, val := range []*big.Int{q, r, w} {
		if err := p.storeIntrinsicResult(regs, &targets, val, stack); err != nil {
			return pc, err
		}
	}
	//
	return pc + n, nil
}

// executeWideShlHint implements the WIDE_SHL hint: it computes value << shift
// truncated to the total bitwidth of the target vector (matching the Bitwise
// SHL instruction, which masks its result to the operation bitwidth) and
// distributes it across the target vector.  The value is treated as a
// little-endian sequence of machine words and shifted with the per-word carry
// chained across the words (see shl64), so operands of any width — including
// those wider than a double word — are handled without big.Int.
func (p *Interpreter[W]) executeWideShlHint(pc, n uint32, targets, sources encoding.Operands,
	pool, stack []W) (uint32, error) {
	var (
		regs  = p.program.Module(p.fid).Registers()
		value = packOperand(regs, &sources, pool, stack)
		// Peek at the target width without consuming the iterator that
		// unpackResult reads below.
		peek  = targets
		width = intrinsicVectorWidth(regs, &peek)
		shift = shiftAmount(packOperand(regs, &sources, pool, stack), uint64(width))
	)
	//
	unpackResult(regs, &targets, shl64(value, shift), stack)
	//
	return pc + n, nil
}

// executeWideShrHint implements the WIDE_SHR hint: it computes value >> shift
// truncated to the total bitwidth of the target vector (matching the Bitwise
// SHR instruction) and distributes it across the target vector, using the same
// word-native, carry-chained approach as executeWideShlHint (see shr64).
func (p *Interpreter[W]) executeWideShrHint(pc, n uint32, targets, sources encoding.Operands,
	pool, stack []W) (uint32, error) {
	var (
		regs  = p.program.Module(p.fid).Registers()
		value = packOperand(regs, &sources, pool, stack)
		peek  = targets
		width = intrinsicVectorWidth(regs, &peek)
		shift = shiftAmount(packOperand(regs, &sources, pool, stack), uint64(width))
	)
	//
	unpackResult(regs, &targets, shr64(value, shift), stack)
	//
	return pc + n, nil
}

// shl64 shifts the multi-limb value held in values — a little-endian sequence of
// machine words (values[0] least significant) — left by the given number of
// bits, returning the (possibly longer) little-endian result.  It chains each
// word's Shl64 carry-out (the high half) into the next word, so the value is
// never materialised in a single fixed-width accumulator and any width is
// handled.  A shift of zero bits degenerates to a whole-word offset.
func shl64[W word.Word[W]](values []W, width uint64) []W {
	var (
		zero      W
		bandwidth = uint64(zero.Bandwidth())
		whole     = width / bandwidth
		bits      = width % bandwidth
		out       = make([]W, uint64(len(values))+whole+1)
		carry     W
	)
	//
	for i, v := range values {
		hi, lo := v.Shl64(bits)
		out[uint64(i)+whole] = lo.Or(carry)
		carry = hi
	}
	//
	out[uint64(len(values))+whole] = carry
	//
	return out
}

// shr64 shifts the multi-limb value held in values — a little-endian sequence of
// machine words — right by the given number of bits, returning the little-endian
// result.  Symmetric to shl64: the low bits of each next-more-significant word
// are folded into the top of the current word.
func shr64[W word.Word[W]](values []W, width uint64) []W {
	var (
		zero      W
		bandwidth = uint64(zero.Bandwidth())
		whole     = width / bandwidth
		bits      = width % bandwidth
	)
	//
	if whole >= uint64(len(values)) {
		// Every bit is shifted out.
		return []W{zero}
	}
	//
	var out = make([]W, uint64(len(values))-whole)
	//
	for j := range out {
		var (
			src = uint64(j) + whole
			val = values[src].Shr64(bits)
		)
		// Fold in the low bits of the next word, which move into the top of this
		// one.
		if bits > 0 && src+1 < uint64(len(values)) {
			_, top := values[src+1].Shl64(bandwidth - bits)
			val = val.Or(top)
		}
		//
		out[j] = val
	}
	//
	return out
}

// packOperand reads the next (base, len) register vector and returns its value
// as a little-endian sequence of machine words (words[0] least significant).
// The lowest-indexed register holds the most-significant limb, so limbs are read
// least-significant first (highest index down to base) and placed at an
// increasing bit offset, spilling into the next word as the offset crosses a
// word boundary.  A bit cursor is used so the limb width need not divide the
// machine word bandwidth.
func packOperand[W word.Word[W]](regs []descriptor.Register[W], iter *encoding.Operands, pool, stack []W) []W {
	var (
		base, nbOfValues, isConst = iter.NextOperand()
		zero                      W
		bandwidth                 = zero.Bandwidth()
		words                     []W
		cur                       W
		off                       uint
	)
	// A constant operand is single value by construction.
	if isConst {
		if nbOfValues != 1 {
			panic(fmt.Sprintf("constant operand expect 1 value, got %d", nbOfValues))
		}

		return []W{pool[base]}
	}
	//
	for i := int(nbOfValues) - 1; i >= 0; i-- {
		var (
			reg    = base + uint16(i)
			w      = bitwidthOf(regs, reg)
			hi, lo = stack[reg].Shl64(uint64(off))
		)
		//
		cur = cur.Or(lo)
		off += w
		// The word is full: emit it and carry the overflow into the next.
		if off >= bandwidth {
			words = append(words, cur)
			cur = hi
			off -= bandwidth
		}
	}
	// Flush any partial final word (and guarantee at least one word).
	if off > 0 || len(words) == 0 {
		words = append(words, cur)
	}
	//
	return words
}

// unpackResult distributes the low bits of a little-endian machine-word sequence
// across the next (base, len) target register vector, truncating to that
// vector's total width.  The lowest-indexed register holds the most-significant
// limb, so limbs are written least-significant first (highest index down to
// base), reading each limb's bits from the word stream at an increasing bit
// offset.  Any bits above the target width (e.g. bits shifted out of range) are
// silently dropped, so — unlike storeIntrinsicResult — there is no overflow.
func unpackResult[W word.Word[W]](regs []descriptor.Register[W], iter *encoding.Operands, words, stack []W) {
	var (
		base      = iter.Next()
		length    = uint(iter.Next())
		zero      W
		bandwidth = zero.Bandwidth()
		off       uint
	)
	//
	for i := int(length) - 1; i >= 0; i-- {
		var (
			reg     = base + uint16(i)
			w       = bitwidthOf(regs, reg)
			wordIdx = off / bandwidth
			bitIdx  = off % bandwidth
			val     W
		)
		//
		if wordIdx < uint(len(words)) {
			val = words[wordIdx].Shr64(uint64(bitIdx))
			// Fold in bits from the next word when this limb straddles a word
			// boundary.
			if bitIdx+w > bandwidth && wordIdx+1 < uint(len(words)) {
				_, top := words[wordIdx+1].Shl64(uint64(bandwidth - bitIdx))
				val = val.Or(top)
			}
		}
		//
		stack[reg] = val.Slice(w)
		off += w
	}
}

// shiftAmount reduces a packed shift-count operand to a single uint64, clamped
// to maxShift.  A shift of at least maxShift bits clears the whole result, so
// any amount that does not fit in the low word — or is already >= maxShift — is
// clamped to maxShift.  This keeps the amount word-native and bounds the
// shl64 / shr64 allocation against a pathologically large count.
func shiftAmount[W word.Word[W]](words []W, maxShift uint64) uint64 {
	// A set bit above the low word implies a count of at least 2^bandwidth,
	// which dwarfs any register width; clamp.
	for _, w := range words[1:] {
		if w.Cmp64(0) != 0 {
			return maxShift
		}
	}
	//
	if words[0].Cmp64(maxShift) >= 0 {
		return maxShift
	}
	//
	return words[0].Uint64()
}

// executeWideDivModHint implements the WIDE_DIVMOD intrinsic: it reconstructs
// the dividend and divisor arguments from their (possibly multi-limb) register
// vectors, then assigns the quotient and the remainder across the two target
// vectors, returning an error if the divisor is zero.  This mirrors the DIVMOD
// instruction but operates over vectored (multi-limb) operands.  big.Int
// arithmetic is used so values spanning several limbs (i.e. wider than the
// machine word) are handled correctly.
func (p *Interpreter[W]) executeWideDivModHint(pc, n uint32, targets, sources encoding.Operands,
	pool, stack []W) (uint32, error) {
	var (
		regs     = p.program.Module(p.fid).Registers()
		dividend = loadIntrinsicOperand(regs, &sources, pool, stack)
		divisor  = loadIntrinsicOperand(regs, &sources, pool, stack)
	)
	//
	if divisor.Sign() == 0 {
		return pc, p.failure("division by zero")
	}
	// Distribute quotient and remainder across their target vectors.
	for _, val := range []*big.Int{new(big.Int).Quo(dividend, divisor), new(big.Int).Rem(dividend, divisor)} {
		if err := p.storeIntrinsicResult(regs, &targets, val, stack); err != nil {
			return pc, err
		}
	}
	//
	return pc + n, nil
}

// intrinsicVectorWidth returns the combined bitwidth of the next (base, len) register
// vector in the iterator, consuming that vector.
func intrinsicVectorWidth[W any](regs []descriptor.Register[W], iter *encoding.Operands) uint {
	var (
		base   = iter.Next()
		length = uint(iter.Next())
		total  uint
	)
	//
	for i := uint(0); i < length; i++ {
		total += bitwidthOf(regs, base+uint16(i))
	}
	//
	return total
}

// loadIntrinsicOperand reconstructs the value of a single hint operand from the
// next (base, len) pair in the iterator.  A constant operand reads its (single
// limb, see split.SplitOperand) value from the constant pool.  For a register
// vector: register splitting lays limbs out most-significant first (see
// descriptor.NewLimbsMap), so the lowest-indexed register (base) holds the
// most-significant limb.  The value is therefore accumulated from the
// most-significant limb down, shifting the running value up by each limb's
// width before folding the limb in.
func loadIntrinsicOperand[W word.Word[W]](regs []descriptor.Register[W], iter *encoding.Operands,
	pool, stack []W) *big.Int {
	//
	var (
		base, nbOfValues, isConst = iter.NextOperand()
		value                     = new(big.Int)
	)
	//
	if isConst {
		if nbOfValues != 1 {
			panic(fmt.Sprintf("constant operand expect 1 value, got %d", nbOfValues))
		}

		return pool[base].BigInt()
	}
	//
	for i := uint16(0); i < nbOfValues; i++ {
		var reg = base + i
		//
		value.Lsh(value, bitwidthOf(regs, reg))
		value.Or(value, stack[reg].BigInt())
	}
	//
	return value
}

// storeIntrinsicResult distributes value across the next (base, len) register
// vector in the iterator.  The lowest-indexed register (base) holds the
// most-significant limb (matching loadIntrinsicOperand), so the least-
// significant limb is written into the highest-indexed register and filling
// proceeds downwards.  It errors if the value does not fit within the vector's
// total width.
func (p *Interpreter[W]) storeIntrinsicResult(regs []descriptor.Register[W], iter *encoding.Operands,
	value *big.Int, stack []W) error {
	var (
		base   = iter.Next()
		length = uint(iter.Next())
		acc    = new(big.Int).Set(value)
		total  uint
	)
	//
	for i := int(length) - 1; i >= 0; i-- {
		var (
			reg   = base + uint16(i)
			width = bitwidthOf(regs, reg)
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
		return p.failure("bit overflow (0x%s not u%d)", value.Text(16), total)
	}
	//
	return nil
}

// executeSkipIf_rr implements the conditional register-register forward branch
// bytecodes (SEQ_rr, SNE_rr, SLT_rr, SGE_rr).  There are no GT/LE forms: the
// encoder normalises those conditions by swapping operands.  The comparison
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

// executeSkipIf_rc implements the conditional register-constant forward branch
// bytecodes (SEQ_rc, SNE_rc, SLT_rc, SGE_rc).  There are no GT/LE forms: the
// encoder normalises those conditions by adjusting the constant (x > c ⇔
// x >= c+1).  The comparison is selected via the Comparator type parameter F.
// If stack[rs0] compares to the inline constant as required, execution skips
// forward to the encoded target; otherwise it falls through to the following
// bytecode.
func executeSkipIf_rc[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, pool []W,
	stack []W) uint32 {
	var (
		c F
		//
		skip, rs0, constant, _, n = encoding.DecodeSkipIf_rc(pc, codes, pool)
		// Calculate skip target
		target = pc + 1 + skip
		// Read rs0
		val0 = stack[rs0]
	)
	//
	if c.Cmp(val0, constant) {
		// true branch
		return target
	}
	// false branch
	return pc + n
}

// executeSkipIf_rcv implements the conditional register-vector versus
// constant-vector forward branch bytecodes (SEQ_rcv, SNE_rcv, SLT_rcv,
// SGE_rcv).  There are no GT/LE forms: the encoder normalises those conditions
// by adjusting the constant (x > c ⇔ x >= c+1).  The comparison is selected
// via the Comparator type parameter F, and proceeds lexicographically from the
// most-significant element (base) downwards, exactly as executeSkipIf_rv.
func executeSkipIf_rcv[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, pool []W,
	stack []W) uint32 {
	var (
		cmp F
		//
		skip, rs0, constants, _, n = encoding.DecodeSkipIf_rcv(pc, codes, pool)
		// Calculate skip target
		target = pc + 1 + skip
	)
	//
	for i := uint16(0); i < rs0.Len; i++ {
		// Read rs0
		val0 := stack[rs0.Base+i]
		// Read corresponding constant element
		val1 := constants[i]
		//
		if i+1 != rs0.Len && val0.Cmp(val1) == 0 {
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

// executeSkipIf_rv implements the conditional register-vector branch bytecodes
// (SEQ_rv, SNE_rv, SLT_rv, SGE_rv).  There are no GT/LE forms: the encoder
// normalises those conditions by swapping operands.  The comparison is selected
// via the Comparator type parameter F.  If stack[rs0] compares to stack[rs1] as
// required, execution jumps to the encoded target; otherwise it falls through
// to the following bytecode.
//
// Register splitting lays limbs out most-significant first (see
// descriptor.NewLimbsMap and split.ApplyLimbsMap), so the lowest-indexed
// register (base) holds the most-significant limb.  The comparison therefore
// scans from the most-significant limb (base) downwards, skipping past equal
// limbs until the first difference (or the least-significant limb) decides.
func executeSkipIf_rv[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		cmp F
		//
		skip, rs0, rs1, _, n = encoding.DecodeSkipIf_rv(pc, codes)
		// Calculate skip target
		target = pc + 1 + skip
	)
	//
	for i := uint16(0); i < rs0.Len; i++ {
		// Read rs0
		val0 := stack[rs0.Base+i]
		// Read rs1
		val1 := stack[rs1.Base+i]
		//
		if i+1 != rs0.Len && val0.Cmp(val1) == 0 {
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
// binary searched against the case values (held, in ascending order, as a
// consecutive run of the constant pool starting at cid) and, on a match,
// control skips forward to that case's target; otherwise control falls
// through past the whole instruction to the following one.
func executeSkipTable[W word.Word[W]](pc uint32, codes []uint32, pool []W, stack []W) uint32 {
	var (
		word0  = codes[pc]
		count  = (word0 >> 24) & 0xff
		cid    = (word0 >> 16) & 0xff
		source = (word0 >> 8) & 0xff
		val    = stack[source]
		nwords = encoding.NumCodesPackedWide(uint(count))
	)
	//
	if mid, ok := slices.BinarySearchFunc(pool[cid:cid+count], val, W.Cmp); ok {
		var skipWord = codes[pc+1+(uint32(mid)/2)]
		//
		if mid%2 == 0 {
			return pc + 1 + (skipWord & 0xffff)
		}
		//
		return pc + 1 + (skipWord >> 16)
	}
	// no match: fall through past the whole instruction
	return pc + 1 + nwords
}

// executeWideSkipTable implements the wide form of the SKIP_M dispatch (see
// executeSkipTable), for a source register or base pool identifier exceeding
// u8.  The source register, base pool identifier and count are read from the
// dedicated second word rather than word 0; the packed skips follow exactly
// as in the narrow form, just shifted one word later.  As for the narrow
// form, a matched case's skip is relative to pc+1 (matching the offset
// computed at encode time), whilst the no-match fall-through lands on the
// true next instruction, past the (wider) header.
func executeWideSkipTable[W word.Word[W]](pc uint32, codes []uint32, pool []W, stack []W) uint32 {
	var (
		count  = codes[pc] >> 16
		word1  = codes[pc+1]
		cid    = word1 >> 16
		source = word1 & 0xffff
		val    = stack[source]
		nwords = encoding.NumCodesPackedWide(uint(count))
	)
	//
	if mid, ok := slices.BinarySearchFunc(pool[cid:cid+count], val, W.Cmp); ok {
		var skipWord = codes[pc+2+(uint32(mid)/2)]
		//
		if mid%2 == 0 {
			return pc + 1 + (skipWord & 0xffff)
		}
		//
		return pc + 1 + (skipWord >> 16)
	}
	// no match: fall through past the whole instruction
	return pc + 2 + nwords
}

// executeDispatch implements the SKIP_B (one-hot) dispatch: the case bits are
// examined in order and control transfers to the (absolute) target of the
// first bit which is set; when no bit is set, control falls through past the
// whole instruction to the following one.
func executeDispatch[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		word0 = codes[pc]
		count = (word0 >> 24) & 0xff
	)
	//
	for i := range count {
		var word = codes[pc+1+i]
		//
		if stack[word&0xffff].Cmp64(0) != 0 {
			return pc + 1 + (word >> 16)
		}
	}
	// no bit set: fall through past the whole instruction
	return pc + 1 + count
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
func executeLdc_w[W word.Word[W]](pc uint32, codes []uint32, pool []W, stack []W) uint32 {
	val, rd, n := encoding.DecodeLdc_w(pc, codes, pool)
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
func (p *Interpreter[W]) executeMul_2n1(pc uint32, codes []uint32, stack []W) (uint32, error) {
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
		return pc, p.failure("arithmetic overflow")
	}
	//
	stack[rd] = lo
	//
	return pc + n, nil
}

// executeMul_1n1c implements MULC: stack[rd] = stack[rs] * constant.
func (p *Interpreter[W]) executeMul_1n1c(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c(pc, codes, pool)
		val                 = stack[rs]
		hi, lo              = val.Mul(constant)
	)
	//
	if hi.Cmp64(0) != 0 {
		return pc, p.failure("arithmetic overflow")
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
	sroms []StaticReadOnly[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		srom              = &sroms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, srom.Descriptor(), stack)
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
	roms []ReadOnly[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		rom               = &roms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, rom.Descriptor(), stack)
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
func (p *Interpreter[W]) executeSub_1n1c(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = encoding.DecodeArith_1n1c(pc, codes, pool)
		val                 = stack[rs]
		res, underflow      = val.Sub(constant)
	)
	//
	if underflow {
		// Mirror descriptor.CalculateSubBitwidth: one is subtracted from the
		// constant because negative values do not need to encode zero, so a
		// power-of-two constant does not cost an extra bit.  (The constant is
		// at least one here — a zero constant cannot underflow.)
		cm1, _ := constant.Sub64(1)
		//
		var (
			module   = p.program.Module(p.fid)
			rd_width = module.Register(rd).Bitwidth().Unwrap()
			rs_width = module.Register(rs).Bitwidth().Unwrap()
			bitwidth = max(rd_width, 1+max(rs_width, cm1.BitLen()))
		)
		// slice enough values
		res = res.Slice(bitwidth)
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeSub_2n1 implements SUB_2n1: stack[rd] = stack[rs0] - stack[rs1],
// returning an error if the subtraction underflows the word type.
func (p *Interpreter[W]) executeSub_2n1(pc uint32, codes []uint32, stack []W) (uint32, error) {
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
		var (
			module    = p.program.Module(p.fid)
			rd_width  = module.Register(rd).Bitwidth().Unwrap()
			rs0_width = module.Register(rs0).Bitwidth().Unwrap()
			rs1_width = module.Register(rs1).Bitwidth().Unwrap()
			bitwidth  = max(rd_width, 1+max(rs0_width, rs1_width))
		)
		// slice enough values
		res = res.Slice(bitwidth)
	}
	//
	stack[rd] = res
	//
	return pc + n, nil
}

// executeSub_nm implements SUB_nm: it seeds the value from the first source,
// subtracts the remaining sources and the constant, and stores the result
// across a vector target using the same low-limb-first rule as the word
// machine, reporting an error on underflow.
func (p *Interpreter[W]) executeSub_nm(pc uint32, codes []uint32, pool []W, stack []W) (uint32, error) {
	var (
		targets, sources, constant, bitwidth, n = encoding.DecodeArith_nm(pc, codes, pool)
		val                                     W
		acc                                     W
		underflow                               bool
	)
	// Seed initial value
	val = stack[sources.Next()]
	// Subtract rest from it
	for sources.HasNext() {
		var src = sources.Next()
		//
		if acc, underflow = acc.Add(stack[src]); underflow {
			return pc, p.failure("arithmetic underflow")
		}
	}
	//
	if acc, underflow = acc.Add(constant); underflow {
		return pc, p.failure("arithmetic underflow")
	} else if val, underflow = val.Sub(acc); underflow {
		val = val.Slice(bitwidth)
	}
	//
	return pc + n, p.storeAcross(pc, p.program.Module(p.fid).Registers(), targets, val, stack)
}

// executeWriteWom_sn implements WR_WOM_nm: it writes ndata consecutive words
// from successive source registers into the write-once memory identified by id,
// starting at the address decoded from the operand registers.
func (p *Interpreter[W]) executeWriteWom_sn(pc uint32, codes []uint32, stack []W,
	woms []WriteOnce[W]) (uint32, error) {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		wom               = &woms[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, wom.Descriptor(), stack)
	//
	for data.HasNext() {
		// Sanity check
		if !wom.CanWrite(address) {
			return pc, p.failure("address %x already written for write-only memory %s", address, wom.descriptor.Name())
		}
		//nolint
		wom.Write(address, stack[data.Next()])
		//
		address++
	}
	//
	return pc + n, nil
}

// executeReadRam_sn implements RD_RAM_nm: it reads ndata consecutive words from
// the random-access memory identified by id, starting at the address decoded
// from the operand registers, into successive destination registers.
func executeReadRam_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	rams []RandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		ram               = &rams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, ram.Descriptor(), stack)
	// One logical access = one timestamp shared by every data lane: tick once,
	// before touching any lane.  (ndata == 0 would tick without reading a lane;
	// harmless but unused -- its only conceivable use is a bare access counter.)
	ram.Tick()
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
	rams []RandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		ram               = &rams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, ram.Descriptor(), stack)
	// One logical access = one timestamp shared by every data lane (see
	// executeReadRam_sn for the ndata == 0 note).
	ram.Tick()
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
	prams []PagedRandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		pram              = &prams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, pram.Descriptor(), stack)
	// One logical access = one timestamp shared by every data lane (see
	// executeReadRam_sn for the ndata == 0 note).
	pram.Tick()
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
	prams []PagedRandomAccess[W]) uint32 {
	//
	var (
		id, addr, data, n = encoding.DecodeReadWrite_sn(pc, codes)
		pram              = &prams[id]
		address           uint64
	)
	//
	address = decodeAddress(addr, pram.Descriptor(), stack)
	// One logical access = one timestamp shared by every data lane (see
	// executeReadRam_sn for the ndata == 0 note).
	pram.Tick()
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
func decodeAddress[W word.Word[W]](regs encoding.Operands, geometry *descriptor.Memory[W], stack []W) uint64 {
	var (
		index      uint64
		registers  = geometry.Registers()
		numInputs  = geometry.NumInputs()
		numOutputs = geometry.NumOutputs()
	)

	for i := range numInputs {
		var (
			bitwidth = uint64(registers[i].Bitwidth().Unwrap())
			val      = stack[regs.Next()]
		)
		//
		index = (index << bitwidth) | val.Uint64()
	}

	return index * uint64(numOutputs)
}

// bitwidthOf looks up a register's bitwidth directly in the module's register
// slice.  Callers should fetch this slice once (via Module.Registers()) and
// reuse it across a loop of registers, rather than calling Module.Register
// per register: since Module is an interface, each such call is an
// unavoidable dynamic dispatch, whereas indexing the concrete slice returned
// by Registers() is not.
func bitwidthOf[W any](regs []descriptor.Register[W], reg RegisterId) uint {
	return regs[reg].Bitwidth().UnwrapOr(math.MaxUint)
}

func loadAcross[W word.Word[W]](regs []descriptor.Register[W], sources encoding.Operands, stack []W) W {
	var (
		value W
		width uint
	)
	//
	for sources.HasNext() {
		reg := sources.Next()
		_, lo := stack[reg].Shl64(uint64(width))
		value = value.Or(lo)
		width += bitwidthOf(regs, reg)
	}
	//
	return value
}

func (p *Interpreter[W]) storeAcross(pc uint32, regs []descriptor.Register[W], targets encoding.Operands, oval W,
	stack []W) error {
	//
	var (
		bitwidth uint
		value    = oval
	)
	//
	for targets.HasNext() {
		var (
			target = targets.Next()
			width  = bitwidthOf(regs, target)
		)
		//
		// Low limbs are written first, matching machine.StoreAcross.  Note the
		// (wrapped) limbs are written even when the value overflows below, so a
		// trace being generated records the wrapped result the constraints reject.
		stack[target] = value.Slice(width)
		value = value.Shr64(uint64(width))
		bitwidth += width
	}
	//
	if value.Cmp64(0) != 0 {
		return p.failure("bit overflow (0x%s not u%d, pc=0x%x)", oval.Text(16), bitwidth, pc)
	}
	//
	return nil
}
