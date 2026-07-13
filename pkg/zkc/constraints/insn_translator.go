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
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// Polynomial defines the type of polynomials over which packets (and register
// splitting in general) operate.
type Polynomial = *poly.ArrayPoly[register.Id]

// Monomial is a convenient alias
type Monomial = poly.Monomial[register.Id]

// InstructionTranslator encapsulates key information for translating an
// individual instruction (e.g. an assignment) into constraints.
type InstructionTranslator[F field.Element[F]] struct {
	reader RegisterReader[F]
	writes dfa.Writes
}

// ReadRegister reads a given register whilst applying forwarding as needed
// depending on the given writes set.
func (p *InstructionTranslator[F]) ReadRegister(rid register.Id) Expr[F] {
	return p.reader.ReadRegister(rid, p.writes.MaybeAssigned(rid))
}

// WriteAndShiftRegisters constructs suitable accessors for the those registers
// written by a given microinstruction, and also shifts them (i.e. so they can
// be combined in a sum).  This activates forwarding for those registers for all
// states after this, and returns suitable expressions for the assignment.
func (p *InstructionTranslator[F]) WriteAndShiftRegisters(targets ...register.Id) []Expr[F] {
	lhs := make([]Expr[F], len(targets))
	offset := big.NewInt(1)
	// build up the lhs
	for i, dst := range targets {
		var (
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

// translateArith translates an integer / field arithmetic assignment of the
// form "targets = op(sources) op constant" into a polynomial constraint.  This
// folds together the word→field lowering previously performed by
// WordToFieldMachine (turning an INT_*/*MOD_P bytecode into a polynomial) with
// the polynomial translation below.  The right-hand side must be built before
// the left-hand side, since the latter activates register forwarding for the
// targets which must not affect the source reads.
func (p *InstructionTranslator[F]) translateArith(op vm.Operation, targets, sources []register.Id,
	constant big.Int) Expr[F] {
	var (
		rhs = p.translatePolynomial(arithPolynomial(op, sources, constant))
		lhs = p.WriteAndShiftRegisters(targets...)
	)
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

// translateConcat translates a concatenation assignment (BIT_CONCAT) which
// joins the source registers into the target register vector, weighting each
// source by 2^(bitwidth of the less-significant sources).  widths gives the bit
// width of each source register, least-significant first.
func (p *InstructionTranslator[F]) translateConcat(targets, sources []register.Id, widths []uint) Expr[F] {
	var (
		rhs = p.translatePolynomial(concatPolynomial(sources, widths))
		lhs = p.WriteAndShiftRegisters(targets...)
	)
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

// arithPolynomial builds the right-hand-side polynomial for an arithmetic
// bytecode: the sources become weight-one monomials, an optional (non-zero)
// constant is appended as a constant monomial, and the whole lot is combined
// according to the operation (sum, difference or product).  This mirrors the
// lowering in the (now removed) WordToFieldMachine path.
func arithPolynomial(op vm.Operation, sources []register.Id, constant big.Int) Polynomial {
	var (
		one   = big.NewInt(1)
		terms = make([]Monomial, len(sources))
	)
	//
	for i, r := range sources {
		terms[i] = poly.NewMonomial(*one, r)
	}
	// Append the constant term (if non-zero).
	if constant.Sign() != 0 {
		terms = append(terms, poly.NewMonomial[register.Id](constant))
	}
	//
	switch op {
	case vm.OP_ADD, vm.OP_ADDMOD_P:
		return polySum(terms...)
	case vm.OP_SUB, vm.OP_SUBMOD_P:
		return polySubtract(terms...)
	case vm.OP_MUL, vm.OP_MULMOD_P:
		return polyProduct(terms...)
	default:
		panic(fmt.Sprintf("unexpected arithmetic operation (%d)", op))
	}
}

// concatPolynomial builds the right-hand-side polynomial for a concatenation
// bytecode: source i is weighted by 2^(sum of the widths of sources 0..i-1),
// with source 0 being the least significant limb.
func concatPolynomial(sources []register.Id, widths []uint) Polynomial {
	var (
		terms = make([]Monomial, len(sources))
		acc   = big.NewInt(1)
	)
	//
	for i, r := range sources {
		var coeff big.Int
		//
		coeff.Set(acc)
		terms[i] = poly.NewMonomial(coeff, r)
		// Shift the running weight left by this source's bit width.
		acc = acc.Lsh(acc, widths[i])
	}
	//
	return polySum(terms...)
}

// polySum constructs the polynomial equal to the sum of the given monomials.
func polySum(terms ...Monomial) Polynomial {
	var p Polynomial
	return p.Set(terms...)
}

// polySubtract constructs the polynomial equal to terms[0] - terms[1] - ...
func polySubtract(terms ...Monomial) Polynomial {
	var p Polynomial
	//
	for i, m := range terms {
		if i == 0 {
			p = p.Set(m)
		} else {
			p.SubTerm(m)
		}
	}
	//
	return p
}

// polyProduct constructs the polynomial equal to the product of the given
// monomials.
func polyProduct(terms ...Monomial) Polynomial {
	var (
		p Polynomial
		m Monomial
	)
	//
	for i, t := range terms {
		if i == 0 {
			m = t
		} else {
			m = m.Mul(t)
		}
	}
	//
	return p.Set(m)
}

// Translate polynomial (c0*x0$0*...*xn$0) + ... + (cm*x0$m*...*xn$m) where cX
// are constant coefficients.  This generates a given translation of terms,
// along with an indication as to whether this is signed or not.
func (p *InstructionTranslator[F]) translatePolynomial(poly Polynomial) (pos []Expr[F]) {
	var (
		terms []Expr[F]
	)
	//
	for i := range poly.Len() {
		ith := poly.Term(i)
		//
		terms = append(terms, p.translateMonomial(ith))
	}
	// Done
	return terms
}

// Translate a monomial of the form c*x0*...*xn where c is a constant coefficient.
func (p *InstructionTranslator[F]) translateMonomial(mono Monomial) Expr[F] {
	var (
		n               = mono.Len()
		coeff           = mono.Coefficient()
		terms []Expr[F] = make([]Expr[F], n+1)
	)
	//
	for i := range mono.Len() {
		terms[i] = p.ReadRegister(mono.Nth(i))
	}
	// Optimise for case where coeff == 1?
	terms[n] = mirc.BigNumber[register.Id, Expr[F]](&coeff)
	//
	return mirc.Product(terms)
}
