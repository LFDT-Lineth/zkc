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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

type Interpreter[W word.Word[W]] struct {
	program Program
	// Program counter position for the bytecode interpreter
	pc uint32
	// Frame pointer position for the bytecode interpreter
	fp uint
	// data stack
	dataStack heap.Heap[W]
	// call stack
	callStack heap.Heap[uint32]
	// read-only memories
	roms []memory.StaticArray[W]
	// write-once memories
	woms []memory.WriteOnce[W]
	// random-access memories
	rams []memory.RandomAccess[W]
	// bipartite random-access memories
	brams []memory.BiPartiteRandomAccess[W]
}

// NewInterpreter constructs a new bytecode interpreter for the given program.
func NewInterpreter[W word.Word[W]](program Program) *Interpreter[W] {
	var (
		roms  []memory.StaticArray[W]
		woms  []memory.WriteOnce[W]
		rams  []memory.RandomAccess[W]
		brams []memory.BiPartiteRandomAccess[W]
	)
	//
	for _, m := range program.modules {
		switch m := m.(type) {
		case *memory.ReadOnly[W]:
			roms = append(roms, m.StaticArray)
		case *memory.StaticReadOnly[W]:
			roms = append(roms, m.StaticArray)
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
		roms:    roms,
		woms:    woms,
		rams:    rams,
		brams:   brams,
	}
}

// Boot implementation of Core interface
func (p *Interpreter[W]) Boot(fun string) (err error) {
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
	//
	return err
}

// Inputs implementation of Core interface
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

// Outputs implementation of Core interface
func (p *Interpreter[W]) Outputs() iter.Iterator[memory.InputOutput[W]] {
	var outputs = make([]memory.InputOutput[W], len(p.woms))
	//
	for i := range p.woms {
		outputs[i] = &p.woms[i]
	}
	//
	return iter.NewArrayIterator(outputs)
}

// Execute implementation of Core interface
func (p *Interpreter[W]) Execute(steps uint) (uint, error) {
	var (
		nsteps    = uint(0)
		err       error
		bytecodes = p.program.bytecodes
	)
	//
	for nsteps < steps && err == nil {
		// decode instruction
		var insn = bytecodes[p.pc]
		// increase step counter
		nsteps++
		//
		switch insn & OPCODE_MASK {
		case LDC:
			p.pc = executeLdc_1(p.pc, p.fp, bytecodes, p.dataStack)
		case MOVE:
			p.pc = executeMove_1s1(p.pc, p.fp, bytecodes, p.dataStack)
		case FAIL:
			return nsteps, errors.New("machine panic")
		case RET:
			// check for termination
			if p.callStack.Size() == 0 {
				return nsteps, nil
			}
			//
			panic("todo")
		case JMP:
			p.pc, _ = decodeJmp1(p.pc, bytecodes)
		case JEQ_RR:
			p.pc = executeJeq_rr(p.pc, p.fp, bytecodes, p.dataStack)
		case JNE_RR:
			p.pc = executeJne_rr(p.pc, p.fp, bytecodes, p.dataStack)
		case JLT_RR:
			p.pc = executeJlt_rr(p.pc, p.fp, bytecodes, p.dataStack)
		case JGT_RR:
			p.pc = executeJgt_rr(p.pc, p.fp, bytecodes, p.dataStack)
		case JLE_RR:
			p.pc = executeJle_rr(p.pc, p.fp, bytecodes, p.dataStack)
		case JGE_RR:
			p.pc = executeJge_rr(p.pc, p.fp, bytecodes, p.dataStack)
		// Input / Output Operations
		case RD_ROM_N_M:
			p.pc = executeReadRom(p.pc, p.fp, bytecodes, p.dataStack, p.roms)
		case WR_WOM_N_M:
			panic("todo")
		case RD_RAM_N_M:
			panic("todo")
		case WR_RAM_N_M:
			panic("todo")
		// Arithmetic Operations
		case ADD:
			p.pc, err = executeAdd_2s1(p.pc, p.fp, bytecodes, p.dataStack)
		default:
			panic("unknown bytecode encountered")
		}
	}
	//
	return nsteps, nil
}

func executeAdd_2s1[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) (uint32, error) {
	var (
		rs0, rs1, rd, n = decodeAdd_2s1(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
		// Add v0 + v1
		res, carry = val0.Add(val1)
	)
	// Check for overflow
	if carry {
		return pc, errors.New("arithmetic overflow")
	}
	//
	stack.Set(fp+uint(rd), res)
	//
	return pc + n, nil
}

func executeJeq_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) == 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeJne_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) != 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeJlt_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) < 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeJgt_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) > 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeJle_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) <= 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeJge_rr[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		npc, rs0, rs1, _, n = decodeJif_rr(pc, codes)
		// Read rs0
		val0 = stack.Get(fp + uint(rs0))
		// Read rs1
		val1 = stack.Get(fp + uint(rs1))
	)
	//
	if val0.Cmp(val1) >= 0 {
		// true branch
		return npc
	}
	// false branch
	return pc + n
}

func executeMove_1s1[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	var (
		rs, rd, n = decodeMove_1s1(pc, codes)
		// Read rs
		val = stack.Get(fp + uint(rs))
	)
	// Write rd
	stack.Set(fp+uint(rd), val)
	//
	return pc + n
}

func executeLdc_1[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W]) uint32 {
	c, rd, n := decodeLdc_1[W](pc, codes)
	stack.Set(fp+uint(rd), c)
	//
	return pc + n
}

func executeReadRom[W word.Word[W]](pc uint32, fp uint, codes []uint32, stack heap.Heap[W],
	roms []memory.StaticArray[W]) uint32 {
	//
	var (
		_, id, addr, data, n = decodeReadWrite_sn(pc, codes)
		rom                  = &roms[id]
		address              = stack.Get(fp + uint(addr[0])).Uint64()
	)
	// sanity check for now
	if len(addr) != 1 {
		panic("todo")
	}
	//
	address = (address * uint64(rom.Geometry().DataLines()))
	//
	for i := range data {
		//nolint
		val, _ := rom.Read(address)
		//
		stack.Set(fp+uint(data[i]), val)
		//
		address++
	}
	//
	return pc + n
}
