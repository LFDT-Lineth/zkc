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
package bytecode

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DivRem computes the (truncated) integer quotient and/or remainder of a
// register by a divisor which is either a register or a constant.  At least
// one of Quotient / Remainder is always present: quotient-only is a DIV,
// remainder-only a REM, and both together a divmod (the source-level "/%"
// operator), which yields both results from a single instruction.  A zero
// divisor aborts execution with a division-by-zero error.
type DivRem[W word.Word[W]] struct {
	// Quotient receives the division result, when present.
	Quotient util.Option[RegisterId]
	// Remainder receives the remainder, when present.
	Remainder util.Option[RegisterId]
	// Dividend is the operand register.
	Dividend RegisterId
	// Divisor is either a (single limb) operand register or a constant.
	Divisor Operand[W]
}

// Uses implementation for Bytecode interface.
func (p *DivRem[W]) Uses() []RegisterId {
	if p.Divisor.IsConstant() {
		return []RegisterId{p.Dividend}
	}
	//
	return []RegisterId{p.Dividend, p.Divisor.AsRegister()}
}

// Definitions implementation for Bytecode interface.
func (p *DivRem[W]) Definitions() []RegisterId {
	var defs []RegisterId
	//
	if p.Quotient.HasValue() {
		defs = append(defs, p.Quotient.Unwrap())
	}
	//
	if p.Remainder.HasValue() {
		defs = append(defs, p.Remainder.Unwrap())
	}
	//
	return defs
}

// Validate implementation for Bytecode interface.
func (p *DivRem[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errs := validateOperands(env, p.Uses(), p.Definitions())
	//
	if p.Quotient.IsEmpty() && p.Remainder.IsEmpty() {
		errs = append(errs, fmt.Errorf("division without quotient or remainder"))
	}
	//
	if p.Divisor.IsConstant() {
		// Already rejected by codegen rejects it upfront, check that it's not introduced
		// by some lowering process.
		if p.Divisor.AsConstant().Cmp64(0) == 0 {
			errs = append(errs, fmt.Errorf("constant divisor is zero"))
		}
	}
	//
	return errs
}

func (p *DivRem[W]) String(mapping Environment[W]) string {
	var (
		dividend = RegisterToString(p.Dividend, mapping)
		divisor  = p.Divisor.String(mapping)
	)
	//
	switch {
	case p.Quotient.HasValue() && p.Remainder.HasValue():
		return fmt.Sprintf("%s, %s = %s /%% %s",
			RegisterToString(p.Quotient.Unwrap(), mapping),
			RegisterToString(p.Remainder.Unwrap(), mapping), dividend, divisor)
	case p.Quotient.HasValue():
		return fmt.Sprintf("%s = %s / %s",
			RegisterToString(p.Quotient.Unwrap(), mapping), dividend, divisor)
	default:
		return fmt.Sprintf("%s = %s %% %s",
			RegisterToString(p.Remainder.Unwrap(), mapping), dividend, divisor)
	}
}
