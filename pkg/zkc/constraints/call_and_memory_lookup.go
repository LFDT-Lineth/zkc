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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// addLookups emits a lookup constraint for every function call
// and every memory access (read or write) made by the given
// function:
// - a call's lookup maps the caller's argument/return registers onto
// the callee's input/output registers.
// - a memory access's lookup maps the accessor's address/data registers
// onto the memory table's address/data columns.
//
// The accessing (source) side is gated on two (potentially combined) conditions:
//
//   - Position: in a multi-line function the access at code line k fires only on
//     rows where the selector IS_PC_k is on; in an OLI function,
//     access row are gated by $ret.
//   - Path: an access may be executed conditionally. The branch condition under
//     which the access is reached is materialised by FactorSkipConditions as a
//     boolean register ("path selector"); only rows where it is on actually access.
//
// Lookups require a register (and not an expression) as the source selector,
// so the path selector is materialised as a fresh 1-bit register (if it is not already).
func addLookups[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	ctx schema.ModuleId,
	fn *vm.Function[W],
	pcSelectors []register.Id,
	ret register.Id,
	infos []vm.Module[W],
	field field.Config) {
	//
	for pc, vec := range fn.Vectors() {
		// Branch table giving the condition under which each code in this vector
		// is reached.
		_, branchTable := vec.BranchTable(field.RegisterWidth)
		// One-hot register groups declared by the vector's Dispatch bytecodes,
		// used to shorten the selector conditions below.
		oneHot := collectOneHotGroups(vec.Bytecodes)
		// Group the lookup-emitting bytecodes by the branch condition under
		// which they execute, so accesses sharing a condition share a single
		// source selector (column).
		// Note that a branch that doesn't emit lookup will be skipped.
		for _, group := range groupLookupsByCondition(vec.Bytecodes, branchTable, oneHot) {
			// Source selector gating the accesses of this group: their branch
			// condition and:
			// - for a multi-line function, its line selector (IS_PC_*)
			// - for a one line function, the $ret register (defining the
			//   non-padding region)
			srcSelector := lookupSourceSelector(mod, ctx, mod.Registers(),
				group.condition, uint(pc), pcSelectors, ret, oneHot)
			//
			for _, entry := range group.entries {
				switch c := entry.code.(type) {
				case *vm.BytecodeCall[W]:
					emitCallLookup(mod, ctx, uint(pc), uint(c.Target),
						toRegisterIds(c.Arguments), toRegisterIds(c.Returns), srcSelector, infos)
				case *vm.BytecodeReadWrite[W]:
					if infos[c.Id].(*vm.Memory[W]).IsReadWrite() {
						emitRamLookup(mod, ctx, uint(pc), entry.cc, c, srcSelector, infos, field)
					} else {
						emitMemoryLookup(mod, ctx, uint(pc), entry.cc, uint(c.Id),
							toRegisterIds(c.Address), toRegisterIds(c.Data), srcSelector, infos)
					}
				}
			}
		}
	}
}

// lookupGroup collects the bytecodes of one vector which emit a lookup and
// execute under the same branch condition, so they can share a single source
// selector.
type lookupGroup[W vm.Word[W]] struct {
	// condition under which every bytecode of this group executes.
	condition dfa.BranchCondition
	// entries identifies the bytecodes (and their positions) of this group.
	entries []lookupEntry[W]
}

// lookupEntry pairs a lookup-emitting bytecode with its position in the
// enclosing vector.
type lookupEntry[W vm.Word[W]] struct {
	cc   uint
	code vm.Bytecode[W]
}

// groupLookupsByCondition partitions the lookup-emitting bytecodes of a vector
// (calls and memory accesses) by the branch condition under
// which they execute.  Conditions are shortened against the given one-hot
// groups first (see rewriteOneHotConditions), so that accesses within a
// switch's default body are gated on the default bit rather than on the
// complement of every case bit.
func groupLookupsByCondition[W vm.Word[W]](codes []vm.Bytecode[W], branchTable dfa.Result[dfa.Path[W]],
	oneHot []oneHotGroup) []lookupGroup[W] {
	var groups []lookupGroup[W]
	//
outer:
	for cc, code := range codes {
		switch code.(type) {
		case *vm.BytecodeCall[W]:
			// always emits a lookup
		case *vm.BytecodeReadWrite[W]:
			// read-write and access-once memories alike emit a lookup.
		default:
			continue
		}
		//
		var (
			condition = rewriteOneHotConditions(reachCondition(branchTable.StateOf(uint(cc))), oneHot)
			entry     = lookupEntry[W]{uint(cc), code}
		)
		// Append to an existing group with the same condition (if any).
		for i := range groups {
			if groups[i].condition.Equals(condition) {
				groups[i].entries = append(groups[i].entries, entry)
				continue outer
			}
		}
		//
		groups = append(groups, lookupGroup[W]{condition, []lookupEntry[W]{entry}})
	}
	//
	return groups
}

// lookupSourceSelector determines the register used to gate the source
// (accessing) side of a (conditional) call's or memory access's lookup
// constraint, given the branch condition under which the access executes and
// the accessor's per-line is_pc_* selectors (empty for an atomic function).
// A gating register always exists: every access is at least position-gated
// (IS_PC_k for a multi-line function, $ret for a one-line function).
func lookupSourceSelector[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	regs []register.Register, cond dfa.BranchCondition, pc uint, pcSelectors []register.Id,
	ret register.Id, oneHot []oneHotGroup) register.Id {
	// Position register gating the rows of this access: the line's IS_PC_k
	// selector for a multi-line function, or $ret for an atomic one (defining
	// the non-padding region).
	var position register.Id
	//
	if len(pcSelectors) != 0 {
		position = pcSelectors[pc]
	} else {
		position = ret
	}
	// An unconditional access is gated by position alone, so the position
	// register (already 1 exactly on its rows) serves directly as selector.
	if cond.IsTrue() {
		return position
	}
	// Conditional access: fold the position atom (position != 0) into the
	// condition and materialise it as a fresh path selector column.
	posAtom := logical.NotEqualsConst(dfa.NewBranchId(false, position), big.Int{})
	cond = cond.And(logical.NewProposition(posAtom))
	//
	return newPathSelector(mod, ctx, regs, cond, oneHot)
}

// newPathSelector creates a fresh 1-bit column gating a conditionally executed
// call or memory access and returns its id.  The column is filled (during trace
// expansion) with, and constrained to equal, the boolean value of the access's
// (already position-gated) branch condition — so it is 1 exactly on the rows
// which perform the access.
func newPathSelector[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	regs []register.Register, cond dfa.BranchCondition, oneHot []oneHotGroup,
) register.Id {
	var padding big.Int
	// Allocate the selector column.
	selId := register.NewId(mod.Width())
	mod.AddRegisters(register.NewComputed(fmt.Sprintf("$lookup_sel_%d", selId.Unwrap()), 1, padding))
	// Fill the flag selector during trace expansion with the boolean value of the condition.
	mod.AddAssignments(assignment.NewComputedRegister[F](pathSelectorComputation(cond, regs), ctx, selId))
	// Bind it for soundness: $lookup_sel == 1 exactly when the condition holds.
	mod.AddConstraints(mir.NewVanishingConstraint(
		fmt.Sprintf("lookup_sel_%d", selId.Unwrap()), ctx, util.None[int](),
		pathSelectorConstraint[F](selId, cond, regs, oneHot)))
	//
	return selId
}

// pathSelectorConstraint builds the binding "$lookup_sel == 1 iff cond" as an
// MIR logical term.
//
// A condition of the shape ⋁ᵢ (rest ∧ bitᵢ != 0 ∧ guardsᵢ) over distinct bits
// of a single one-hot group (see splitOneHotDisjunction) is bound
// arithmetically as
//
//	if rest { sel == Σᵢ bitᵢ·⟦guardsᵢ⟧ } else { sel == 0 }
func pathSelectorConstraint[F field.Element[F]](selId register.Id, cond dfa.BranchCondition,
	regs []register.Register, oneHot []oneHotGroup) mir.LogicalTerm[F] {
	var (
		sel  = mirc.Variable[register.Id, Expr[F]](selId, 1, 0)
		one  = mirc.Number[register.Id, Expr[F]](1)
		zero = mirc.Number[register.Id, Expr[F]](0)
	)
	//
	if rest, pieces, ok := splitOneHotDisjunction(cond, oneHot); ok {
		var (
			remainder = mirc.TranslateBranchCondition(conditionOfAtoms(rest), callRegisterReader[F]{regs})
			sum       Expr[F]
		)
		//
		for i, piece := range pieces {
			ith := mirc.Variable[register.Id, Expr[F]](piece.bit, 1, 0)
			// Each guard tests a width-1 register against zero, so its 0/1
			// indicator is the register itself (!=) or its complement (==).
			for _, guard := range piece.guards {
				factor := mirc.Variable[register.Id, Expr[F]](guard.Left.Id, 1, 0)
				//
				if guard.Sign {
					factor = one.Subtract(factor)
				}
				//
				ith = ith.Multiply(factor)
			}
			//
			if i == 0 {
				sum = ith
			} else {
				sum = sum.Add(ith)
			}
		}
		//
		return remainder.ThenElse(sel.Equals(sum), sel.Equals(zero)).AsLogical()
	}
	//
	condition := mirc.TranslateBranchCondition(cond, callRegisterReader[F]{regs})
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
func emitCallLookup[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	pc, calleeId uint, args, returns []register.Id,
	srcSelector register.Id, infos []vm.Module[W]) {
	var (
		callee     = infos[calleeId].(*vm.Function[W])
		calleeRegs = toRegisters(callee.Registers())
		handle     = fmt.Sprintf("call_%d_%d_%d", ctx, pc, calleeId)
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
	var source = lookup.FilteredVector(ctx, srcSelector, srcIds...)
	// Build the target (callee) vector.
	var target mir.LookupVector
	// Native module don't have a $ret function
	// (do we need one ? see https://github.com/LFDT-Lineth/zkc/issues/2025)
	if callee.IsNative() {
		target = lookup.UnfilteredVector(calleeId, tgtIds...)
	} else {
		// Both multi-line and atomic (one-line) callees expose a $ret line which is 1
		// on active rows; use it as the lookup selector.

		// TODO: see https://github.com/LFDT-Lineth/zkc/issues/1975
		// Atomic callees have $ret line as well. Only OLI that touches memmory should have one.
		var retId = register.NewId(uint(len(calleeRegs)))
		//
		target = lookup.FilteredVector(calleeId, retId, tgtIds...)
	}
	//
	mod.AddConstraints(mir.NewLookupConstraint[F](handle, []mir.LookupVector{target}, []mir.LookupVector{source}))
}

// emitMemoryLookup constructs and adds a single lookup constraint mapping an
// accessor's address/data registers onto the address/data columns of a memory
// table (SROM, ROM or WOM).  Reads and writes share the same shape: both bind
// the (address, data) tuple to a row of the table.
//
// The target side is gated so padding rows are never valid table entries:
//
//   - SROM: the table is static (fully enumerated at compile time), so every
//     row is valid and the target is unfiltered.
//   - ROM / WOM: the table is trace-provided, so the target is filtered on the
//     $access_bit column, which is 1 exactly on non-padding rows.
//
// For a WOM this lookup also enforces write-once consistency: the table holds
// each address exactly once (address monotony), so two writes of different
// values to the same address cannot both match a row.
func emitMemoryLookup[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	ctx schema.ModuleId, pc, cc, memId uint,
	address, data []register.Id, srcSelector register.Id, infos []vm.Module[W]) {
	var (
		mem     = infos[memId].(*vm.Memory[W])
		memRegs = toRegisters(mem.Registers())
		// The bytecode index (cc) disambiguates two accesses to the same memory
		// on the same code line.
		handle = fmt.Sprintf("memory_%d_%d_%d_%d", ctx, pc, cc, memId)
		// Source ids: the accessor's address registers followed by its data
		// registers.
		srcIds = append(append([]register.Id{}, address...), data...)
		// Target ids: the memory's address registers (its inputs) followed by
		// its data registers (its outputs), which together occupy ids
		// 0..len(memRegs).  Their count matches the number of source ids.
		tgtIds = make([]register.Id, len(srcIds))
	)
	//
	for i := range tgtIds {
		tgtIds[i] = register.NewId(uint(i))
	}
	// Build the source (accessor) vector.
	var source = lookup.FilteredVector(ctx, srcSelector, srcIds...)
	// Build the target (memory) vector.
	var target mir.LookupVector
	//
	if mem.IsStatic() {
		// Static tables enumerate their full contents, so every row is a valid
		// table entry and the target side is unfiltered.
		target = lookup.UnfilteredVector(memId, tgtIds...)
	} else {
		// ROM / WOM tables expose a $access_bit column which is 1 on active
		// rows; use it as the lookup selector.  It is allocated immediately
		// after the address/data registers (see translateAccessOnceMemory).
		var accessId = register.NewId(uint(len(memRegs)))
		//
		target = lookup.FilteredVector(memId, accessId, tgtIds...)
	}
	//
	mod.AddConstraints(mir.NewLookupConstraint[F](handle, []mir.LookupVector{target}, []mir.LookupVector{source}))
}

// emitRamLookup constructs and adds the lookup constraint tying one read-write
// (RAM) memory access to a row of that memory's table: the accessor's address,
// data and (threaded) timestamp registers map onto the table's ADDRESS,
// VALUE_WRITTEN and TIMESTAMP_WRITTEN columns.  The access's read/write kind is
// pinned by the target-side filter: a write targets the table filtered by
// EXEC_WRITE (= EXEC * IS_WRITE), a read by EXEC_READ (= EXEC * (1-IS_WRITE)).
// A read's data registers are its outputs — the value read back — which the
// table exposes in VALUE_WRITTEN too, since a read row "writes back" what it
// found (VALUE_READ == VALUE_WRITTEN there).
//
// Together with the table's local constraints this pins every access's row;
// that VALUE_READ genuinely returns the last value written to the address
// remains for the offline memory-checking bus (see translateReadWriteMemory).
func emitRamLookup[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	ctx schema.ModuleId, pc, cc uint, rw *vm.BytecodeReadWrite[W],
	srcSelector register.Id, infos []vm.Module[W], fieldCfg field.Config) {
	//
	var (
		mem    = infos[rw.Id].(*vm.Memory[W])
		layout = computeRamLayout(mem, fieldCfg)
		// The bytecode index (cc) disambiguates two accesses to the same memory
		// on the same code line.
		handle = fmt.Sprintf("ram_%d_%d_%d_%d", ctx, pc, cc, rw.Id)
		// Source ids: the accessor's address, data and timestamp registers.
		// The timestamp is threaded on the constraint path only, so it is
		// always present here; its limbs split exactly like the table's
		// timestamp columns (same width, same field).
		srcIds = append(append(append([]register.Id{},
			toRegisterIds(rw.Address)...),
			toRegisterIds(rw.Data)...),
			toRegisterIds(rw.Stamp)...)
		// Target ids: the table's ADDRESS, VALUE_WRITTEN and TIMESTAMP_WRITTEN
		// columns, in the layout's fixed order.
		tgtIds = append(append(append([]register.Id{},
			layout.address...),
			layout.valueWritten...),
			layout.tsWritten...)
		// Target-side filter pinning the access kind.
		kind = layout.execRead
	)
	//
	if len(rw.Stamp) == 0 {
		panic(fmt.Sprintf("read-write memory access without a threaded timestamp (%s)", handle))
	}
	//
	if rw.Write {
		kind = layout.execWrite
	}
	//
	var (
		source = lookup.FilteredVector(ctx, srcSelector, srcIds...)
		target = lookup.FilteredVector(uint(rw.Id), kind, tgtIds...)
	)
	//
	mod.AddConstraints(mir.NewLookupConstraint[F](handle, []mir.LookupVector{target}, []mir.LookupVector{source}))
}
