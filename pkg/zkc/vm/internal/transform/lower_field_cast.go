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
	"math/big"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerFieldCasts inserts the canonicality checks required by field conversions.
func LowerFieldCasts[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	var (
		modules = slices.Clone(program.Modules())
		helpers = newFieldCastHelpers[W](uint(len(modules)), program.Field().Modulus())
	)

	for i, mod := range modules {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			modules[i] = lowerFieldCastFunction(fn, helpers)
		}
	}

	return descriptor.NewProgram(program.Field(), append(modules, helpers.modules...)...)
}

func lowerFieldCastFunction[W word.Word[W]](fn *descriptor.Function[W], helpers *fieldCastHelpers[W],
) *descriptor.Function[W] {
	var (
		registers = fn.Registers()
		vectors   = make([]BytecodeVector[W], len(fn.Vectors()))
	)

	for i, vec := range fn.Vectors() {
		vectors[i] = vec.Map(func(_ uint, insn Bytecode[W]) []Bytecode[W] {
			switch cast := insn.(type) {
			case *bytecode.UintToField[W]:
				return append(helpers.check(cast.Source, registers), cast)
			case *bytecode.FieldToUint[W]:
				return append([]Bytecode[W]{cast}, helpers.check(cast.Target, registers)...)
			default:
				return []Bytecode[W]{insn}
			}
		})
	}

	return descriptor.NewFunction(fn.Name(), registers, fn.IsNative(), vectors)
}

type fieldCastHelpers[W word.Word[W]] struct {
	base    uint
	modulus *big.Int
	ids     map[string]uint
	modules []descriptor.Module[W]
}

func newFieldCastHelpers[W word.Word[W]](base uint, modulus *big.Int) *fieldCastHelpers[W] {
	return &fieldCastHelpers[W]{base: base, modulus: modulus, ids: make(map[string]uint)}
}

func (p *fieldCastHelpers[W]) check(regs []bytecode.RegisterId, registers []descriptor.Register[W]) []Bytecode[W] {
	widths := make([]uint, len(regs))
	total := uint(0)

	for i, reg := range regs {
		widths[i] = registers[reg].Bitwidth().Unwrap()
		total += widths[i]
	}
	// A value of total bits cannot reach P: no check needed.
	if total < uint(p.modulus.BitLen()) {
		return nil
	}

	return []Bytecode[W]{bytecode.CallFun[W](uint16(p.ensure(widths)), regs, nil)}
}

func (p *fieldCastHelpers[W]) ensure(widths []uint) uint {
	var builder strings.Builder

	builder.WriteString("$field_range")

	for _, w := range widths {
		fmt.Fprintf(&builder, "_u%d", w)
	}

	name := builder.String()
	if id, ok := p.ids[name]; ok {
		return id
	}

	var padding W

	inputs := make([]bytecode.RegisterId, len(widths))

	regs := make([]descriptor.Register[W], len(widths))
	for i, width := range widths {
		inputs[i] = bytecode.RegisterId(i)
		regs[i] = descriptor.NewRegister(register.INPUT_REGISTER, fmt.Sprintf("arg%d", i), util.Some(width), padding)
	}

	alloc := newRegAllocator(regs)
	code := append(canonicalityCode(inputs, alloc, p.modulus), bytecode.NewRet[W]())
	id := p.base + uint(len(p.modules))
	p.ids[name] = id
	// The pipeline's comparison-lowering pass has already run, so lower the
	// helper's relational SkipIfs at construction.
	p.modules = append(p.modules, lowerComparisonFunction(descriptor.NewFunction(name,
		alloc.Registers(), false, []BytecodeVector[W]{bytecode.NewVector(code...)})))

	return id
}

func canonicalityCode[W word.Word[W]](regs []bytecode.RegisterId, alloc *regAllocator[W],
	modulus *big.Int) []Bytecode[W] {
	type branch struct {
		insn      *bytecode.SkipIf[W]
		position  int
		failOnHit bool
	}

	var (
		checks   []Bytecode[W]
		branches []branch
		shift    uint
		limbs    = make([]W, len(regs))
	)

	for i, reg := range regs {
		width := alloc.Register(reg).Bitwidth().Unwrap()
		limb := new(big.Int).Rsh(modulus, shift)
		limb.And(limb, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width), big.NewInt(1)))
		limbs[i] = limbs[i].SetBigInt(limb)
		shift += width
	}

	for i, reg := range slices.Backward(regs) {
		width := alloc.Register(reg).Bitwidth().Unwrap()
		constant := alloc.Allocate("", util.Some(width))
		checks = append(checks, bytecode.LoadConst(constant, limbs[i]))

		lt := bytecode.NewSkipIf[W](bytecode.CONDITION_LT, 0, reg, constant)
		branches = append(branches, branch{lt, len(checks), false})
		checks = append(checks, lt)

		if i > 0 {
			gt := bytecode.NewSkipIf[W](bytecode.CONDITION_GT, 0, reg, constant)
			branches = append(branches, branch{gt, len(checks), true})
			checks = append(checks, gt)
		}
	}

	fail := len(checks)

	checks = append(checks, bytecode.NewFail[W](nil, nil))
	for _, branch := range branches {
		target := len(checks)
		if branch.failOnHit {
			target = fail
		}

		branch.insn.Skip = uint16(target - branch.position - 1)
	}

	return checks
}
