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

import "fmt"

// DivRem computes the (truncated) integer quotient or remainder of two
// registers.  The operation is identified by Opcode, which is one of DIV or
// REM.  A zero divisor aborts execution with a division-by-zero error.
type DivRem struct {
	// Opcode selects the operation (DIV or REM).
	Opcode uint32
	// Target receives the result.
	Target RegisterId
	// Dividend and Divisor are the operand registers.
	Dividend, Divisor RegisterId
}

// Uses implementation for Bytecode interface.
func (p *DivRem) Uses() []RegisterId {
	return []RegisterId{p.Dividend, p.Divisor}
}

// Definitions implementation for Bytecode interface.
func (p *DivRem) Definitions() []RegisterId {
	return []RegisterId{p.Target}
}

// Validate implementation for Bytecode interface.
func (p *DivRem) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *DivRem) String(mapping Environment) string {
	// var (
	// 	target   = RegisterToString(p.Target, mapping)
	// 	dividend = RegisterToString(p.Dividend, mapping)
	// 	divisor  = RegisterToString(p.Divisor, mapping)
	// 	symbol   = "/"
	// )
	// //
	// if p.Opcode == REM {
	// 	symbol = "%"
	// }
	// //
	// return fmt.Sprintf("%s = %s %s %s", target, dividend, symbol, divisor)
	panic("todo")
}

// DivHint computes quotient, remainder and range witness for a division hint
// (i.e. as produced by the LowerDivisions transform).  Specifically, Quotient =
// Dividend / Divisor, Remainder = Dividend % Divisor and Witness = Divisor -
// Remainder - 1, with correctness validated by subsequent arithmetic checks.  A
// zero divisor aborts execution with a division-by-zero error.
type DivHint struct {
	// Quotient, Remainder and Witness receive the results.
	Quotient, Remainder, Witness RegisterId
	// Dividend and Divisor are the operand registers.
	Dividend, Divisor RegisterId
}

// Uses implementation for Bytecode interface.
func (p *DivHint) Uses() []RegisterId {
	return []RegisterId{p.Dividend, p.Divisor}
}

// Definitions implementation for Bytecode interface.
func (p *DivHint) Definitions() []RegisterId {
	return []RegisterId{p.Quotient, p.Remainder, p.Witness}
}

// Validate implementation for Bytecode interface.
func (p *DivHint) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *DivHint) String(env Environment) string {
	var (
		quotient  = RegisterToString(p.Quotient, env)
		remainder = RegisterToString(p.Remainder, env)
		witness   = RegisterToString(p.Witness, env)
		dividend  = RegisterToString(p.Dividend, env)
		divisor   = RegisterToString(p.Divisor, env)
	)
	//
	return fmt.Sprintf("%s::%s::%s = hint(%s, %s)", quotient, remainder, witness, dividend, divisor)
}
