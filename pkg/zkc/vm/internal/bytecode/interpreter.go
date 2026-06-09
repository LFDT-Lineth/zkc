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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
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
	// Function entry addresses indexed by module identifier.
	addresses map[uint]uint32
	// Cached register counts for each module.
	moduleInfo []moduleInfo
	// Current function module identifier.
	fid uint
	// Program counter: offset into program.bytecodes of the next bytecode to
	// decode and execute.
	pc uint32
	// Frame pointer: base offset within dataStack of the current function's
	// activation record.  Registers are addressed relative to this.
	fp uint
	// Data stack holding the activation records (registers) of active function
	// calls.  The current frame begins at fp.
	dataStack heap.Heap[W]
	// Call stack holding caller state for nested calls.
	callStack []callFrame
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

type callFrame struct {
	// fid/fp identify the caller frame to restore after RET.
	fid uint
	fp  uint
	// returnPc is the next bytecode after CALL in the caller.
	returnPc uint32
	// returns names the caller registers that receive callee outputs.
	returns []Reg
}

type moduleInfo struct {
	ninputs  int
	noutputs int
}

// NewInterpreter constructs a new bytecode interpreter for the given program.
// The program's memory modules are partitioned by access discipline (static
// read-only, read-only, write-once, random-access and bipartite random-access)
// so that read/write bytecodes can locate them directly by identifier during
// execution.  The interpreter is created in an unbooted state; Boot must be
// called to select an entry point and supply inputs before calling Execute.
func NewInterpreter[W word.Word[W]](program Program[W]) *Interpreter[W] {
	var (
		addresses = make(map[uint]uint32, len(program.symbols))
		modinfo   = make([]moduleInfo, len(program.modules))
		sroms     []memory.StaticReadOnly[W]
		roms      []memory.ReadOnly[W]
		woms      []memory.WriteOnce[W]
		rams      []memory.RandomAccess[W]
		brams     []memory.BiPartiteRandomAccess[W]
	)
	//
	for addr, mid := range program.symbols {
		addresses[mid] = addr
	}
	//
	for i, m := range program.modules {
		modinfo[i] = newModuleInfo(m)
		//
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
		program:    program,
		addresses:  addresses,
		moduleInfo: modinfo,
		pc:         0,
		fp:         0,
		sroms:      sroms,
		roms:       roms,
		woms:       woms,
		rams:       rams,
		brams:      brams,
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
	if p.pc, ok = p.addresses[fid]; !ok {
		return fmt.Errorf("missing symbol for \"%s\"", fun)
	}
	//
	p.fid = fid
	p.fp = 0
	p.callStack = p.callStack[:0]
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
		case DEBUG:
			// DEBUG is ignored for now; tests only assert program outputs.
			p.pc++
		case LDC:
			p.pc = executeLdc_1(p.pc, bytecodes, frame)
		case MOVE:
			p.pc = executeMove_1s1(p.pc, bytecodes, frame)
		case CALL:
			p.pc, err = p.executeCall(p.pc, bytecodes, frame)
			// CALL changes the active frame, so refresh the register window.
			frame = p.dataStack.SliceEnd(p.fp)
		case RET:
			// check for termination
			if len(p.callStack) == 0 {
				return nsteps, nil
			}
			//
			p.pc, err = p.executeReturn(p.pc, bytecodes)
			// RET restores the caller frame, so refresh the register window.
			frame = p.dataStack.SliceEnd(p.fp)
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
		case ADDC:
			p.pc, err = executeAdd_1n1c(p.pc, bytecodes, frame)
		case SUB_2n1:
			p.pc, err = executeSub_2n1(p.pc, bytecodes, frame)
		case SUBC:
			p.pc, err = executeSub_1n1c(p.pc, bytecodes, frame)
		case MUL_2n1:
			p.pc, err = executeMul_2n1(p.pc, bytecodes, frame)
		case MULC:
			p.pc, err = executeMul_1n1c(p.pc, bytecodes, frame)
		case ARITHV:
			p.pc, err = p.executeArithVec(p.pc, bytecodes, frame)
		case CAT:
			p.pc, err = p.executeCat(p.pc, bytecodes, frame)
		case NOT:
			p.pc, err = p.executeNot(p.pc, bytecodes, frame)
		case AND, OR, XOR:
			p.pc, err = p.executeBitwise(p.pc, bytecodes, frame)
		case SHL, SHR:
			p.pc, err = p.executeShift(p.pc, bytecodes, frame)
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

func (p *Interpreter[W]) executeCall(pc uint32, codes []uint32, caller []W) (uint32, error) {
	var (
		mid, args, returns, n = decodeCallOperands(pc, codes)
		address               uint32
		ok                    bool
	)
	//
	if address, ok = p.addresses[uint(mid)]; !ok {
		return pc, fmt.Errorf("missing symbol for function module %d", mid)
	}
	//
	calleeModule := p.program.Module(uint(mid))
	// Function registers are laid out inputs, then outputs, then locals.
	ninputs := p.moduleInfo[mid].ninputs
	//
	if len(args) != ninputs {
		return pc, fmt.Errorf("call to %s expects %d arguments, got %d", calleeModule.Name(), ninputs, len(args))
	}
	//
	frame := callFrame{
		fid:      p.fid,
		fp:       p.fp,
		returnPc: pc + n,
		returns:  returns,
	}
	calleeFp := p.dataStack.Size()
	// Allocate the callee frame above the caller frame on the data stack.
	p.dataStack.Alloc(calleeModule.Width())
	//
	callee := p.dataStack.SliceEnd(calleeFp)
	// Callee inputs occupy the first registers in its frame, in argument order.
	for i, arg := range args {
		if err := storeReg(calleeModule, Reg(i), caller[arg], callee); err != nil {
			p.dataStack.Free(calleeModule.Width())
			return pc, err
		}
	}
	//
	p.callStack = append(p.callStack, frame)
	// Switch execution to the callee frame and entry address.
	p.fid = uint(mid)
	p.fp = calleeFp
	//
	return address, nil
}

func (p *Interpreter[W]) executeReturn(pc uint32, _ []uint32) (uint32, error) {
	var (
		frame        = p.callStack[len(p.callStack)-1]
		calleeModule = p.program.Module(p.fid)
		callerModule = p.program.Module(frame.fid)
		callee       = p.dataStack.Slice(p.fp, p.fp+calleeModule.Width())
		caller       = p.dataStack.Slice(frame.fp, frame.fp+callerModule.Width())
		info         = p.moduleInfo[p.fid]
		// Callee outputs follow its inputs in register order, so ninputs is the
		// offset of the first output; one output is produced per return target.
		ninputs  = info.ninputs
		noutputs = info.noutputs
	)
	//
	if len(frame.returns) != noutputs {
		return pc, fmt.Errorf("return from %s produces %d values, got %d targets",
			calleeModule.Name(), noutputs, len(frame.returns))
	}
	//
	for i, ret := range frame.returns {
		// Write each callee output into the caller register named by CALL.
		if err := storeReg(callerModule, ret, callee[i+ninputs], caller); err != nil {
			return pc, err
		}
	}
	//
	// Drop the callee frame and restore the saved caller state.
	p.dataStack.Free(calleeModule.Width())
	p.callStack = p.callStack[:len(p.callStack)-1]
	p.fid = frame.fid
	p.fp = frame.fp
	//
	return frame.returnPc, nil
}

func newModuleInfo(module Module) moduleInfo {
	var info moduleInfo
	//
	for _, reg := range module.Registers() {
		if reg.IsInput() {
			info.ninputs++
		}
		//
		if reg.IsOutput() {
			info.noutputs++
		}
	}
	//
	return info
}

// executeArithVec computes one logical arithmetic value and stores it across a
// vector target using the same low-limb-first rule as the word machine.
func (p *Interpreter[W]) executeArithVec(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		op, targets, sources, constant, n = decodeArith_vec[W](pc, codes)
		val                               W
	)
	//
	switch op {
	case arithop_ADD:
		val = constant
		//
		for _, src := range sources {
			var overflow bool
			//
			val, overflow = val.Add(stack[src])
			//
			if overflow {
				return pc, errors.New("arithmetic overflow")
			}
		}
	case arithop_SUB:
		var underflow bool
		//
		for i, src := range sources {
			// The first source seeds the value; the rest are subtracted from it.
			if i == 0 {
				val = stack[src]
			} else if val, underflow = val.Sub(stack[src]); underflow {
				return pc, errors.New("arithmetic underflow")
			}
		}
		//
		if val, underflow = val.Sub(constant); underflow {
			return pc, errors.New("arithmetic underflow")
		}
	case arithop_MUL:
		var overflow bool
		//
		val = constant
		//
		for _, src := range sources {
			var of bool
			//
			val, of = val.Mul(stack[src])
			overflow = overflow || of
		}
		//
		// A zero result is exact even when an intermediate product overflowed
		// (matches executeMul in the slow word machine).
		if overflow && val.Cmp64(0) != 0 {
			return pc, errors.New("arithmetic overflow")
		}
	default:
		panic("unknown arithmetic operation")
	}
	//
	return pc + n, storeAcross(p.program.Module(p.fid), targets, val, stack)
}

// executeCat matches executeConcat in the slow word machine.
func (p *Interpreter[W]) executeCat(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		targets, sources, n = decodeCatOperands(pc, codes)
		module              = p.program.Module(p.fid)
		val                 W
	)
	//
	for i := len(sources); i > 0; i-- {
		var (
			reg   = sources[i-1]
			width = bitwidthOf(module, reg)
		)
		//
		val = val.Shl64(uint64(width))
		// Sources[0] occupies the least-significant bits of the result.
		val = val.Or(stack[reg])
	}
	//
	return pc + n, storeAcross(module, targets, val, stack)
}

// executeNot computes a bitwise complement within the encoded mask width.
func (p *Interpreter[W]) executeNot(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rd, rs, bitwidth, n = decodeNot_1n1(pc, codes)
		val                 = stack[rs].Not(uint(bitwidth))
	)
	//
	return pc + n, storeReg(p.program.Module(p.fid), rd, val, stack)
}

// executeBitwise computes a binary bitwise operation (AND, OR or XOR).
func (p *Interpreter[W]) executeBitwise(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		op, rd, lhs, rhs, n = decodeBitwise_2n1(pc, codes)
		val                 W
	)
	//
	switch op {
	case AND:
		val = stack[lhs].And(stack[rhs])
	case OR:
		val = stack[lhs].Or(stack[rhs])
	case XOR:
		val = stack[lhs].Xor(stack[rhs])
	default:
		panic("unknown bitwise operation")
	}
	//
	return pc + n, storeReg(p.program.Module(p.fid), rd, val, stack)
}

// executeShift shifts a value left (SHL, masked to the encoded width) or right
// (SHR) by the amount held in a register.
func (p *Interpreter[W]) executeShift(pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		op, rd, rs, amt, bitwidth, n = decodeShift_2n1(pc, codes)
		val                          W
	)
	//
	switch op {
	case SHL:
		val = stack[rs].Shl(uint(bitwidth), stack[amt])
	case SHR:
		val = stack[rs].Shr(stack[amt])
	default:
		panic("unknown shift operation")
	}
	//
	return pc + n, storeReg(p.program.Module(p.fid), rd, val, stack)
}

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

// executeAdd_1n1c implements ADDC: stack[rd] = stack[rs] + constant.
func executeAdd_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = decodeArith_1n1c[W](pc, codes)
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

// executeMul_1n1c implements MULC: stack[rd] = stack[rs] * constant.
func executeMul_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = decodeArith_1n1c[W](pc, codes)
		val                 = stack[rs]
		res, overflow       = val.Mul(constant)
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

// executeSub_1n1c implements SUBC: stack[rd] = stack[rs] - constant.
func executeSub_1n1c[W word.Word[W]](pc uint32, codes []uint32, stack []W) (uint32, error) {
	var (
		rs, rd, constant, n = decodeArith_1n1c[W](pc, codes)
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

func bitwidthOf(module Module, reg Reg) uint {
	var r = module.Register(register.NewId(uint(reg)))
	//
	if r.IsNative() {
		return math.MaxUint
	}
	//
	return r.Width()
}

func storeReg[W word.Word[W]](module Module, target Reg, value W, stack []W) error {
	var bitwidth = bitwidthOf(module, target)
	//
	if !value.FitsWithin(bitwidth) {
		return fmt.Errorf("bit overflow (0x%s not u%d)", value.Text(16), bitwidth)
	}
	//
	stack[target] = value
	//
	return nil
}

func storeAcross[W word.Word[W]](module Module, targets []Reg, value W, stack []W) error {
	if len(targets) == 1 {
		return storeReg(module, targets[0], value, stack)
	}
	//
	var bitwidth uint
	//
	for _, target := range targets {
		var width = bitwidthOf(module, target)
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
