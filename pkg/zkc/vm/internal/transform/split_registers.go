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
package transform

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// SplitRegisters splits all registers in a program to meet a given field's
// bandwidth and maximum register width.  This will split all registers wider
// than the maximum permitted width into two or more "limbs" (i.e. subregisters
// which do not exceeded the permitted width). For example, consider a register
// "r" of width u32. Subdividing this register into registers of at most 8bits
// will result in four limbs: r'0, r'1, r'2 and r'3 where (by convention) r'0 is
// the least significant.
func SplitRegisters[W word.Word[W]](cfg field.Config, program descriptor.Program[W]) descriptor.Program[W] {
	var (
		mods    = program.Modules()
		mapping = newLimbsMap[W](cfg, mods)
		out     = make([]descriptor.Module[W], len(mods))
	)
	//
	for i, ith := range mods {
		// Determine limb mapping for this module
		limbsMap := mapping.Module(uint(i))
		//
		out[i] = splitModule[W](limbsMap, ith)
	}
	//
	return descriptor.NewProgram(out...)
}

// newLimbsMap constructs the program-wide limbs map from a program's descriptor
// modules, deriving a register map per module from its (schema) registers.
func newLimbsMap[W word.Word[W]](cfg field.Config, modules []descriptor.Module[W]) module.LimbsMap {
	var ms []register.Map = array.Map(modules, func(_ uint, m descriptor.Module[W]) register.Map {
		name := trace.ModuleName{Name: m.Name(), Multiplier: 1}
		return register.ArrayMap(name, descriptor.ToRegisters(m.Registers()...)...)
	})
	// NOTE: generic parameter is meaningless, and only retained for backwards
	// compatibility.
	return module.NewLimbsMap[uint](cfg, ms...)
}

func splitModule[W word.Word[W]](mapping register.LimbsMap, m descriptor.Module[W]) descriptor.Module[W] {
	switch m := m.(type) {
	case *descriptor.Function[W]:
		return splitFunction[W](mapping, m)
	case *descriptor.Memory[W]:
		return splitMemory[W](mapping, m)
	default:
		panic("unknown module encountered")
	}
}

func splitMemory[W word.Word[W]](mapping register.LimbsMap, m *descriptor.Memory[W]) descriptor.Module[W] {
	var registers = limbsToDescriptor[W](mapping.Limbs())
	//
	switch {
	case m.IsStatic():
		panic("support subdivision for static ROM")
	case m.IsWriteOnly(), m.IsReadOnly():
		return descriptor.NewMemory(m.Name(), registers, m.Kind(), nil)
	default:
		panic(fmt.Sprintf("unknown memory \"%s\"", m.Name()))
	}
}

func splitFunction[W word.Word[W]](mapping register.LimbsMap, m *descriptor.Function[W]) descriptor.Module[W] {
	var (
		registers = limbsToDescriptor[W](mapping.Limbs())
		code      = splitInstructions[W](mapping, m.Vectors())
	)
	//
	return descriptor.NewFunction(m.Name(), registers, m.IsNative(), code)
}

func splitInstructions[W word.Word[W]](mapping register.LimbsMap, code []BytecodeVector[W]) []BytecodeVector[W] {
	var ncode = make([]BytecodeVector[W], len(code))
	//
	for i, c := range code {
		ncode[i] = splitInstruction[W](mapping, c)
	}
	//
	return ncode
}

func splitInstruction[W word.Word[W]](limbsMap register.LimbsMap, vec BytecodeVector[W]) BytecodeVector[W] {
	var insns []Bytecode[W]
	//
	for _, c := range vec.Bytecodes {
		switch c := c.(type) {
		// =======================================================
		// Base bytecodes
		// =======================================================
		case *bytecode.Call:
			insns = append(insns, splitRegisters[W](limbsMap, c))
		case *bytecode.Debug:
			insns = append(insns, &bytecode.Debug{Chunks: c.Chunks, Sources: splitRegVecs(limbsMap, c.Sources)})
		case *bytecode.Fail:
			insns = append(insns, &bytecode.Fail{Chunks: c.Chunks, Sources: splitRegVecs(limbsMap, c.Sources)})
		case *bytecode.Jmp:
			insns = append(insns, c)
		case *bytecode.ReadWrite:
			insns = append(insns, splitRegisters[W](limbsMap, c))
		case *bytecode.Ret:
			insns = append(insns, c)
		case *bytecode.Skip:
			insns = append(insns, c)
		case *bytecode.SkipIf:
			insns = append(insns, splitRegisters[W](limbsMap, c))

		// =======================================================
		// Arithmetic bytecodes
		// =======================================================
		case *bytecode.Arith[W]:
			switch c.Op {
			case bytecode.OP_ADD:
				insns = append(insns, splitAddition[W](limbsMap, c)...)
			case bytecode.OP_SUB:
				insns = append(insns, splitSubtraction[W](limbsMap, c)...)
			case bytecode.OP_MUL:
				insns = append(insns, splitMultiplication[W](limbsMap, c)...)
			default:
				panic(fmt.Sprintf("unsupported arithmetic operation (%d)", c.Op))
			}
		default:
			panic(fmt.Sprintf("unsupported bytecode (%T)", c))
		}
	}
	//
	return bytecode.NewVector(insns...)
}

func splitRegisters[W word.Word[W]](limbsMap register.LimbsMap, insn Bytecode[W]) Bytecode[W] {
	switch c := insn.(type) {
	case *bytecode.Call:
		args := applyLimbs(limbsMap, c.Arguments...)
		rets := applyLimbs(limbsMap, c.Returns...)
		//
		return bytecode.CallFun(c.Target, c.Flags, args, rets)
	case *bytecode.ReadWrite:
		addr := applyLimbs(limbsMap, c.Address...)
		data := applyLimbs(limbsMap, c.Data...)
		//
		if c.Write {
			return bytecode.NewMemWrite(c.Id, addr, data)
		}
		//
		return bytecode.NewMemRead(c.Id, addr, data)
	case *bytecode.SkipIf:
		left := applyLimbs(limbsMap, c.Left.Registers()...)
		right := applyLimbs(limbsMap, c.Right.Registers()...)
		// Construct vectored form of skip_if
		return &bytecode.SkipIf{Op: c.Op, Left: bytecode.NewRegVec(left...), Right: bytecode.NewRegVec(right...),
			Skip: c.Skip}
	default:
		panic("unsupported instruction")
	}
}

// splitRegVecs splits each register vector (e.g. a debug / fail formatted
// argument) into the limbs of its constituent registers.
func splitRegVecs(limbsMap register.LimbsMap, vecs []bytecode.RegVec) []bytecode.RegVec {
	var nvecs = make([]bytecode.RegVec, len(vecs))
	//
	for i, v := range vecs {
		nvecs[i] = bytecode.NewRegVec(applyLimbs(limbsMap, v.Registers()...)...)
	}
	//
	return nvecs
}

func splitAddition[W word.Word[W]](limbsMap register.LimbsMap, insn *bytecode.Arith[W]) []Bytecode[W] {
	var (
		target  = applyLimbs(limbsMap, insn.Target...)
		sources = applyLimbs(limbsMap, insn.Source...)
	)
	// FIXME: this is a temporary place holder to allow some tests to actually
	// run.  It is not a proper implementation of this function.
	if len(target) > 1 {
		// A pure copy (single source, zero constant) splits into limb-wise
		// copies, provided source and target decompose into identical limbs.
		// Such copies arise (for example) from function inlining.
		if len(insn.Source) == 1 && insn.Constant.Cmp64(0) == 0 && limbsAligned(limbsMap, target, sources) {
			var insns = make([]Bytecode[W], len(target))
			//
			for i := range target {
				insns[i] = bytecode.Move[W](target[i], sources[i])
			}
			//
			return insns
		}
		// TODO: this is where we actually need to do something
		panic("todo")
	}
	//
	return []Bytecode[W]{bytecode.AddConst(target[0], sources, insn.Constant)}
}

// limbsAligned checks whether two sets of limbs decompose identically (i.e. pair
// up one-to-one with matching widths), such that an assignment between the
// original registers can be split into limb-wise assignments.
func limbsAligned(limbsMap register.LimbsMap, target, sources []bytecode.RegisterId) bool {
	var limbs = limbsMap.Limbs()
	//
	if len(target) != len(sources) {
		return false
	}
	//
	for i := range target {
		var (
			ith = limbs[target[i]]
			jth = limbs[sources[i]]
		)
		//
		if ith.IsNative() != jth.IsNative() || (!ith.IsNative() && ith.Width() != jth.Width()) {
			return false
		}
	}
	//
	return true
}

func splitSubtraction[W word.Word[W]](_ register.LimbsMap, _ *bytecode.Arith[W]) []Bytecode[W] {
	panic("todo")
}

func splitMultiplication[W word.Word[W]](_ register.LimbsMap, _ *bytecode.Arith[W]) []Bytecode[W] {
	panic("todo")
}

// applyLimbs maps a set of (bytecode) registers onto the corresponding limb
// registers, returning the result in bytecode-register currency.
func applyLimbs(limbsMap register.LimbsMap, rids ...bytecode.RegisterId) []bytecode.RegisterId {
	var ids = make([]register.Id, len(rids))
	//
	for i, r := range rids {
		ids[i] = register.NewId(uint(r))
	}
	//
	var (
		lids  = register.ApplyLimbsMap(limbsMap, ids...)
		limbs = make([]bytecode.RegisterId, len(lids))
	)
	//
	for i, l := range lids {
		limbs[i] = util.Cast[uint16](l.Unwrap())
	}
	//
	return limbs
}

// limbsToDescriptor converts a set of (schema) limb registers into descriptor
// registers, preserving each limb's kind, width and padding.
func limbsToDescriptor[W word.Word[W]](limbs []register.Register) []descriptor.Register[W] {
	var out = make([]descriptor.Register[W], len(limbs))
	//
	for i, r := range limbs {
		var padding W
		//
		padding = padding.SetBigInt(r.Padding())
		//
		if r.IsNative() {
			out[i] = descriptor.NewRegister(r.Kind(), r.Name(), util.None[uint](), padding)
		} else {
			out[i] = descriptor.NewRegister(r.Kind(), r.Name(), util.Some(r.Width()), padding)
		}
	}
	//
	return out
}
