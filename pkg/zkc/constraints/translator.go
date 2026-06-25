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
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// GenerateMirConstraints is responsible for converting a field machine into a
// corresponding set of MIR constraints.
func GenerateMirConstraints[F field.Element[F]](fm *vm.FieldMachine[F]) mir.Schema[F] {
	var (
		modules = make([]mir.Module[F], len(fm.Modules()))
	)
	//
	for i, m := range fm.Modules() {
		modules[i] = translateModule[F](uint(i), m)
	}
	//
	return schema.NewUniformSchema(modules)
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

func translateModule[F field.Element[F]](ctx schema.ModuleId, fm vm.Module) mir.Module[F] {
	switch fm := fm.(type) {
	case *vm.FieldFunction:
		return translateFunction[F](ctx, *fm)
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

func translateReadOnlyMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	return translateAccessOnceMemory(ctx, fm, name)
}

// Write once memory and read only memory are equivalent on the constraints level
func translateWriteOnceMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	return translateAccessOnceMemory(ctx, fm, name)
}

func translateReadWriteMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var (
		name           = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
		memoryModule   *schema.Table[F, mir.Constraint[F]]
		padding        big.Int
		timestampWidth = uint(32)
	)

	// Initialise module and add all registers
	memoryModule = memoryModule.Init(name, false, true, false, fm.IsNative(), false, 0)
	memoryModule.AddRegisters(fm.Registers()...)

	var (
		timestampRead    = register.NewId(memoryModule.Width() + 0)
		timestampWritten = register.NewId(memoryModule.Width() + 1)
		timestampDelta   = register.NewId(memoryModule.Width() + 2)
	)

	memoryModule.AddRegisters(register.NewComputed("timestamp_read", timestampWidth, padding))
	memoryModule.AddRegisters(register.NewComputed("timestamp_write", timestampWidth, padding))
	memoryModule.AddRegisters(register.NewComputed("timestamp_delta", timestampWidth, padding))

	var (
		addressWidth uint
		valueWidth   uint
	)

	for i, l := range fm.Registers() {
		if uint(i) < fm.Geometry().AddressLines() {
			addressWidth += l.Width()
		} else if uint(i) < fm.Geometry().AddressLines()+fm.Geometry().DataLines() {
			valueWidth += l.Width()
		}
	}

	var (
		addressDelta = register.NewId(memoryModule.Width() + 0)
	)

	memoryModule.AddRegisters(register.NewComputed("address_delta", addressWidth, padding))
	memoryModule.AddRegisters(register.NewComputed("valueRead", valueWidth, padding))

	var (
		execPhase = register.NewId(memoryModule.Width() + 0)
		finlPhase = register.NewId(memoryModule.Width() + 1)
	)

	memoryModule.AddRegisters(register.NewComputed("exec", 1, padding))
	memoryModule.AddRegisters(register.NewComputed("finl", 1, padding))

	var (
		rTime         = mirc.Variable[register.Id, Expr[F]](timestampRead, timestampWidth, 0)
		wTime         = mirc.Variable[register.Id, Expr[F]](timestampWritten, timestampWidth, 0)
		dTime         = mirc.Variable[register.Id, Expr[F]](timestampDelta, timestampWidth, 0)
		addrRegs      = fm.Geometry().AddressRegisters()
		addrRegOffset = uint(0)
		prevAddr      = concatenateRegisters[F](addrRegs, addrRegOffset, -1)
		addr          = concatenateRegisters[F](addrRegs, addrRegOffset, 0)
		addrDelta     = mirc.Variable[register.Id, Expr[F]](addressDelta, 1, 0)
		prevExec      = mirc.Variable[register.Id, Expr[F]](execPhase, 1, -1)
		prevFinl      = mirc.Variable[register.Id, Expr[F]](finlPhase, 1, -1)
		exec          = mirc.Variable[register.Id, Expr[F]](execPhase, 1, 0)
		finl          = mirc.Variable[register.Id, Expr[F]](finlPhase, 1, 0)
		zero          = mirc.Number[register.Id, Expr[F]](0)
		one           = mirc.Number[register.Id, Expr[F]](1)
	)

	// ================================================
	// constraints
	// ================================================

	// (non padding) rows are either created during standard execution (exec ≡ true)
	// or during the finalization phase (finl ≡ true)
	flagExclusivity := mir.NewVanishingConstraint("flag_exclusivity", ctx, util.None[int](),
		mirc.Product([]Expr[F]{exec, finl}).Equals(zero).AsLogical())

	// both exec and (exec + finl) should, on any trace segment, look like one of these :
	//
	//  ¹ ┼       ┌─────         ¹ ┼
	//    │       │                │
	//  ⁰ ┴  ─────┘        or    ⁰ ┴  ───────────
	//
	// exec may not be nondecreasing; the (exec, finl) pair may look like so :
	//
	//  ¹ ┼       ┌─────┐∙∙∙∙∙∙   ( ∙ ≡ finl)
	//    │       │     │
	//  ⁰ ┴  ─────┘∙∙∙∙∙└──────   ( ─ ≡ exec)
	flagMonotony1 := mir.NewVanishingConstraint("finl_monotony", ctx, util.None[int](),
		mirc.If(prevFinl.NotEquals(zero), finl.Equals(one)).AsLogical())
	flagMonotony2 := mir.NewVanishingConstraint("exec+finl_monotony", ctx, util.None[int](),
		mirc.If(mirc.Sum([]Expr[F]{prevExec, prevFinl}).NotEquals(zero),
			mirc.Sum([]Expr[F]{exec, finl}).Equals(one)).AsLogical())

	// we want WT > RT which we prove via WT = RT + (1 + ΔT)
	// which works given that ΔT is ≥ 0
	timestampMonotony := mir.NewVanishingConstraint("timestamp_monotony", ctx, util.None[int](),
		mirc.If(exec.NotEquals(zero), wTime.Equals(rTime.Add(dTime, one))).AsLogical())

	// // we impose value constancy by enforcing that the received value be the same as the sent value
	// rcvExec := mir.NewReceiveConstraint[F]("reading_in_execution_phase",
	// []register.Id{address, timestampRead, valueRead})
	// sndExec := mir.NewSendConstraint[F]("writing_in_execution_phase",
	// []register.Id{address, timestampWritten, valueWritten})

	addressMonotonyInFinl := mir.NewVanishingConstraint("address_monotony_in_finalization_phase", ctx, util.None[int](),
		mirc.If(mirc.Product([]Expr[F]{finl, prevFinl}).NotEquals(zero),
			addr.Equals(prevAddr.Add(addrDelta, one))).AsLogical())

	constraints := []mir.Constraint[F]{
		flagExclusivity,
		flagMonotony1,
		flagMonotony2,
		timestampMonotony,
		// rcvExec,
		// sndExec,
		addressMonotonyInFinl,
	}
	memoryModule.AddConstraints(constraints...)

	return memoryModule
}

// translateAccessOnceMemory handles both
//   - read once memory
//   - write once memory
func translateAccessOnceMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F], name trace.ModuleName) (mod mir.Module[F]) {
	var (
		memoryModule *schema.Table[F, mir.Constraint[F]]
		padding      big.Int
	)

	// Initialise module and add all registers
	memoryModule = memoryModule.Init(name, false, true, false, fm.IsNative(), false, 0)
	memoryModule.AddRegisters(fm.Registers()...)

	var (
		access = register.NewId(memoryModule.Width() + 0)
	)

	memoryModule.AddRegisters(register.NewComputed("access", 1, padding))

	var (
		addrRegs      = fm.Geometry().AddressRegisters()
		addrRegOffset = uint(0)
		addr          = concatenateRegisters[F](addrRegs, addrRegOffset, 0)
		nextAddr      = concatenateRegisters[F](addrRegs, addrRegOffset, 1)
		acc           = mirc.Variable[register.Id, Expr[F]](access, 1, 0)
		nextAcc       = mirc.Variable[register.Id, Expr[F]](access, 1, 1)
		zero          = mirc.Number[register.Id, Expr[F]](0)
		one           = mirc.Number[register.Id, Expr[F]](1)
	)

	// ================================================
	// constraints
	// ================================================

	// ================================================
	// ACCESS bit constraints
	// ================================================

	// ACCESS[0] = 0
	accessBitVanishesInPadding := mir.NewVanishingConstraint("access_bit_vanishes_in_padding", ctx, util.Some[int](0),
		acc.Equals(zero).AsLogical())
	// ACCESS[i] = 1 => ACCESS[i + 1] = 1
	accessBitMonotony := mir.NewVanishingConstraint("access_bit_monotony", ctx, util.None[int](),
		mirc.If(acc.Equals(one), nextAcc.Equals(one)).AsLogical())

	// ================================================
	// []ADDRESS constraints
	// ================================================

	// If ACCESS[i] = 0 Then ADDRESS[i] = 0
	addressesVanishInPadding := mir.NewVanishingConstraint("addresses_vanish_in_padding", ctx, util.None[int](),
		mirc.If(acc.Equals(zero), addr.Equals(zero)).AsLogical())
	// If ACCESS[i] = 0 ∧ ACCESS[i + 1] = 1 Then ADDRESS[i + 1] = 0
	firstNonpaddingAddressIsZero := mir.NewVanishingConstraint("first_nontrivial_address_is_zero", ctx, util.None[int](),
		mirc.If(acc.Equals(zero),
			mirc.If(nextAcc.NotEquals(zero),
				nextAddr.Equals(zero))).AsLogical())
	// If ACCESS[i] = 1 Then ADDRESS[i + 1] = 1 + ADDRESS[i]
	addressMonotony := mir.NewVanishingConstraint("address_monotony", ctx, util.None[int](),
		mirc.If(acc.NotEquals(zero), nextAddr.Equals(addr.Add(one))).AsLogical())

	constraints := []mir.Constraint[F]{
		accessBitVanishesInPadding,
		accessBitMonotony,
		addressesVanishInPadding,
		firstNonpaddingAddressIsZero,
		addressMonotony,
	}
	memoryModule.AddConstraints(constraints...)

	return memoryModule
}

// concatenateRegisters concatenates registers in 'big endian order', e.g.
// [a, b, c] should correspond to a :: b :: c. It assumes that register ids
// are contiguous (and start at id = base).
func concatenateRegisters[F field.Element[F]](registers []register.Register, base uint, shift int) Expr[F] {
	terms := make([]Expr[F], len(registers))
	// cumulative width of registers
	tail := uint(0)
	//
	for j := len(registers) - 1; j >= 0; j-- {
		w := registers[j].Width()
		v := mirc.Variable[register.Id, Expr[F]](register.NewId(base+uint(j)), w, shift)
		weight := new(big.Int).Lsh(big.NewInt(1), tail) // 2^tail
		terms[j] = v.Multiply(mirc.BigNumber[register.Id, Expr[F]](weight))
		tail += w
	}
	//
	return mirc.Sum(terms)
}

func translateFunction[F field.Element[F]](ctx schema.ModuleId, fm vm.FieldFunction) mir.Module[F] {
	var (
		padding big.Int
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
		framing Framing[F]
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
			// one boolean selector register per instruction (code line)
			pcSelectors = make([]register.Id, len(fm.Code()))
		)
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
	// Done
	return mod
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
