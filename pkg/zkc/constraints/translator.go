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
		)
		// Create program counter
		mod.AddRegisters(register.NewComputed(io.PC_NAME, pcWidth, padding))
		// Create return line
		mod.AddRegisters(register.NewComputed(io.RET_NAME, 1, padding))
		// Initialise multi-line framing
		framing, constraints = initMultiLineFraming[F](ctx, pc, ret, fm)
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

func initMultiLineFraming[F field.Element[F]](ctx module.Id, pc, ret register.Id, fn vm.FieldFunction,
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
	//
	constraints := []mir.Constraint[F]{padding, init, reset, first}
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
	return mirc.NewMultiLineFraming[register.Id, Expr[F]](pc, pcWidth, ret, 1), constraints
}
