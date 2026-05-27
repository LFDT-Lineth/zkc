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
package compiler

import (
	"math/big"
	"slices"

	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/asm/io/micro"
	"github.com/consensys/go-corset/pkg/asm/io/micro/dfa"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
)

// RegisterReader is a simplified view of a translator which is suitable for
// reading registers only.
type RegisterReader[E any] interface {
	// Register returns information about a given register
	Register(io.RegisterId) io.Register
	// RegisterWidths returns the bitwidth of a given set of registers.
	RegisterWidths(reg ...io.RegisterId) []uint
	// ReadRegister constructs a suitable accessor for referring to a given register.
	// This applies forwarding as appropriate.
	ReadRegister(reg io.RegisterId, forwarding bool) E
}

// Translator encapsulates general information related to the mapping from
// instructions down to constraints.
type Translator[F field.Element[F], T any, E Expr[T, E], M Module[F, T, E, M]] struct {
	// Framining identifies any required control lines.
	Framing Framing[T, E]
	// Registers of the given machine
	Registers []io.Register
	// ioLines identifies which registers are used purely for buses.  This is
	// because these registers can be treated in a more relaxed fashion than
	// other registers.
	ioLines bit.Set
	// Mapping from registers to column IDs in the underlying constraint system.
	Columns []T
}

// Translate a micro instruction at a given Program Counter value into a given
// constraint.
func (p *Translator[F, T, E, M]) Translate(pc uint, insn micro.Instruction) E {
	var (
		nCodes      = uint(len(insn.Codes))
		writes      = insn.Writes()
		branchTable = insn.BranchTable()
		constraint  = True[T, E]()
		// Assignments determines whether the given instruction definitely
		// assignments, may assign or does not assign any given registers.  This
		// is necessary to apply constancy information.
		assignments util.Option[dfa.Writes]
	)
	//
	for cc := uint(0); cc < nCodes; cc++ {
		var (
			localWrites = writes.StateOf(cc)
			local       E
		)
		//
		switch c := insn.Codes[cc].(type) {
		case *micro.Assign:
			var str = StateTranslator[F, T, E, M]{*p, localWrites}
			//
			local = str.translateAssign(c)
		case *micro.Division, *micro.InOut:
			// do nothing
			continue
		case *micro.Fail:
			local = False[T, E]()
		case *micro.Jmp:
			assignments = joinAssignments(assignments, localWrites)
			local = p.Framing.Goto(c.Target)
		case *micro.Ret:
			assignments = joinAssignments(assignments, localWrites)
			local = p.Framing.Return()
		case *micro.SkipIf, *micro.Skip:
			// do nothing
			continue
		default:
			panic("unreachable")
		}
		// Add control-flow requirements
		local = If(TranslateBranchCondition(branchTable.StateOf(cc).Condition, p), local)
		// Include local constraint
		constraint = constraint.And(local)
	}
	// Apply constancies constraints (for all except first instruction)
	if pc > 0 {
		constraint = p.WithConstancyConstraints(assignments.Unwrap(), branchTable, insn, constraint)
	}
	// Add framing guards
	return If(p.Framing.Guard(pc), constraint)
}

// WithConstancyConstraints adds constancy constraints for all registers which
// are either not mutated at all by an instruction, or are sometimes mutated by
// an instruction.  Constancy constraints are required when the value of a
// register should be copied from the previous state into this state (i.e.
// because it was not changed by this instruction and, hence, must retain its
// original value).
//
// A key challenge lies with registers that are sometimes assigned by the
// instruction, and sometimes not assigned (i.e. maybe but not definitely
// assigned).  To resolve this we first determine the conditions under which
// they are assigned, and negate this to determine the conditions under which
// they are not assigned.
//
// NOTE: it is possible to further optimise this process by taking into account
// which registers are actually used (i.e. live) after this instruction.
func (p *Translator[F, T, E, M]) WithConstancyConstraints(writes dfa.Writes, branchTable dfa.Result[dfa.Branch],
	insn micro.Instruction, condition E) E {
	//
	for i, reg := range p.Registers {
		var (
			regId = register.NewId(uint(i))
			colId = p.Columns[i]
			// Value of register on this row of the trace.
			r_i = Variable[T, E](colId, reg.Width(), 0)
			// Value of register on previous row of the trace.
			r_im1 = Variable[T, E](colId, reg.Width(), -1)
		)
		//
		if reg.IsInput() || p.ioLines.Contains(uint(i)) {
			// inputs are given global constancy constraints elsewhere, whilst
			// I/O lines are never given constancy constraints (because they are
			// always assigned in place).
			continue
		} else if !writes.MaybeAssigned(regId) {
			// Register never mutated by this instruction, so always copy value
			// from previous row into this.
			condition = condition.And(r_i.Equals(r_im1))
		} else if !writes.DefinitelyAssigned(regId) {
			// Variable is sometimes (but not always) assigned by this
			// instruction.  This is the difficult case.  First determine
			// condition under which this register is assigned.
			wCondition := determineWriteConditions(regId, branchTable, insn)
			// Next, negate condition to determine when it is **not** assigned
			wCondition = wCondition.Negate()
			// Finally translate condition and include constancy constraint
			condition = condition.And(If(TranslateBranchCondition(wCondition, p), r_i.Equals(r_im1)))
		}
	}
	//
	return condition
}

// ReadRegister constructs a suitable accessor for referring to a given register.
// This applies forwarding as appropriate.
func (p *Translator[F, T, E, M]) ReadRegister(regId io.RegisterId, forwarding bool) E {
	var (
		reg   = p.Registers[regId.Unwrap()]
		colId = p.Columns[regId.Unwrap()]
	)
	//
	if reg.IsInput() {
		// Inputs don't need to refer back
		return Variable[T, E](colId, reg.Width(), 0)
	} else if forwarding {
		// Forwarded
		return Variable[T, E](colId, reg.Width(), 0)
	}
	// Not forwarded
	return Variable[T, E](colId, reg.Width(), -1)
}

// Register implementation for RegisterReader interface
func (p *Translator[F, T, E, M]) Register(reg io.RegisterId) io.Register {
	return p.Registers[reg.Unwrap()]
}

// RegisterWidths implementation for RegisterReader interface
func (p *Translator[F, T, E, M]) RegisterWidths(regs ...io.RegisterId) []uint {
	var widths = make([]uint, len(regs))
	//
	for i, r := range regs {
		widths[i] = p.Registers[r.Unwrap()].Width()
	}
	//
	return widths
}

// StateTranslator packages up key information regarding how an individual state
// of the machine is compiled down to the lower level.
type StateTranslator[F field.Element[F], T any, E Expr[T, E], M Module[F, T, E, M]] struct {
	mapping Translator[F, T, E, M]
	// Set of registers writes on the current branch.
	writes dfa.Writes
}

// WriteRegister constructs a suitable accessors for a register written by a
// given microinstruction.  This activates forwarding for that register for all
// states after this, and returns a suitable expression for the assignment.
func (p *StateTranslator[F, T, E, M]) WriteRegister(dst io.RegisterId) E {
	var (
		ith = p.mapping.Registers[dst.Unwrap()]
		lhs = Variable[T, E](p.mapping.Columns[dst.Unwrap()], ith.Width(), 0)
	)
	//
	return lhs
}

// WriteRegisters constructs suitable accessors for the those registers written
// by a given microinstruction.  This activates forwarding for those registers
// for all states after this, and returns suitable expressions for the
// assignment.
func (p *StateTranslator[F, T, E, M]) WriteRegisters(targets []io.RegisterId) []E {
	lhs := make([]E, len(targets))
	// build up the lhs
	for i, dst := range targets {
		var ith = p.mapping.Registers[dst.Unwrap()]

		lhs[i] = Variable[T, E](p.mapping.Columns[dst.Unwrap()], ith.Width(), 0)
	}
	//
	return lhs
}

// WriteAndShiftRegisters constructs suitable accessors for the those registers
// written by a given microinstruction, and also shifts them (i.e. so they can
// be combined in a sum).  This activates forwarding for those registers for all
// states after this, and returns suitable expressions for the assignment.
func (p *StateTranslator[F, T, E, M]) WriteAndShiftRegisters(targets []io.RegisterId) []E {
	lhs := make([]E, len(targets))
	offset := big.NewInt(1)
	// build up the lhs
	for i, dst := range targets {
		var ith = p.mapping.Registers[dst.Unwrap()]
		//
		lhs[i] = Variable[T, E](p.mapping.Columns[dst.Unwrap()], ith.Width(), 0)
		//
		if i != 0 {
			lhs[i] = BigNumber[T, E](offset).Multiply(lhs[i])
		}
		// left shift offset by given register width.
		offset.Lsh(offset, p.mapping.Registers[dst.Unwrap()].Width())
	}
	//
	return lhs
}

// ReadRegister constructs a suitable accessor for referring to a given register.
// This applies forwarding as appropriate.
func (p *StateTranslator[F, T, E, M]) ReadRegister(regId io.RegisterId) E {
	var (
		reg   = p.mapping.Registers[regId.Unwrap()]
		colId = p.mapping.Columns[regId.Unwrap()]
	)
	//
	if reg.IsInput() {
		// Inputs don't need to refer back
		return Variable[T, E](colId, reg.Width(), 0)
	} else if p.writes.MaybeAssigned(regId) {
		// Forwarded
		return Variable[T, E](colId, reg.Width(), 0)
	}
	// Not forwarded
	return Variable[T, E](colId, reg.Width(), -1)
}

// ReadRegisters constructs appropriate column accesses for a given set of
// registers.  When appropriate, forwarding will be applied automatically.
func (p *StateTranslator[F, T, E, M]) ReadRegisters(sources []io.RegisterId) []E {
	rhs := make([]E, len(sources))
	// build up the lhs
	for i, src := range sources {
		rhs[i] = p.ReadRegister(src)
	}
	//
	return rhs
}

func joinAssignments(lhs util.Option[dfa.Writes], rhs dfa.Writes) util.Option[dfa.Writes] {
	if lhs.HasValue() {
		return util.Some(lhs.Unwrap().Join(rhs))
	}
	//
	return util.Some(rhs)
}

// Determine the conditions under which an assignment to a given register can
// occur.  This is relatively straightforward to determine given the information
// already generated.  Specifically, we already have the entry condition
// required to execute every instruction.  Therefore, we just need to identify
// all instructions which can assign the given register and take the disjunction
// of all their entry conditions.
func determineWriteConditions(reg register.Id, branchTable dfa.Result[dfa.Branch], insn micro.Instruction,
) dfa.BranchCondition {
	//
	var condition = dfa.FALSE
	//
	for i, c := range insn.Codes {
		if slices.Contains(c.RegistersWritten(), reg) {
			condition = condition.Or(branchTable.StateOf(uint(i)).Condition)
		}
	}
	//
	return condition
}
