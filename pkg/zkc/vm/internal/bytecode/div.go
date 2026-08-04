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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DivRem computes the (truncated) integer quotient or remainder of a register
// by a divisor which is either a register or a constant.  The operation is
// identified by Opcode, which is one of DIV or REM.  A zero divisor aborts
// execution with a division-by-zero error.
type DivRem[W word.Word[W]] struct {
	// Opcode selects the operation (DIV or REM).
	Opcode uint32
	// Target receives the result.
	Target RegisterId
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
	return []RegisterId{p.Target}
}

// Validate implementation for Bytecode interface.
func (p *DivRem[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errs := validateOperands(env, p.Uses(), p.Definitions())
	// A constant divisor of zero would unconditionally abort execution, so its
	// presence indicates a broken transform (codegen rejects it up front).
	if p.Divisor.IsConstant() && p.Divisor.AsConstant().Cmp64(0) == 0 {
		errs = append(errs, fmt.Errorf("constant divisor is zero"))
	}
	//
	return errs
}

func (p *DivRem[W]) String(mapping Environment[W]) string {
	var (
		target   = RegisterToString(p.Target, mapping)
		dividend = RegisterToString(p.Dividend, mapping)
		divisor  = p.Divisor.String(mapping)
		symbol   = "/"
	)
	//
	if p.Opcode != 0 {
		symbol = "%"
	}
	//
	return fmt.Sprintf("%s = %s %s %s", target, dividend, symbol, divisor)
}
