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
	"math"

	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
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
// a bytecode vector into to MIR constraints.
type VectorInsnTranslator[W vm.Word[W], F field.Element[F]] struct {
	context     schema.ModuleId
	pc          uint
	vec         vm.BytecodeVector[W]
	enclosing   *vm.Function[W]
	writeMap    dfa.Result[dfa.Writes]
	branchTable dfa.Result[dfa.Path[W]]
	framing     Framing[F]
	// oneHot holds the one-hot register groups declared by the vector's
	// Dispatch bytecodes, used to shorten branch conditions at translation.
	oneHot []oneHotGroup
}

// NewVectorTranslator constructs a translator for a specific bytecode vector.
func NewVectorTranslator[W vm.Word[W], F field.Element[F]](ctx schema.ModuleId, pc uint,
	vec vm.BytecodeVector[W], framing Framing[F], enclosing *vm.Function[W],
	field field.Config) VectorInsnTranslator[W, F] {
	//
	// generate writeMap & branch table
	writeMap, branchTable := vec.BranchTable(field.RegisterWidth)
	//
	return VectorInsnTranslator[W, F]{
		ctx, pc, vec, enclosing, writeMap, branchTable, framing,
		collectOneHotGroups(vec.Bytecodes),
	}
}

func (p *VectorInsnTranslator[W, F]) translate() Expr[F] {
	//
	var (
		constraint = mirc.True[register.Id, Expr[F]]()
		//
		nCodes = uint(len(p.vec.Bytecodes))
		// Assignments determines whether the given bytecode definitely
		// assigns, may assign or does not assign any given registers.  This
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
		switch c := p.vec.Bytecodes[cc].(type) {
		case *vm.BytecodeDebug[W]:
			// no-operation
			continue
		case *vm.BytecodeCall[W], *vm.BytecodeReadWrite[W]:
			// Translation of calls, and memory read/write is done at the function
			// level (see addLookups), as it modifies the module itself (adding
			// source selectors), requires knowledge of target modules, etc.
			continue
		case *vm.BytecodeCheckCast[W]:
			// Width checks are enforced by the range-proof constraints emitted for
			// each register, so a cast check needs no constraint of its own.  This
			// mirrors the bytecode→word decompiler, which drops casts entirely.
			continue
		case *vm.BytecodeFail[W]:
			assignments = joinAssignments(assignments, localWrites)
			local = mirc.False[register.Id, Expr[F]]()
		case *vm.BytecodeJmp[W]:
			assignments = joinAssignments(assignments, localWrites)
			local = p.framing.Goto(uint(c.Target))
		case *vm.BytecodeArith[W]:
			it := InstructionTranslator[W, F]{p, localWrites}
			//
			switch c.Op {
			case vm.OP_ADD:
				local = it.translateAdd(c.Target, c.Source, c.Constant)
			case vm.OP_MUL:
				local = it.translateMul(c.Target, c.Source, c.Constant)
			case vm.OP_SUB:
				var (
					// Determine whether this is a subtract with borrow
					isSbb        = vm.IsSubtractWithBorrow(*c, p.enclosing)
					hasSignedBit = hasSignedBit(c.Target, p.enclosing)
				)
				//
				if isSbb && hasSignedBit {
					local = it.translateSignedSub(c.Target, c.Source, c.Constant)
				} else if isSbb {
					panic("subtract with borrow bytecode missing sign bit")
				} else {
					local = it.translateSub(c.Target, c.Source, c.Constant)
				}
			default:
				panic("unknown arithmetic operation")
			}
			// translate integer arithmetic assignment

		case *vm.BytecodeFieldArith[W]:
			it := InstructionTranslator[W, F]{p, localWrites}

			switch c.Op {
			case vm.OP_ADDMOD_P:
				local = it.translateFieldAdd(c.Target, c.Sources, c.Constant)
			case vm.OP_SUBMOD_P:
				local = it.translateFieldSub(c.Target, c.Sources, c.Constant)
			case vm.OP_MULMOD_P:
				local = it.translateFieldMul(c.Target, c.Sources, c.Constant)
			default:
				panic("unknown field operation")
			}
		case *vm.BytecodeCat[W]:
			it := InstructionTranslator[W, F]{p, localWrites}
			// translate concatenation assignment
			local = it.translateConcat(c.Targets, c.Sources, p.sourceWidths(c.Sources))
		case *vm.BytecodeUintToField[W]:
			it := InstructionTranslator[W, F]{p, localWrites}
			// uint→𝔽: the native target equals the assembled sources modulo P
			// (field equality is modulo P, so no explicit reduction is needed).
			local = it.translateConcat([]vm.RegisterId{c.Target}, c.Source, p.sourceWidths(c.Source))
		case *vm.BytecodeFieldToUint[W]:
			it := InstructionTranslator[W, F]{p, localWrites}
			// 𝔽→uint: the assembled target limbs equal the native source.
			local = it.translateConcat(c.Target, []vm.RegisterId{c.Source},
				p.sourceWidths([]vm.RegisterId{c.Source}))
		case *vm.BytecodeRet[W]:
			assignments = joinAssignments(assignments, localWrites)
			local = p.framing.Return()
		case *vm.BytecodeIntrinsic[W]:
			// Non-deterministic assignment: the target registers are already
			// recorded in the write map for constancy analysis; no polynomial
			// constraint is generated here, since correctness is enforced by
			// subsequent arithmetic checks.
			continue
		case *vm.BytecodeSkipIf[W], *vm.BytecodeSkip[W], *vm.BytecodeDispatch[W]:
			// control flow is captured via the branch table; no constraint here
			continue
		case *vm.BytecodeSwitch[W]:
			// Switch bytecodes are rewritten by LowerSwitch when compiling
			// the bci code; one surviving to this point indicates a broken
			// transform pipeline.
			panic("unlowered switch bytecode reached constraint translation")
		default:
			panic(fmt.Sprintf("unexpected bytecode (%T)", c))
		}
		//
		condition := TranslateBranchCondition(p.branchTable.StateOf(cc), p.oneHot, p)
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
// are either not mutated at all by a bytecode, or are sometimes mutated by
// a bytecode.  Constancy constraints are required when the value of a
// register should be copied from the previous state into this state (i.e.
// because it was not changed by this bytecode and, hence, must retain its
// original value).
//
// A key challenge lies with registers that are sometimes assigned by the
// bytecode, and sometimes not assigned (i.e. maybe but not definitely
// assigned).  To resolve this we first determine the conditions under which
// they are assigned, and negate this to determine the conditions under which
// they are not assigned.
//
// NOTE: it is possible to further optimise this process by taking into account
// which registers are actually used (i.e. live) after this instruction.
func (p *VectorInsnTranslator[W, F]) WithConstancyConstraints(writes dfa.Writes, condition Expr[F]) Expr[F] {
	//
	for i, reg := range p.enclosing.Registers() {
		var (
			regId = register.NewId(uint(i))
			// Value of register on this row of the trace.
			r_i = mirc.Variable[register.Id, Expr[F]](regId, reg.Bitwidth().UnwrapOr(math.MaxUint), 0)
			// Value of register on previous row of the trace.
			r_im1 = mirc.Variable[register.Id, Expr[F]](regId, reg.Bitwidth().UnwrapOr(math.MaxUint), -1)
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
			// instruction.  This is the difficult case.  Determine
			// condition under which this register is not assigned.
			wCondition := p.determineConstancyCondition(regId, p.branchTable, p.vec.Bytecodes)
			// Finally translate condition and include constancy constraint
			condition = condition.And(mirc.If(wCondition, r_i.Equals(r_im1)))
		}
	}
	//
	return condition
}

// Determine the conditions under which an assignment to a given register can
// occur.  This is relatively straightforward to determine given the information
// already generated.  Specifically, we already have the entry condition
// required to execute every bytecode.  Therefore, we just need to identify
// all bytecodes which can assign the given register and take the disjunction
// of all their entry conditions.
func (p *VectorInsnTranslator[W, F]) determineConstancyCondition(reg register.Id, branchTable dfa.Result[dfa.Path[W]],
	codes []vm.Bytecode[W]) Expr[F] {
	//
	var condition = mirc.True[register.Id, Expr[F]]()
	//
	for i, c := range codes {
		if containsRegister(c.Definitions(), reg) {
			var (
				pathCondition = branchTable.StateOf(uint(i))
				nc            = TranslateNegatedBranchCondition(pathCondition, p.oneHot, p)
			)
			//
			condition = condition.And(nc)
		}
	}
	//
	return condition
}

// RegisterWidths implementation for RegisterReader interface
func (p *VectorInsnTranslator[W, F]) RegisterWidths(regs ...io.RegisterId) []uint {
	var widths = make([]uint, len(regs))
	//
	for i, r := range regs {
		widths[i] = p.Register(r).WidthOrNative()
	}
	//
	return widths
}

// ReadRegister constructs a suitable accessor for referring to a given register.
// This applies forwarding as appropriate.
func (p *VectorInsnTranslator[W, F]) ReadRegister(regId register.Id, forwarding bool) Expr[F] {
	var (
		reg = p.Register(regId)
	)
	//
	if reg.IsInput() {
		// Inputs don't need to refer back
		return mirc.Variable[register.Id, Expr[F]](regId, bitwidthOf(reg), 0)
	} else if forwarding {
		// Forwarded
		return mirc.Variable[register.Id, Expr[F]](regId, bitwidthOf(reg), 0)
	}
	// Not forwarded
	return mirc.Variable[register.Id, Expr[F]](regId, bitwidthOf(reg), -1)
}

// Register implementation for RegisterReader interface
func (p *VectorInsnTranslator[W, F]) Register(reg register.Id) register.Register {
	return toRegister(p.enclosing.Registers()[reg.Unwrap()])
}

// sourceWidths returns the bit widths of the given source registers, in order.
func (p *VectorInsnTranslator[W, F]) sourceWidths(ids []vm.RegisterId) []uint {
	widths := make([]uint, len(ids))
	//
	for i, id := range ids {
		widths[i] = p.enclosing.Register(id).Bitwidth().UnwrapOr(math.MaxUint)
	}
	//
	return widths
}

func joinAssignments(lhs util.Option[dfa.Writes], rhs dfa.Writes) util.Option[dfa.Writes] {
	if lhs.HasValue() {
		return util.Some(lhs.Unwrap().Join(rhs))
	}
	//
	return util.Some(rhs)
}

// toRegisterIds converts a slice of bytecode register identifiers into schema
// register identifiers.
func toRegisterIds(ids []vm.RegisterId) []register.Id {
	regs := make([]register.Id, len(ids))
	//
	for i, id := range ids {
		regs[i] = register.NewId(uint(id))
	}
	//
	return regs
}

// containsRegister reports whether the given bytecode register identifier list
// contains the given schema register identifier.
func containsRegister(ids []vm.RegisterId, reg register.Id) bool {
	for _, id := range ids {
		if uint(id) == reg.Unwrap() {
			return true
		}
	}
	//
	return false
}

func hasSignedBit[W vm.Word[W]](target []vm.RegisterId, enclosing vm.Module[W]) bool {
	var (
		n    = len(target)
		last = target[n-1]
	)
	// Check whether last register is binary or not.
	return enclosing.Register(last).Bitwidth().UnwrapOr(0) == 1
}

func bitwidthOf(reg register.Register) uint {
	if reg.IsNative() {
		return math.MaxUint
	}
	//
	return reg.Width()
}
