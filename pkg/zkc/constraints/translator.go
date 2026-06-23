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
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// GenerateMirConstraints is responsible for converting a field machine into a
// corresponding set of MIR constraints.
func GenerateMirConstraints[F field.Element[F]](fm *vm.FieldMachine[F]) mir.Schema[F] {
	var (
		modules = make([]mir.Module[F], len(fm.Modules()))
		// Pre-compute per-module metadata required to wire up call lookups (e.g.
		// the callee register layout and whether it is atomic).
		infos = computeModuleInfos(fm.Modules())
	)
	//
	for i, m := range fm.Modules() {
		modules[i] = translateModule[F](uint(i), m, infos)
	}
	//
	return schema.NewUniformSchema(modules)
}

// moduleInfo captures the metadata about a (potential) callee module which is
// required to construct a lookup constraint at a call site.
type moduleInfo struct {
	// function indicates this module is a field function (as opposed to a
	// memory module).
	function bool
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
			infos[i] = moduleInfo{true, fn.IsAtomic(), fn.Registers()}
		}
	}
	//
	return infos
}

// GenerateAirConstraints is responsible for converting a field machine into a
// corresponding set of AIR constraints.
func GenerateAirConstraints[F field.Element[F]](fm *vm.FieldMachine[F], field field.Config) air.Schema[F] {
	var (
		mirc = GenerateMirConstraints(fm)
	)
	//
	return mir.LowerToAir(mirc, field.BandWidth, mir.DEFAULT_OPTIMISATION_LEVEL)
}

func translateModule[F field.Element[F]](ctx schema.ModuleId, fm vm.Module, infos []moduleInfo) mir.Module[F] {
	switch fm := fm.(type) {
	case *vm.FieldFunction:
		return translateFunction[F](ctx, *fm, infos)
	case vm.Memory[F]:
		if fm.IsStatic() {
			return translateStaticMemory(ctx, fm)
		} else if fm.IsReadOnly() {
			return translateReadOnlyMemory(ctx, fm)
		} else if fm.IsWriteOnly() {
			return translateWriteOnceMemory(ctx, fm)
		}
		//
		return translateReadWriteMemory(ctx, fm)
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", fm.Name()))
	}
}

func translateStaticMemory[F field.Element[F]](_ schema.ModuleId, m vm.Memory[F]) mir.Module[F] {
	var (
		mod      *schema.Table[F, mir.Constraint[F]]
		name     = trace.ModuleName{Name: m.Name(), Multiplier: 1}
		nInputs  = m.Geometry().AddressLines()
		nOutputs = m.Geometry().DataLines()
		inputs   = m.Registers()[:nInputs]
		outputs  = m.Registers()[nInputs : nInputs+nOutputs]
	)
	// Initialise module as a static reference table.
	mod = mod.Init(name, false, true, false, m.IsNative(), true, 0)
	// Add all registers
	mod.AddRegisters(m.Registers()...)
	// Populate the table contents from the pre-loaded memory.
	mod.SetStaticContents(foldContents(inputs, outputs, m.Contents()))
	//
	return mod
}

func translateReadOnlyMemory[F field.Element[F]](_ schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var (
		mod  *schema.Table[F, mir.Constraint[F]]
		name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	)
	// Initialise module
	mod = mod.Init(name, false, true, false, fm.IsNative(), false, 0)
	// Add all registers
	mod.AddRegisters(fm.Registers()...)
	// TODO: implement ROM constraints
	return mod
}

func translateWriteOnceMemory[F field.Element[F]](_ schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var (
		mod  *schema.Table[F, mir.Constraint[F]]
		name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	)
	// Initialise module
	mod = mod.Init(name, false, true, false, fm.IsNative(), false, 0)
	// Add all registers
	mod.AddRegisters(fm.Registers()...)
	// TODO: implement WOM constraints
	return mod
}

func translateReadWriteMemory[F field.Element[F]](_ schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var (
		mod  *schema.Table[F, mir.Constraint[F]]
		name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	)
	// Initialise module
	mod = mod.Init(name, false, true, false, fm.IsNative(), false, 0)
	// Add all registers
	mod.AddRegisters(fm.Registers()...)
	// TODO: implement WOM constraints
	return mod
}

func translateFunction[F field.Element[F]](ctx schema.ModuleId, fm vm.FieldFunction, infos []moduleInfo,
) mir.Module[F] {
	var (
		padding big.Int
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
		framing Framing[F]
		// one boolean selector register per instruction (code line); empty for
		// atomic functions (which have no program counter).
		pcSelectors []register.Id
	)
	// Initialise module
	mod = mod.Init(name, false, true, false, fm.IsNative(), false, 0)
	// Add all registers
	mod.AddRegisters(fm.Registers()...)
	// Native functions are backed by an external circuit, so we emit only the
	// register layout and skip all framing / instruction-level constraints.
	if fm.IsNative() {
		return mod
	}
	// Add control registers (as required)
	if !fm.IsAtomic() {
		var (
			constraints []mir.Constraint[F]
			pc          = register.NewId(mod.Width())
			ret         = register.NewId(mod.Width() + 1)
			// determine suitable width of PC register
			pcWidth = bit.Width(uint(1 + len(fm.Code())))
		)

		pcSelectors = make([]register.Id, len(fm.Code()))
		// Create program counter
		mod.AddRegisters(register.NewComputed(io.PC_NAME, pcWidth, padding))
		// Create return line
		mod.AddRegisters(register.NewComputed(io.RET_NAME, 1, padding))
		// Create one-hot program counter selectors
		for c := range pcSelectors {
			pcSelectors[c] = register.NewId(mod.Width())
			mod.AddRegisters(register.NewComputed(io.SelectorName(uint(c)), 1, padding))
		}
		// Initialise multi-line framing
		framing, constraints = initMultiLineFraming[F](ctx, pc, ret, pcSelectors, fm)
		// Include framing constraints
		mod.AddConstraints(constraints...)
	} else {
		framing = mirc.NewAtomicFraming[register.Id, Expr[F]]()
	}
	// Transle all instructions
	for pc, vec := range fm.Code() {
		var (
			handle = fmt.Sprintf("pc%d", pc)
			// construct translator for this instruction
			tr = NewVectorTranslator(ctx, uint(pc), vec, framing, fm.Registers())
			// extract logical constraint
			constraint = tr.translate().AsLogical()
		)
		// translate into AIR constraints
		mod.AddConstraints(mir.NewVanishingConstraint(handle, ctx, util.None[int](), constraint))
	}
	// Emit lookup constraints for any function calls made by this function.
	addCallLookups[F](mod, ctx, fm, pcSelectors, infos)
	// Done
	return mod
}

// addCallLookups emits a lookup constraint for every function call (Call or
// UnconditionalCall) made by the given function.  The lookup maps the caller's
// argument/return registers onto the callee's input/output registers.
//
// The caller (source) side is gated by the call's position in the program: in a
// multi-line caller the call at code line k fires exactly on rows where the
// one-hot selector IS_PC_k is high, so that selector is used as the source
// selector; in an atomic caller every row is a call row, so the source is
// unfiltered.  The callee (target) side is filtered by its $ret line for
// multi-line callees, or unfiltered for atomic callees.
func addCallLookups[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	fm vm.FieldFunction, pcSelectors []register.Id, infos []moduleInfo) {
	//
	for pc, vec := range fm.Code() {
		// Determine the source selector for calls on this line (none for atomic
		// callers, which have no program counter selectors).
		var srcSelector util.Option[register.Id]
		if len(pcSelectors) != 0 {
			srcSelector = util.Some(pcSelectors[pc])
		}
		//
		for _, code := range vec.Codes {
			switch c := code.(type) {
			case *instruction.Call:
				emitCallLookup[F](mod, ctx, fm.Registers(), uint(pc), c.Id, c.Arguments, c.Returns, srcSelector, infos)
			case *instruction.UnconditionalCall:
				emitCallLookup[F](mod, ctx, fm.Registers(), uint(pc), c.Id, c.Arguments, c.Returns, srcSelector, infos)
			}
		}
	}
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

		source = lookup.FilteredVector[F, *mir.RegisterAccess[F]](ctx, sel, registerAccesses[F](callerRegs, srcIds)...)
	} else {
		source = lookup.UnfilteredVector[F, *mir.RegisterAccess[F]](ctx, registerAccesses[F](callerRegs, srcIds)...)
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
		target = lookup.UnfilteredVector[F, *mir.RegisterAccess[F]](calleeId, tgtTerms...)
	} else {
		// Multi-line callees expose a $ret line (immediately after the $pc line)
		// which is 1 on active rows; use it as the lookup selector.
		var (
			retId = register.NewId(uint(len(callee.registers)) + 1)
			ret   = term.RawRegisterAccess[F, mir.Term[F]](retId, 1, 0)
		)

		target = lookup.FilteredVector[F, *mir.RegisterAccess[F]](calleeId, ret, tgtTerms...)
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

func initMultiLineFraming[F field.Element[F]](ctx module.Id, pc, ret register.Id, pcSelectors []register.Id,
	fn vm.FieldFunction,
) (Framing[F], []mir.Constraint[F]) {
	var (
		// determine suitable width of PC register
		pcWidth = bit.Width(uint(1 + len(fn.Code())))
		// set with of RET register
		retWidth = uint(1)
		//
		pc_i    = mirc.Variable[register.Id, Expr[F]](pc, pcWidth, 0)
		pc_im1  = mirc.Variable[register.Id, Expr[F]](pc, pcWidth, -1)
		ret_i   = mirc.Variable[register.Id, Expr[F]](ret, retWidth, 0)
		ret_im1 = mirc.Variable[register.Id, Expr[F]](ret, retWidth, -1)
		zero    = mirc.Number[register.Id, Expr[F]](0)
		one     = mirc.Number[register.Id, Expr[F]](1)
	)
	// PC[i]==0 ==> RET[i]==0 (prevents lookup in padding)
	padding := mir.NewVanishingConstraint("padding", ctx, util.None[int](),
		mirc.If(pc_i.Equals(zero), ret_i.Equals(zero)).AsLogical())
	// PC[i-1]==0 && PC[i]!=0 ==> PC[i]==1
	init := mir.NewVanishingConstraint("init", ctx, util.None[int](),
		mirc.If(pc_im1.Equals(zero), mirc.If(pc_i.NotEquals(zero), pc_i.Equals(one))).AsLogical())
	// RET[i-1]!=0 ==> PC[i]==1
	reset := mir.NewVanishingConstraint("reset", ctx, util.None[int](),
		mirc.If(ret_im1.NotEquals(zero), pc_i.Equals(one)).AsLogical())
	// PC[0] != 0 ==> PC[0] == 1
	first := mir.NewVanishingConstraint("first", ctx, util.Some(0),
		mirc.If(pc_i.NotEquals(zero), pc_i.Equals(one)).AsLogical())
	// Build one-hot selector terms.  The selector for code line c is high
	// exactly when PC==c+1 (PC==0 is reserved for padding).
	var (
		selectorTerms = make([]Expr[F], len(pcSelectors))
		weightedTerms = make([]Expr[F], len(pcSelectors))
	)
	//
	for c, sel := range pcSelectors {
		sel_i := mirc.Variable[register.Id, Expr[F]](sel, 1, 0)
		selectorTerms[c] = sel_i
		weightedTerms[c] = mirc.Number[register.Id, Expr[F]](uint(c + 1)).Multiply(sel_i)
	}
	// S = sum of selectors (the activity indicator).
	sum := mirc.Sum(selectorTerms)
	// PC == sum_c (c+1)*IS_PC_c (reconstruction)
	decoding := mir.NewVanishingConstraint("pc_decoding", ctx, util.None[int](),
		pc_i.Equals(mirc.Sum(weightedTerms)).AsLogical())
	// PC*S == PC i.e. exactly one selector is high whenever PC!=0 (and, via
	// pc_decoding, none when PC==0).
	exclusivity := mir.NewVanishingConstraint("is_pc_exclusivity", ctx, util.None[int](),
		pc_i.Multiply(sum).Equals(pc_i).AsLogical())
	//
	constraints := []mir.Constraint[F]{padding, init, reset, first, decoding, exclusivity}
	// Add constancies for all input registers (if applicable):
	for i, r := range fn.Registers() {
		if r.IsInput() {
			var (
				ith     = register.NewId(uint(i))
				name    = fmt.Sprintf("const_%s", r.Name())
				reg_i   = mirc.Variable[register.Id, Expr[F]](ith, r.Width(), 0)
				reg_im1 = mirc.Variable[register.Id, Expr[F]](ith, r.Width(), -1)
			)
			// (5)    (PC[i]!=0 && PC[i]!=1 ==> reg[i] = reg[i-1]
			constraints = append(constraints,
				mir.NewVanishingConstraint(name, ctx, util.None[int](),
					mirc.If(pc_i.NotEquals(zero), mirc.If(pc_i.NotEquals(one), reg_i.Equals(reg_im1))).AsLogical()))
		}
	}
	//
	return mirc.NewMultiLineFraming[register.Id, Expr[F]](pc, pcWidth, ret, 1, pcSelectors), constraints
}
