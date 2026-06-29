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
	"math/big"

	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// moduleInfo captures the metadata about a (potential) callee module which is
// required to construct a lookup constraint at a call site.
type moduleInfo struct {
	// atomic indicates this is a one-line function (which therefore has no
	// $ret control line and is used as an unfiltered lookup target).
	atomic bool
	// registers gives the callee register layout: inputs, followed by outputs,
	// followed by any locals.
	registers []register.Register
}

// computeModuleInfos collects the metadata needed to wire up call lookups for
// every module in the machine.
func computeModuleInfos(modules []vm.Module) []moduleInfo {
	infos := make([]moduleInfo, len(modules))
	//
	for i, m := range modules {
		if fn, ok := m.(*vm.FieldFunction); ok {
			infos[i] = moduleInfo{fn.IsAtomic(), fn.Registers()}
		}
	}
	//
	return infos
}

// addCallLookups emits a lookup constraint for every function call (Call or
// UnconditionalCall) made by the given function.  The lookup maps the caller's
// argument/return registers onto the callee's input/output registers.
//
// The caller (source) side is gated on two (potentially combined) conditions:
//
//   - Position: in a multi-line caller the call at code line k fires only on
//     rows where the selector IS_PC_k is on; in an atomic caller
//     every row is a call row, so there is no positional gating.
//   - Path: a call may be executed conditionally. The branch condition under which the
//     call is reached is materialised by FactorSkipConditions as a boolean
//     register ("path selector"); only rows where it is on actually call.
//
// Lookups require a register (and not an expression) as the source selector,
// so the path selector is materialised as a fresh 1-bit register (if it is not already).
func addCallLookups[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	fm vm.FieldFunction, pcSelectors []register.Id, infos []moduleInfo) {
	//
	for pc, vec := range fm.Code() {
		// Branch table giving the condition under which each code in this vector
		// is reached.
		_, branchTable := vec.BranchTable()
		//
		for cc, code := range vec.Codes {
			var (
				args     []register.Id
				returns  []register.Id
				calleeId uint
				// Source selector gating the call (None => unfiltered).
				srcSelector util.Option[register.Id]
			)
			//
			switch c := code.(type) {
			case *instruction.Call:
				calleeId, args, returns = c.Id, c.Arguments, c.Returns
				// A (conditional) call is gated by its branch condition and, in a
				// multi-line caller, its line selector.  Pass the module's full
				// register set (which includes the IS_PC_* selectors, beyond the
				// function's own registers) so the condition can reference them.
				srcSelector = callSourceSelector(mod, ctx, mod.Registers(),
					branchTable.StateOf(uint(cc)).Condition, uint(pc), pcSelectors)
			case *instruction.UnconditionalCall:
				//TODO: perf, see https://github.com/LFDT-Lineth/zkc/issues/1935
				//
				// An unconditional call fires on every row, so it has no selector at
				// all (neither positional nor path).
				calleeId, args, returns = c.Id, c.Arguments, c.Returns
			default:
				continue
			}
			//
			emitCallLookup(mod, ctx, fm.Registers(), uint(pc), calleeId, args, returns, srcSelector, infos)
		}
	}
}

// callSourceSelector determines the register used to gate the source (caller)
// side of a (conditional) call's lookup constraint, given the branch condition
// under which the call executes and the caller's per-line is_pc_* selectors
// (empty for an atomic caller).
func callSourceSelector[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	regs []register.Register, cond dfa.BranchCondition, pc uint, pcSelectors []register.Id,
) util.Option[register.Id] {
	// TODO: perf, see https://github.com/LFDT-Lineth/zkc/issues/1936
	//
	// In a multi-line caller the call only fires on rows executing its line, so
	// fold the line's PC selector (IS_PC_k != 0) into the condition as an
	// extra single-bit atom.  An atomic caller has no line selectors, so its
	// gating is the branch condition alone.
	if len(pcSelectors) != 0 {
		isPc := logical.NotEqualsConst(dfa.NewBranchId(false, pcSelectors[pc]), big.Int{})
		cond = cond.And(logical.NewProposition(isPc))
	}
	// Reached on every row: no gating at all.
	if cond.IsTrue() {
		return util.None[register.Id]()
	}
	// The (gated) condition is already a single materialised boolean: reuse it
	// directly, with no fresh column.
	if b, ok := singleBitGuard(cond, regs); ok {
		return util.Some(b)
	}
	// General case: materialise a fresh path selector column.
	return util.Some(newPathSelector(mod, ctx, regs, cond))
}

// singleBitGuard recognises a branch condition which is, over a single register
// b, either "b != 0" or "b == 1" — both of which are 1 exactly when the guarded
// path is taken, so b can serve directly as the lookup selector.  Other forms
// ("b == 0", "b != 1", a register-valued or non-{0,1} RHS, a multi-register
// group) cannot, and fall back to a materialised path selector column.
//
// Such a call guard is always materialised on a 1-bit register by
// FactorSkipConditions, so a wider operand here indicates a broken invariant
// and panics.
func singleBitGuard(cond dfa.BranchCondition, regs []register.Register) (register.Id, bool) {
	conjuncts := cond.Conjuncts()
	if len(conjuncts) != 1 {
		return register.UnusedId(), false
	}
	//
	atoms := conjuncts[0].Atoms()
	if len(atoms) != 1 {
		return register.UnusedId(), false
	}
	// Require a single register compared against a constant.
	atom := atoms[0]
	if atom.Left.Width != 1 || !atom.Right.HasSecond() {
		return register.UnusedId(), false
	}
	// Only "b != 0" (inequality vs 0) and "b == 1" (equality vs 1) let us reuse b.
	rhs := atom.Right.Second()
	neqZero := !atom.Sign && rhs.Sign() == 0
	eqOne := atom.Sign && rhs.Cmp(big.NewInt(1)) == 0
	//
	if !neqZero && !eqOne {
		return register.UnusedId(), false
	}
	// The operand must be a genuine boolean (1-bit) register.
	id := atom.Left.Id
	if w := regs[id.Unwrap()].Width(); w != 1 {
		panic(fmt.Sprintf("expected 1-bit branch register for call guard, got width %d", w))
	}
	//
	return id, true
}

// newPathSelector creates a fresh 1-bit column gating a conditionally executed
// call and returns its id.  The column is filled (during trace expansion) with,
// and constrained to equal, the boolean value of the call's (already position-
// gated) branch condition — so it is 1 exactly on the rows which perform the
// call.
func newPathSelector[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	regs []register.Register, cond dfa.BranchCondition,
) register.Id {
	var padding big.Int
	// Allocate the selector column.
	selId := register.NewId(mod.Width())
	mod.AddRegisters(register.NewComputed(fmt.Sprintf("$call_sel_%d", selId.Unwrap()), 1, padding))
	// Fill the flag selector during trace expansion with the boolean value of the condition.
	mod.AddAssignments(assignment.NewComputedRegister[F](pathSelectorComputation(cond, regs), true, ctx, selId))
	// Bind it for soundness: $call_sel == 1 exactly when the condition holds.
	mod.AddConstraints(mir.NewVanishingConstraint(
		fmt.Sprintf("call_sel_%d", selId.Unwrap()), ctx, util.None[int](),
		pathSelectorConstraint[F](selId, cond, regs)))
	//
	return selId
}

// pathSelectorConstraint builds the binding "$call_sel == 1 iff cond" as an MIR
// logical term.
func pathSelectorConstraint[F field.Element[F]](selId register.Id, cond dfa.BranchCondition,
	regs []register.Register) mir.LogicalTerm[F] {
	var (
		condition = mirc.TranslateBranchCondition(cond, callRegisterReader[F]{regs})
		sel       = mirc.Variable[register.Id, Expr[F]](selId, 1, 0)
		one       = mirc.Number[register.Id, Expr[F]](1)
		zero      = mirc.Number[register.Id, Expr[F]](0)
	)
	//
	return condition.ThenElse(sel.Equals(one), sel.Equals(zero)).AsLogical()
}

// pathSelectorComputation builds the trace-expansion computation for a path
// selector: the boolean value of the branch condition (1 when taken, else 0).
func pathSelectorComputation(cond dfa.BranchCondition, regs []register.Register) term.Computation[word.BigEndian] {
	var (
		condition = mirc.TranslateBranchCondition(cond, callRegisterReader[word.BigEndian]{regs})
		logical   = term.NewLogicalComputation[word.BigEndian, mir.LogicalTerm[word.BigEndian],
			mir.Term[word.BigEndian]](condition.AsLogical())
		one  = term.Const[word.BigEndian, term.Computation[word.BigEndian]](field.One[word.BigEndian]())
		zero = term.Const[word.BigEndian, term.Computation[word.BigEndian]](field.Zero[word.BigEndian]())
	)
	//
	return term.IfElse(logical, one, zero)
}

// callRegisterReader is a minimal mirc.RegisterReader over a register layout,
// reading every branch register on the current row (shift 0): the row on which
// the gated call fires holds the register's assigned value.
type callRegisterReader[F field.Element[F]] struct {
	regs []register.Register
}

func (p callRegisterReader[F]) Register(id register.Id) register.Register { return p.regs[id.Unwrap()] }

func (p callRegisterReader[F]) RegisterWidths(ids ...register.Id) []uint {
	widths := make([]uint, len(ids))
	//
	for i, id := range ids {
		widths[i] = p.regs[id.Unwrap()].Width()
	}
	//
	return widths
}

func (p callRegisterReader[F]) ReadRegister(id register.Id, _ bool) Expr[F] {
	return mirc.Variable[register.Id, Expr[F]](id, p.regs[id.Unwrap()].Width(), 0)
}

// emitCallLookup constructs and adds a single lookup constraint mapping the
// caller's argument/return registers onto the callee's input/output registers.
func emitCallLookup[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	callerRegs []register.Register, pc, calleeId uint, args, returns []register.Id,
	srcSelector util.Option[register.Id], infos []moduleInfo) {
	var (
		callee = infos[calleeId]
		handle = fmt.Sprintf("call_%d_%d_%d", ctx, pc, calleeId)
		// Source ids: the caller's argument registers followed by its return
		// registers.
		srcIds = append(append([]register.Id{}, args...), returns...)
		// Target ids: the callee's input registers (0..nInputs) followed by its
		// output registers (nInputs..nInputs+nOutputs).  Their count matches the
		// number of source ids.
		tgtIds = make([]register.Id, len(srcIds))
	)
	//
	for i := range tgtIds {
		tgtIds[i] = register.NewId(uint(i))
	}
	// Build the source (caller) vector.
	var source mir.LookupVector[F]

	if srcSelector.HasValue() {
		var (
			selId = srcSelector.Unwrap()
			sel   = term.RawRegisterAccess[F, mir.Term[F]](selId, 1, 0)
		)

		source = lookup.FilteredVector(ctx, sel, registerAccesses[F](callerRegs, srcIds)...)
	} else {
		source = lookup.UnfilteredVector(ctx, registerAccesses[F](callerRegs, srcIds)...)
	}
	// Build the target (callee) vector.
	var (
		target   mir.LookupVector[F]
		tgtTerms = registerAccesses[F](callee.registers, tgtIds)
	)
	//
	if callee.atomic {
		// Atomic callees have no $ret line: every callee row is a valid table
		// entry, so the target side is unfiltered.
		target = lookup.UnfilteredVector(calleeId, tgtTerms...)
	} else {
		// Multi-line callees expose a $ret line (immediately after the $pc line)
		// which is 1 on active rows; use it as the lookup selector.
		var (
			retId = register.NewId(uint(len(callee.registers)) + 1)
			ret   = term.RawRegisterAccess[F, mir.Term[F]](retId, 1, 0)
		)

		target = lookup.FilteredVector(calleeId, ret, tgtTerms...)
	}
	//
	mod.AddConstraints(mir.NewLookupConstraint(handle, []mir.LookupVector[F]{target}, []mir.LookupVector[F]{source}))
}

// registerAccesses builds MIR register accesses (at row offset 0) for the given
// register ids, looking up each register's width in the supplied layout.
func registerAccesses[F field.Element[F]](regs []register.Register, ids []register.Id) []*mir.RegisterAccess[F] {
	terms := make([]*mir.RegisterAccess[F], len(ids))
	//
	for i, id := range ids {
		terms[i] = term.RawRegisterAccess[F, mir.Term[F]](id, regs[id.Unwrap()].Width(), 0)
	}
	//
	return terms
}
