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

// DivRem computes the (truncated) integer quotient and remainder of a
// register by a divisor which is either a register or a constant.  Both
// results are always computed and written: a source-level "/" or "%" simply
// directs the unwanted result into a fresh scratch register.
type DivRem[W word.Word[W]] struct {
	// Quotient receives the division result.
	Quotient RegisterId
	// Remainder receives the remainder.
	Remainder RegisterId
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
	return []RegisterId{p.Quotient, p.Remainder}
}

// Validate implementation for Bytecode interface.
func (p *DivRem[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errs := validateOperands(env, p.Uses(), p.Definitions())
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
	return fmt.Sprintf("%s, %s = %s /%% %s",
		RegisterToString(p.Quotient, mapping),
		RegisterToString(p.Remainder, mapping),
		RegisterToString(p.Dividend, mapping),
		p.Divisor.String(mapping))
}
