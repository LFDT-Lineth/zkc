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
package constraints

import (
	"fmt"
	"reflect"
	"slices"

	mirc "github.com/consensys/go-corset/pkg/asm/compiler"
	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/asm/io/micro/dfa"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
)

// Expr is a useful alias for an MIR expression
type Expr[F field.Element[F]] = mirc.MirExpr[F]

// Module is a useful alias for an MIR module.
type Module[F field.Element[F]] = mirc.MirModule[F]

// Framing is a useful alias
type Framing[F field.Element[F]] = mirc.Framing[register.Id, Expr[F]]

// RegisterReader is a conenvient alias
type RegisterReader[F field.Element[F]] = mirc.RegisterReader[Expr[F]]

// VectorInsnTranslator encapsulates general information related to the mapping from
// an instruction vector into to MIR constraints.
type VectorInsnTranslator[F field.Element[F]] struct {
	context     schema.ModuleId
	pc          uint
	vec         vm.Vector[vm.FieldInstruction]
	registers   []register.Register
	writeMap    dfa.Result[dfa.Writes]
	branchTable dfa.Result[dfa.Branch]
	framing     Framing[F]
}

// NewVectorTranslator constructs a translator for a specific vector
// instruction.
func NewVectorTranslator[F field.Element[F]](ctx schema.ModuleId, pc uint,
	vec vm.Vector[vm.FieldInstruction], framing Framing[F], registers []register.Register) VectorInsnTranslator[F] {
	// generate writeMap & branch table
	writeMap, branchTable := vec.BranchTable()
	//
	return VectorInsnTranslator[F]{
		ctx, pc, vec, registers, writeMap, branchTable, framing,
	}
}

func (p *VectorInsnTranslator[F]) translate() Expr[F] {
	//
	var (
		constraint = mirc.True[register.Id, Expr[F]]()
		//
		nCodes = uint(len(p.vec.Codes))
		// Assignments determines whether the given instruction definitely
		// assignments, may assign or does not assign any given registers.  This
		// is necessary to apply constancy information.
		assignments util.Option[dfa.Writes]
	)
	//
	for cc := range nCodes {
		//
		var (
			localWrites = p.writeMap.StateOf(cc)
			local       Expr[F]
		)
		//
		switch c := p.vec.Codes[cc].(type) {
		case *instruction.Debug:
			// no-operation
			continue
		case *instruction.Call, *instruction.MemRead, *instruction.MemWrite:
			// TODO: these need to be implemented as assignments to their
			// respected selector line (i.e. to enable the conditional lookup).
			continue
		case *instruction.Fail:
			assignments = joinAssignments(assignments, localWrites)
			local = mirc.False[register.Id, Expr[F]]()
		case *instruction.Jump:
			assignments = joinAssignments(assignments, localWrites)
			local = p.framing.Goto(c.Immediate)
		case *instruction.FieldAssign[F]:
			var (
				// construct instruction translator
				it = InstructionTranslator[F]{p, localWrites}
			)
			// translate assignment instruction
			local = it.translateAssignment(*c)
		case *instruction.Return:
			assignments = joinAssignments(assignments, localWrites)
			local = p.framing.Return()
		case *instruction.FieldHint:
			// Non-deterministic assignment: the target registers are already
			// recorded in the write map for constancy analysis; no polynomial
			// constraint is generated here, since correctness is enforced by
			// subsequent arithmetic checks.
			continue
		case *instruction.SkipIf, *instruction.Skip:
			// do nothing
			continue
		default:
			var t = reflect.TypeOf(c)
			panic(fmt.Sprintf("unexpected instruction (%s)", t.String()))
		}
		//
		condition := mirc.TranslateBranchCondition(p.branchTable.StateOf(cc).Condition, p)
		// Add control-flow requirements
		local = mirc.If(condition, local)
		// Include local constraint
		constraint = constraint.And(local)
	}
	// Apply constancies constraints (for all except first instruction)
	if p.pc > 0 {
		constraint = p.WithConstancyConstraints(assignments.Unwrap(), constraint)
	}
	// Add framing guards
	return mirc.If(p.framing.Guard(p.pc), constraint)
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
func (p *VectorInsnTranslator[F]) WithConstancyConstraints(writes dfa.Writes, condition Expr[F]) Expr[F] {
	//
	for i, reg := range p.registers {
		var (
			regId = register.NewId(uint(i))
			// Value of register on this row of the trace.
			r_i = mirc.Variable[register.Id, Expr[F]](regId, reg.Width(), 0)
			// Value of register on previous row of the trace.
			r_im1 = mirc.Variable[register.Id, Expr[F]](regId, reg.Width(), -1)
		)
		//
		if reg.IsInput() {
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
			wCondition := determineWriteConditions(regId, p.branchTable, p.vec.Codes)
			// Next, negate condition to determine when it is **not** assigned
			wCondition = wCondition.Negate()
			// Finally translate condition and include constancy constraint
			condition = condition.And(mirc.If(mirc.TranslateBranchCondition(wCondition, p), r_i.Equals(r_im1)))
		}
	}
	//
	return condition
}

// RegisterWidths implementation for RegisterReader interface
func (p *VectorInsnTranslator[F]) RegisterWidths(regs ...io.RegisterId) []uint {
	var widths = make([]uint, len(regs))
	//
	for i, r := range regs {
		widths[i] = p.Register(r).Width()
	}
	//
	return widths
}

// ReadRegister constructs a suitable accessor for referring to a given register.
// This applies forwarding as appropriate.
func (p *VectorInsnTranslator[F]) ReadRegister(regId register.Id, forwarding bool) Expr[F] {
	var (
		reg = p.Register(regId)
	)
	//
	if reg.IsInput() {
		// Inputs don't need to refer back
		return mirc.Variable[register.Id, Expr[F]](regId, reg.Width(), 0)
	} else if forwarding {
		// Forwarded
		return mirc.Variable[register.Id, Expr[F]](regId, reg.Width(), 0)
	}
	// Not forwarded
	return mirc.Variable[register.Id, Expr[F]](regId, reg.Width(), -1)
}

// Register implementation for RegisterReader interface
func (p *VectorInsnTranslator[F]) Register(reg register.Id) register.Register {
	return p.registers[reg.Unwrap()]
}

// nolint
func (p *VectorInsnTranslator[F]) debugString(condition Expr[F]) string {
	return condition.String(func(r register.Id) string { return p.Register(r).Name() })
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
func determineWriteConditions(reg register.Id, branchTable dfa.Result[dfa.Branch], codes []vm.FieldInstruction,
) dfa.BranchCondition {
	//
	var condition = dfa.FALSE
	//
	for i, c := range codes {
		if slices.Contains(c.Definitions(), reg) {
			condition = condition.Or(branchTable.StateOf(uint(i)).Condition)
		}
	}
	//
	return condition
}
