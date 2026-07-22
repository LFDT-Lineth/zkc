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

// LowerFieldCasts inserts the canonicality checks required when extracting a
// native field value into uint registers.  Uint→𝔽 casts reduce modulo P and
// therefore require no check.  For 𝔽→uint, the extracted value has several
// integer representatives (W, W+P, …) whenever the target is wide enough to
// reach P, so a check is needed to pin the canonical one (W < P).
//
// The check is emitted as a high-level "value < P" comparison against the field
// modulus.  The standard comparison-lowering (LowerComparisons) and register-
// splitting (SplitRegisters) passes then turn it into an efficient subtract-
// with-borrow chain (a single Arith(OP_SUB) yielding per-limb differences plus
// a final borrow, whose sign bit is the answer), rather than a lexicographic
// limb-by-limb comparison.  Consequently this transform MUST run before
// LowerComparisons and SplitRegisters.
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
				// uint→𝔽 reduces modulo P, so it needs no canonicality check.
				return []Bytecode[W]{cast}
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

// check returns the bytecode invoking the canonicality helper for a 𝔽→uint
// extraction into the given target registers.  When the target is too narrow to
// ever reach P, every value it can hold is already canonical and no check is
// required.
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

	return []Bytecode[W]{bytecode.CallFun[W](uint16(p.ensure(widths, total)), regs, nil)}
}

// ensure lazily constructs (and memoises) the canonicality helper function for a
// target of the given limb widths.  The helper reconstructs the value from its
// limbs and fails unless that value is a canonical field element (i.e. strictly
// less than the modulus P).  The "value < P" test is emitted as a high-level
// relational SkipIf; downstream passes lower it into a subtract-with-borrow
// chain (see the package-level LowerFieldCasts comment).
func (p *fieldCastHelpers[W]) ensure(widths []uint, total uint) uint {
	var builder strings.Builder

	builder.WriteString("$field_range")

	for _, w := range widths {
		fmt.Fprintf(&builder, "_u%d", w)
	}

	name := builder.String()
	if id, ok := p.ids[name]; ok {
		return id
	}

	var (
		padding W
		regs    []descriptor.Register[W]
		inputs  = make([]bytecode.RegisterId, len(widths))
		code    []Bytecode[W]
	)
	// Input registers: the target limbs of the 𝔽→uint extraction.
	for i, width := range widths {
		inputs[i] = bytecode.RegisterId(len(regs))
		regs = append(regs, descriptor.NewRegister(register.INPUT_REGISTER,
			fmt.Sprintf("arg%d", i), util.Some(width), padding))
	}
	// Reconstruct the value being checked.  A single-limb target already is the
	// value; multiple limbs are concatenated (least-significant first).
	valueReg := inputs[0]
	if len(inputs) > 1 {
		valueReg = bytecode.RegisterId(len(regs))
		regs = append(regs, descriptor.NewRegister(register.COMPUTED_REGISTER,
			"value", util.Some(total), padding))
		code = append(code, bytecode.Concat[W]([]bytecode.RegisterId{valueReg}, inputs))
	}
	// Load the modulus P into a register (SkipIf compares two registers) and
	// fail unless the value is canonical (value < P).
	pReg := bytecode.RegisterId(len(regs))
	regs = append(regs, descriptor.NewRegister(register.COMPUTED_REGISTER,
		"P", util.Some(total), padding))

	var pConst W

	pConst = pConst.SetBigInt(p.modulus)
	// value < P → skip the fail (canonical); otherwise fall through and fail.
	code = append(code,
		bytecode.LoadConst(pReg, pConst),
		bytecode.NewSkipIf[W](bytecode.CONDITION_LT, 1, valueReg, pReg),
		bytecode.NewFail[W](nil, nil),
		bytecode.NewRet[W](),
	)

	id := p.base + uint(len(p.modules))
	p.ids[name] = id
	p.modules = append(p.modules, descriptor.NewFunction(name, regs, false,
		[]BytecodeVector[W]{bytecode.NewVector(code...)}))

	return id
}
