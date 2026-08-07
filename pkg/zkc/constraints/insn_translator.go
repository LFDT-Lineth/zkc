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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// Polynomial defines the type of polynomials over which packets (and register
// splitting in general) operate.
type Polynomial = *poly.ArrayPoly[register.Id]

// Monomial is a convenient alias
type Monomial = poly.Monomial[register.Id]

// InstructionTranslator encapsulates key information for translating an
// individual instruction (e.g. an assignment) into constraints.
type InstructionTranslator[W vm.Word[W], F field.Element[F]] struct {
	reader RegisterReader[F]
	writes dfa.Writes
}

// ReadRegister reads a given register whilst applying forwarding as needed
// depending on the given writes set.
func (p *InstructionTranslator[W, F]) ReadRegister(rid vm.RegisterId) Expr[F] {
	var _rid = register.NewId(uint(rid))
	return p.reader.ReadRegister(_rid, p.writes.MaybeAssigned(_rid))
}

// WriteAndShiftRegisters constructs suitable accessors for the those registers
// written by a given microinstruction, and also shifts them (i.e. so they can
// be combined in a sum).  This activates forwarding for those registers for all
// states after this, and returns suitable expressions for the assignment.
func (p *InstructionTranslator[W, F]) WriteAndShiftRegisters(targets ...vm.RegisterId) []Expr[F] {
	lhs := make([]Expr[F], len(targets))
	offset := big.NewInt(1)
	// build up the lhs
	for i, _dst := range targets {
		var (
			dst       = register.NewId(uint(_dst))
			ith       = p.reader.Register(dst)
			ith_width = bitwidthOf(ith)
		)
		//
		lhs[i] = mirc.Variable[register.Id, Expr[F]](dst, ith_width, 0)
		//
		if i != 0 {
			lhs[i] = mirc.BigNumber[register.Id, Expr[F]](offset).Multiply(lhs[i])
		}
		// left shift offset by given register width.
		if !ith.IsNative() {
			offset.Lsh(offset, ith_width)
		} else if i != 0 || len(targets) != 1 {
			// NOTE: this should be unreachable, but is included as a safety
			// check.  Basically, when assigning a native register there should
			// only ever be exactly one target.
			panic(fmt.Sprintf("invalid native assignment (target %d/%d)", i, len(targets)))
		}
	}
	//
	return lhs
}

// ============================================================================
// Assignments
// ============================================================================

func (p *InstructionTranslator[W, F]) translateAdd(targets, sources []vm.RegisterId,
	constant W) Expr[F] {
	var (
		lhs           = p.WriteAndShiftRegisters(targets...)
		rhs []Expr[F] = make([]Expr[F], len(sources))
	)
	//
	for i := range len(sources) {
		rhs[i] = p.ReadRegister(sources[i])
	}
	// Optimise case where coeff == 0
	if constant.Cmp64(0) != 0 {
		rhs = append(rhs, mirc.BigNumber[register.Id, Expr[F]](constant.BigInt()))
	}
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

func (p *InstructionTranslator[W, F]) translateMul(targets, sources []vm.RegisterId,
	constant W) Expr[F] {
	var (
		lhs           = p.WriteAndShiftRegisters(targets...)
		rhs []Expr[F] = make([]Expr[F], len(sources))
	)
	//
	for i := range len(sources) {
		rhs[i] = p.ReadRegister(sources[i])
	}
	// Optimise case where coeff == 1
	if constant.Cmp64(1) != 0 {
		rhs = append(rhs, mirc.BigNumber[register.Id, Expr[F]](constant.BigInt()))
	}
	//
	return mirc.Sum(lhs).Equals(mirc.Product(rhs...))
}

func (p *InstructionTranslator[W, F]) translateSub(targets, sources []vm.RegisterId,
	constant W) Expr[F] {
	var (
		minusOne           = mirc.BigNumber[register.Id, Expr[F]](big.NewInt(-1))
		lhs                = p.WriteAndShiftRegisters(targets...)
		rhs      []Expr[F] = make([]Expr[F], len(sources))
	)
	//
	for i := range len(sources) {
		if i == 0 {
			rhs[i] = p.ReadRegister(sources[i])
		} else {
			rhs[i] = mirc.Product(minusOne, p.ReadRegister(sources[i]))
		}
	}
	// Optimise case where coeff == 0
	if constant.Cmp64(0) != 0 {
		var c = constant.BigInt()
		//
		rhs = append(rhs, mirc.BigNumber[register.Id, Expr[F]](c.Neg(c)))
	}
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

func (p *InstructionTranslator[W, F]) translateSignedSub(targets, sources []vm.RegisterId,
	constant W) Expr[F] {
	var (
		minusOne           = mirc.BigNumber[register.Id, Expr[F]](big.NewInt(-1))
		lhs                = p.WriteAndShiftRegisters(targets...)
		rhs      []Expr[F] = make([]Expr[F], len(sources))
	)
	//
	for i := range len(sources) {
		if i == 0 {
			rhs[i] = p.ReadRegister(sources[i])
		} else {
			rhs[i] = mirc.Product(minusOne, p.ReadRegister(sources[i]))
		}
	}
	// Optimise case where coeff == 0
	if constant.Cmp64(0) != 0 {
		var c = constant.BigInt()
		//
		rhs = append(rhs, mirc.BigNumber[register.Id, Expr[F]](c.Neg(c)))
	}
	// Rebalance equation
	lhs, rhs = rebalanceSubtraction(lhs, rhs)
	// Done
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

// Consider an assignment b, X := Y - 1.  This should be translated into the
// constraint: X + 1 == Y + 256.b (assuming b is u1, and X/Y are u8).
func rebalanceSubtraction[F field.Element[F]](lhs []Expr[F], rhs []Expr[F]) ([]Expr[F], []Expr[F]) {
	var (
		n = len(lhs) - 1
		// Extract sign bit
		sign = lhs[n]
	)
	// Remove sign bit
	lhs = lhs[:n]
	// Move sign bit onto rhs
	rhs = append(rhs, sign)
	// Done
	return lhs, rhs
}

func (p *InstructionTranslator[W, F]) translateFieldAdd(target vm.RegisterId, sources []vm.RegisterId,
	constant W) Expr[F] {
	// NOTE: safe to reuse normal add translation here
	return p.translateAdd([]vm.RegisterId{target}, sources, constant)
}

func (p *InstructionTranslator[W, F]) translateFieldMul(target vm.RegisterId, sources []vm.RegisterId,
	constant W) Expr[F] {
	// NOTE: safe to reuse normal mul translation here
	return p.translateMul([]vm.RegisterId{target}, sources, constant)
}

func (p *InstructionTranslator[W, F]) translateFieldSub(target vm.RegisterId, sources []vm.RegisterId,
	constant W) Expr[F] {
	// NOTE: safe to reuse normal (unsigned) sub translation here
	return p.translateSub([]vm.RegisterId{target}, sources, constant)
}

// translateConcat translates a concatenation assignment (BIT_CONCAT) which
// joins the source registers into the target register vector, weighting each
// source by 2^(bitwidth of the less-significant sources).  widths gives the bit
// width of each source register, least-significant first.
func (p *InstructionTranslator[W, F]) translateConcat(targets, sources []vm.RegisterId, widths []uint) Expr[F] {
	var (
		lhs           = p.WriteAndShiftRegisters(targets...)
		rhs []Expr[F] = make([]Expr[F], len(sources))
		acc           = big.NewInt(1)
	)
	//
	for i := range len(sources) {
		var coeff = mirc.BigNumber[register.Id, Expr[F]](acc)
		// Construct shifted term
		rhs[i] = mirc.Product(coeff, p.ReadRegister(sources[i]))
		// Shift the running weight left by this source's bit width (unless this
		// is the last source, in which case don't bother).  Note, for native
		// registers the last source will have a very large width, so we must
		// ignore it.
		if i+1 != len(sources) {
			acc = acc.Lsh(acc, widths[i])
		}
	}
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}
