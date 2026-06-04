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
package asm

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/hir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Element provides a convenient shorthand.
type Element[F any] = field.Element[F]

// UniformSchema provides a convenient shorthand.
type UniformSchema[F field.Element[F]] = sc.UniformSchema[F, mir.Module[F]]

// FieldAgnostic captures the notion of an entity (e.g. module, constraint or
// assignment) which is agnostic to the underlying field being used.

// Concretize field agnostic entities (e.g. modules, constraints or assignments)
// to a specific concrete field.  To make this possible, any registers used
// within (and constraints, etc) will be subdivided as necessary to ensure a
// maximum bandwidth requirement is met. Here, bandwidth refers to the maximum
// number of data bits which can be stored in the underlying field. As a simple
// example, the prime field F_7 has a bandwidth of 2bits.  Two parameters are
// given in the field configuration to specify the target field: the maximum
// bandwidth (as determined by the modulus); the maximum register width (which
// should be smaller than the bandwidth).  The maximum register width determines
// the maximum permitted width of any register after subdivision. Since every
// register value will be stored as a field element, it follows that the maximum
// width cannot be greater than the bandwidth. However, in practice, we want it
// to be marginally less than the bandwidth to ensure there is some capacity for
// calculations involving registers.
//
// As part of concretization, registers wider than the maximum permitted width
// are split into two or more "limbs" (i.e. subregisters which do not exceeded
// the permitted width). For example, consider a register "r" of width u32.
// Subdividing this register into registers of at most 8bits will result in four
// limbs: r'0, r'1, r'2 and r'3 where (by convention) r'0 is the least
// significant.  As part of this process, constraints may also need to be
// divided when they exceed the maximum permitted bandwidth.  For example,
// consider a simple constraint such as "x = y + 1" using 16bit registers x,y.
// Subdividing for a bandwidth of 10bits and a maximum register width of 8bits
// means splitting each register into two limbs, and transforming our constraint
// into:
//
// 256*x'1 + x'0 = 256*y'1 + y'0 + 1
//
// However, as it stands, this constraint exceeds our bandwidth requirement
// since it requires at least 17bits of information to safely evaluate each
// side.  Thus, the constraint itself must be subdivided into two parts:
//
// 256*c + x'0 = y'0 + 1  // lower
//
//	x'1 = y'1 + c  // upper
//
// Here, c is a 1bit register introduced as part of the transformation to act as
// a "carry" between the two constraints.
func Concretize[F Element[F]](cfg field.Config, hp MicroHirProgram,
) (MicroMirProgram[F], module.LimbsMap) {
	var (
		fns = hp.program.Components()
		// Lower HIR program first.  This is necessary to ensure any registers
		// added during this process are included in the subsequent limbs map.
		p = NewMixedProgram(hp.program, hir.LowerToMir(fns, hp.externs)...)
		// Construct a limbs map which determines the mapping of all registers
		// into their limbs.
		mapping = module.NewLimbsMap[F](cfg, p.Modules().Collect()...)
	)
	// Split registers in assembly functions
	ap := subdivideProgram(mapping, p.program)
	// Concretize legacy components
	mirModules := mir.Concretize[word.BigEndian, F](mapping, ap.Components(), p.Externs())
	// Done
	return NewMixedProgram(ap, mirModules...), mapping
}

// Compile a mixed micro program into a uniform MIR schema.
func Compile[F Element[F]](p MicroMirProgram[F]) UniformSchema[F] {
	var (
		// Construct a limbs map which determines the mapping of all registers
		// into their limbs.
		n = len(p.Components())
		// Construct compiler
		comp    = compiler.NewCompiler[F, register.Id, compiler.MirExpr[F], compiler.MirModule[F]]()
		modules = make([]mir.Module[F], p.Width())
	)
	// Compile subdivided assembly components into MIR
	comp.Compile(p.program)
	// Copy over compiled components
	for i, m := range comp.Modules() {
		modules[i] = ir.BuildModule[F, mir.Constraint[F], mir.Term[F], mir.Module[F]](m.Module)
	}
	// Concretize legacy components
	copy(modules[n:], p.Externs())
	// compile constant registers (in non-zkasm modules).
	mir.InitialiseConstantRegisters(uint(n), modules[n:])
	// Done
	return schema.NewUniformSchema(modules)
}

// Subdivide a given program.  In principle, this should be located within
// io.Program, however this would require io.Instruction to have a
// SplitRegisters method (which we want to avoid right now).
func subdivideProgram(mapping module.LimbsMap, p MicroProgram) MicroProgram {
	var (
		fns  = p.Components()
		nfns = make([]MicroComponent, len(fns))
	)
	// Split functions
	for i, fn := range fns {
		nfns[i] = subdivideComponent(mapping, fn)
	}
	// Constuct subdivided program
	p = io.NewProgram(nfns)
	// Construct executor for padding inference
	executor := io.NewExecutor(p)
	// Infer padding
	for _, nfn := range nfns {
		io.InferPadding(nfn, executor)
	}
	// Done
	return p
}

// Subdivide a given component according to a given limbs map.  This means all
// registers are translated into limbs according to the map.  Furthermore, other
// forms of subdivision are applied as necessary based on the type of the
// component.
func subdivideComponent(mapping module.LimbsMap, fn MicroComponent) MicroComponent {
	switch fn := fn.(type) {
	case *MicroFunction:
		return subdivideFunction(mapping, fn)
	default:
		panic("unknown component")
	}
}

// Subdivide a given function according to a given limbs map.  This means all
// registers are translated into limbs according to the map and, furthermore,
// all instructions are split as necessary to prevent overflowing the underlying
// field element.
func subdivideFunction(mapping module.LimbsMap, fn *MicroFunction) *MicroFunction {
	var (
		modmap = mapping.ModuleOf(fn.Name())
		// Construct suitable splitting environment
		env = register.NewAllocator[term.Computation[word.BigEndian]](modmap.LimbsMap())
		// Updated instruction sequence
		ninsns []micro.Instruction
		nbuses []io.Bus = make([]io.Bus, len(fn.Buses()))
	)
	// Split instructions
	for _, insn := range fn.Code() {
		ninsns = append(ninsns, insn.SplitRegisters(modmap, env))
	}
	// Split buses
	for i, bus := range fn.Buses() {
		nbuses[i] = bus.Split(modmap, env)
	}
	//
	nfn := io.NewFunction(fn.Name(), fn.IsPublic(), env.Registers(), nbuses, ninsns)
	// Done
	return &nfn
}
