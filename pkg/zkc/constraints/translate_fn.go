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

	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	tracer "github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

func (p *constraintTranslator[W, F]) translateFunction(ctx schema.ModuleId, fn *vm.Function[W]) mir.Module[F] {
	var (
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = fn.Name()
		regs    = toRegisters(fn.Registers())
		framing Framing[F]
		// IS_PC_<k> program counter selectors, only for MLI.
		pcSelectors []register.Id
		// $ret register, used to guard lookup for OLI.
		// TODO: see https://github.com/LFDT-Lineth/zkc/issues/1975
		ret register.Id
	)
	// Initialise module
	mod = mod.Init(name, false, false, false, fn.IsNative(), false)
	// Add all registers
	mod.AddRegisters(regs...)
	// Native functions are backed by an external circuit, so we emit only the
	// register layout and skip all framing / instruction-level constraints.
	if fn.IsNative() {
		return mod
	}

	ret = register.NewId(mod.Width())
	// Add control registers for Multi Line Instruction
	if !fn.IsOneLine() {
		var (
			constraints []mir.Constraint[F]
			pc          = register.NewId(mod.Width() + 1)
		)

		// Create return line
		mod.AddRegisters(register.NewComputed(tracer.RET_NAME, 1))
		// Create program counter
		mod.AddRegisters(register.NewComputed(tracer.PC_NAME, fn.PcWidth()))
		// Add IS_PC_<k> program counter selectors (one per code line)
		pcSelectors = make([]register.Id, len(fn.Vectors()))
		for c := range pcSelectors {
			pcSelectors[c] = register.NewId(mod.Width())
			mod.AddRegisters(register.NewComputed(tracer.SelectorName(uint(c)), 1))
		}
		// Initialise multi-line framing
		framing, constraints = initMultiLineFraming[F](ctx, pc, ret, pcSelectors, regs, len(fn.Vectors()))
		// Include framing constraints
		mod.AddConstraints(constraints...)
	} else {
		framing = mirc.NewAtomicFraming[register.Id, Expr[F]]()

		mod.AddRegisters(register.NewComputed(tracer.RET_NAME, 1))
	}
	// Translate all bytecode vectors
	for pc, vec := range fn.Vectors() {
		var (
			handle = func() string {
				if fn.IsOneLine() {
					return "inst"
				}
				// PC_0 is for padding
				return fmt.Sprintf("pc%d", pc+1)
			}()
			// construct translator for this bytecode vector
			tr = NewVectorTranslator(ctx, uint(pc), vec, framing, fn, p.program.Field())
			// extract logical constraint
			constraint = tr.translate()
		)
		// For atomic functions, gate the constraint on the $ret
		// activity line so padding rows ($ret==0) are unconstrained.
		// TODO: this is a temporary nuclear option as it brings bad perf:
		// - all constraints are gated on $ret, so raising the degree of all constraints by one
		// - add one column ($ret)
		// see https://github.com/LFDT-Lineth/zkc/issues/1975
		// Note: we might still need to do it for OLI touching memory.
		if fn.IsOneLine() {
			iomf := mirc.Variable[register.Id, Expr[F]](ret, 1, 0).
				NotEquals(mirc.Number[register.Id, Expr[F]](0))
			constraint = mirc.If(iomf, constraint)
		}
		// translate into MIR constraints
		mod.AddConstraints(mir.NewVanishingConstraint(handle, ctx, util.None[int](), constraint.AsLogical()))
	}
	// Add range proof constraints for all registers.
	// Note: while adding lookups from calls and memory read/write  might add (bit) registers,
	// it is safe to add range proof constraints for all registers before, as the registers
	// that will be introduced later will be already range-proved (as a product of bit registers).
	// Note that registers coming from control flow have been added to the module before this point,
	// so they will be range-proved as well.
	p.addRangeProofConstraints(mod, ctx, mod.Registers())
	// Emit lookup constraints for any function calls and memory accesses made
	// by this function (recording send ports for calls into global functions).
	p.addLookups(mod, ctx, fn, pcSelectors, ret)
	// Done
	return mod
}

func initMultiLineFraming[F field.Element[F]](ctx module.Id, pc, ret register.Id, pcSelectors []register.Id,
	regs []register.Register, numLines int,
) (Framing[F], []mir.Constraint[F]) {
	var (
		// determine suitable width of PC register
		pcWidth = bit.Width(uint(1 + numLines))
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
	// Build one-hot selector terms.  The selector for code line c is 1
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
	// PC*S == PC i.e. exactly one selector is 1 whenever PC!=0 (and, via
	// pc_decoding, none when PC==0).
	exclusivity := mir.NewVanishingConstraint("is_pc_exclusivity", ctx, util.None[int](),
		pc_i.Multiply(sum).Equals(pc_i).AsLogical())
	//
	constraints := []mir.Constraint[F]{padding, init, reset, first, decoding, exclusivity}
	// Add constancies for all input registers (if applicable):
	for i, r := range regs {
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
