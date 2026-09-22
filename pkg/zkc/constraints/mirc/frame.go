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
package mirc

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Framing is used to manage additional registers required to ensure soundness.
// In particular, framinging applies to multi-line functions as these require a
// program counter, and various control lines to manage padding and non-terminal
// states.
type Framing[F field.Element[F]] interface {
	// Guard provides a suitable guard for the instruction at a given PC offset.
	// This is optional as some forms of framing don't require it.
	Guard(pc uint) Expr[F]
	// Goto indicates the current instruction is jumping to the given PC value.
	Goto(pc uint) Expr[F]
	// Return provides a suitable transition to the next frame.
	Return() Expr[F]
}

// NewAtomicFraming constructs a suitable framing for a one-line instruction.
func NewAtomicFraming[F field.Element[F]]() Framing[F] {
	return &OneLineFraming[F]{}
}

// NewMultiLineFraming constructs framing for a multi-line function.
// It assumes the caller has allocated $ret, PC and IS_PC_<k> selector registers and passes their ids here.
func NewMultiLineFraming[F field.Element[F]](pc register.Id, pcWidth uint, ret register.Id, retWidth uint,
	selectors []register.Id) Framing[F] {
	return &MultiLineFraming[F]{LegacyMultiLineFraming[F]{pc, pcWidth, ret, retWidth}, selectors}
}

// ============================================================================
// Atomic (i.e. One-Line) Framing
// ============================================================================

// OneLineFraming is suitable for one-line functions, as these require no
// control lines.
type OneLineFraming[F field.Element[F]] struct {
}

// Goto implementation for Framing interface.
func (p *OneLineFraming[F]) Goto(pc uint) Expr[F] {
	panic("unreachable")
}

// Guard implementation for Framing interface.
func (p *OneLineFraming[F]) Guard(pc uint) Expr[F] {
	return True[F]()
}

// Return implementation for Framing interface.
func (p *OneLineFraming[F]) Return() Expr[F] {
	return True[F]()
}

// ============================================================================
// Multi-Line Framing
// ============================================================================

// LegacyMultiLineFraming provides suitable control lines for multi-line
// functions, guarding each instruction with an equality on the program counter.
type LegacyMultiLineFraming[F field.Element[F]] struct {
	// Program Counter indicates which instruction is being executed.
	pc register.Id
	// Width of program counter (for reference)
	pcWidth uint
	// Return indicates when an instruction returns from the current function.
	// That is, the current frame is terminated.
	ret register.Id
	// Width of return line (for reference)
	retWidth uint
}

// Goto implementation for Framing interface.
func (p *LegacyMultiLineFraming[F]) Goto(pc uint) Expr[F] {
	// PC[i+1] = target
	var (
		zero   = Number[F](0)
		pc_ip1 = Variable[F](p.pc, p.pcWidth, 1)
		ret    = Variable[F](p.ret, p.retWidth, 0)
	)
	// Next pc is target of this jump. NOTE: pc+1 as pc==0 is for padding
	eq := pc_ip1.Equals(Number[F](pc + 1))
	// Return flag cannot be set
	return eq.And(ret.Equals(zero))
}

// Guard implementation for Framing interface.
func (p *LegacyMultiLineFraming[F]) Guard(pc uint) Expr[F] {
	// NOTE: pc+1 as pc==0 is for padding
	return Variable[F](p.pc, p.pcWidth, 0).Equals(Number[F](pc + 1))
}

// Return implementation for Framing interface.
func (p *LegacyMultiLineFraming[F]) Return() Expr[F] {
	var one = Number[F](1)
	// return line must be high; next PC must be zero.
	return Variable[F](p.ret, p.retWidth, 0).Equals(one)
}

// MultiLineFraming provides suitable control lines for multi-line functions,
// guarding each instruction with a dedicated boolean selector register.  It
// reuses the legacy framing for Goto/Return (which act on the program counter
// and return line) and overrides only the per-instruction Guard.
type MultiLineFraming[F field.Element[F]] struct {
	LegacyMultiLineFraming[F]
	// selectors provides one boolean selector register per instruction, indexed
	// by code line.  The selector for code line c is high exactly when PC==c+1.
	selectors []register.Id
}

// Guard implementation for Framing interface.  Guards directly on the selector
// for this instruction.  Since the selector is a width-1 register, the resulting
// condition lowers to the bare column with no inverse.
func (p *MultiLineFraming[F]) Guard(pc uint) Expr[F] {
	return Variable[F](p.selectors[pc], 1, 0).NotEquals(Number[F](0))
}
