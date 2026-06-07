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
	case vm.InputOutputMemory[F]:
		if fm.IsStatic() {
			return translateStaticMemory(ctx, fm)
		} else if fm.IsReadOnly() {
			return translateReadOnlyMemory(ctx, fm)
		}
		//
		return translateWriteOnceMemory(ctx, fm)
	case vm.Memory[F]:
		return translateReadWriteMemory(ctx, fm)
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", fm.Name()))
	}
}

func translateStaticMemory[F field.Element[F]](_ schema.ModuleId, m vm.InputOutputMemory[F]) mir.Module[F] {
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
	ctx schema.ModuleId, fm vm.InputOutputMemory[F]) mir.Module[F] {
	var name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	return translateMemoryCommon(ctx, fm, name)
}

// Write once memory and read only memory are equivalent on the constraints level
func translateWriteOnceMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.InputOutputMemory[F]) mir.Module[F] {
	var name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	return translateMemoryCommon(ctx, fm, name)
}

func translateReadWriteMemory[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F]) mir.Module[F] {
	var name = trace.ModuleName{Name: fm.Name(), Multiplier: 1}
	return translateMemoryCommon(ctx, fm, name)
}

func translateMemoryCommon[F field.Element[F]](
	ctx schema.ModuleId, fm vm.Memory[F], name trace.ModuleName) mir.Module[F] {
	var (
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

	// var address = register.NewId(memoryModule.Width() + 0)
	// var valueRead   = register.NewId(memoryModule.Width() + 1)
	memoryModule.AddRegisters(register.NewComputed("address", addressWidth, padding))
	memoryModule.AddRegisters(register.NewComputed("valueRead", valueWidth, padding))

	var (
		execPhase = register.NewId(memoryModule.Width() + 0)
		finlPhase = register.NewId(memoryModule.Width() + 1)
	)

	memoryModule.AddRegisters(register.NewComputed("exec", 1, padding))
	memoryModule.AddRegisters(register.NewComputed("finl", 1, padding))

	var (
		rTime = mirc.Variable[register.Id, Expr[F]](timestampRead, timestampWidth, 0)
		wTime = mirc.Variable[register.Id, Expr[F]](timestampWritten, timestampWidth, 0)
		dTime = mirc.Variable[register.Id, Expr[F]](timestampDelta, timestampWidth, 0)
		// addr     = mirc.Variable[register.Id, Expr[F]](address,           addressWidth,   0)
		// val      = mirc.Variable[register.Id, Expr[F]](value,             valueWidth,     0)
		execPrev = mirc.Variable[register.Id, Expr[F]](execPhase, 1, -1)
		finlPrev = mirc.Variable[register.Id, Expr[F]](finlPhase, 1, -1)
		exec     = mirc.Variable[register.Id, Expr[F]](execPhase, 1, 0)
		finl     = mirc.Variable[register.Id, Expr[F]](finlPhase, 1, 0)
		zero     = mirc.Number[register.Id, Expr[F]](0)
		one      = mirc.Number[register.Id, Expr[F]](1)
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
		mirc.If(finlPrev.NotEquals(zero), finl.Equals(one)).AsLogical())
	flagMonotony2 := mir.NewVanishingConstraint("exec+finl_monotony", ctx, util.None[int](),
		mirc.If(mirc.Sum([]Expr[F]{execPrev, finlPrev}).NotEquals(zero),
			mirc.Sum([]Expr[F]{exec, finl}).Equals(one)).AsLogical())

	// we want to prove WT - RT = 1 + ΔT (which forces WT > RT given that ΔT is ≥ 0)
	// instead we prove WT = RT + 1 + ΔT
	timestampMonotony := mir.NewVanishingConstraint("timestamp_monotony", ctx, util.None[int](),
		mirc.If(exec.NotEquals(zero), wTime.Equals(rTime.Add(dTime, one))).AsLogical())

	// var isImmutable bool
	// switch fm.(type) {
	// case vm.InputOutputMemory[F]: isImmutable = true
	// case vm.Memory[F]: isImmutable = false
	// default: panic("unknown memory type")
	// }
	//
	// var valueWritten = register.Id
	// if isImmutable {
	// 	valueWritten = valueRead
	// } else {
	// 	valueWritten   = register.NewId(memoryModule.Width() + 0)
	// 	memoryModule.AddRegisters(register.NewComputed("valueWritten", valueWidth, padding))
	// }
	//
	// // we impose value constancy by enforcing that the received value be the same as the sent value
	// rcvExec := mir.NewReceiveConstraint[F]("reading_in_execution_phase",
	// []register.Id{address, timestampRead, valueRead})
	// sndExec := mir.NewSendConstraint[F]("writing_in_execution_phase",
	// []register.Id{address, timestampWritten, valueWritten})

	constraints := []mir.Constraint[F]{flagExclusivity, flagMonotony1,
		flagMonotony2, timestampMonotony} // , rcvExec, sndExec}
	memoryModule.AddConstraints(constraints...)

	return memoryModule
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
