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
package micro

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
)

// Code provides an abstract notion of an atomic "machine operation", where a
// single instruction is comprised of multiple such microcodes.  To ensure
// efficiency, we want to pack as many microcodes into each instruction as we
// can.  However, there are restrictions here meaning we cannot pack arbitrarily
// many microcodes into a single instruction.  For example, we cannot pack two
// microcodes together which have conflicting writes (i.e. both write to the
// same register).
type Code interface {
	// Clone this instruction
	Clone() Code
	// Execute a given micro-code, using a given local state.  This may update
	// the register values, and returns either the number of micro-codes to
	// "skip over" when executing the enclosing instruction or, if skip==0, a
	// destination program counter (which can signal return of enclosing
	// function).
	MicroExecute(state io.State) (skip uint, pc uint)
	// Registers returns the set of registers read this micro instruction.
	RegistersRead() []io.RegisterId
	// Registers returns the set of registers written by this micro instruction.
	RegistersWritten() []io.RegisterId
	// Produce a suitable string representation of this instruction.  This is
	// primarily used for debugging.
	String(register.Map) string
	// Split this micro code using registers of arbirary width into one or more
	// micro codes using registers of a fixed maximum width.
	Split(mapping register.LimbsMap, env agnostic.RegisterAllocator) []Code
	// Validate that this instruction is well-formed.  For example, that it is
	// balanced, that there are no conflicting writes, that all temporaries have
	// been allocated, etc.  The maximum bit capacity of the underlying field is
	// needed for this calculation, so as to allow an instruction to check it
	// does not overflow the underlying field.
	Validate(fieldWidth uint, fn register.Map) error
}

// Instruction represents the composition of one or more micro instructions
// which are to be executed "in parallel".  This roughly following the ideas of
// vector machines and vectorisation.  In order to ensure parallel execution is
// safe, there are restrictions on how microcodes can be combined.  For example,
// two microcodes writing to the same register are said to be "conflicting" and,
// hence, this is not permitted.  Likewise, it is not possible to branch into
// the middle of a microinstruction.
type Instruction struct {
	Codes []Code
}

// NewInstruction constructs a new instruction from a given set of micro-codes.
func NewInstruction(codes ...Code) Instruction {
	return Instruction{codes}
}

// Terminal checks whether or not this instruction can result in a return from
// the enclosing function.  That is, whether or not this instruction contains a
// "ret" micro-code.
func (p Instruction) Terminal() bool {
	for _, c := range p.Codes {
		if _, ok := c.(*Ret); ok {
			return true
		}
	}
	//
	return false
}

// Execute this instruction with the given local and global state.  The next
// program counter position is returned, or io.RETURN if the enclosing
// function has terminated (i.e. because a return instruction was
// encountered).
func (p Instruction) Execute(state io.State) uint {
	var (
		skip uint = 1
		pc   uint
	)
	//
	for cc := uint(0); skip != 0; {
		// Decode next micro-code
		code := p.Codes[cc]
		// Execut micro-code
		skip, pc = code.MicroExecute(state)
		// Skip as requested
		cc += skip
	}
	//
	return pc
}

// JumpTargets returns the set of all jump targets used within this instruction.
// This is relatively easy to determine simply by looking for jmp codes.
func (p Instruction) JumpTargets() []uint {
	var targets []uint
	//
	for _, code := range p.Codes {
		if jmp, ok := code.(*Jmp); ok {
			targets = append(targets, jmp.Target)
		}
	}
	//
	return targets
}

// LastJump returns the index of the right-most jmp instruction (or false if
// none exists). This is relatively easy to determine simply by looking for jmp
// codes.
func (p Instruction) LastJump(n uint) (uint, bool) {
	//
	for i := n; i > 0; {
		i = i - 1
		//
		if _, ok := p.Codes[i].(*Jmp); ok {
			return i, true
		}
	}
	//
	return 0, false
}

// Registers returns the set of registers read/written by this instruction.
func (p Instruction) Registers() []io.RegisterId {
	return append(p.RegistersRead(), p.RegistersWritten()...)
}

// RegistersRead returns the set of registers read by this instruction.
func (p Instruction) RegistersRead() []io.RegisterId {
	var (
		regs bit.Set
		read []io.RegisterId
	)
	//
	for _, c := range p.Codes {
		for _, id := range c.RegistersRead() {
			if !regs.Contains(id.Unwrap()) {
				regs.Insert(id.Unwrap())
				read = append(read, id)
			}
		}
	}
	//
	return read
}

// RegistersWritten returns the set of registers written by this instruction.
func (p Instruction) RegistersWritten() []io.RegisterId {
	var (
		regs    bit.Set
		written []io.RegisterId
	)
	//
	for _, c := range p.Codes {
		for _, id := range c.RegistersWritten() {
			if !regs.Contains(id.Unwrap()) {
				regs.Insert(id.Unwrap())
				written = append(written, id)
			}
		}
	}
	//
	return written
}

// SplitRegisters implementation for the SplittableInstruction interface.  A key
// challenge for this method is the correct handling of skip instructions.
// Specifically, the targets for a skip change as the number of instructions
// increase.
func (p Instruction) SplitRegisters(mapping register.LimbsMap, env agnostic.RegisterAllocator) Instruction {
	var (
		ncodes  []Code
		packets [][]Code = make([][]Code, len(p.Codes))
		targets []uint   = make([]uint, len(p.Codes))
		index   uint
	)
	// Split micro-codes whilst retaining original indices.
	for i, code := range p.Codes {
		packets[i] = code.Split(mapping, env)
	}
	// Construct mapping
	for i := range targets {
		targets[i] = index
		index += uint(len(packets[i]))
	}
	// Finalise skip targets
	for i, packet := range packets {
		for j, c := range packet {
			c = retargetInsn(uint(i), uint(j), uint(len(packet)), c, targets)
			ncodes = append(ncodes, c)
		}
	}
	//
	return Instruction{Codes: ncodes}
}

func (p Instruction) String(fn register.Map) string {
	var builder strings.Builder
	//
	for i, code := range p.Codes {
		if i != 0 {
			builder.WriteString(" ; ")
		}
		//
		builder.WriteString(code.String(fn))
	}
	//
	return builder.String()
}

// Validate that this micro-instruction is well-formed.  For example, each
// micro-instruction contained within must be well-formed, and the overall
// requirements for a vector instruction must be met, etc.
func (p Instruction) Validate(fieldWidth uint, fn register.Map) error {
	// Construct write map
	var (
		nCodes   = uint(len(p.Codes))
		writeMap = p.Writes()
	)
	// Validate individual instructions
	for _, r := range p.Codes {
		if err := r.Validate(fieldWidth, fn); err != nil {
			return err
		}
	}
	// Validate no Read/Write conflicts
	for i := range nCodes {
		var (
			ithState = writeMap.StateOf(i)
			ith      = p.Codes[i]
		)
		// Sanity check for conflicting reads
		for _, r := range ith.RegistersRead() {
			if ithState.MaybeAssigned(r) && !ithState.DefinitelyAssigned(r) {
				name := fn.Register(r).Name()
				return fmt.Errorf("conflicting read on register \"%s\" in \"%s\"", name, ith.String(fn))
			}
		}
		// Sanity check conflicting writes
		for _, r := range ith.RegistersWritten() {
			if ithState.MaybeAssigned(r) {
				name := fn.Register(r).Name()
				return fmt.Errorf("conflicting write on register \"%s\" in \"%s\"", name, ith.String(fn))
			}
		}
	}
	// Done
	return nil
}

// Writes constructs the write map for this micro instruction.
func (p Instruction) Writes() dfa.Result[dfa.Writes] {
	return dfa.Construct(dfa.Writes{}, p.Codes, writeDfaTransfer)
}

// BranchTable constructs a per-micro-code slice giving, for each constituent
// micro-code, the condition under which control reaches it.  The condition is
// expressed in terms of the values of registers as observed _on entry to the
// vector instruction_: it is a disjunction (over execution paths) of
// conjunctions (the SkipIf comparisons taken — or not taken — along the path).
// The entry micro-code therefore always has condition TRUE, while later
// micro-codes accumulate the SkipIf guards of the path that reaches them.
//
// The table is built by a forward data-flow analysis over the micro-codes.
// Unconditional terminators (Fail, Ret, Jmp) propagate nothing; an
// unconditional Skip propagates the incoming condition to its target; SkipIf
// splits into the "skip taken" condition (joined into the skip target) and the
// "skip not taken" condition (joined into the following micro-code).  Where
// multiple paths converge on the same micro-code, their conditions are
// or-joined.
//
// The pre-computed write map is needed because the conditions reference
// registers which may have been freshly written earlier in the same vector;
// each BranchGroupId records whether forwarding is in effect for that
// reference, which is determined by consulting the write state at the SkipIf's
// offset.
func (p Instruction) BranchTable() dfa.Result[dfa.Branch] {
	// Construct suitable branch table for this instruction vector.
	var btf = branchTableTransfer(p.Writes())
	//
	return dfa.Construct(dfa.Branch{Condition: dfa.TRUE}, p.Codes, btf)
}

// ============================================================================
// Helpers
// ============================================================================

func branchTableTransfer(writeMap dfa.Result[dfa.Writes]) dfa.BranchTransferFunction[Code] {
	return func(offset uint, code Code, state dfa.Branch) []dfa.Transfer[dfa.Branch] {
		var (
			arcs   []dfa.Transfer[dfa.Branch]
			writes = writeMap.StateOf(offset)
		)
		//
		switch code := code.(type) {
		case *Fail, *Ret, *Jmp:
			return nil
		case *Skip:
			// join into branch target
			return append(arcs, dfa.NewTransfer(state, offset+code.Skip+1))
		case *SkipIf:
			var (
				// Determine true branch
				trueBranch = extendSkipIf(state, true, code, writes)
				// Determine false branch
				falseBranch = extendSkipIf(state, false, code, writes)
			)
			// join into branch target
			arcs = append(arcs, dfa.NewTransfer(trueBranch, offset+code.Skip+1))
			// join into following instruction
			return append(arcs, dfa.NewTransfer(falseBranch, offset+1))
		}
		// Transfer to following instruction
		return append(arcs, dfa.NewTransfer(state, offset+1))
	}
}

func extendSkipIf(tail dfa.Branch, sign bool, code *SkipIf, writes dfa.Writes) dfa.Branch {
	var (
		head      dfa.BranchEquality
		rightUsed = code.Right.HasFirst()
		tailc     = tail.Condition
		left      = dfa.NewBranchId(writes.MayAnybeAssigned(code.Left.Registers()...), code.Left.Registers()...)
	)
	//
	switch {
	case !sign && rightUsed:
		right := code.Right.First()
		head = logical.Equals(left, dfa.NewBranchId(writes.MayAnybeAssigned(right.Registers()...), right.Registers()...))
	case !sign && !rightUsed:
		head = logical.EqualsConst(left, code.Right.Second())
	case sign && rightUsed:
		right := code.Right.First()
		head = logical.NotEquals(left, dfa.NewBranchId(writes.MayAnybeAssigned(right.Registers()...), right.Registers()...))
	case sign && !rightUsed:
		head = logical.NotEqualsConst(left, code.Right.Second())
	}
	// NOTE: the reason this method is needed is because we have no implicit
	// rerpesentation of logical truth or falsehood.  This means an empty path
	// does not behave in the expected manner.
	if len(tailc.Conjuncts()) == 0 {
		return dfa.Branch{Condition: logical.NewProposition(head)}
	}
	//
	return dfa.Branch{Condition: tailc.And(logical.NewProposition(head))}
}

// Data-flow transfer function for the writes analysis
func writeDfaTransfer(offset uint, code Code, state dfa.Writes) []dfa.Transfer[dfa.Writes] {
	var arcs []dfa.Transfer[dfa.Writes]
	//
	switch code := code.(type) {
	case *Fail, *Ret, *Jmp:
		return nil
	case *Skip:
		// join into branch target
		return append(arcs, dfa.NewTransfer(state, offset+code.Skip+1))
	case *SkipIf:
		// join into branch target
		arcs = append(arcs, dfa.NewTransfer(state, offset+code.Skip+1))
		// fall through
	}
	// Construct state after this code
	nState := state.Write(code.RegistersWritten()...)
	// Transfer to following instruction
	arcs = append(arcs, dfa.NewTransfer(nState, offset+1))
	// Done
	return arcs
}

func retargetInsn(oldIndex uint, pktIndex, pktSize uint, code Code, mapping []uint) Code {
	var (
		newIndex     = mapping[oldIndex] + pktIndex
		leftInPacket = pktSize - pktIndex - 1
	)
	// Check whether this is an internal skip instruction (or not)
	switch c := code.(type) {
	case *Skip:
		if c.Skip < leftInPacket {
			return c
		}
	case *SkipIf:
		if c.Skip < leftInPacket {
			return c
		}
	}
	// Retarget skip instructions
	switch c := code.(type) {
	case *Skip:
		// Determine true skip target
		target := oldIndex + 1 + (c.Skip - leftInPacket)
		// Determine new location of skip target
		nTarget := mapping[target]
		//
		return &Skip{Skip: nTarget - newIndex - 1}
	case *SkipIf:
		// Determine true skip target
		target := oldIndex + 1 + (c.Skip - leftInPacket)
		// Determine new location of skip target
		nTarget := mapping[target]
		//
		return &SkipIf{Left: c.Left,
			Right: c.Right,
			Skip:  nTarget - newIndex - 1,
		}
	default:
		return code
	}
}
