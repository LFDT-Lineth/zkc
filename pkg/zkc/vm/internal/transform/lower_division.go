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
	"slices"
	"strconv"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerDivisions rewrites INT_DIV and INT_REM bytecodes into a non-deterministic
// hint followed by arithmetic validation (see expandDivRem for the emitted
// sequence and the rationale for its structure):
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy  = q * y
//	qyr = qy + r
//	0   = x - qyr            // written into a 0-width register: asserts x == q*y + r
//	rw1 = r + w + 1
//	0   = y - rw1            // written into a 0-width register: asserts y == r + w + 1
//
// Since the hint produces both quotient and remainder, a complementary DIV /
// REM pair over identical operands shares a single such block rather than
// expanding twice (see planDivRemMerges for the pairing conditions).
//
// NOTE: this transform must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerDivisionFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func lowerDivisionFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
		plan    = planDivRemMerges(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(j uint, b Bytecode[W]) []Bytecode[W] {
			return lowerDivisionCode(bcPosition{uint(i), j}, b, plan, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), fn.Effects(), nvecs)
}

func lowerDivisionCode[W word.Word[W]](
	pos bcPosition,
	b Bytecode[W],
	plan divRemPlan[W],
	registers split.Allocator[W],
) []Bytecode[W] {
	dr, ok := b.(*bytecode.DivRem[W])
	if !ok {
		return []Bytecode[W]{b}
	}
	// A DIV / REM pair over the same operands shares a single hint+validation
	// block emitted at the first member's position (see planDivRemMerges); the
	// second member is dropped.
	if plan.deletes[pos] {
		return nil
	}
	//
	if m, ok := plan.merges[pos]; ok {
		var (
			nX = registers.Register(m.x).Bitwidth().Unwrap()
			nY = divisorWidth(m.y, registers)
			w  = registers.Allocate("", util.Some(nY))
		)
		//
		return expandDivRem(m.q, m.r, w, m.x, m.y, nX, nY, registers)
	}
	//
	switch dr.Opcode {
	case encoding.DIV:
		return expandDivision(dr.Target, dr.Dividend, dr.Divisor, registers)
	case encoding.REM:
		return expandRemainder(dr.Target, dr.Dividend, dr.Divisor, registers)
	default:
		return []Bytecode[W]{b}
	}
}

// expandDivision replaces INT_DIV(q, x, y) with the hint+validation sequence.
func expandDivision[W word.Word[W]](q, x bytecode.RegisterId, y bytecode.Operand[W],
	registers split.Allocator[W]) []Bytecode[W] {
	//
	var (
		nX = registers.Register(x).Bitwidth().Unwrap()
		nY = divisorWidth(y, registers)
		r  = registers.Allocate("", util.Some(nY))
		w  = registers.Allocate("", util.Some(nY))
	)
	//
	return expandDivRem(q, r, w, x, y, nX, nY, registers)
}

// expandRemainder replaces INT_REM(r, x, y) with the hint+validation sequence.
func expandRemainder[W word.Word[W]](r, x bytecode.RegisterId, y bytecode.Operand[W],
	registers split.Allocator[W]) []Bytecode[W] {
	//
	var (
		nX = registers.Register(x).Bitwidth().Unwrap()
		nY = divisorWidth(y, registers)
		q  = registers.Allocate("", util.Some(nX))
		w  = registers.Allocate("", util.Some(nY))
	)
	//
	return expandDivRem(q, r, w, x, y, nX, nY, registers)
}

// divisorWidth returns the bitwidth of the given divisor operand: the declared
// register width for a register divisor, or the (minimal) width of the value
// itself for a constant divisor.
func divisorWidth[W word.Word[W]](y bytecode.Operand[W], registers split.Allocator[W]) uint {
	if y.IsConstant() {
		return uint(y.AsConstant().BigInt().BitLen())
	}
	//
	return registers.Register(y.AsRegister()).Bitwidth().Unwrap()
}

// expandDivRem builds the shared hint+validation sequence for both INT_DIV and
// INT_REM, given the (already allocated) quotient q, remainder r and range
// witness w registers.  It emits:
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy  = q * y
//	qyr = qy + r
//	0   = x - qyr                   // asserts x == q*y + r
//	rw1 = r + w + 1
//	0   = y - rw1                   // asserts y == r + w + 1 (i.e. r + w < y, so r < y)
//
// The validity checks are deliberately structured as two-operand differences
// asserted to be zero (x == qyr and y == rw1) rather than as a single
// three-operand subtraction (e.g. x - qy - r == 0).  A three-operand zero
// assertion cannot be split limb-wise without a borrow chain, and for wide
// operands that borrow grows past the field register width.  A two-operand zero
// assertion splits into independent per-limb equalities (see split.Subtraction),
// which needs no borrows.
func expandDivRem[W word.Word[W]](q, r, w, x bytecode.RegisterId, y bytecode.Operand[W], nX, nY uint,
	registers split.Allocator[W]) []Bytecode[W] {
	var (
		zero = word.Const64[W](0)
		one  = word.Const64[W](1)
		qy   = registers.Allocate("q", util.Some(nX))
		// qyr = qy + r must hold q*y + r which, by the DIV_HINT semantics
		// (q = x/y, r = x%y), equals the dividend x exactly and so fits nX
		// bits; the extra bit guards the addition's transient carry.
		qyr = registers.Allocate("", util.Some(nX+1))
		// rw1 = r + w + 1 holds the divisor exactly (nY bits) + 1 to ensure no overflow
		nRW = max(registers.Register(r).Bitwidth().Unwrap(),
			registers.Register(w).Bitwidth().Unwrap()) + 1
		rw1 = registers.Allocate("", util.Some(nRW))
		// NOTE: must separate z0 & z1 to avoid write conflict (for now).
		z0 = registers.Allocate("", util.Some[uint](0))
		z1 = registers.Allocate("", util.Some[uint](0))

		mulQY, subZ1 Bytecode[W]
	)
	//
	if y.IsConstant() {
		mulQY = bytecode.MulConst(qy, []bytecode.RegisterId{q}, y.AsConstant())
		subZ1 = bytecode.SubConst(z1, []bytecode.RegisterId{rw1}, y.AsConstant())
	} else {
		mulQY = bytecode.MulConst(qy, []bytecode.RegisterId{q, y.AsRegister()}, one)
		subZ1 = bytecode.SubConst(z1, []bytecode.RegisterId{y.AsRegister(), rw1}, zero)
	}
	//
	return []Bytecode[W]{
		bytecode.NewIntrinsic(bytecode.DIV_HINT,
			[]bytecode.RegisterVector{
				bytecode.NewRegisterVector(q), bytecode.NewRegisterVector(r), bytecode.NewRegisterVector(w),
			},
			[]bytecode.Operand[W]{bytecode.NewRegisterOperand[W](x), y}),
		mulQY,
		bytecode.AddConst(qyr, []bytecode.RegisterId{qy, r}, zero),
		bytecode.SubConst(z0, []bytecode.RegisterId{x, qyr}, zero),
		bytecode.AddConst(rw1, []bytecode.RegisterId{r, w}, one),
		subZ1,
	}
}

// ============================================================================
// DIV / REM pair merging
// ============================================================================

// bcPosition identifies a bytecode by its vector index and its index within
// that vector.
type bcPosition struct{ vec, idx uint }

// divRemMerge describes a DIV / REM pair sharing one hint+validation block: q
// receives the quotient (the DIV's target), r the remainder (the REM's
// target), over dividend x and divisor y.
type divRemMerge[W word.Word[W]] struct {
	q, r, x bytecode.RegisterId
	y       bytecode.Operand[W]
}

// divRemPlan records, per bytecode position, how the lowering must deviate
// from the one-block-per-DivRem default: a position in merges expands into the
// combined block of a DIV / REM pair, whilst a position in deletes (the second
// member of such a pair) expands into nothing.
type divRemPlan[W word.Word[W]] struct {
	merges  map[bcPosition]divRemMerge[W]
	deletes map[bcPosition]bool
}

// pendingDivRem records an as-yet-unpaired DivRem seen during the planning
// scan.
type pendingDivRem[W word.Word[W]] struct {
	pos      bcPosition
	opcode   uint32
	target   bytecode.RegisterId
	dividend bytecode.RegisterId
	divisor  bytecode.Operand[W]
}

// divRemKey identifies the operands of a DivRem for pairing purposes: two
// DivRems with equal keys compute over the same dividend register and the same
// divisor (same register, or same constant value).
type divRemKey struct {
	dividend bytecode.RegisterId
	divisor  string
}

// planDivRemMerges scans a function for complementary DIV / REM pairs over
// identical operands whose expansions can share a single DIV_HINT (the hint
// produces both quotient and remainder, so expanding each member separately
// duplicates the entire hint+validation block).  Merging emits the combined
// block at the first member's position — hoisting the second member's write —
// and deletes the second member, which is sound only when, between the two:
//
//   - control flow can neither leave nor enter (no branching bytecode, and no
//     vector which is the target of a jump);
//   - neither operand is redefined; and
//   - the second member's target is neither read nor written (its write moves
//     to the first member's position).
//
// The scan is a single linear pass maintaining the unpaired DivRems seen so
// far, conservatively cleared at every control-flow bytecode and jump target.
// Same-opcode duplicates are not merged (that requires a copy bytecode); the
// first occurrence stays the pairing candidate.
func planDivRemMerges[W word.Word[W]](fn *descriptor.Function[W]) divRemPlan[W] {
	var (
		vectors = fn.Vectors()
		plan    = divRemPlan[W]{
			merges:  make(map[bcPosition]divRemMerge[W]),
			deletes: make(map[bcPosition]bool),
		}
		pending     = make(map[divRemKey]pendingDivRem[W])
		jumpTargets = collectJumpTargets(vectors)
	)
	//
	for i, vec := range vectors {
		// A jump target begins a new path: bytecodes before it do not dominate
		// those after, so no pending candidate survives.
		if jumpTargets[uint(i)] {
			clear(pending)
		}
		//
		for j, insn := range vec.Bytecodes {
			var pos = bcPosition{uint(i), uint(j)}
			//
			switch insn.(type) {
			case *bytecode.Jmp[W], *bytecode.Skip[W], *bytecode.SkipIf[W],
				*bytecode.Switch[W], *bytecode.Dispatch[W], *bytecode.Ret[W], *bytecode.Fail[W]:
				clear(pending)
				continue
			}
			// Drop candidates whose operands are redefined by this bytecode.
			invalidatePending(pending, insn.Definitions())
			//
			dr, ok := insn.(*bytecode.DivRem[W])
			if !ok {
				continue
			}
			//
			var key = divRemKey{dr.Dividend, divisorKey(dr.Divisor)}
			//
			if e, ok := pending[key]; ok && e.opcode != dr.Opcode {
				// Complementary pair found: #1 (e) is consumed either way.
				delete(pending, key)
				//
				if mergeableTarget(dr, e) && !usedOrDefinedBetween(vectors, dr.Target, e.pos, pos) {
					q, r := e.target, dr.Target
					//
					if dr.Opcode == encoding.DIV {
						q, r = dr.Target, e.target
					}
					//
					plan.merges[e.pos] = divRemMerge[W]{q: q, r: r, x: dr.Dividend, y: e.divisor}
					plan.deletes[pos] = true
					//
					continue
				}
			} else if ok {
				// Same-opcode duplicate: the first occurrence stays.
				continue
			}
			// This DivRem becomes the pairing candidate for its key, unless its
			// target overlaps its own operands (e.g. x = x / k), in which case
			// no later operation can see the same dividend value.
			if insertablePending(dr) {
				pending[key] = pendingDivRem[W]{pos, dr.Opcode, dr.Target, dr.Dividend, dr.Divisor}
			}
		}
	}
	//
	return plan
}

// collectJumpTargets returns the set of vector indices targeted by a Jmp.
// Skip / SkipIf / Switch / Dispatch offsets are intra-vector and Call returns
// to the following bytecode, so Jmp targets are the only inter-vector
// control-flow entry points.
func collectJumpTargets[W word.Word[W]](vectors []BytecodeVector[W]) map[uint]bool {
	var targets = make(map[uint]bool)
	//
	for _, vec := range vectors {
		for _, insn := range vec.Bytecodes {
			if jmp, ok := insn.(*bytecode.Jmp[W]); ok {
				targets[uint(jmp.Target)] = true
			}
		}
	}
	//
	return targets
}

// invalidatePending drops every pending candidate one of whose operand
// registers is written by the current bytecode (constant divisors cannot be
// redefined).
func invalidatePending[W word.Word[W]](pending map[divRemKey]pendingDivRem[W], defs []bytecode.RegisterId) {
	if len(defs) == 0 || len(pending) == 0 {
		return
	}
	//
	for k, e := range pending {
		if slices.Contains(defs, e.dividend) ||
			(!e.divisor.IsConstant() && slices.Contains(defs, e.divisor.AsRegister())) {
			delete(pending, k)
		}
	}
}

// divisorKey renders a divisor operand as a canonical pairing key: the
// register identifier for a register divisor, or the constant value for a
// constant divisor (so equal constants pair regardless of how they were
// produced).
func divisorKey[W word.Word[W]](divisor bytecode.Operand[W]) string {
	if divisor.IsConstant() {
		return "c" + divisor.AsConstant().Text(16)
	}
	//
	return "r" + strconv.Itoa(int(divisor.AsRegister()))
}

// mergeableTarget checks that the second member's target does not overlap the
// pair's operands or the first member's target: the merged block writes both
// targets at the first member's position, so neither may collide with a
// register the block reads (or with each other).
func mergeableTarget[W word.Word[W]](dr *bytecode.DivRem[W], e pendingDivRem[W]) bool {
	if dr.Target == e.target || dr.Target == dr.Dividend {
		return false
	}
	//
	return dr.Divisor.IsConstant() || dr.Target != dr.Divisor.AsRegister()
}

// insertablePending checks that a DivRem's target does not overlap its own
// operands, which would make its dividend (or divisor) unavailable to any
// later pairing candidate.
func insertablePending[W word.Word[W]](dr *bytecode.DivRem[W]) bool {
	if dr.Target == dr.Dividend {
		return false
	}
	//
	return dr.Divisor.IsConstant() || dr.Target != dr.Divisor.AsRegister()
}

// usedOrDefinedBetween reports whether the given register is read or written
// by any bytecode strictly between from and to (in program order).  The
// planning scan clears its candidates at every control-flow bytecode, so the
// region is guaranteed to be straight-line code.
func usedOrDefinedBetween[W word.Word[W]](vectors []BytecodeVector[W], reg bytecode.RegisterId,
	from, to bcPosition) bool {
	//
	for v := from.vec; v <= to.vec; v++ {
		var (
			codes = vectors[v].Bytecodes
			lo    = 0
			hi    = len(codes)
		)
		//
		if v == from.vec {
			lo = int(from.idx) + 1
		}
		//
		if v == to.vec {
			hi = int(to.idx)
		}
		//
		for i := lo; i < hi; i++ {
			if slices.Contains(codes[i].Uses(), reg) || slices.Contains(codes[i].Definitions(), reg) {
				return true
			}
		}
	}
	//
	return false
}
