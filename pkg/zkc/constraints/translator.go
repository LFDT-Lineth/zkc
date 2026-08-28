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

	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	tracer "github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// GenerateMirConstraints is responsible for converting a bytecode program into
// a corresponding set of MIR constraints.  The translation operates directly
// over the bytecode program (its modules, registers and bytecode vectors),
// without going through the legacy word / field machine.
func GenerateMirConstraints[W vm.Word[W], F field.Element[F]](program vm.Program[W]) mir.Schema[F] {
	var (
		modules = make([]mir.Module[F], len(program.Modules()))
		// construct translator
		translator = newConstraintTranslator[W, F](program)
	)
	//
	for i, m := range program.Modules() {
		modules[i] = translator.translateModule(uint(i), m)
	}
	// Emit the bus connecting every global function with its call sites.  This
	// can only happen now, since a bus spans modules and, hence, requires them
	// all to have been translated.  Each bus is placed in its callee's module;
	// as for lookups, placement is cosmetic but must be deterministic.
	for i, m := range program.Modules() {
		if fn, ok := m.(*vm.Function[W]); ok && fn.Kind().IsGlobal() {
			translator.emitCallBus(modules[i], vm.ModuleId(i), fn)
		}
	}
	//
	return schema.NewUniformSchema(modules)
}

// GenerateAirConstraints is responsible for converting a bytecode program into
// a corresponding set of AIR constraints.
func GenerateAirConstraints[W vm.Word[W], F field.Element[F]](program vm.Program[W]) air.Schema[F] {
	var (
		mirc = GenerateMirConstraints[W, F](program)
	)
	//
	return mir.LowerToAir(mirc, program.Field().BandWidth, mir.DEFAULT_OPTIMISATION_LEVEL)
}

// constraintTranslator provides global context required for generating
// constraints.
type constraintTranslator[W vm.Word[W], F field.Element[F]] struct {
	program vm.Program[W]
	// rangeTables indexes the static range-check tables by width, so each
	// register can be range-proved by a lookup into the matching $range_un
	// table.
	rangeTables map[uint]rangeTable
	// sendPorts records the send port of every call into a global function,
	// keyed by the callee.  These are accumulated as each caller is translated,
	// since a bus spans modules and, hence, can only be constructed once every
	// module has been translated (see emitCallBus).
	sendPorts map[vm.ModuleId][]mir.BusPort
	// maxStaticWidth determines the maximum register width for which a static table
	// can be use to range-check it. Wider registers require recursive range
	// modules, and the call is materialized by a function call during codegen.
	maxStaticWidth uint
}

// newConstraintTranslator constructs a new translator with the given context
// required for translation.
func newConstraintTranslator[W vm.Word[W], F field.Element[F]](program vm.Program[W]) constraintTranslator[W, F] {
	var (
		// Calculate maximum register width which can be range-checked using just a
		// static reference table.
		maxStaticWidth = util_math.FloorLog2(program.MaxStaticHeight())
		// Index the static range-check tables by width, so each register can be
		// range-proved by a lookup into the matching $range_un table.
		rangeTables = indexRangeTables[W, F](program, maxStaticWidth)
		// Accumulates the send port of every call into a global function.
		sendPorts = make(map[vm.ModuleId][]mir.BusPort)
	)
	//
	return constraintTranslator[W, F]{program, rangeTables, sendPorts, maxStaticWidth}
}

// addSendPort records a send port for the bus of a given (global) callee.
func (p *constraintTranslator[W, F]) addSendPort(calleeId vm.ModuleId, port mir.BusPort) {
	p.sendPorts[calleeId] = append(p.sendPorts[calleeId], port)
}

func (p *constraintTranslator[W, F]) translateModule(ctx schema.ModuleId, m vm.Module[W]) mir.Module[F] {
	switch m := m.(type) {
	case *vm.Function[W]:
		return p.translateFunction(ctx, m)
	case *vm.Memory[W]:
		if m.IsStatic() {
			return p.translateStaticMemory(ctx, m)
		} else if m.IsReadOnly() {
			return p.translateReadOnlyMemory(ctx, m)
		} else if m.IsWriteOnly() {
			return p.translateWriteOnceMemory(ctx, m)
		}
		//
		return p.translateReadWriteMemory(ctx, m)
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", m.Name()))
	}
}

func (p *constraintTranslator[W, F]) translateStaticMemory(_ schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var (
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = m.Name()
		regs    = toRegisters(m.Registers())
		inputs  = toRegisters(m.AddressRegisters())
		outputs = toRegisters(m.DataRegisters())
		// Convert the static contents from words into field elements.
		contents        = toFieldElements[W, F](m.StaticContents())
		paddedHeight    = util_math.NextPowerOfTwo(uint(len(contents)))
		maxStaticHeight = p.program.MaxStaticHeight()
	)
	if paddedHeight > maxStaticHeight {
		panic(fmt.Sprintf("static memory \"%s\" exceeds maximum allowed height of %d", m.Name(), maxStaticHeight))
	}
	// Initialise module as a static reference table.  Memory modules are never
	// native.
	mod = mod.Init(name, false, false, false, false, true)
	// Add all registers
	mod.AddRegisters(regs...)
	// Populate the table contents from the pre-loaded memory, padded to the
	// next power-of-two height.
	mod.SetStaticContents(padStaticTables(foldContents(inputs, outputs, contents)))
	//
	return mod
}

func (p *constraintTranslator[W, F]) translateReadOnlyMemory(ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var name = m.Name()
	return p.translateAccessOnceMemory(ctx, m, name)
}

// Write once memory and read only memory are equivalent on the constraints level
func (p *constraintTranslator[W, F]) translateWriteOnceMemory(ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	var name = m.Name()
	return p.translateAccessOnceMemory(ctx, m, name)
}

// translateAccessOnceMemory handles both
//   - read once memory
//   - write once memory
func (p *constraintTranslator[W, F]) translateAccessOnceMemory(ctx schema.ModuleId, m *vm.Memory[W], name string,
) (mod mir.Module[F]) {
	var (
		memoryModule *schema.Table[F, mir.Constraint[F]]
		regs         = toRegisters(m.Registers())
	)

	// Initialise module and add all registers.  Note the ACCESS[0]=0 /
	// addresses-vanish-in-padding constraints rely on the leading padding row
	// inserted during trace expansion.  Memory modules are never native.
	memoryModule = memoryModule.Init(name, m.IsPublic() && m.IsWriteOnly(), !m.IsPublic() && m.IsWriteOnly(),
		false, false, false)
	memoryModule.AddRegisters(regs...)

	var access = register.NewId(memoryModule.Width())
	memoryModule.AddRegisters(register.NewComputed(tracer.ACCESS_BIT_NAME, 1))

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
			multiLineAddressConstraints(ctx, memoryModule, addrRegs, prevAccess, currAccess, zero, one)...)
	} else {
		constraints = append(constraints,
			singleLineAddressConstraints(ctx, addrRegs, currAccess, nextAccess, zero, one)...)
	}

	memoryModule.AddConstraints(constraints...)
	p.addRangeProofConstraints(memoryModule, ctx, memoryModule.Registers())

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
	prevAccess, currAccess, zero, one Expr[F]) []mir.Constraint[F] {
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
		memoryModule.AddRegisters(register.NewComputed(tracer.AtFlagName(uint(k)), 1))
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
