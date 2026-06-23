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
package micro

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// SkipIf microcode performs a conditional skip over a given number of codes. The
// condition is either that two registers are equal, or that they are not equal.
// This has two variants: register-register; and, register-constant.  The latter
// is indiciated when the right register is marked as UNUSED.
type SkipIf struct {
	// Left and right comparisons
	Left register.Vector
	//
	Right VecExpr
	// Skip
	Skip uint
}

// Clone this micro code.
func (p *SkipIf) Clone() Code {
	//
	return &SkipIf{
		Left:  p.Left.Clone(),
		Right: p.Right,
		Skip:  p.Skip,
	}
}

// MicroExecute a given micro-code, using a given local state.  This may update
// the register values, and returns either the number of micro-codes to "skip
// over" when executing the enclosing instruction or, if skip==0, a destination
// program counter (which can signal return of enclosing function).
func (p *SkipIf) MicroExecute(state io.State) (uint, uint) {
	var (
		offset uint
		lhs    big.Int
		rhs    = p.Right.Eval(state)
	)
	//
	for _, rid := range p.Left.Registers() {
		var (
			reg = state.Registers()[rid.Unwrap()]
			ith big.Int
		)
		// Load & clone ith value
		ith.Set(state.Load(rid))
		// Shift into position
		ith.Lsh(&ith, offset)
		// Include in total
		lhs.Add(&lhs, &ith)
		//
		offset += reg.Width()
	}
	//
	if lhs.Cmp(rhs) != 0 {
		return 1 + p.Skip, 0
	} else {
		return 1, 0
	}
}

// RegistersRead returns the set of registers read by this instruction.
func (p *SkipIf) RegistersRead() []io.RegisterId {
	var regs []io.RegisterId
	// Add all registers on the left-hand side
	regs = append(regs, p.Left.Registers()...)
	// Add all registers on the right-hand side (if applicable)
	if p.Right.HasFirst() {
		regs = append(regs, p.Right.First().Registers()...)
	}
	//
	return regs
}

// RegistersWritten returns the set of registers written by this instruction.
func (p *SkipIf) RegistersWritten() []io.RegisterId {
	return nil
}

// Split this micro code using registers of arbirary width into one or more
// micro codes using registers of a fixed maximum width.
func (p *SkipIf) Split(mapping register.LimbsMap, _ agnostic.RegisterAllocator) []Code {
	// Sanity check lhs
	if len(p.Left.Registers()) != 1 {
		panic("expecting only a single register on the left-hand side")
	}
	//
	var left = p.Left.Registers()[0]
	//
	if p.Right.HasSecond() {
		return splitRegConst(left, p.Right.Second(), p.Skip, mapping)
	}
	// Sanity check rhs
	if len(p.Right.First().Registers()) != 1 {
		panic("expecting a single register on the right-hand side")
	}
	//
	var right = p.Right.First().Registers()[0]
	//
	return splitRegReg(left, right, p.Skip, mapping)
}

func (p *SkipIf) String(fn register.Map) string {
	var (
		l = p.Left.String(fn)
		r = p.Right.String(fn)
	)
	//
	return fmt.Sprintf("skip %s != %s %d", l, r, p.Skip)
}

// Validate checks whether or not this instruction is correctly balanced.
func (p *SkipIf) Validate(fieldWidth uint, fn register.Map) error {
	var (
		lw = p.Left.BitWidth(fn)
		rw = p.Right.Bitwidth(fn)
	)
	//
	if p.Right.HasSecond() {
		//
		if lw < rw {
			return fmt.Errorf("bit overflow (u%d into u%d)", lw, rw)
		}
	}
	//
	return nil
}

func splitRegConst(left register.Id, right big.Int, skip uint, mapping register.LimbsMap) []Code {
	var (
		lhsLimbs = mapping.LimbIds(left)
		lhs      = register.NewVector(slices.Clone(lhsLimbs)...)
	)
	//
	return []Code{&SkipIf{lhs, ConstVecExpr(right), skip}}
}

func splitRegReg(left, right register.Id, skip uint, mapping register.LimbsMap) []Code {
	var (
		lhsLimbs = mapping.LimbIds(left)
		rhsLimbs = mapping.LimbIds(right)
		lhs      = register.NewVector(slices.Clone(lhsLimbs)...)
		rhs      = register.NewVector(slices.Clone(rhsLimbs)...)
	)
	//
	return []Code{&SkipIf{lhs, NewVecExpr(rhs), skip}}
}
