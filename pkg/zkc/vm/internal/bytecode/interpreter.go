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
package bytecode

import (
	"errors"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
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
	// The program (modules, bytecodes, constant pool and symbols) being executed.
	program Program[W]
	// Program counter: offset into program.bytecodes of the next bytecode to
	// decode and execute.
	pc uint32
	// Frame pointer: base offset within dataStack of the current function's
	// activation record.  Registers are addressed relative to this.
	fp uint
	// Data stack holding the activation records (registers) of active function
	// calls.  The current frame begins at fp.
	dataStack heap.Heap[W]
	// Call stack holding return addresses (bytecode offsets) for nested calls.
	callStack heap.Heap[uint32]
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
	// (Large) bipartite random-access memories which may be freely read and
	// written.
	brams []memory.BiPartiteRandomAccess[W]
}

// NewInterpreter constructs a new bytecode interpreter for the given program.
// The program's memory modules are partitioned by access discipline (static
// read-only, read-only, write-once, random-access and bipartite random-access)
// so that read/write bytecodes can locate them directly by identifier during
// execution.  The interpreter is created in an unbooted state; Boot must be
// called to select an entry point and supply inputs before calling Execute.
func NewInterpreter[W word.Word[W]](program Program[W]) *Interpreter[W] {
	var (
		sroms []memory.StaticReadOnly[W]
		roms  []memory.ReadOnly[W]
		woms  []memory.WriteOnce[W]
		rams  []memory.RandomAccess[W]
		brams []memory.BiPartiteRandomAccess[W]
	)
	//
	for _, m := range program.modules {
		switch m := m.(type) {
		case *memory.ReadOnly[W]:
			roms = append(roms, *m)
		case *memory.StaticReadOnly[W]:
			sroms = append(sroms, *m)
		case *memory.WriteOnce[W]:
			woms = append(woms, *m)
		case *memory.RandomAccess[W]:
			rams = append(rams, *m)
		case *memory.BiPartiteRandomAccess[W]:
			brams = append(brams, *m)
		}
	}
	//
	return &Interpreter[W]{
		program: program,
		pc:      0,
		fp:      0,
		sroms:   sroms,
		roms:    roms,
		woms:    woms,
		rams:    rams,
		brams:   brams,
	}
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
		bytecodes     = p.program.bytecodes
		frame     []W = p.dataStack.SliceEnd(p.fp)
	)
	//
	for nsteps < steps && err == nil {
		// decode instruction
		var opcode = bytecodes[p.pc] & OPCODE_MASK
		// increase step counter
		nsteps++
		//
		switch opcode & OPCODE_MASK {
		case FAIL:
			return nsteps, errors.New("machine panic")
		case CHECKCAST:
			p.pc, err = executeCheckCast(p.pc, bytecodes, frame)
		case LDC:
			p.pc = executeLdc_1(p.pc, bytecodes, frame)
		case MOVE:
			p.pc = executeMove_1s1(p.pc, bytecodes, frame)
		case CALL:
			panic("todo")
		case RET:
			// check for termination
			if p.callStack.Size() == 0 {
				return nsteps, nil
			}
			//
			panic("todo")
		case JMP:
			p.pc, _ = decodeJmp1(p.pc, bytecodes)
		case JEQ_rr:
			p.pc = executeJif_rr[W, util.Equal[W]](p.pc, bytecodes, frame)
		case JNE_rr:
			p.pc = executeJif_rr[W, util.NotEqual[W]](p.pc, bytecodes, frame)
		case JLT_rr:
			p.pc = executeJif_rr[W, util.LessThan[W]](p.pc, bytecodes, frame)
		case JGT_rr:
			p.pc = executeJif_rr[W, util.GreaterThan[W]](p.pc, bytecodes, frame)
		case JLE_rr:
			p.pc = executeJif_rr[W, util.LessThanOrEqual[W]](p.pc, bytecodes, frame)
		case JGE_rr:
			p.pc = executeJif_rr[W, util.GreaterThanOrEqual[W]](p.pc, bytecodes, frame)
			// Input / Output Operations
		case RD_ROM_nm:
			p.pc = executeReadRom_sn(p.pc, bytecodes, frame, p.roms)
		case RD_SROM_nm:
			p.pc = executeReadSrom_sn(p.pc, bytecodes, frame, p.sroms)
		case WR_WOM_nm:
			p.pc = executeWriteWom_sn(p.pc, bytecodes, frame, p.woms)
		case RD_RAM_nm:
			p.pc = executeReadRam_sn(p.pc, bytecodes, frame, p.rams)
		case WR_RAM_nm:
			p.pc = executeWriteRam_sn(p.pc, bytecodes, frame, p.rams)
		// Arithmetic Operations
		case ADD_2n1:
			p.pc, err = executeAdd_2n1(p.pc, bytecodes, frame)
		case SUB_2n1:
			p.pc, err = executeSub_2n1(p.pc, bytecodes, frame)
		case MUL_2n1:
			p.pc, err = executeMul_2n1(p.pc, bytecodes, frame)
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
// access and bipartite random-access) are reset to empty.
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
	// reset (big) brams
	for i := range p.brams {
		p.brams[i].Initialise(nil)
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

// executeAdd_2n1 implements ADD_2n1: stack[rd] = stack[rs0] + stack[rs1],
// returning an error if the addition overflows the word type.
func executeAdd_2n1[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs0, rs1, rd, n = decodeArith_2n1(pc, codes)
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

// executeCheckCast implements CHECKCAST: it checks that the value in register
// rd fits within the given bit-width, returning an error if it does not.  The
// register itself is left unchanged.
func executeCheckCast[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, bitwidth, n = decodeCheckCast(pc, codes)
		value           = stack[rd]
	)
	// perform check
	if !value.FitsWithin(uint(bitwidth)) {
		return pc, fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
	}
	//
	return pc + n, nil
}

// executeJif_rr implements the conditional register-register branch bytecodes
// (JEQ_rr, JNE_rr, JLT_rr, JGT_rr, JLE_rr, JGE_rr).  The comparison is selected
// via the Comparator type parameter F.  If stack[rs0] compares to stack[rs1] as
// required, execution jumps to the encoded target; otherwise it falls through
// to the following bytecode.
func executeJif_rr[W word.Word[W], F util.Comparator[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		c F
		//
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
	)
	//
	if c.Cmp(val0, val1) {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

// executeLdc_1 implements LDC: it loads a constant value into register rd.
func executeLdc_1[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	val, rd, n := decodeLdc_1[W](pc, codes)
	//
	stack[rd] = val
	//
	return pc + n
}

// executeMove_1s1 implements MOVE: it copies the value of register rs into
// register rd.
func executeMove_1s1[W word.Word[W]](pc uint32, codes []uint32, stack []W) uint32 {
	var (
		rs, rd, n = decodeMove_1s1(pc, codes)
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
		rs0, rs1, rd, n = decodeArith_2n1(pc, codes)
		// Read rs0
		val0 = stack[rs0]
		// Read rs1
		val1 = stack[rs1]
		// Add v0 * v1
		res, overflow = val0.Mul(val1)
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

// executeReadSrom_sn implements RD_SROM_nm: it reads ndata consecutive words
// from the static read-only memory identified by id, starting at the address
// decoded from the operand registers, into successive destination registers.
func executeReadSrom_sn[W word.Word[W]](pc uint32, codes []uint32, stack []W,
	sroms []memory.StaticReadOnly[W]) uint32 {
	//
	var (
		id, _, ndata, regs, n = decodeReadWrite_sn(pc, codes)
		srom                  = &sroms[id]
		address               uint64
	)
	//
	address, regs = decodeAddress(regs, srom.Geometry(), stack)
	//
	for range ndata {
		//nolint
		stack[regs.Next()], _ = srom.Read(address)
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
		id, _, ndata, regs, n = decodeReadWrite_sn(pc, codes)
		rom                   = &roms[id]
		address               uint64
	)
	//
	address, regs = decodeAddress(regs, rom.Geometry(), stack)
	//
	for range ndata {
		//nolint
		stack[regs.Next()], _ = rom.Read(address)
		//
		address++
	}
	//
	return pc + n
}

// executeSub_2n1 implements SUB_2n1: stack[rd] = stack[rs0] - stack[rs1],
// returning an error if the subtraction underflows the word type.
func executeSub_2n1[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs0, rs1, rd, n = decodeArith_2n1(pc, codes)
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
		id, _, ndata, regs, n = decodeReadWrite_sn(pc, codes)
		wom                   = &woms[id]
		address               uint64
	)
	//
	address, regs = decodeAddress(regs, wom.Geometry(), stack)
	//
	for range ndata {
		//nolint
		wom.Write(address, stack[regs.Next()])
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
		id, _, ndata, regs, n = decodeReadWrite_sn(pc, codes)
		ram                   = &rams[id]
		address               uint64
	)
	//
	address, regs = decodeAddress(regs, ram.Geometry(), stack)
	//
	for range ndata {
		//nolint
		stack[regs.Next()], _ = ram.Read(address)
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
		id, _, ndata, regs, n = decodeReadWrite_sn(pc, codes)
		rom                   = &rams[id]
		address               uint64
	)
	//
	address, regs = decodeAddress(regs, rom.Geometry(), stack)
	//
	for range ndata {
		//nolint
		rom.Write(address, stack[regs.Next()])
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
func decodeAddress[W word.Word[W]](regs Op8Iter, geometry memory.Geometry[W], stack []W) (uint64, Op8Iter) {
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

	return index * uint64(numOutputs), regs
}
