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
	"math/big"

	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	finsn "github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/field"
)

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
		var ith = p.reader.Register(dst)
		//
		lhs[i] = mirc.Variable[register.Id, Expr[F]](dst, ith.Width(), 0)
		//
		if i != 0 {
			lhs[i] = mirc.BigNumber[register.Id, Expr[F]](offset).Multiply(lhs[i])
		}
		// left shift offset by given register width.
		offset.Lsh(offset, ith.Width())
	}
	//
	return lhs
}

// ============================================================================
// Assignments
// ============================================================================

func (p *InstructionTranslator[F]) translateAssignment(insn instruction.FieldAssign[F]) Expr[F] {
	var (
		// build rhs
		rhs = p.translatePolynomial(insn.Source)
		// build lhs (must be after rhs)
		lhs = p.WriteAndShiftRegisters(insn.Target.Registers()...)
	)
	//
	return mirc.Sum(lhs).Equals(mirc.Sum(rhs))
}

// Translate polynomial (c0*x0$0*...*xn$0) + ... + (cm*x0$m*...*xn$m) where cX
// are constant coefficients.  This generates a given translation of terms,
// along with an indication as to whether this is signed or not.
func (p *InstructionTranslator[F]) translatePolynomial(poly finsn.Polynomial) (pos []Expr[F]) {
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
func (p *InstructionTranslator[F]) translateMonomial(mono finsn.Monomial) Expr[F] {
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
