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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

// Translate this instruction into low-level constraints.
func (p *StateTranslator[F, T, E, M]) translateAssign(code *micro.Assign) E {
	var (
		// Determine sign of polynomial
		_, signed = agnostic.WidthOfPolynomial(code.Source, agnostic.ArrayEnvironment(p.mapping.Registers))
		// build rhs
		rhs = p.translatePolynomial(code.Source)
		// build lhs (must be after rhs)
		lhs = p.WriteAndShiftRegisters(code.Targets)
	)
	// Construct equation
	if signed && !hasSignBit(code.Targets, p.mapping.Registers) {
		str := assignToString(p.mapping.Registers, code.Targets, code.Source)
		panic(fmt.Sprintf("assignment missing sign bit (%s)", str))
	} else if signed {
		// Signed case, so rebalance
		lhs, rhs = p.rebalanceAssign(lhs, rhs)
	}
	//
	return Sum(lhs).Equals(Sum(rhs))
}

// Consider an assignment b, X := Y - 1.  This should be translated into the
// constraint: X + 1 == Y + 256.b (assuming b is u1, and X/Y are u8).
func (p *StateTranslator[F, T, E, M]) rebalanceAssign(lhs []E, rhs []E) ([]E, []E) {
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

// Translate polynomial (c0*x0$0*...*xn$0) + ... + (cm*x0$m*...*xn$m) where cX
// are constant coefficients.  This generates a given translation of terms,
// along with an indication as to whether this is signed or not.
func (p *StateTranslator[F, T, E, M]) translatePolynomial(poly agnostic.StaticPolynomial) (pos []E) {
	var (
		terms []E
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
func (p *StateTranslator[F, T, E, M]) translateMonomial(mono agnostic.StaticMonomial) E {
	var (
		n         = mono.Len()
		coeff     = mono.Coefficient()
		terms []E = make([]E, n+1)
	)
	//
	for i := range mono.Len() {
		terms[i] = p.ReadRegister(mono.Nth(i))
	}
	// Optimise for case where coeff == 1?
	terms[n] = BigNumber[T, E](&coeff)
	//
	return Product(terms)
}

func hasSignBit(targets []register.Id, regs []register.Register) bool {
	var (
		n = len(targets) - 1
	)
	//
	if n <= 0 {
		// if only a single target, then it cannot be considered to be a sign
		// bit.
		return false
	}
	// Look for single sign bit
	return regs[targets[n].Unwrap()].Width() == 1
}

// useful for debugging
//
// nolint
func assignToString(registers []register.Register, lhs []register.Id, rhs agnostic.StaticPolynomial) string {
	var builder strings.Builder
	//
	for i, ith := range lhs {
		if i != 0 {
			builder.WriteString(",")
		}
		builder.WriteString(registers[ith.Unwrap()].Name())
	}
	//
	builder.WriteString(" := ")
	//
	builder.WriteString(poly.String(rhs, func(id register.Id) string {
		return registers[id.Unwrap()].Name()
	}))
	//
	return builder.String()
}
