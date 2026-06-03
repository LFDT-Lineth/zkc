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
package compiler

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
)

// TranslateBranchCondition translates a given branch condition within the
// context of a given state reader.
func TranslateBranchCondition[T any, E Expr[T, E]](p dfa.BranchCondition, reader RegisterReader[E]) E {
	var condition E
	// Sanity check for obvious cases
	if p.IsTrue() {
		var zero = BigNumber[T, E](big.NewInt(0))
		return zero.Equals(zero)
	} else if p.IsFalse() {
		panic("unreachable")
	}
	// Expand the condition to ensure it is as simplified as we can make it.
	// This is necessary because simplification on MIR terms is very limited
	// and, without this, we cannot reasonably compile some examples.
	p = expandBranchCondition(p, reader)
	// Translate (assuming an expanded branch condition)
	for i, c := range p.Conjuncts() {
		ith := translateBranchConjunct[T, E](c, reader)
		//
		if i == 0 {
			condition = ith
		} else {
			condition = condition.Or(ith)
		}
	}
	//
	return condition
}

// Translate a given branch condition within the context of a given state
// reader.
func translateBranchConjunct[T any, E Expr[T, E]](p dfa.BranchConjunction, reader RegisterReader[E]) E {
	var condition E
	//
	for i, atom := range p.Atoms() {
		ith := translateBranchEquality[T, E](atom, reader)
		//
		if i == 0 {
			condition = ith
		} else {
			condition = condition.And(ith)
		}
	}
	//
	return condition
}

// Translate a given condition within the context of a given state translator.
func translateBranchEquality[T any, E Expr[T, E]](p dfa.BranchEquality, reader RegisterReader[E]) E {
	var (
		left  = ReadRegister[T, E](p.Left, reader)
		right E
	)
	//
	if p.Right.HasSecond() {
		bi := p.Right.Second()
		right = BigNumber[T, E](&bi)
	} else {
		right = ReadRegister[T, E](p.Right.First(), reader)
	}
	//
	if p.Sign {
		return left.Equals(right)
	}
	//
	return left.NotEquals(right)
}

// ReadRegister constructs a suitable accessor for referring to a given register.
// This applies forwarding as appropriate.
func ReadRegister[T any, E Expr[T, E]](reg dfa.BranchId, reader RegisterReader[E]) E {
	if reg.Width != 1 {
		panic("invalid singleton group it")
	}
	//
	return reader.ReadRegister(reg.Id, reg.Forwarding)
}

func expandBranchCondition[T any, E Expr[T, E]](p dfa.BranchCondition, reader RegisterReader[E]) dfa.BranchCondition {
	var condition dfa.BranchCondition = dfa.FALSE
	//
	for i, atom := range p.Conjuncts() {
		ith := expandBranchConjunct(atom, reader)
		//
		if i == 0 {
			condition = ith
		} else {
			condition = condition.Or(ith)
		}
	}
	//
	return condition
}

func expandBranchConjunct[T any, E Expr[T, E]](p dfa.BranchConjunction, reader RegisterReader[E]) dfa.BranchCondition {
	var condition dfa.BranchCondition = dfa.TRUE
	//
	for i, atom := range p.Atoms() {
		ith := expandBranchEquality(atom, reader)
		//
		if i == 0 {
			condition = ith
		} else {
			condition = condition.And(ith)
		}
	}
	//
	return condition
}

// Translate a given condition within the context of a given state translator.
func expandBranchEquality[T any, E Expr[T, E]](p dfa.BranchEquality, reader RegisterReader[E]) dfa.BranchCondition {
	if p.Right.HasSecond() {
		bi := p.Right.Second()
		rhs := splitConstant(bi, reader.RegisterWidths(p.Left.Registers()...))
		//
		if p.Sign {
			return expandBranchEqualityRegConst(p.Left, rhs)
		}
		//
		return expandBranchNonEqualityRegConst(p.Left, rhs)
	} else if p.Sign {
		return expandBranchEqualityRegReg(p.Left, p.Right.First())
	}

	return expandBranchNonEqualityRegReg(p.Left, p.Right.First())
}

func expandBranchEqualityRegConst(lhs dfa.BranchId, rhs []big.Int) dfa.BranchCondition {
	var (
		condition dfa.BranchCondition = dfa.TRUE
		m                             = uint(len(rhs))
		n                             = min(lhs.Width, m)
	)
	//
	for i := range n {
		ith := lhs.Get(i)
		neq := logical.EqualsConst(ith, rhs[i])
		condition = condition.And(logical.NewProposition(neq))
	}
	// expand lhs as needed
	for i := n; i < lhs.Width; i++ {
		ith := lhs.Get(i)
		neq := logical.EqualsConst(ith, zero)
		condition = condition.And(logical.NewProposition(neq))
	}
	// sanity check
	if m > n {
		panic("constant is too large")
	}
	//
	return condition
}

func expandBranchNonEqualityRegConst(lhs dfa.BranchId, rhs []big.Int) dfa.BranchCondition {
	var (
		condition dfa.BranchCondition = dfa.FALSE
		m                             = uint(len(rhs))
		n                             = min(lhs.Width, m)
	)
	//
	for i := range n {
		ith := lhs.Get(i)
		neq := logical.NotEqualsConst(ith, rhs[i])
		condition = condition.Or(logical.NewProposition(neq))
	}
	// expand rhs as needed
	for i := n; i < lhs.Width; i++ {
		ith := lhs.Get(i)
		neq := logical.NotEqualsConst(ith, zero)
		condition = condition.Or(logical.NewProposition(neq))
	}
	// sanity check
	if m > n {
		panic("constant is too large")
	}
	//
	return condition
}

func expandBranchEqualityRegReg(lhs dfa.BranchId, rhs dfa.BranchId) dfa.BranchCondition {
	var (
		condition dfa.BranchCondition = dfa.TRUE
		n                             = min(lhs.Width, rhs.Width)
	)
	//
	for i := range n {
		lth, rth := lhs.Get(i), rhs.Get(i)
		neq := logical.Equals(lth, rth)
		condition = condition.And(logical.NewProposition(neq))
	}
	// expand lhs as needed
	for i := n; i < lhs.Width; i++ {
		ith := lhs.Get(i)
		neq := logical.EqualsConst(ith, zero)
		condition = condition.And(logical.NewProposition(neq))
	}
	// expand rhs as needed
	for i := n; i < rhs.Width; i++ {
		ith := rhs.Get(i)
		neq := logical.EqualsConst(ith, zero)
		condition = condition.And(logical.NewProposition(neq))
	}
	//
	return condition
}

func expandBranchNonEqualityRegReg(lhs dfa.BranchId, rhs dfa.BranchId) dfa.BranchCondition {
	var (
		condition dfa.BranchCondition = dfa.FALSE
		n                             = min(lhs.Width, rhs.Width)
	)
	//
	for i := range n {
		lth, rth := lhs.Get(i), rhs.Get(i)
		neq := logical.NotEquals(lth, rth)
		condition = condition.Or(logical.NewProposition(neq))
	}
	// expand lhs as needed
	for i := n; i < lhs.Width; i++ {
		ith := lhs.Get(i)
		neq := logical.NotEqualsConst(ith, zero)
		condition = condition.Or(logical.NewProposition(neq))
	}
	// expand rhs as needed
	for i := n; i < rhs.Width; i++ {
		ith := rhs.Get(i)
		neq := logical.NotEqualsConst(ith, zero)
		condition = condition.Or(logical.NewProposition(neq))
	}
	//
	return condition
}

// Alias for big integer representation of 0.
var zero big.Int = *big.NewInt(0)
var one big.Int = *big.NewInt(1)

func splitConstant(constant big.Int, widths []uint) []big.Int {
	var (
		acc   big.Int
		limbs []big.Int = make([]big.Int, len(widths))
	)
	// Clone constant
	acc.Set(&constant)
	//
	for i, limbWidth := range widths {
		var (
			limb  big.Int
			bound = big.NewInt(1)
		)
		// bound = 1 << limbWidth
		bound.Lsh(bound, limbWidth)
		// limb = acc & (bound - 1)
		limb.And(&acc, bound.Sub(bound, &one))
		// done
		limbs[i] = limb
		//
		acc.Rsh(&acc, limbWidth)
	}
	// sanity check
	if acc.Cmp(&zero) != 0 {
		panic("invalid constant")
	}
	//
	return limbs
}

// this is primarily for debugging purposes
// nolint
func groupId2String[T any](reader RegisterReader[T]) func(dfa.BranchId) string {
	return func(gid dfa.BranchId) string {
		var (
			first = reader.Register(gid.Id).Name()
			id    string
		)
		//
		if gid.Width == 1 {
			id = first
		} else {
			last := reader.Register(register.NewId(gid.Id.Unwrap() + gid.Width - 1)).Name()
			id = fmt.Sprintf("{%s..%s}", first, last)
		}
		//
		if gid.Forwarding {
			return id
		}
		//
		return fmt.Sprintf("'%s", id)
	}
}
