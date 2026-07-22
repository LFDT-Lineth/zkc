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
	"math/bits"

	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// GenerateMirConstraints is responsible for converting a bytecode program into
// a corresponding set of MIR constraints.  The translation operates directly
// over the bytecode program (its modules, registers and bytecode vectors),
// without going through the legacy word / field machine.
func GenerateMirConstraints[W vm.Word[W], F field.Element[F]](program vm.Program[W], field field.Config,
	maxStaticDepth uint) mir.Schema[F] {
	var (
		infos   = program.Modules()
		modules = make([]mir.Module[F], len(infos))
		// maxStaticWidth is the largest X for which 2^X <= maxStaticDepth (the
		// max static table size), i.e. floor(log2(maxStaticDepth)).
		// It represents the maximum register width for which a static table can be use to range-check it.
		// Wider registers require recursive range modules.
		maxStaticWidth = uint(bits.Len(maxStaticDepth) - 1)
		// Index the static range-check tables by width, so each register can be
		// range-proved by a lookup into the matching $range_un table.
		rangeTables = indexRangeTables[W, F](infos, maxStaticWidth)
	)
	//
	for i, m := range infos {
		modules[i] = translateModule[W, F](uint(i), m, infos, rangeTables, maxStaticWidth)
	}
	//
	return schema.NewUniformSchema(modules)
}

// GenerateAirConstraints is responsible for converting a bytecode program into
// a corresponding set of AIR constraints.
func GenerateAirConstraints[W vm.Word[W], F field.Element[F]](program vm.Program[W], field field.Config,
	maxStaticDepth uint) air.Schema[F] {
	var (
		mirc = GenerateMirConstraints[W, F](program, field, maxStaticDepth)
	)
	//
	return mir.LowerToAir(mirc, field.BandWidth, mir.DEFAULT_OPTIMISATION_LEVEL)
}

func translateModule[W vm.Word[W], F field.Element[F]](ctx schema.ModuleId, m vm.Module[W],
	infos []vm.Module[W], rangeTables map[uint]rangeTable, maxStaticWidth uint) mir.Module[F] {
	switch m := m.(type) {
	case *vm.Function[W]:
		return translateFunction[W, F](ctx, m, infos, rangeTables, maxStaticWidth)
	case *vm.Memory[W]:
		if m.IsStatic() {
			return translateStaticMemory[W, F](ctx, m)
		} else if m.IsReadOnly() {
			return translateReadOnlyMemory[W, F](ctx, m)
		} else if m.IsWriteOnly() {
			return translateWriteOnceMemory[W, F](ctx, m)
		}
		//
		return translateReadWriteMemory[W, F](ctx, m)
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", m.Name()))
	}
}

func translateStaticMemory[W vm.Word[W], F field.Element[F]](_ schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var (
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = trace.ModuleName{Name: m.Name(), Multiplier: 1}
		regs    = toRegisters(m.Registers())
		inputs  = toRegisters(m.AddressRegisters())
		outputs = toRegisters(m.DataRegisters())
		// Convert the static contents from words into field elements.
		contents = toFieldElements[W, F](m.StaticContents())
	)
	// Initialise module as a static reference table.  Memory modules are never
	// native.
	mod = mod.Init(name, false, true, false, false, true, 0)
	// Add all registers
	mod.AddRegisters(regs...)
	// Populate the table contents from the pre-loaded memory.
	mod.SetStaticContents(foldContents(inputs, outputs, contents))
	//
	return mod
}

func translateReadOnlyMemory[W vm.Word[W], F field.Element[F]](
	ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var name = trace.ModuleName{Name: m.Name(), Multiplier: 1}
	return translateAccessOnceMemory[W, F](ctx, m, name)
}

// Write once memory and read only memory are equivalent on the constraints level
func translateWriteOnceMemory[W vm.Word[W], F field.Element[F]](
	ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var name = trace.ModuleName{Name: m.Name(), Multiplier: 1}
	return translateAccessOnceMemory[W, F](ctx, m, name)
}

func translateReadWriteMemory[W vm.Word[W], F field.Element[F]](
	ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var (
		regs = toRegisters(m.Registers())
		mod  *schema.Table[F, mir.Constraint[F]]
		name = trace.ModuleName{Name: m.Name(), Multiplier: 1}
	)
	// Initialise module.  Memory modules are never native.
	mod = mod.Init(name, false, true, false, false, false, 0)
	// Add all registers
	mod.AddRegisters(regs...)
	// TODO: read-write (RAM) constraints are disabled for now — the timestamp
	// columns they rely on are not yet filled by the trace observer (see git
	// history for the WIP body).
	return mod
}

// translateAccessOnceMemory handles both
//   - read once memory
//   - write once memory
func translateAccessOnceMemory[W vm.Word[W], F field.Element[F]](
	ctx schema.ModuleId, m *vm.Memory[W], name trace.ModuleName) (mod mir.Module[F]) {
	var (
		memoryModule *schema.Table[F, mir.Constraint[F]]
		padding      big.Int
		regs         = toRegisters(m.Registers())
	)

	// Initialise module and add all registers.  AllowPadding (first flag) must
	// be true so a leading padding row is inserted, which the ACCESS[0]=0 /
	// addresses-vanish-in-padding constraints rely on.  Memory modules are never
	// native.
	memoryModule = memoryModule.Init(name, true, true, false, false, false, 0)
	memoryModule.AddRegisters(regs...)

	var access = register.NewId(memoryModule.Width())
	memoryModule.AddRegisters(register.NewComputed(io.ACCESS_BIT_NAME, 1, padding))

	var (
		addrRegs           = toRegisters(m.AddressRegisters())
		isMultiLineAddress = m.NumInputs() > 1
		prevAccess         = mirc.Variable[register.Id, Expr[F]](access, 1, -1)
		currAccess         = mirc.Variable[register.Id, Expr[F]](access, 1, 0)
		nextAccess         = mirc.Variable[register.Id, Expr[F]](access, 1, 1)
		zero               = mirc.Number[register.Id, Expr[F]](0)
		one                = mirc.Number[register.Id, Expr[F]](1)
		constraints        = []mir.Constraint[F]{}
	)

	// ================================================
	// ACCESS bit constraints
	// ================================================

	// the ACCESS bit separates accessible rows from non-accessible rows
	// ACCESS[i] = 0 ⇔ i is a padding row. We impose that traces start
	// with a padding row and that padding rows can't follow non-padding rows,
	// i.e. ACCESS bit monontony

	// ACCESS[0] = 0
	accessBitVanishesInPadding := mir.NewVanishingConstraint("access_bit_vanishes_in_padding", ctx, util.Some(0),
		currAccess.Equals(zero).AsLogical())
	// ACCESS[i - 1] = 1 => ACCESS[i] = 1
	accessBitMonotony := mir.NewVanishingConstraint("access_bit_monotony", ctx, util.None[int](),
		mirc.If(prevAccess.Equals(one), currAccess.Equals(one)).AsLogical())

	constraints = append(constraints,
		accessBitVanishesInPadding,
		accessBitMonotony,
	)

	// ================================================
	// []ADDRESS constraints
	// ================================================

	// We will impose the following:
	//
	//	- if ACCESS[i] = 0
	//		- Then []ADDRESS[i] ≡ 0
	//	- if ACCESS[i-1] = 0 ∧ ACCESS[i] = 1 then
	//		- []ADDRESS[i] ≡ 0

	if isMultiLineAddress {
		constraints = append(constraints,
			multiLineAddressConstraints(ctx, memoryModule, addrRegs, prevAccess, currAccess, zero, one, padding)...)
	} else {
		constraints = append(constraints,
			singleLineAddressConstraints(ctx, addrRegs, currAccess, nextAccess, zero, one)...)
	}

	memoryModule.AddConstraints(constraints...)

	return memoryModule
}

// singleLineAddressConstraints builds the []ADDRESS constraints for a memory
// with a single address register: the address vanishes in padding, is zero on
// the first non-padding row, and increments by one across non-padding rows.
func singleLineAddressConstraints[F field.Element[F]](
	ctx schema.ModuleId, addrRegs []register.Register, currAccess, nextAccess, zero, one Expr[F],
) []mir.Constraint[F] {
	var (
		currAddr = mirc.Variable[register.Id, Expr[F]](register.NewId(uint(0)), addrRegs[0].Width(), 0)
		nextAddr = mirc.Variable[register.Id, Expr[F]](register.NewId(uint(0)), addrRegs[0].Width(), 1)
	)

	return []mir.Constraint[F]{
		// If ACCESS[i] = 0 Then ADDRESS[i] = 0
		mir.NewVanishingConstraint("addresses_vanish_in_padding", ctx, util.None[int](),
			mirc.If(currAccess.Equals(zero), currAddr.Equals(zero)).AsLogical()),
		// If ACCESS[i] = 0 ∧ ACCESS[i + 1] = 1 Then ADDRESS[i + 1] = 0
		mir.NewVanishingConstraint("first_nontrivial_address_is_zero", ctx, util.None[int](),
			mirc.If(currAccess.Equals(zero),
				mirc.If(nextAccess.NotEquals(zero), nextAddr.Equals(zero))).AsLogical()),
		// If ACCESS[i] = 1 Then ADDRESS[i + 1] = 1 + ADDRESS[i]
		mir.NewVanishingConstraint("address_monotony", ctx, util.None[int](),
			mirc.If(currAccess.NotEquals(zero), nextAddr.Equals(currAddr.Add(one))).AsLogical()),
	}
}

// multiLineAddressConstraints builds the []ADDRESS constraints for a memory
// whose address spans more than one register (limb). It adds the one-hot
// at_flag registers that locate the carry-stop limb when the address is
// incremented by one, and constrains the limbs around it. [0]ADDRESS is the
// most significant limb.
//
// The at_flag scheme: exactly one '@k' flag is active on any non-padding row
// whose predecessor is also non-padding, i.e. Σ_k @k[i] = ACCESS[i-1] ∙ ACCESS[i]
// (the @k are bitwidth-1 registers, so binarity is implicit). When @k[i] = 1:
//   - [a]ADDRESS unchanged from rows i-1 to i, for 0 ≤ a < k, the most significant limbs
//   - [k]ADDRESS[i-1] ≠ max, [k]ADDRESS[i] = 1 + [k]ADDRESS[i-1]   (carry stop)
//   - [b]ADDRESS[i-1] = max, [b]ADDRESS[i] = 0    for k < b < L    (roll over)
func multiLineAddressConstraints[F field.Element[F]](
	ctx schema.ModuleId, memoryModule *schema.Table[F, mir.Constraint[F]], addrRegs []register.Register,
	prevAccess, currAccess, zero, one Expr[F], padding big.Int,
) []mir.Constraint[F] {
	var (
		L            = len(addrRegs)
		prevAddrRegs = make([]Expr[F], L)
		currAddrRegs = make([]Expr[F], L)
	)
	for k := range L {
		prevAddrRegs[k] = mirc.Variable[register.Id, Expr[F]](register.NewId(uint(k)), addrRegs[k].Width(), -1)
		currAddrRegs[k] = mirc.Variable[register.Id, Expr[F]](register.NewId(uint(k)), addrRegs[k].Width(), 0)
	}

	// Add the one-hot at_flag registers (one per limb) and cache their access
	// expressions plus each limb's max value.
	var (
		atFlagVars        = make([]Expr[F], L)
		addrLimbMaxValues = make([]Expr[F], L)
	)
	for k := range L {
		atFlag := register.NewId(memoryModule.Width())
		memoryModule.AddRegisters(register.NewComputed(io.AtFlagName(uint(k)), 1, padding))
		atFlagVars[k] = mirc.Variable[register.Id, Expr[F]](atFlag, 1, 0)
		addrLimbMaxValues[k] = mirc.BigNumber[register.Id, Expr[F]](addrRegs[k].MaxValue())
	}

	constraints := []mir.Constraint[F]{
		// Σ_k @k[i] = ACCESS[i-1] ∙ ACCESS[i]
		mir.NewVanishingConstraint("at_flag_sum_equals_access_bit_product", ctx, util.None[int](),
			mirc.Sum(atFlagVars).Equals(mirc.Product(prevAccess, currAccess)).AsLogical()),
	}

	for k := range L {
		// if ACCESS[i] = 0 Then [k]ADDRESS[i] = 0
		constraints = append(constraints, mir.NewVanishingConstraint(
			fmt.Sprintf("addr_%d_vanishes_in_padding", k), ctx, util.None[int](),
			mirc.If(currAccess.Equals(zero), currAddrRegs[k].Equals(zero)).AsLogical()))
		// if ACCESS[i-1] = 0 ∧ ACCESS[i] = 1 Then [k]ADDRESS[i] = 0
		constraints = append(constraints, mir.NewVanishingConstraint(
			fmt.Sprintf("addr_%d_vanishes_on_first_non_padding_row", k), ctx, util.None[int](),
			mirc.If(prevAccess.Equals(zero),
				mirc.If(currAccess.Equals(one), currAddrRegs[k].Equals(zero))).AsLogical()))
	}

	// Per-limb address-update constraints, all guarded by ACCESS[i-1] = 1 ∧ @k[i] = 1.
	for k := range L {
		guarded := func(name string, body Expr[F]) mir.Constraint[F] {
			return mir.NewVanishingConstraint(name, ctx, util.None[int](),
				mirc.If(prevAccess.Equals(one), mirc.If(atFlagVars[k].Equals(one), body)).AsLogical())
		}

		// more significant limbs (0 ≤ a < k): unchanged
		for a := range k {
			constraints = append(constraints, guarded(
				fmt.Sprintf("@%d_addr_%d_curr_equals_prev", k, a),
				currAddrRegs[a].Equals(prevAddrRegs[a])))
		}

		// k-th limb (carry stop): prev ≠ max, curr = 1 + prev
		constraints = append(constraints,
			guarded(fmt.Sprintf("@%d_addr_%d_prev_not_max_value", k, k),
				prevAddrRegs[k].NotEquals(addrLimbMaxValues[k])),
			guarded(fmt.Sprintf("@%d_addr_%d_equals_one_plus_prev", k, k),
				currAddrRegs[k].Equals(prevAddrRegs[k].Add(one))))

		// less significant limbs (k < b < L): roll over (prev = max, curr = 0)
		for b := k + 1; b < L; b++ {
			constraints = append(constraints,
				guarded(fmt.Sprintf("@%d_addr_%d_prev_equals_max_value", k, b),
					prevAddrRegs[b].Equals(addrLimbMaxValues[b])),
				guarded(fmt.Sprintf("@%d_addr_%d_curr_equals_zero", k, b),
					currAddrRegs[b].Equals(zero)))
		}
	}

	return constraints
}

func translateFunction[W vm.Word[W], F field.Element[F]](ctx schema.ModuleId, fn *vm.Function[W],
	infos []vm.Module[W], rangeTables map[uint]rangeTable, maxStaticWidth uint) mir.Module[F] {
	var (
		padding big.Int
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = trace.ModuleName{Name: fn.Name(), Multiplier: 1}
		regs    = toRegisters(fn.Registers())
		framing Framing[F]
		// IS_PC_<k> program counter selectors, only for MLI.
		pcSelectors []register.Id
		// $ret register, used to guard lookup for OLI.
		// TODO: see https://github.com/LFDT-Lineth/zkc/issues/1975
		ret register.Id
	)
	// Initialise module
	mod = mod.Init(name, false, true, false, fn.IsNative(), false, 0)
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
		mod.AddRegisters(register.NewComputed(io.RET_NAME, 1, padding))
		// Create program counter
		mod.AddRegisters(register.NewComputed(io.PC_NAME, fn.PcWidth(), padding))
		// Add IS_PC_<k> program counter selectors (one per code line)
		pcSelectors = make([]register.Id, len(fn.Vectors()))
		for c := range pcSelectors {
			pcSelectors[c] = register.NewId(mod.Width())
			mod.AddRegisters(register.NewComputed(io.SelectorName(uint(c)), 1, padding))
		}
		// Initialise multi-line framing
		framing, constraints = initMultiLineFraming[F](ctx, pc, ret, pcSelectors, regs, len(fn.Vectors()))
		// Include framing constraints
		mod.AddConstraints(constraints...)
	} else {
		framing = mirc.NewAtomicFraming[register.Id, Expr[F]]()

		mod.AddRegisters(register.NewComputed(io.RET_NAME, 1, padding))
	}
	// Translate all bytecode vectors
	for pc, vec := range fn.Vectors() {
		var (
			handle = fmt.Sprintf("pc%d", pc)
			// construct translator for this bytecode vector
			tr = NewVectorTranslator(ctx, uint(pc), vec, framing, fn)
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
	addRangeProofConstraints(mod, ctx, mod.Registers(), rangeTables, maxStaticWidth)
	// Emit lookup constraints for any function calls made by this function.
	addCallLookups(mod, ctx, fn, pcSelectors, ret, infos, regs)
	// TODO: add memory read / write constraints (as lookups).
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
